package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/digitalbiblesociety/dido/internal/config"
	"github.com/digitalbiblesociety/dido/internal/dtw"
	"github.com/digitalbiblesociety/dido/internal/mfcc"
	"github.com/digitalbiblesociety/dido/internal/pipeline"
	"github.com/digitalbiblesociety/dido/internal/usfm"
)

// collectingSink captures every progressEvent in arrival order under a
// mutex so the parallel coordinator's notifications can be asserted
// without worrying about goroutine scheduling.
type collectingSink struct {
	mu     sync.Mutex
	events []progressEvent
}

func (c *collectingSink) Send(e progressEvent) {
	c.mu.Lock()
	c.events = append(c.events, e)
	c.mu.Unlock()
}
func (c *collectingSink) Close() {}

func (c *collectingSink) byKind(kind string) []progressEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []progressEvent
	for _, e := range c.events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// fakePlans builds N books × M chapters with cheap on-disk USFM files
// so the batch worker's pre-parse step succeeds. Audio paths are
// placeholders since the injected aligner doesn't read them.
func fakePlans(t *testing.T, n, chapsPerBook int) []bookPlan {
	t.Helper()
	dir := t.TempDir()
	var plans []bookPlan
	for i := 1; i <= n; i++ {
		code := fmt.Sprintf("B%02d", i)
		usfmPath := filepath.Join(dir, code+".usfm")
		if err := os.WriteFile(usfmPath, []byte("\\id "+code+"\n"), 0o644); err != nil {
			t.Fatalf("write usfm: %v", err)
		}
		p := bookPlan{
			book:     book{Seq: i, Code: code, Name: code + "name"},
			USFM:     usfmPath,
			AudioDir: dir,
		}
		for ch := 1; ch <= chapsPerBook; ch++ {
			p.Chapters = append(p.Chapters, chapterTask{
				Chapter:  ch,
				AudioMP3: fmt.Sprintf("%s_%03d.mp3", code, ch),
				Stem:     fmt.Sprintf("%s-%02d", code, ch),
			})
		}
		plans = append(plans, p)
	}
	return plans
}

// TestRunBatchParallelEmitsCompleteEventStream covers the happy path:
// every chapter completes, every book gets a book_start + book_done,
// and chapter_done counts match the plan.
func TestRunBatchParallelEmitsCompleteEventStream(t *testing.T) {
	plans := fakePlans(t, 3, 4) // 3 books × 4 chapters = 12 jobs
	sink := &collectingSink{}
	c := cliFlags{workers: 4}

	align := func(_ string, _ []usfm.Verse, _ int, _, _ string,
		_ pipeline.TaskConfig, _ config.RuntimeConfig) (chapterResult, error) {
		return chapterResult{Fragments: 7}, nil
	}

	runBatchWorkerWith(plans, t.TempDir(), c, pipeline.TaskConfig{},
		config.Default(), sink, align)

	if got := len(sink.byKind("book_start")); got != 3 {
		t.Errorf("book_start = %d, want 3", got)
	}
	if got := len(sink.byKind("chapter_done")); got != 12 {
		t.Errorf("chapter_done = %d, want 12", got)
	}
	if got := len(sink.byKind("book_done")); got != 3 {
		t.Errorf("book_done = %d, want 3", got)
	}
	if got := len(sink.byKind("batch_done")); got != 1 {
		t.Errorf("batch_done = %d, want 1", got)
	}
	// book_start should always precede chapter_done for the same book.
	seenStart := map[int]bool{}
	sink.mu.Lock()
	for _, e := range sink.events {
		if e.Kind == "book_start" {
			seenStart[e.Book.Seq] = true
		}
		if e.Kind == "chapter_done" && !seenStart[e.Book.Seq] {
			t.Errorf("chapter_done for seq=%d arrived before book_start", e.Book.Seq)
		}
	}
	sink.mu.Unlock()
}

