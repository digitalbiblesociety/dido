package dtw

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Run with: go test -bench=. -benchmem ./internal/dtw/
//
// These benchmarks run the stripe DTW on the canonical (mfcc1_12_1332,
// mfcc2_12_868) fixture pair at several delta values, then run the exact
// (full) DTW on synthetic matrices for a baseline.

func loadMFCCText(t testing.TB, name string) [][]float64 {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "cdtw")
	data, err := os.ReadFile(filepath.Join(base, name))
	if err != nil {
		t.Fatal(err)
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
			var v float64
			if _, err := fmt.Sscan(f, &v); err != nil {
				t.Fatal(err)
			}
			row[i] = v
		}
		rows = append(rows, row)
	}
	return rows
}

func benchStripe(b *testing.B, delta int) {
	m1 := loadMFCCText(b, "mfcc1_12_1332")
	m2 := loadMFCCText(b, "mfcc2_12_868")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputePathStripe(m1, m2, delta)
	}
}

func BenchmarkStripeDelta300(b *testing.B)  { benchStripe(b, 300) }
func BenchmarkStripeDelta1000(b *testing.B) { benchStripe(b, 1000) }
func BenchmarkStripeDelta3000(b *testing.B) { benchStripe(b, 3000) }

func BenchmarkExactDTW(b *testing.B) {
	// 200x150 frames matches the dimensions of typical aligned-fragment
	// query/audio slices used by SD.
	m1 := synthMFCC(b, 13, 200)
	m2 := synthMFCC(b, 13, 150)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputePathExact(m1, m2)
	}
}

func synthMFCC(t testing.TB, l, n int) [][]float64 {
	t.Helper()
	m := make([][]float64, l)
	for i := range m {
		m[i] = make([]float64, n)
		for j := range m[i] {
			// Deterministic pseudo-MFCC values.
			m[i][j] = float64((i*7+j*13)%97) - 48
		}
	}
	return m
}
