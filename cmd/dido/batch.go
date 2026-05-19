package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// BatchTask is one task in a batch JSON file. Field names match SIL
// go-aeneas v0.0.5 and SAB's AeneasTask serialiser byte-for-byte.
type BatchTask struct {
	Description    string `json:"description"`
	AudioFilename  string `json:"audioFilename"`
	PhraseFilename string `json:"phraseFilename"`
	Parameters     string `json:"parameters"`
	OutputFilename string `json:"outputFilename"`
}

type batchFile struct {
	Tasks []BatchTask `json:"tasks"`
}

// parseBatchFile accepts both a bare top-level array (go-aeneas / SAB
// convention) and a wrapped {"tasks":[...]} object.
func parseBatchFile(path string) ([]BatchTask, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read batch file: %w", err)
	}
	trim := bytes.TrimLeft(raw, " \t\r\n")
	if len(trim) == 0 {
		return nil, fmt.Errorf("batch file is empty")
	}
	switch trim[0] {
	case '[':
		var tasks []BatchTask
		if err := json.Unmarshal(raw, &tasks); err != nil {
			return nil, fmt.Errorf("parse batch JSON (array form): %w", err)
		}
		return tasks, nil
	case '{':
		var bf batchFile
		if err := json.Unmarshal(raw, &bf); err != nil {
			return nil, fmt.Errorf("parse batch JSON (object form): %w", err)
		}
		return bf.Tasks, nil
	default:
		return nil, fmt.Errorf("batch JSON must start with '[' or '{'; got %q", trim[0])
	}
}

// taskRunner is the seam between the batch dispatcher and the per-task
// alignment body. Tests inject a fake; production passes runTask.
type taskRunner func(audioPath, textPath, configStr, outputPath string) error

// runBatch is the entry point for `dido --batch <file>`. First-error
// aborts the rest of the queue (matches go-aeneas v0.0.5).
func runBatch(path string) error {
	return runBatchWith(path, runTask, batchWorkers(), os.Stderr)
}

func runBatchWith(path string, runner taskRunner, workers int, logw io.Writer) error {
	tasks, err := parseBatchFile(path)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return fmt.Errorf("batch file %q contains no tasks", path)
	}

	fmt.Fprintf(logw, "Batch file: %s (%d task%s)\n",
		path, len(tasks), plural(len(tasks)))

	if workers < 1 {
		workers = 1
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}

	var (
		wg       sync.WaitGroup
		okCount  atomic.Int64
		failed   atomic.Int64
		firstErr atomicErr
		aborted  atomic.Bool
	)
	jobs := make(chan BatchTask)
	start := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				if aborted.Load() {
					continue
				}
				ts := time.Now()
				err := runner(t.AudioFilename, t.PhraseFilename, t.Parameters, t.OutputFilename)
				if err != nil {
					failed.Add(1)
					firstErr.set(fmt.Errorf("task %q: %w", t.Description, err))
					aborted.Store(true)
					fmt.Fprintf(logw, "  FAIL  %s — %v\n", t.Description, err)
					continue
				}
				okCount.Add(1)
				fmt.Fprintf(logw, "  OK    %s  (%s)\n",
					t.Description, time.Since(ts).Round(time.Millisecond))
			}
		}()
	}

	for _, t := range tasks {
		if aborted.Load() {
			break
		}
		jobs <- t
	}
	close(jobs)
	wg.Wait()

	fmt.Fprintf(logw, "Done in %s. %d ok, %d failed, %d skipped (post-abort).\n",
		time.Since(start).Round(time.Second),
		okCount.Load(), failed.Load(),
		int64(len(tasks))-okCount.Load()-failed.Load())

	return firstErr.get()
}

func runTask(audioPath, textPath, configStr, outputPath string) error {
	return run(audioPath, textPath, configStr, outputPath)
}

// batchWorkers defaults to 2 because each task's MFCC/DTW pipeline
// already fans out across NumCPU; higher batch concurrency
// oversubscribes the cores. DIDO_BATCH_WORKERS overrides.
func batchWorkers() int {
	if env := os.Getenv("DIDO_BATCH_WORKERS"); env != "" {
		if n, err := strconv.Atoi(env); err == nil && n > 0 {
			return n
		}
	}
	if runtime.NumCPU() <= 1 {
		return 1
	}
	return 2
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// atomicErr is a first-writer-wins error holder.
type atomicErr struct {
	mu  sync.Mutex
	err error
}

func (a *atomicErr) set(err error) {
	a.mu.Lock()
	if a.err == nil {
		a.err = err
	}
	a.mu.Unlock()
}

func (a *atomicErr) get() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}
