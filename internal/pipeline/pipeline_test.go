package pipeline

import (
	"os/exec"
	"testing"

	"github.com/digitalbiblesociety/dido/internal/config"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/timing"
	"github.com/digitalbiblesociety/dido/internal/tts"
)

// Exercise the serial MFCC scratch buffers on both branches under -race.
func TestAlignmentWithMatchingSampleRates(t *testing.T) {
	if _, err := exec.LookPath("espeak-ng"); err != nil {
		t.Skip("espeak-ng unavailable")
	}
	caches := mfccCaches{recording: make(compiledCache), synthesis: make(compiledCache)}
	src := staticSamples(make([]float64, tts.DefaultSampleRate), tts.DefaultSampleRate)
	frags := []*text.Fragment{{Lines: []string{"hello"}}}
	_, err := executeWithSource(src, timing.Zero, frags, TaskConfig{Language: "eng"}, config.Default(), true, caches)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFilteredTextForSynthesis(t *testing.T) {
	tf, err := text.ReadLines([]string{"INTRO", "INTRO hello"}, text.FormatPlain, text.Params{IgnoreRegex: "^INTRO"})
	if err != nil {
		t.Fatal(err)
	}
	frags := append(tf.Fragments(), &text.Fragment{Lines: []string{"unfiltered"}})
	got := toTTSFragments(frags, "eng")
	for i, want := range []string{"", "hello", "unfiltered"} {
		if got[i].Text != want {
			t.Errorf("fragment %d: got %q, want %q", i, got[i].Text, want)
		}
	}
}