// TestRunBatchParallelActuallyParallelises confirms that wall time
// scales with workers: 8 chapters × 100ms each finishes in well under
// 8×100ms when workers=4.
func TestRunBatchParallelActuallyParallelises(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive; skip in -short")
	}
	plans := fakePlans(t, 2, 4) // 2 books × 4 chapters = 8 jobs
	sink := &collectingSink{}
	c := cliFlags{workers: 4}

	align := func(_ string, _ []usfm.Verse, _ int, _, _ string,
		_ pipeline.TaskConfig, _ config.RuntimeConfig) (chapterResult, error) {
		time.Sleep(100 * time.Millisecond)
		return chapterResult{Fragments: 1}, nil
	}

	start := time.Now()
	runBatchWorkerWith(plans, t.TempDir(), c, pipeline.TaskConfig{},
		config.Default(), sink, align)
	elapsed := time.Since(start)

	// 4 workers × 100ms × 2 rounds = 200ms ideal. Allow up to 500ms for
	// scheduler jitter; an accidentally-sequential run would take 800ms.
	if elapsed > 500*time.Millisecond {
		t.Errorf("parallel run took %s, want under 500ms (4 workers × 100ms × 2)", elapsed)
	}
}

// TestRunBatchParallelAbortsOnFirstError verifies the "abort whole
// batch on first error" policy: once one chapter errors the queue is
// drained, and the total number of completed chapters is bounded.
func TestRunBatchParallelAbortsOnFirstError(t *testing.T) {
	plans := fakePlans(t, 5, 6) // 30 jobs total
	sink := &collectingSink{}
	c := cliFlags{workers: 2}

	var calls atomic.Int32
	align := func(_ string, _ []usfm.Verse, _ int, _, _ string,
		_ pipeline.TaskConfig, _ config.RuntimeConfig) (chapterResult, error) {
		n := calls.Add(1)
		if n == 3 {
			return chapterResult{}, errors.New("synthetic chapter failure")
		}
		// Tiny sleep so the dispatcher can fill the queue before the
		// failing chapter lands and triggers ctx cancel.
		time.Sleep(5 * time.Millisecond)
		return chapterResult{Fragments: 1}, nil
	}

	runBatchWorkerWith(plans, t.TempDir(), c, pipeline.TaskConfig{},
		config.Default(), sink, align)

	if got := len(sink.byKind("chapter_err")); got != 1 {
		t.Errorf("chapter_err = %d, want exactly 1", got)
	}
	if int(calls.Load()) >= 30 {
		t.Errorf("aligner was called %d times — abort didn't stop the queue", calls.Load())
	}
	failedSeqs := map[int]bool{}
	for _, e := range sink.byKind("book_failed") {
		failedSeqs[e.Book.Seq] = true
	}
	if len(failedSeqs) == 0 {
		t.Error("expected at least one book_failed after abort")
	}
}

