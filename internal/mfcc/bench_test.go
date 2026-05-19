package mfcc

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/digitalbiblesociety/dido/internal/audio"
)

// Run with: go test -bench=. -benchmem ./internal/mfcc/
//
// These benchmarks measure the Go MFCC pipeline (radix-2 FFT + mel filter bank
// + DCT) on the standard mono WAV fixtures. We add parallel parity benchmarks
// in internal/parity that compare against Python's cmfcc on the same inputs.

func fixturePath(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(file), "..", "..", "testdata")
	all := append([]string{base}, parts...)
	return filepath.Join(all...)
}

func benchMFCC(b *testing.B, fixture string) {
	wf, err := audio.Open(fixturePath("audioformats", fixture))
	if err != nil {
		b.Fatal(err)
	}
	defer wf.Close()
	samples, err := wf.ReadAllSamples()
	if err != nil {
		b.Fatal(err)
	}
	p := DefaultParams()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ComputeFromData(samples, wf.Info.SampleRate, p); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMFCC_Mono16000(b *testing.B) { benchMFCC(b, "mono.16000.wav") }
func BenchmarkMFCC_Mono22050(b *testing.B) { benchMFCC(b, "mono.22050.wav") }
func BenchmarkMFCC_Mono44100(b *testing.B) { benchMFCC(b, "mono.44100.wav") }
func BenchmarkMFCC_Mono48000(b *testing.B) { benchMFCC(b, "mono.48000.wav") }
func BenchmarkMFCC_Exact5600(b *testing.B) { benchMFCC(b, "exact.5600.16000.wav") }

// benchMFCCCompiled measures the steady-state Compute cost when the static
// tables are pre-compiled once and reused across calls — the use case the
// SD detector and any batch caller cares about.
func benchMFCCCompiled(b *testing.B, fixture string) {
	wf, err := audio.Open(fixturePath("audioformats", fixture))
	if err != nil {
		b.Fatal(err)
	}
	defer wf.Close()
	samples, err := wf.ReadAllSamples()
	if err != nil {
		b.Fatal(err)
	}
	c, err := Compile(DefaultParams(), wf.Info.SampleRate)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Compute(samples); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMFCCCompiled_Mono16000(b *testing.B) { benchMFCCCompiled(b, "mono.16000.wav") }
func BenchmarkMFCCCompiled_Exact5600(b *testing.B) { benchMFCCCompiled(b, "exact.5600.16000.wav") }

// BenchmarkMFCCShortSlices simulates the SD detector pattern: many short
// audio buffers MFCC'd in a loop. Compile-once is materially faster than
// the original per-call ComputeFromData here.
func BenchmarkMFCCShortSlices(b *testing.B) {
	const sr uint32 = 16000
	// 50 buffers of ~0.5 s each.
	bufs := make([][]float64, 50)
	for i := range bufs {
		bufs[i] = make([]float64, sr/2)
	}
	p := DefaultParams()

	b.Run("compile_each_call", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, buf := range bufs {
				if _, err := ComputeFromData(buf, sr, p); err != nil {
					b.Fatal(err)
				}
			}
		}
	})

	b.Run("compile_once", func(b *testing.B) {
		c, err := Compile(p, sr)
		if err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, buf := range bufs {
				if _, err := c.Compute(buf); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

// BenchmarkFFT512 isolates one frame's worth of FFT (the inner loop hot spot).
func BenchmarkFFT512(b *testing.B) {
	m := 512
	sinFull := precomputeSinTable(m)
	sinHalf := precomputeSinTable(m / 2)
	x := make([]float64, m)
	y := make([]float64, m+m/2+2)
	for i := range x {
		x[i] = float64(i%17) - 8
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(x, x[:1])
		for k := 1; k < m; k++ {
			x[k] = float64(k%17) - 8
		}
		for k := range y {
			y[k] = 0
		}
		rfftInPlace(x, y, m, sinFull, sinHalf)
	}
}
