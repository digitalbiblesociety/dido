package audio

import (
	"bytes"
	"math"
	"path/filepath"
	"runtime"
	"testing"
)

// Run with: go test -bench=. -benchmem ./internal/audio/

func fixture(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(file), "..", "..", "testdata")
	all := append([]string{base}, parts...)
	return filepath.Join(all...)
}

func benchRead(b *testing.B, name string) {
	path := fixture("audioformats", name)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wf, err := Open(path)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := wf.ReadAllSamples(); err != nil {
			b.Fatal(err)
		}
		wf.Close()
	}
}

func BenchmarkReadMono16000(b *testing.B) { benchRead(b, "mono.16000.wav") }
func BenchmarkReadMono22050(b *testing.B) { benchRead(b, "mono.22050.wav") }
func BenchmarkReadMono44100(b *testing.B) { benchRead(b, "mono.44100.wav") }
func BenchmarkReadMono48000(b *testing.B) { benchRead(b, "mono.48000.wav") }

// BenchmarkReadReusedScratch measures the steady-state allocation cost when
// the byte scratch buffer is reused across many ReadSamples calls — the
// typical batch-processing pattern.
func BenchmarkReadReusedScratch(b *testing.B) {
	path := fixture("audioformats", "mono.16000.wav")
	wf, err := Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer wf.Close()
	n := wf.Info.NumSamples
	dst := make([]float64, n)
	scratch := make([]byte, int(n)*int(wf.Info.BytesPerSample))
	b.SetBytes(int64(n) * int64(wf.Info.BytesPerSample))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := wf.ReadSamplesWithScratch(0, n, dst, scratch); err != nil {
			b.Fatal(err)
		}
	}
}

// benchWrite times WriteMonoPCM16 on a synthetic sine wave of `seconds`
// duration at 16 kHz. Captures the cost of the bulk-encode path now used
// in production.
func benchWrite(b *testing.B, seconds float64) {
	const sr uint32 = 16000
	n := int(float64(sr) * seconds)
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = 0.5 * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
	}
	var buf bytes.Buffer
	buf.Grow(n*2 + 44)
	b.SetBytes(int64(n) * 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := WriteMonoPCM16(&buf, sr, samples); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWrite1s(b *testing.B)  { benchWrite(b, 1.0) }
func BenchmarkWrite10s(b *testing.B) { benchWrite(b, 10.0) }
func BenchmarkWrite60s(b *testing.B) { benchWrite(b, 60.0) }
