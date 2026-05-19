package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// writeBatch writes contents to a temp file and returns its path.
func writeBatch(t *testing.T, contents string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write batch: %v", err)
	}
	return p
}

// recordingRunner counts invocations and lets the caller fail on
// specific descriptions or specific call indexes.
type recordingRunner struct {
	mu          sync.Mutex
	calls       []BatchTask
	failOnDesc  map[string]error
	failOnIndex map[int]error
	count       atomic.Int64
}

func (r *recordingRunner) run(audio, text, params, output string) error {
	idx := int(r.count.Add(1) - 1)
	bt := BatchTask{AudioFilename: audio, PhraseFilename: text, Parameters: params, OutputFilename: output}
	r.mu.Lock()
	r.calls = append(r.calls, bt)
	r.mu.Unlock()
	if err, ok := r.failOnIndex[idx]; ok {
		return err
	}
	if err, ok := r.failOnDesc[output]; ok {
		// piggy-back on OutputFilename as a sentinel (the description
		// is not passed to runner; OutputFilename is unique per task)
		return err
	}
	return nil
}

func TestParseBatchFile_BareArray(t *testing.T) {
	p := writeBatch(t, `[
		{"description":"GEN 1","audioFilename":"a.mp3","phraseFilename":"p.txt","parameters":"task_language=eng","outputFilename":"o.tsv"},
		{"description":"GEN 2","audioFilename":"b.mp3","phraseFilename":"q.txt","parameters":"task_language=eng","outputFilename":"p.tsv"}
	]`)
	tasks, err := parseBatchFile(p)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("want 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Description != "GEN 1" || tasks[1].AudioFilename != "b.mp3" {
		t.Errorf("field mapping wrong: %+v", tasks)
	}
}

func TestParseBatchFile_WrappedObject(t *testing.T) {
	p := writeBatch(t, `{"tasks":[
		{"description":"X","audioFilename":"a","phraseFilename":"b","parameters":"c","outputFilename":"d"}
	]}`)
	tasks, err := parseBatchFile(p)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Description != "X" {
		t.Errorf("wrapped form mis-parsed: %+v", tasks)
	}
}

func TestParseBatchFile_LeadingWhitespace(t *testing.T) {
	p := writeBatch(t, "   \n  \t[]")
	tasks, err := parseBatchFile(p)
	if err != nil {
		t.Fatalf("whitespace tolerance failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("empty array → 0 tasks, got %d", len(tasks))
	}
}

func TestParseBatchFile_Malformed(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty file", ""},
		{"not JSON", "hello world"},
		{"truncated array", "[ {"},
		{"truncated object", "{\"tasks\":"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := writeBatch(t, c.body)
			if _, err := parseBatchFile(p); err == nil {
				t.Errorf("expected error for %q", c.body)
			}
		})
	}
}

func TestRunBatch_EmptyTasksRejected(t *testing.T) {
	p := writeBatch(t, `[]`)
	err := runBatchWith(p, func(_, _, _, _ string) error { return nil }, 2, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no tasks") {
		t.Errorf("empty tasks should be rejected, got: %v", err)
	}
}

func TestRunBatch_AllSucceed(t *testing.T) {
	p := writeBatch(t, `[
		{"description":"A","audioFilename":"a","phraseFilename":"a.txt","parameters":"","outputFilename":"a.out"},
		{"description":"B","audioFilename":"b","phraseFilename":"b.txt","parameters":"","outputFilename":"b.out"},
		{"description":"C","audioFilename":"c","phraseFilename":"c.txt","parameters":"","outputFilename":"c.out"}
	]`)
	r := &recordingRunner{}
	var buf bytes.Buffer
	if err := runBatchWith(p, r.run, 2, &buf); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got := int(r.count.Load()); got != 3 {
		t.Errorf("runner invocations: got %d, want 3", got)
	}
	if !strings.Contains(buf.String(), "3 ok, 0 failed") {
		t.Errorf("summary line missing 3/0; got: %s", buf.String())
	}
}

func TestRunBatch_FirstErrorAborts(t *testing.T) {
	p := writeBatch(t, `[
		{"description":"ok-1","audioFilename":"a","phraseFilename":"a","parameters":"","outputFilename":"o1"},
		{"description":"will-fail","audioFilename":"b","phraseFilename":"b","parameters":"","outputFilename":"o2"},
		{"description":"would-run","audioFilename":"c","phraseFilename":"c","parameters":"","outputFilename":"o3"},
		{"description":"would-run","audioFilename":"d","phraseFilename":"d","parameters":"","outputFilename":"o4"}
	]`)
	wantErr := errors.New("synthetic alignment failure")
	r := &recordingRunner{
		failOnDesc: map[string]error{"o2": wantErr},
	}
	// workers=1 to keep ordering deterministic and ensure the
	// post-fail tasks see aborted=true on dequeue.
	err := runBatchWith(p, r.run, 1, io.Discard)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("returned error doesn't wrap original: %v", err)
	}
	got := int(r.count.Load())
	// First task succeeds, second fails. Aborted=true is set by the
	// failing task before the dispatcher loops, so tasks 3 and 4 are
	// either never enqueued or dequeued-and-dropped — runner sees at
	// most 2 calls.
	if got > 2 {
		t.Errorf("runner called %d times after abort; should be ≤ 2", got)
	}
}

func TestRunBatch_FieldsFlowToRunner(t *testing.T) {
	p := writeBatch(t, `[
		{"description":"D1","audioFilename":"/in/a.mp3","phraseFilename":"/in/a.txt","parameters":"task_language=eng|os_task_file_format=tsv","outputFilename":"/out/a.tsv"}
	]`)
	r := &recordingRunner{}
	if err := runBatchWith(p, r.run, 1, io.Discard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(r.calls))
	}
	c := r.calls[0]
	if c.AudioFilename != "/in/a.mp3" || c.PhraseFilename != "/in/a.txt" ||
		c.Parameters != "task_language=eng|os_task_file_format=tsv" ||
		c.OutputFilename != "/out/a.tsv" {
		t.Errorf("runner saw %+v", c)
	}
}

func TestBatchWorkers_EnvOverride(t *testing.T) {
	t.Setenv("DIDO_BATCH_WORKERS", "7")
	if got := batchWorkers(); got != 7 {
		t.Errorf("env override: got %d, want 7", got)
	}
	t.Setenv("DIDO_BATCH_WORKERS", "garbage")
	got := batchWorkers()
	if got != 1 && got != 2 {
		t.Errorf("invalid env: got %d, want 1 or 2 (the defaults)", got)
	}
	t.Setenv("DIDO_BATCH_WORKERS", "")
	got = batchWorkers()
	if got != 1 && got != 2 {
		t.Errorf("unset env: got %d, want 1 or 2 (the defaults)", got)
	}
}
