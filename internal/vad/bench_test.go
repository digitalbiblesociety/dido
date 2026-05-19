package vad

import (
	"math/rand"
	"testing"
)

// Run with: go test -bench=. -benchmem ./internal/vad/

func benchVAD(b *testing.B, n int) {
	rng := rand.New(rand.NewSource(7))
	energy := make([]float64, n)
	for i := range energy {
		energy[i] = -10 + 5*rng.Float64()
	}
	p := Params{LogEnergyThreshold: 0.699, MinNonspeechLength: 20}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RunVAD(energy, p)
	}
}

func BenchmarkVAD1k(b *testing.B)   { benchVAD(b, 1_000) }
func BenchmarkVAD10k(b *testing.B)  { benchVAD(b, 10_000) }
func BenchmarkVAD100k(b *testing.B) { benchVAD(b, 100_000) }
