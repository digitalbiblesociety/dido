package parity

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/digitalbiblesociety/dido/internal/audio"
	"github.com/digitalbiblesociety/dido/internal/dtw"
	"github.com/digitalbiblesociety/dido/internal/mfcc"
	"github.com/digitalbiblesociety/dido/internal/vad"
)

// These benchmarks run the same workload through Go and through a long-lived
// Python helper process so we can compare wall-time apples-to-apples.
//
// Run with:
//   go test -bench=Parity -benchmem -benchtime=3x ./internal/parity/
//
// Set AENEAS_PYTHON to override the python3 binary.
// The benchmarks are skipped if Python aeneas isn't importable.
//
// Because both implementations measure wall time (not allocations or
// instructions retired), result shape is "Go vs Python" rather than "Go".
// The Python helper's per-op cost includes JSON serialisation overhead;
// for large payloads that overhead is non-trivial, so prefer interpreting
// these as "end-user wall time on this workload".

// sharedServer is a process-wide Python helper used across benchmarks.
var (
	sharedServerOnce sync.Once
	sharedServer     *Server
	sharedServerErr  error
)

func getServer(b *testing.B) *Server {
	b.Helper()
	SkipIfUnavailable(b)
	sharedServerOnce.Do(func() {
		sharedServer, sharedServerErr = StartServer()
	})
	if sharedServerErr != nil {
		b.Fatalf("start server: %v", sharedServerErr)
	}
	return sharedServer
}

// --- MFCC ------------------------------------------------------------------

