// Package parity provides a Go-side harness for invoking the upstream
// Python aeneas implementation and comparing its outputs against the Go port.
//
// The helper is structured around a small Python script (pyhelper.py)
// that accepts a JSON request on stdin and writes a JSON response on stdout.
// Tests that need a reference value call RunOnce; benchmarks that issue
// many requests use a Server.
package parity

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// HelperPath returns the absolute path to pyhelper.py.
func HelperPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "pyhelper.py")
}

// PythonBin returns the python3 executable to invoke.
// Can be overridden with the AENEAS_PYTHON env var.
func PythonBin() string {
	if p := os.Getenv("AENEAS_PYTHON"); p != "" {
		return p
	}
	return "python3"
}

// CheckAvailable returns nil if the Python aeneas helper can be invoked,
// or a descriptive error suitable for t.Skip.
func CheckAvailable() error {
	cmd := exec.Command(PythonBin(), "-c", "import aeneas; import numpy")
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("python aeneas not importable (%w); set AENEAS_PYTHON or install aeneas", err)
	}
	if _, err := os.Stat(HelperPath()); err != nil {
		return fmt.Errorf("pyhelper.py not found: %w", err)
	}
	return nil
}

// SkipIfUnavailable skips the calling test if Python aeneas is not importable.
func SkipIfUnavailable(t testing.TB) {
	t.Helper()
	if err := CheckAvailable(); err != nil {
		t.Skipf("skipping parity test: %v", err)
	}
}

// RunOnce sends one request to a freshly-spawned Python helper and decodes
// the JSON response into result. result must be a pointer to a struct or map.
func RunOnce(req any, result any) error {
	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	cmd := exec.Command(PythonBin(), HelperPath())
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("python helper failed: %w; stderr=%q", err, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), result); err != nil {
		return fmt.Errorf("decode response: %w; stdout=%q", err, stdout.Bytes())
	}
	if envelope, ok := result.(*Envelope); ok && envelope.Error != "" {
		return fmt.Errorf("python helper error: %s", envelope.Error)
	}
	return nil
}

// Envelope is used to detect remote errors in generic decodes.
type Envelope struct {
	Error string `json:"error"`
	Trace string `json:"trace,omitempty"`
}

// Server is a long-lived Python helper process for use in benchmarks
// or test loops where startup overhead would dominate.
type Server struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr bytes.Buffer
	mu     sync.Mutex
	closed bool
}

// StartServer launches the helper in server mode.
// Callers must Close the server when finished.
func StartServer() (*Server, error) {
	cmd := exec.Command(PythonBin(), HelperPath())
	cmd.Env = append(os.Environ(), "PYHELPER_MODE=server", "PYTHONUNBUFFERED=1")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	s := &Server{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)}
	cmd.Stderr = &s.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start pyhelper server: %w", err)
	}
	return s, nil
}

// Do issues one request/response cycle. Not safe for concurrent use.
func (s *Server) Do(req any, result any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("server is closed")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	payload = append(payload, '\n')
	if _, err := s.stdin.Write(payload); err != nil {
		return fmt.Errorf("write request: %w", err)
	}
	line, err := s.stdout.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("read response: %w; stderr=%q", err, s.stderr.String())
	}
	if err := json.Unmarshal(line, result); err != nil {
		return fmt.Errorf("decode: %w; raw=%q", err, line)
	}
	if envelope, ok := result.(*Envelope); ok && envelope.Error != "" {
		return fmt.Errorf("pyhelper error: %s", envelope.Error)
	}
	return nil
}

// Close shuts the server down. Idempotent.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	_ = s.stdin.Close()
	return s.cmd.Wait()
}

// CompareMatrices returns ("", true) if a and b are equal within atol or rtol,
// otherwise a descriptive message and false.
//
//	pass iff |a-b| <= atol + rtol * |b|  (numpy convention)
func CompareMatrices(name string, a, b [][]float64, atol, rtol float64) (string, bool) {
	if len(a) != len(b) {
		return fmt.Sprintf("%s: row count differs: got %d, want %d", name, len(a), len(b)), false
	}
	if len(a) == 0 {
		return "", true
	}
	if len(a[0]) != len(b[0]) {
		return fmt.Sprintf("%s: col count differs: got %d, want %d", name, len(a[0]), len(b[0])), false
	}
	var maxAbs, maxRel float64
	var maxI, maxJ int
	var nMismatch int
	for i := range a {
		for j := range a[i] {
			diff := math.Abs(a[i][j] - b[i][j])
			tol := atol + rtol*math.Abs(b[i][j])
			if diff > tol {
				nMismatch++
				if diff > maxAbs {
					maxAbs = diff
					maxI, maxJ = i, j
					if b[i][j] != 0 {
						maxRel = diff / math.Abs(b[i][j])
					}
				}
			}
		}
	}
	if nMismatch == 0 {
		return "", true
	}
	return fmt.Sprintf(
		"%s: %d/%d elements differ; worst at [%d][%d] got=%g want=%g abs=%g rel=%g (atol=%g rtol=%g)",
		name, nMismatch, len(a)*len(a[0]), maxI, maxJ, a[maxI][maxJ], b[maxI][maxJ], maxAbs, maxRel, atol, rtol,
	), false
}

// CompareIntPaths returns ("", true) iff a and b are point-for-point equal.
func CompareIntPaths(name string, a, b [][2]int) (string, bool) {
	if len(a) != len(b) {
		return fmt.Sprintf("%s: length differs: got %d, want %d", name, len(a), len(b)), false
	}
	for i := range a {
		if a[i] != b[i] {
			return fmt.Sprintf("%s: first divergence at index %d: got (%d,%d), want (%d,%d)",
				name, i, a[i][0], a[i][1], b[i][0], b[i][1]), false
		}
	}
	return "", true
}

// CompareBoolMasks returns ("", true) iff masks have identical length and content.
func CompareBoolMasks(name string, a, b []bool) (string, bool) {
	if len(a) != len(b) {
		return fmt.Sprintf("%s: length differs: got %d, want %d", name, len(a), len(b)), false
	}
	var first = -1
	var n int
	for i := range a {
		if a[i] != b[i] {
			n++
			if first == -1 {
				first = i
			}
		}
	}
	if n == 0 {
		return "", true
	}
	return fmt.Sprintf("%s: %d/%d frames differ; first at %d (got %v, want %v)",
		name, n, len(a), first, a[first], b[first]), false
}

// Fixture returns an absolute path to a file under testdata/.
// Example: Fixture("audioformats", "mono.16000.wav").
func Fixture(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(file), "..", "..", "testdata")
	all := append([]string{base}, parts...)
	return filepath.Clean(filepath.Join(all...))
}

// ReadMFCCText reads a whitespace-separated MFCC text file (as produced by
// numpy.savetxt) into a [rows][cols] matrix.
func ReadMFCCText(path string) ([][]float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	rows := make([][]float64, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		fields := strings.Fields(ln)
		row := make([]float64, len(fields))
		for i, f := range fields {
			v, err := parseFloat(f)
			if err != nil {
				return nil, fmt.Errorf("parse %q: %w", f, err)
			}
			row[i] = v
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscan(s, &f)
	return f, err
}
