package tts

import (
	"os/exec"
	"testing"
)

// BenchmarkSynthesizeMultiple measures synthesis throughput for N fragments.
// Run with: go test -bench=. -benchtime=1x ./internal/tts/
func BenchmarkSynthesizeMultiple4(b *testing.B)  { benchSynth(b, 4) }
func BenchmarkSynthesizeMultiple16(b *testing.B) { benchSynth(b, 16) }
func BenchmarkSynthesizeMultiple64(b *testing.B) { benchSynth(b, 64) }

func benchSynth(b *testing.B, n int) {
	if _, err := exec.LookPath("espeak-ng"); err != nil {
		b.Skip("espeak-ng not in PATH")
	}
	frags := make([]Fragment, n)
	for i := range frags {
		frags[i] = Fragment{Identifier: "f", Language: "eng", Text: "the quick brown fox"}
	}
	b.ResetTimer()
	for range b.N {
		if _, err := SynthesizeMultiple(frags, ""); err != nil {
			b.Fatal(err)
		}
	}
}
