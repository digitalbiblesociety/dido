package tts

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func skipIfNoEspeak(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("espeak-ng"); err != nil {
		t.Skip("espeak-ng not in PATH; skipping TTS test")
	}
}

func TestVoiceFor(t *testing.T) {
	cases := []struct{ lang, want string }{
		{"eng", "en"},
		{"fra", "fr"},
		{"deu", "de"},
		{"en", "en"},
		{"en-gb", "en-gb"},
		{"ukr", "uk"}, // Ukrainian (native voice since espeak-ng ≥1.49)
		{"uk", "uk"},
		{"zzz", "en"}, // unknown falls back to default
	}
	for _, c := range cases {
		got := VoiceFor(c.lang)
		if got != c.want {
			t.Errorf("VoiceFor(%q) = %q, want %q", c.lang, got, c.want)
		}
	}
}

func TestResolveBinaryPathOverride(t *testing.T) {
	path, err := ResolveBinaryPath("/usr/bin/espeak-ng")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/usr/bin/espeak-ng" {
		t.Errorf("got %q", path)
	}
}

func TestResolveBinaryPathLookup(t *testing.T) {
	skipIfNoEspeak(t)
	path, err := ResolveBinaryPath("")
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Error("empty path returned")
	}
}

func TestSynthesizeMultipleEmptyText(t *testing.T) {
	skipIfNoEspeak(t)
	fragments := []Fragment{
		{Identifier: "f1", Language: "eng", Text: "Hello world."},
		{Identifier: "f2", Language: "eng", Text: ""},
		{Identifier: "f3", Language: "eng", Text: "Goodbye."},
	}
	res, err := SynthesizeMultiple(fragments, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.SampleRate != DefaultSampleRate {
		t.Errorf("sample rate: got %d, want %d", res.SampleRate, DefaultSampleRate)
	}
	if len(res.Intervals) != 3 {
		t.Fatalf("intervals: got %d, want 3", len(res.Intervals))
	}
	// f2 is empty: begin == end
	if !res.Intervals[1].HasZeroLength() {
		t.Errorf("empty fragment interval should be zero-length: got %v", res.Intervals[1])
	}
	// intervals are monotonically non-decreasing
	for i := 1; i < len(res.Intervals); i++ {
		if res.Intervals[i].Begin.Less(res.Intervals[i-1].Begin) {
			t.Errorf("intervals not monotonic at %d", i)
		}
	}
	// total samples == end of last non-empty fragment
	lastEnd := res.Intervals[2].End.Float64()
	expectedSamples := int(lastEnd * float64(DefaultSampleRate))
	if len(res.Samples) != expectedSamples {
		// Allow ±1 for rounding
		diff := len(res.Samples) - expectedSamples
		if diff < -1 || diff > 1 {
			t.Errorf("sample count: got %d, expected ~%d (end=%.3fs)", len(res.Samples), expectedSamples, lastEnd)
		}
	}
}

// TestSynthesisErrorFormat verifies the SynthesisError prints the fragment
// metadata and truncates very long Text payloads.
func TestSynthesisErrorFormat(t *testing.T) {
	short := &SynthesisError{
		FragmentID: "f000042",
		Language:   "eng",
		Voice:      "en",
		Text:       "Hello, world.",
		Err:        errors.New("exit status 1"),
	}
	msg := short.Error()
	for _, w := range []string{"f000042", "eng", "en", "Hello, world.", "exit status 1"} {
		if !strings.Contains(msg, w) {
			t.Errorf("SynthesisError message missing %q: %s", w, msg)
		}
	}

	// Truncation: long Text should be replaced by an ellipsis-suffixed prefix.
	long := strings.Repeat("a", 200)
	se := &SynthesisError{FragmentID: "x", Text: long, Err: errors.New("e")}
	msg = se.Error()
	if !strings.Contains(msg, "…") {
		t.Errorf("expected truncation ellipsis in %s", msg)
	}
	if strings.Contains(msg, long) {
		t.Error("full long text should not appear verbatim")
	}
}

// TestResolveTTSConcurrency exercises the priority chain: explicit arg →
// env var → NumCPU. NumCPU is non-trivial to fake; covered indirectly by
// the "default branch" assertion below.
func TestResolveTTSConcurrency(t *testing.T) {
	t.Setenv("DIDO_TTS_WORKERS", "")
	if got := resolveTTSConcurrency(4); got != 4 {
		t.Errorf("explicit arg: got %d, want 4", got)
	}
	t.Setenv("DIDO_TTS_WORKERS", "7")
	if got := resolveTTSConcurrency(0); got != 7 {
		t.Errorf("env var: got %d, want 7", got)
	}
	t.Setenv("DIDO_TTS_WORKERS", "garbage")
	got := resolveTTSConcurrency(0)
	if got < 1 {
		t.Errorf("invalid env should fall back to ≥1, got %d", got)
	}
	t.Setenv("DIDO_TTS_WORKERS", "0")
	got = resolveTTSConcurrency(0)
	if got < 1 {
		t.Errorf("zero env should fall back to ≥1, got %d", got)
	}
	t.Setenv("DIDO_TTS_WORKERS", "")
	got = resolveTTSConcurrency(0)
	if got < 1 {
		t.Errorf("no env should yield NumCPU ≥1, got %d", got)
	}
}

// TestSynthesisErrorUnwrap confirms errors.As/Is round-trips.
func TestSynthesisErrorUnwrap(t *testing.T) {
	inner := errors.New("boom")
	wrapped := &SynthesisError{Err: inner}

	var se *SynthesisError
	if !errors.As(wrapped, &se) {
		t.Fatal("errors.As should match SynthesisError")
	}
	if !errors.Is(wrapped, inner) {
		t.Fatal("errors.Is should unwrap to the inner error")
	}
}

func TestSynthesizeMultipleAllEmpty(t *testing.T) {
	skipIfNoEspeak(t)
	fragments := []Fragment{
		{Identifier: "f1", Language: "eng", Text: ""},
	}
	res, err := SynthesizeMultiple(fragments, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Samples) != 0 {
		t.Errorf("expected 0 samples for all-empty input, got %d", len(res.Samples))
	}
}
