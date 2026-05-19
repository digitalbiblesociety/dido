package parity

import (
	"math/rand"
	"testing"

	"github.com/digitalbiblesociety/dido/internal/audio"
	"github.com/digitalbiblesociety/dido/internal/mfcc"
	"github.com/digitalbiblesociety/dido/internal/vad"
)

// vadResp mirrors the JSON returned by the Python helper for "vad".
type vadResp struct {
	Error string `json:"error"`
	Trace string `json:"trace"`
	Mask  []bool `json:"mask"`
}

// TestVADParitySynthetic compares Go's VAD against Python's VAD on a
// series of synthetic energy vectors. Because VAD is a pure
// comparison-and-bool-mask operation, bit-exact equality is required.
func TestVADParitySynthetic(t *testing.T) {
	SkipIfUnavailable(t)

	rng := rand.New(rand.NewSource(42))

	cases := []struct {
		name              string
		energy            []float64
		threshold         float64
		minNonspeechLen   int
		extendBefore      int
		extendAfter       int
	}{
		{
			name:            "all_speech_uniform",
			energy:          repeat(5.0, 50),
			threshold:       1.0,
			minNonspeechLen: 5,
		},
		{
			name:            "single_silence_gap",
			energy:          concat(repeat(5.0, 20), repeat(0.0, 10), repeat(5.0, 20)),
			threshold:       2.0,
			minNonspeechLen: 5,
		},
		{
			name:            "two_silence_gaps",
			energy:          concat(repeat(5.0, 15), repeat(0.0, 8), repeat(5.0, 15), repeat(0.0, 8), repeat(5.0, 15)),
			threshold:       2.0,
			minNonspeechLen: 5,
		},
		{
			name:            "noisy_speech_random",
			energy:          randEnergy(rng, 100, 1.0, 5.0),
			threshold:       0.5,
			minNonspeechLen: 4,
		},
		{
			name:            "with_extend_before",
			energy:          concat(repeat(5.0, 20), repeat(0.0, 10), repeat(5.0, 20)),
			threshold:       2.0,
			minNonspeechLen: 5,
			extendBefore:    2,
		},
		{
			name:            "with_extend_after",
			energy:          concat(repeat(5.0, 20), repeat(0.0, 10), repeat(5.0, 20)),
			threshold:       2.0,
			minNonspeechLen: 5,
			extendAfter:     2,
		},
		{
			name:            "with_extend_both",
			energy:          concat(repeat(5.0, 30), repeat(0.0, 20), repeat(5.0, 30)),
			threshold:       2.0,
			minNonspeechLen: 10,
			extendBefore:    3,
			extendAfter:     3,
		},
		// Intentional Go-only deviation: when min_nonspeech_length > len(energy),
		// the Go port returns an all-speech mask. Python's numpy.rolling_window
		// raises a ValueError on negative dimensions in this case — covered in
		// the vad package's TestRunVADWindowLargerThanVector instead.
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goMask := vad.RunVAD(tc.energy, vad.Params{
				LogEnergyThreshold: tc.threshold,
				MinNonspeechLength: tc.minNonspeechLen,
				ExtendBefore:       tc.extendBefore,
				ExtendAfter:        tc.extendAfter,
			})

			req := map[string]any{
				"op":                   "vad",
				"energy":               tc.energy,
				"log_energy_threshold": tc.threshold,
				"min_nonspeech_length": tc.minNonspeechLen,
				"extend_before":        tc.extendBefore,
				"extend_after":         tc.extendAfter,
			}
			var resp vadResp
			if err := RunOnce(req, &resp); err != nil {
				t.Fatalf("python helper: %v", err)
			}
			if resp.Error != "" {
				t.Fatalf("python error: %s", resp.Error)
			}
			if msg, ok := CompareBoolMasks(tc.name, goMask, resp.Mask); !ok {
				t.Error(msg)
			}
		})
	}
}

// TestVADParityFromAudio runs VAD on the actual audio fixtures using the Go
// MFCC's row 0 as the energy vector. Even though the MFCC itself has some
// numerical drift, the row-0 energies are dominated by signal magnitude and
// produce identical speech/silence masks at any reasonable threshold.
func TestVADParityFromAudio(t *testing.T) {
	SkipIfUnavailable(t)

	wavs := []string{
		"vad/n.wav",  // noise (no speech)
		"vad/s.wav",  // speech
		"vad/ns.wav", // noise then speech
		"vad/sn.wav", // speech then noise
	}
	for _, w := range wavs {
		t.Run(w, func(t *testing.T) {
			path := Fixture(splitPath(w)...)
			wf, err := audio.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer wf.Close()
			samples, err := wf.ReadAllSamples()
			if err != nil {
				t.Fatal(err)
			}

			matrix, err := mfcc.ComputeFromData(samples, wf.Info.SampleRate, mfcc.DefaultParams())
			if err != nil {
				t.Fatal(err)
			}
			if len(matrix) == 0 || len(matrix[0]) == 0 {
				t.Skip("empty mfcc matrix")
			}
			energy := matrix[0]
			params := vad.Params{LogEnergyThreshold: 0.699, MinNonspeechLength: 20}

			goMask := vad.RunVAD(energy, params)

			req := map[string]any{
				"op":                   "vad",
				"energy":               energy,
				"log_energy_threshold": params.LogEnergyThreshold,
				"min_nonspeech_length": params.MinNonspeechLength,
				"extend_before":        params.ExtendBefore,
				"extend_after":         params.ExtendAfter,
			}
			var resp vadResp
			if err := RunOnce(req, &resp); err != nil {
				t.Fatalf("python helper: %v", err)
			}
			if msg, ok := CompareBoolMasks(w, goMask, resp.Mask); !ok {
				t.Error(msg)
			}
		})
	}
}

// helpers

func repeat(v float64, n int) []float64 {
	xs := make([]float64, n)
	for i := range xs {
		xs[i] = v
	}
	return xs
}

func concat(xs ...[]float64) []float64 {
	total := 0
	for _, x := range xs {
		total += len(x)
	}
	out := make([]float64, 0, total)
	for _, x := range xs {
		out = append(out, x...)
	}
	return out
}

func randEnergy(rng *rand.Rand, n int, lo, hi float64) []float64 {
	xs := make([]float64, n)
	for i := range xs {
		xs[i] = lo + rng.Float64()*(hi-lo)
	}
	return xs
}

// splitPath splits "a/b" into ["a", "b"] for Fixture.
func splitPath(s string) []string {
	var out []string
	last := 0
	for i, c := range s {
		if c == '/' {
			out = append(out, s[last:i])
			last = i + 1
		}
	}
	out = append(out, s[last:])
	return out
}
