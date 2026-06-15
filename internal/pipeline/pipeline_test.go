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
