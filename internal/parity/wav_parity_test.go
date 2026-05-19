package parity

import (
	"math"
	"testing"

	"github.com/digitalbiblesociety/dido/internal/audio"
)

// wavSamplesResp mirrors the JSON returned by the Python helper for "wav_samples".
type wavSamplesResp struct {
	Error         string    `json:"error"`
	Trace         string    `json:"trace"`
	SampleRate    int       `json:"sample_rate"`
	NumSamples    int       `json:"num_samples"`
	FirstSamples  []float64 `json:"first_samples"`
}

// TestWAVParity verifies that the Go WAV reader produces identical samples to
// Python aeneas's AudioFile reader on mono PCM 16-bit WAV inputs. (Python's
// AudioFile actually uses scipy.io.wavfile under the hood when no FFmpeg
// conversion is required.)
func TestWAVParity(t *testing.T) {
	SkipIfUnavailable(t)

	cases := []struct {
		name string
		file []string
	}{
		{"mono_16000", []string{"audioformats", "mono.16000.wav"}},
		{"mono_22050", []string{"audioformats", "mono.22050.wav"}},
		{"mono_44100", []string{"audioformats", "mono.44100.wav"}},
		{"mono_48000", []string{"audioformats", "mono.48000.wav"}},
		{"exact_5600_16000", []string{"audioformats", "exact.5600.16000.wav"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := Fixture(tc.file...)
			wf, err := audio.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer wf.Close()
			samples, err := wf.ReadAllSamples()
			if err != nil {
				t.Fatal(err)
			}

			req := map[string]any{
				"op":          "wav_samples",
				"wav":         path,
				"max_samples": 256,
			}
			var resp wavSamplesResp
			if err := RunOnce(req, &resp); err != nil {
				t.Fatalf("python helper: %v", err)
			}

			if uint32(resp.SampleRate) != wf.Info.SampleRate {
				t.Errorf("sample rate: got %d, want %d", wf.Info.SampleRate, resp.SampleRate)
			}
			if uint32(resp.NumSamples) != wf.Info.NumSamples {
				t.Errorf("num samples: got %d, want %d", wf.Info.NumSamples, resp.NumSamples)
			}
			for i, want := range resp.FirstSamples {
				if i >= len(samples) {
					t.Fatalf("Go sample buffer shorter than reference (i=%d)", i)
				}
				// scipy.io.wavfile and our reader both divide int16 by 32768,
				// so the float64 values are bit-identical.
				if math.Abs(samples[i]-want) > 1e-12 {
					t.Errorf("sample[%d]: got %g, want %g", i, samples[i], want)
				}
			}
		})
	}
}