func benchParityMFCC(b *testing.B, fixtureFile string) {
	wf, err := audio.Open(Fixture("audioformats", fixtureFile))
	if err != nil {
		b.Fatal(err)
	}
	defer wf.Close()
	samples, err := wf.ReadAllSamples()
	if err != nil {
		b.Fatal(err)
	}
	sr := wf.Info.SampleRate
	p := mfcc.DefaultParams()

	b.Run("go", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := mfcc.ComputeFromData(samples, sr, p); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("py", func(b *testing.B) {
		srv := getServer(b)
		req := map[string]any{
			"op":          "mfcc_data",
			"samples":     samples,
			"sample_rate": sr,
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var resp struct {
				Error string `json:"error"`
			}
			if err := srv.Do(req, &resp); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkParityMFCC_Mono16000(b *testing.B) { benchParityMFCC(b, "mono.16000.wav") }
func BenchmarkParityMFCC_Mono22050(b *testing.B) { benchParityMFCC(b, "mono.22050.wav") }
func BenchmarkParityMFCC_Mono44100(b *testing.B) { benchParityMFCC(b, "mono.44100.wav") }
func BenchmarkParityMFCC_Mono48000(b *testing.B) { benchParityMFCC(b, "mono.48000.wav") }

// --- DTW -------------------------------------------------------------------

func benchParityDTW(b *testing.B, delta int) {
	path1 := Fixture("cdtw", "mfcc1_12_1332")
	path2 := Fixture("cdtw", "mfcc2_12_868")
	m1, err := ReadMFCCText(path1)
	if err != nil {
		b.Fatal(err)
	}
	m2, err := ReadMFCCText(path2)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("go", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			dtw.ComputePathStripe(m1, m2, delta)
		}
	})
	b.Run("py", func(b *testing.B) {
		srv := getServer(b)
		req := map[string]any{
			"op":    "dtw_path",
			"mfcc1": path1,
			"mfcc2": path2,
			"delta": delta,
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var resp struct {
				Error string `json:"error"`
			}
			if err := srv.Do(req, &resp); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkParityDTW_Delta300(b *testing.B)  { benchParityDTW(b, 300) }
func BenchmarkParityDTW_Delta1000(b *testing.B) { benchParityDTW(b, 1000) }
func BenchmarkParityDTW_Delta3000(b *testing.B) { benchParityDTW(b, 3000) }

// --- VAD -------------------------------------------------------------------

func benchParityVAD(b *testing.B, n int) {
	rng := rand.New(rand.NewSource(7))
	energy := make([]float64, n)
	for i := range energy {
		energy[i] = -10 + 5*rng.Float64()
	}
	p := vad.Params{LogEnergyThreshold: 0.699, MinNonspeechLength: 20}
	b.Run("go", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vad.RunVAD(energy, p)
		}
	})
	b.Run("py", func(b *testing.B) {
		srv := getServer(b)
		req := map[string]any{
			"op":                   "vad",
			"energy":               energy,
			"log_energy_threshold": p.LogEnergyThreshold,
			"min_nonspeech_length": p.MinNonspeechLength,
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var resp struct {
				Error string `json:"error"`
			}
			if err := srv.Do(req, &resp); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkParityVAD1k(b *testing.B)   { benchParityVAD(b, 1_000) }
func BenchmarkParityVAD10k(b *testing.B)  { benchParityVAD(b, 10_000) }
func BenchmarkParityVAD100k(b *testing.B) { benchParityVAD(b, 100_000) }

// --- Aggregate report ------------------------------------------------------

// TestParitySummary prints a single-line summary table comparing Go vs Python
// for one canonical workload of each operation. It runs only with -v and is
// otherwise a no-op. Useful as a quick "are we still ahead" check.
func TestParitySummary(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("run with -v")
	}
	SkipIfUnavailable(t)
	srv, err := StartServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	results := []struct {
		op    string
		goSec float64
		pySec float64
	}{}

	// MFCC
	{
		wf, err := audio.Open(Fixture("audioformats", "mono.16000.wav"))
		if err != nil {
			t.Fatal(err)
		}
		samples, _ := wf.ReadAllSamples()
		sr := wf.Info.SampleRate
		wf.Close()
		p := mfcc.DefaultParams()
		t0 := time.Now()
		mfcc.ComputeFromData(samples, sr, p)
		goSec := time.Since(t0).Seconds()
		req := map[string]any{"op": "mfcc_data", "samples": samples, "sample_rate": sr}
		t1 := time.Now()
		var resp struct {
			Error string `json:"error"`
		}
		_ = srv.Do(req, &resp)
		pySec := time.Since(t1).Seconds()
		results = append(results, struct {
			op    string
			goSec float64
			pySec float64
		}{"MFCC(mono.16000.wav)", goSec, pySec})
	}

	// DTW stripe
	{
		path1 := Fixture("cdtw", "mfcc1_12_1332")
		path2 := Fixture("cdtw", "mfcc2_12_868")
		m1, _ := ReadMFCCText(path1)
		m2, _ := ReadMFCCText(path2)
		t0 := time.Now()
		dtw.ComputePathStripe(m1, m2, 3000)
		goSec := time.Since(t0).Seconds()
		req := map[string]any{"op": "dtw_path", "mfcc1": path1, "mfcc2": path2, "delta": 3000}
		t1 := time.Now()
		var resp struct {
			Error string `json:"error"`
		}
		_ = srv.Do(req, &resp)
		pySec := time.Since(t1).Seconds()
		results = append(results, struct {
			op    string
			goSec float64
			pySec float64
		}{"DTW stripe d=3000", goSec, pySec})
	}

	// VAD
	{
		energy := make([]float64, 100_000)
		rng := rand.New(rand.NewSource(7))
		for i := range energy {
			energy[i] = -10 + 5*rng.Float64()
		}
		p := vad.Params{LogEnergyThreshold: 0.699, MinNonspeechLength: 20}
		t0 := time.Now()
		vad.RunVAD(energy, p)
		goSec := time.Since(t0).Seconds()
		req := map[string]any{
			"op":                   "vad",
			"energy":               energy,
			"log_energy_threshold": p.LogEnergyThreshold,
			"min_nonspeech_length": p.MinNonspeechLength,
		}
		t1 := time.Now()
		var resp struct {
			Error string `json:"error"`
		}
		_ = srv.Do(req, &resp)
		pySec := time.Since(t1).Seconds()
		results = append(results, struct {
			op    string
			goSec float64
			pySec float64
		}{"VAD 100k frames", goSec, pySec})
	}

	t.Logf("%-30s %14s %14s %10s", "operation", "go (s)", "py (s)", "speedup")
	for _, r := range results {
		t.Logf("%-30s %14.6f %14.6f %9.2fx", r.op, r.goSec, r.pySec,
			ratio(r.pySec, r.goSec))
	}
}

func ratio(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// String formatter used elsewhere in this package — silence "unused" warnings.
var _ = fmt.Sprintf
