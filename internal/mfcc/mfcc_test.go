package mfcc

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/digitalbiblesociety/dido/internal/audio"
)

// TestComputeFromData runs MFCC on the standard test WAV and verifies
// basic shape and value invariants.
func TestComputeFromData(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "audioformats", "mono.16000.wav")
	wf, err := audio.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wf.Close()

	samples, err := wf.ReadAllSamples()
	if err != nil {
		t.Fatal(err)
	}

	p := DefaultParams()
	mfcc, err := ComputeFromData(samples, wf.Info.SampleRate, p)
	if err != nil {
		t.Fatal(err)
	}

	if len(mfcc) != p.MFCCSize {
		t.Fatalf("mfcc rows: got %d, want %d", len(mfcc), p.MFCCSize)
	}
	expectedFrames := int(float64(len(samples)) / float64(p.WindowShift*float64(wf.Info.SampleRate)))
	if len(mfcc[0]) != expectedFrames {
		t.Errorf("num frames: got %d, want %d", len(mfcc[0]), expectedFrames)
	}

	// MFCC values should be finite and in a reasonable range.
	for i := range mfcc {
		for j, v := range mfcc[i] {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Errorf("mfcc[%d][%d] = %f is not finite", i, j, v)
			}
		}
	}
}

// TestDefaultParamsMatchC verifies our default parameters match the C defaults.
func TestDefaultParamsMatchC(t *testing.T) {
	p := DefaultParams()
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"FilterBankSize", float64(p.FilterBankSize), 40},
		{"MFCCSize", float64(p.MFCCSize), 13},
		{"FFTOrder", float64(p.FFTOrder), 512},
		{"LowerFrequency", p.LowerFrequency, 133.3333},
		{"UpperFrequency", p.UpperFrequency, 6855.4976},
		{"EmphasisFactor", p.EmphasisFactor, 0.97},
		{"WindowLength", p.WindowLength, 0.025},
		{"WindowShift", p.WindowShift, 0.010},
	}
	for _, tt := range tests {
		if math.Abs(tt.got-tt.want) > 1e-6 {
			t.Errorf("%s: got %f, want %f", tt.name, tt.got, tt.want)
		}
	}
}

// TestMelConversions checks hz2mel/mel2hz round-trip.
func TestMelConversions(t *testing.T) {
	for _, hz := range []float64{100, 440, 1000, 4000, 8000} {
		mel := hz2mel(hz)
		back := mel2hz(mel)
		if math.Abs(back-hz) > 1e-6 {
			t.Errorf("round-trip hz=%f: mel=%f, back=%f", hz, mel, back)
		}
	}
}

// TestParamsValidate exercises Params.Validate's coverage of bad inputs.
func TestParamsValidate(t *testing.T) {
	t.Run("default_ok", func(t *testing.T) {
		if err := DefaultParams().Validate(); err != nil {
			t.Fatalf("default params should validate, got: %v", err)
		}
	})
	cases := []struct {
		name string
		mut  func(*Params)
		want string // substring expected in the error
	}{
		{"fftorder_zero", func(p *Params) { p.FFTOrder = 0 }, "FFTOrder must be > 0"},
		{"fftorder_negative", func(p *Params) { p.FFTOrder = -1 }, "FFTOrder must be > 0"},
		{"fftorder_not_power_of_2", func(p *Params) { p.FFTOrder = 500 }, "power of 2"},
		{"filterbank_zero", func(p *Params) { p.FilterBankSize = 0 }, "FilterBankSize must be > 0"},
		{"mfccsize_zero", func(p *Params) { p.MFCCSize = 0 }, "MFCCSize must be > 0"},
		{"mfccsize_too_big", func(p *Params) { p.MFCCSize = 50 }, "cannot exceed FilterBankSize"},
		{"lower_negative", func(p *Params) { p.LowerFrequency = -1 }, "LowerFrequency must be ≥ 0"},
		{"upper_leq_lower", func(p *Params) { p.UpperFrequency = 100 }, "must be > LowerFrequency"},
		{"window_length_zero", func(p *Params) { p.WindowLength = 0 }, "WindowLength must be > 0"},
		{"window_shift_zero", func(p *Params) { p.WindowShift = 0 }, "WindowShift must be > 0"},
		{"emphasis_too_big", func(p *Params) { p.EmphasisFactor = 1.5 }, "EmphasisFactor must be in [0, 1]"},
		{"emphasis_negative", func(p *Params) { p.EmphasisFactor = -0.1 }, "EmphasisFactor must be in [0, 1]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultParams()
			tc.mut(&p)
			err := p.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

// TestComputeFromDataBadParams checks that ComputeFromData rejects bad
// configurations and the bad-sampleRate edge case.
func TestComputeFromDataBadParams(t *testing.T) {
	samples := make([]float64, 1000)

	if _, err := ComputeFromData(samples, 16000, Params{}); err == nil {
		t.Error("zero params should fail validation")
	}
	if _, err := ComputeFromData(samples, 0, DefaultParams()); err == nil {
		t.Error("sampleRate=0 should fail")
	}
	if _, err := ComputeFromData([]float64{}, 16000, DefaultParams()); err == nil {
		t.Error("empty data should fail")
	}
	// WindowShift small enough that shift * sampleRate floors to 0.
	p := DefaultParams()
	p.WindowShift = 0.00001
	if _, err := ComputeFromData(samples, 16000, p); err == nil {
		t.Error("zero-sample frame shift should fail")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestHammingCoefficients spot-checks the Hamming window.
func TestHammingCoefficients(t *testing.T) {
	// Hamming(N): w[k] = 0.54 - 0.46*cos(2pi*k/(N-1))
	// w[0] = 0.54 - 0.46 = 0.08, w[N/2] ≈ 1.0
	h := precomputeHamming(512)
	if math.Abs(h[0]-0.08) > 1e-9 {
		t.Errorf("hamming[0] = %f, want 0.08", h[0])
	}
	if h[256] < 0.99 {
		t.Errorf("hamming[256] = %f, want ~1.0", h[256])
	}
}
