package parity

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Book-of-Psalms total-time benchmarks: run the entire book through each
// pipeline and report cumulative wall time.
//
// Run with:
//
//	go test -bench=Book -benchtime=1x -timeout=2h ./internal/parity/
//
// Environment:
//
//	PSALMS_PARITY_DIR  — fixtures dir (default: /tmp/psalms-parity)
//	PSALMS_PARITY_GO_BIN — path to the built dido binary (default:
//	                       <repo>/dido-book-bench; built lazily)
//	AENEAS_PYTHON      — python binary (default: python3)
//
// Fixture layout the benchmarks expect:
//
//	$PSALMS_PARITY_DIR/wav/001.wav .. 150.wav    (mono 16 kHz)
//	$PSALMS_PARITY_DIR/text/001.txt .. 150.txt   (one verse per line)
//
// If WAVs are missing the test logs which Psalms are missing and skips. Use
// tools/psalms-parity/run_all.sh (or the README there) to generate them.

const (
	bookFirst    = 1
	bookLast     = 150
	bookConfig   = "task_language=eng|is_text_type=plain|os_task_file_format=json"
	totalPsalms  = bookLast - bookFirst + 1
)

func fixturesDir() string {
	if v := os.Getenv("PSALMS_PARITY_DIR"); v != "" {
		return v
	}
	return "/tmp/psalms-parity"
}

// repoRoot returns the absolute path to the project root (two levels up from
// this file's containing internal/parity package).
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// ensureBookFixtures checks that the WAV + text fixtures for the full book
// are present. Returns nil iff everything is in place; otherwise a descriptive
// error suitable for b.Skip.
func ensureBookFixtures() error {
	dir := fixturesDir()
	var missing []int
	for n := bookFirst; n <= bookLast; n++ {
		wav := filepath.Join(dir, "wav", fmt.Sprintf("%03d.wav", n))
		text := filepath.Join(dir, "text", fmt.Sprintf("%03d.txt", n))
		if _, err := os.Stat(wav); err != nil {
			missing = append(missing, n)
			continue
		}
		if _, err := os.Stat(text); err != nil {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%d of %d Psalm fixtures missing under %s (e.g. Psalm %03d); "+
			"run tools/psalms-parity/run_all.sh first",
			len(missing), totalPsalms, dir, missing[0])
	}
	return nil
}

// goBinary lazily builds the dido binary used by the book benchmark and
// caches the path. The binary is left in the repo root so re-runs are fast.
var (
	goBinaryOnce sync.Once
	goBinaryPath string
	goBinaryErr  error
)

func goBinary(b *testing.B) string {
	b.Helper()
	goBinaryOnce.Do(func() {
		if v := os.Getenv("PSALMS_PARITY_GO_BIN"); v != "" {
			goBinaryPath = v
			return
		}
		// Build into the fixtures dir so we don't litter the repo root.
		if err := os.MkdirAll(fixturesDir(), 0o755); err != nil {
			goBinaryErr = err
			return
		}
		goBinaryPath = filepath.Join(fixturesDir(), "dido-bench")
		cmd := exec.Command("go", "build", "-o", goBinaryPath, "./cmd/aeneas")
		cmd.Dir = repoRoot()
		if out, err := cmd.CombinedOutput(); err != nil {
			goBinaryErr = fmt.Errorf("go build: %v: %s", err, out)
		}
	})
	if goBinaryErr != nil {
		b.Fatalf("build dido: %v", goBinaryErr)
	}
	return goBinaryPath
}

// BenchmarkPsalmsBookGo aligns all 150 Psalms via the Go pipeline (subprocess
// invocation, matching what an end user runs from the shell) and reports the
// cumulative wall time. b.N is forced to 1 by -benchtime=1x — each iteration
// is on the order of minutes.
func BenchmarkPsalmsBookGo(b *testing.B) {
	if err := ensureBookFixtures(); err != nil {
		b.Skip(err.Error())
	}
	bin := goBinary(b)
	outDir := filepath.Join(fixturesDir(), "go-book")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		b.Fatal(err)
	}
	wavDir := filepath.Join(fixturesDir(), "wav")
	textDir := filepath.Join(fixturesDir(), "text")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runBookSubprocess(b, "go", func(n int) *exec.Cmd {
			return exec.Command(bin,
				filepath.Join(wavDir, fmt.Sprintf("%03d.wav", n)),
				filepath.Join(textDir, fmt.Sprintf("%03d.txt", n)),
				bookConfig,
				filepath.Join(outDir, fmt.Sprintf("%03d.json", n)),
			)
		})
	}
}

// BenchmarkPsalmsBookPython aligns all 150 Psalms via the upstream Python
// aeneas pipeline (`python3 -m aeneas.tools.execute_task`) and reports the
// cumulative wall time. Mirrors BenchmarkPsalmsBookGo so the two numbers are
// directly comparable.
func BenchmarkPsalmsBookPython(b *testing.B) {
	if err := CheckAvailable(); err != nil {
		b.Skipf("skipping: %v", err)
	}
	if err := ensureBookFixtures(); err != nil {
		b.Skip(err.Error())
	}
	py := PythonBin()
	outDir := filepath.Join(fixturesDir(), "py-book")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		b.Fatal(err)
	}
	wavDir := filepath.Join(fixturesDir(), "wav")
	textDir := filepath.Join(fixturesDir(), "text")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runBookSubprocess(b, "python", func(n int) *exec.Cmd {
			return exec.Command(py,
				"-m", "aeneas.tools.execute_task",
				filepath.Join(wavDir, fmt.Sprintf("%03d.wav", n)),
				filepath.Join(textDir, fmt.Sprintf("%03d.txt", n)),
				bookConfig,
				filepath.Join(outDir, fmt.Sprintf("%03d.json", n)),
			)
		})
	}
}

// runBookSubprocess invokes the per-psalm command for every Psalm in the book,
// times the entire batch (b.ResetTimer is the responsibility of the caller),
// and surfaces the cumulative wall time + a real-time-factor metric via
// b.ReportMetric. On per-psalm failure it aborts the benchmark.
func runBookSubprocess(b *testing.B, label string, build func(n int) *exec.Cmd) {
	b.Helper()
	start := time.Now()
	for n := bookFirst; n <= bookLast; n++ {
		cmd := build(n)
		out, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("[%s] Psalm %03d failed: %v\n%s", label, n, err, out)
		}
	}
	elapsed := time.Since(start)

	b.ReportMetric(elapsed.Seconds(), "book_s")
	b.ReportMetric(float64(totalPsalms), "psalms")
	b.ReportMetric(elapsed.Seconds()/float64(totalPsalms), "s/psalm")
}