// TestRunBatchParallelHonoursResume confirms that pre-existing timing
// files short-circuit alignment in the parallel path the same way they
// did sequentially.
func TestRunBatchParallelHonoursResume(t *testing.T) {
	plans := fakePlans(t, 1, 3)
	outDir := t.TempDir()
	for _, ct := range plans[0].Chapters[:2] {
		if err := os.WriteFile(filepath.Join(outDir, ct.Stem+"-timing.txt"),
			[]byte("dummy\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sink := &collectingSink{}
	c := cliFlags{workers: 2, resume: true}

	var calls atomic.Int32
	align := func(_ string, _ []usfm.Verse, _ int, _, _ string,
		_ pipeline.TaskConfig, _ config.RuntimeConfig) (chapterResult, error) {
		calls.Add(1)
		return chapterResult{Fragments: 1}, nil
	}

	runBatchWorkerWith(plans, outDir, c, pipeline.TaskConfig{},
		config.Default(), sink, align)

	if got := int(calls.Load()); got != 1 {
		t.Errorf("aligner calls = %d, want 1 (two chapters resumed)", got)
	}
	if got := len(sink.byKind("chapter_resume")); got != 2 {
		t.Errorf("chapter_resume = %d, want 2", got)
	}
	if got := len(sink.byKind("chapter_done")); got != 1 {
		t.Errorf("chapter_done = %d, want 1", got)
	}
}

// TestEffectiveWorkersAutoDefault pins the 0-input behaviour. The
// default is intentionally conservative (≤ 2) because the inner
// alignment pipeline is already parallel across NumCPU goroutines —
// stacking batch-level workers on top oversubscribes the cores.
func TestEffectiveWorkersAutoDefault(t *testing.T) {
	got := effectiveWorkers(0)
	if got < 1 || got > 2 {
		t.Errorf("effectiveWorkers(0) = %d, want 1 or 2 (conservative default)", got)
	}
	if got := effectiveWorkers(3); got != 3 {
		t.Errorf("effectiveWorkers(3) = %d, want 3", got)
	}
	if got := effectiveWorkers(10_000); got != 64 {
		t.Errorf("effectiveWorkers(10000) = %d, want 64 (clamped)", got)
	}
}

// TestScaleInnerPipelineWorkersDividesCPUBudget locks in the CPU-split
// rule: when batch workers > 1 the inner MFCC/DTW caps drop so total
// compute concurrency stays bounded. The previous values are restored
// when the returned func runs — important because mfcc.MaxWorkers and
// dtw.CostMatrixMaxWorkers are global state.
func TestScaleInnerPipelineWorkersDividesCPUBudget(t *testing.T) {
	prevMFCC, prevDTW := mfcc.MaxWorkers, dtw.CostMatrixMaxWorkers
	defer func() {
		mfcc.MaxWorkers = prevMFCC
		dtw.CostMatrixMaxWorkers = prevDTW
	}()

	mfcc.MaxWorkers = 0 // 0 means "use NumCPU" inside the pipeline
	dtw.CostMatrixMaxWorkers = 0

	restore := scaleInnerPipelineWorkers(4)
	got := mfcc.MaxWorkers
	if got < 1 {
		t.Errorf("mfcc.MaxWorkers after scale = %d, want >= 1", got)
	}
	// Loose upper bound: inner × batch shouldn't exceed NumCPU.
	if got*4 > runtime.NumCPU()*2 {
		t.Errorf("inner=%d × batch=4 oversubscribes NumCPU=%d", got, runtime.NumCPU())
	}
	if got != dtw.CostMatrixMaxWorkers {
		t.Errorf("mfcc=%d but dtw=%d; want both equal", got, dtw.CostMatrixMaxWorkers)
	}
	restore()
	if mfcc.MaxWorkers != 0 || dtw.CostMatrixMaxWorkers != 0 {
		t.Errorf("after restore, mfcc=%d dtw=%d, want both 0",
			mfcc.MaxWorkers, dtw.CostMatrixMaxWorkers)
	}
}

// TestChapterDoneCoverageMatchesPlan is a defensive duplicate check:
// every chapter in the plan must appear in the event stream exactly
// once. Catches races where two workers double-count the last job.
func TestChapterDoneCoverageMatchesPlan(t *testing.T) {
	plans := fakePlans(t, 4, 5)
	sink := &collectingSink{}
	c := cliFlags{workers: 6}

	align := func(_ string, _ []usfm.Verse, _ int, _, _ string,
		_ pipeline.TaskConfig, _ config.RuntimeConfig) (chapterResult, error) {
		return chapterResult{Fragments: 1}, nil
	}
	runBatchWorkerWith(plans, t.TempDir(), c, pipeline.TaskConfig{},
		config.Default(), sink, align)

	var keys []string
	sink.mu.Lock()
	for _, e := range sink.events {
		if e.Kind == "chapter_done" {
			keys = append(keys, fmt.Sprintf("%d/%d", e.Book.Seq, e.Chap))
		}
	}
	sink.mu.Unlock()
	sort.Strings(keys)

	if len(keys) != 4*5 {
		t.Errorf("chapter_done count = %d, want %d", len(keys), 4*5)
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate chapter_done %q", k)
		}
		seen[k] = true
	}
}
