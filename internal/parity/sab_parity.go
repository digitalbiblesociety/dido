package parity

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// BuildDido compiles cmd/dido to a temp binary and returns its path.
func BuildDido(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "dido")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/dido")
	cmd.Dir = moduleRoot(t)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build dido: %v", err)
	}
	return bin
}

// moduleRoot derives the dido module root from this file's path —
// two parents up from internal/parity/. Independent of test cwd.
func moduleRoot(t testing.TB) string {
	t.Helper()
	root := filepath.Join(filepath.Dir(HelperPath()), "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	return abs
}

// TSVRow: `<start>\t<end>\t<id>\n` on disk.
type TSVRow struct {
	ID    string
	Begin float64
	End   float64
}

func ParseTSV(t testing.TB, path string) []TSVRow {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	var rows []TSVRow
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64<<10), 4<<20)
	lineNo := 0
	for scan.Scan() {
		lineNo++
		line := strings.TrimRight(scan.Text(), "\r")
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("%s line %d: want 3 tab-fields, got %d: %q", path, lineNo, len(fields), line)
		}
		begin, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			t.Fatalf("%s line %d: parse begin: %v", path, lineNo, err)
		}
		end, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.Fatalf("%s line %d: parse end: %v", path, lineNo, err)
		}
		rows = append(rows, TSVRow{ID: fields[2], Begin: begin, End: end})
	}
	if err := scan.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return rows
}

// TSVDelta summarises the drift between two TSV outputs.
type TSVDelta struct {
	N        int
	MaxAbs   float64
	Mean     float64
	P95      float64
	IDDiffs  []string
	RowDelta []float64
}

// CompareTSV asserts both outputs have the same row count and the
// same IDs in the same order; per-row drift is computed and returned
// for observability but not asserted on. Frame-accurate timing parity
// is covered by mfcc_parity_test.go / dtw_parity_test.go against
// synthetic fixtures — the end-to-end SAB tests only verify the
// SAB-shape contract (row count, ID order, column layout, head/tail
// placement) so they don't false-positive on aeneas C-extension
// drift or TTS-version skew between machines.
func CompareTSV(t testing.TB, a, b []TSVRow) TSVDelta {
	t.Helper()
	d := TSVDelta{}
	if len(a) != len(b) {
		t.Fatalf("TSV row count mismatch: a=%d, b=%d", len(a), len(b))
	}
	d.N = len(a)
	abs := make([]float64, 0, 2*len(a))
	rowDelta := make([]float64, len(a))
	for i := range a {
		if a[i].ID != b[i].ID {
			d.IDDiffs = append(d.IDDiffs,
				fmt.Sprintf("row %d: a=%q b=%q", i, a[i].ID, b[i].ID))
		}
		db := math.Abs(a[i].Begin - b[i].Begin)
		de := math.Abs(a[i].End - b[i].End)
		abs = append(abs, db, de)
		rowDelta[i] = math.Max(db, de)
	}
	if len(d.IDDiffs) > 0 {
		t.Fatalf("TSV id divergences: %v", d.IDDiffs)
	}
	d.RowDelta = rowDelta
	if len(abs) > 0 {
		sorted := make([]float64, len(abs))
		copy(sorted, abs)
		sort.Float64s(sorted)
		d.MaxAbs = sorted[len(sorted)-1]
		var sum float64
		for _, v := range sorted {
			sum += v
		}
		d.Mean = sum / float64(len(sorted))
		p95Idx := int(float64(len(sorted)) * 0.95)
		if p95Idx >= len(sorted) {
			p95Idx = len(sorted) - 1
		}
		d.P95 = sorted[p95Idx]
	}
	return d
}

// RunPyAeneas invokes `python3 -m aeneas.tools.execute_task` with
// SAB's exact bash-script shape (--skip-validator + UTF-8 io).
func RunPyAeneas(t testing.TB, audio, phrases, params, output string) {
	t.Helper()
	cmd := exec.Command(PythonBin(), "-m", "aeneas.tools.execute_task",
		audio, phrases, params, output, "--skip-validator")
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=UTF-8")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python aeneas execute_task: %v\n%s", err, out)
	}
}

func RunDidoBatch(t testing.TB, didoBin, batchJSON string) {
	t.Helper()
	cmd := exec.Command(didoBin, "--batch", batchJSON)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dido --batch: %v\n%s", err, out)
	}
}

func WriteFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
