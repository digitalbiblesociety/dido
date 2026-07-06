package config

import (
	"testing"
)

func TestDefaultValues(t *testing.T) {
	rc := Default()
	if rc.DTWAlgorithm != "stripe" {
		t.Errorf("DTWAlgorithm want stripe, got %q", rc.DTWAlgorithm)
	}
	if rc.MFCCFilters != 40 {
		t.Errorf("MFCCFilters want 40, got %d", rc.MFCCFilters)
	}
	if rc.TTS != "espeak-ng" {
		t.Errorf("TTS want espeak-ng, got %q", rc.TTS)
	}
	if !rc.SafetyChecks {
		t.Error("SafetyChecks should default true")
	}
}

func TestParseConfigString(t *testing.T) {
	m := ParseConfigString("key1=val1|key2=val2")
	if m["key1"] != "val1" {
		t.Errorf("key1 want val1, got %q", m["key1"])
	}
	if m["key2"] != "val2" {
		t.Errorf("key2 want val2, got %q", m["key2"])
	}
}

func TestParseConfigStringQuoted(t *testing.T) {
	m := ParseConfigString(`"k=v"`)
	if m["k"] != "v" {
		t.Errorf("got %q", m["k"])
	}
}

func TestParseOverridesDefaults(t *testing.T) {
	rc := Parse("dtw_algorithm=exact|mfcc_size=20|tts=festival")
	if rc.DTWAlgorithm != "exact" {
		t.Errorf("want exact, got %q", rc.DTWAlgorithm)
	}
	if rc.MFCCSize != 20 {
		t.Errorf("want 20, got %d", rc.MFCCSize)
	}
	if rc.TTS != "festival" {
		t.Errorf("want festival, got %q", rc.TTS)
	}
	// untouched field should keep its default
	if rc.MFCCFilters != 40 {
		t.Errorf("untouched MFCCFilters want 40, got %d", rc.MFCCFilters)
	}
}

func TestSetGranularity(t *testing.T) {
	rc := Default()
	rc.SetGranularity(2)
	if rc.DTWMargin != rc.DTWMarginL2 {
		t.Errorf("SetGranularity(2): DTWMargin want %f got %f", rc.DTWMarginL2, rc.DTWMargin)
	}
	if rc.MFCCWindowLength != rc.MFCCWindowLengthL2 {
		t.Error("SetGranularity(2): MFCCWindowLength mismatch")
	}
}

func TestSetTTS(t *testing.T) {
	rc := Default()
	rc.TTSL2 = "festival"
	rc.TTSPathL2 = "/usr/bin/festival"
	rc.SetTTS(2)
	if rc.TTS != "festival" {
		t.Errorf("SetTTS(2): want festival, got %q", rc.TTS)
	}
	if rc.TTSPath != "/usr/bin/festival" {
		t.Errorf("SetTTS(2): path want /usr/bin/festival, got %q", rc.TTSPath)
	}
}

func TestParseStrictUnknownKeys(t *testing.T) {
	// Known key only: no unknowns.
	if _, u := ParseStrict("mfcc_size=20"); len(u) != 0 {
		t.Errorf("expected no unknown keys, got %v", u)
	}
	// Typo of a known key — should be reported.
	_, u := ParseStrict("mfcc_size=20|mffc_size=20|task_language=eng")
	// mfcc_size is known, mffc_size is unknown, task_language is unknown
	// to RuntimeConfig (it's a task-level key) so both get reported here.
	want := map[string]bool{"mffc_size": true, "task_language": true}
	if len(u) != len(want) {
		t.Fatalf("got unknowns %v, want %v", u, want)
	}
	for _, k := range u {
		if !want[k] {
			t.Errorf("unexpected unknown key %q", k)
		}
	}
	// Sorted output.
	if u[0] >= u[1] {
		t.Errorf("unknown keys not sorted: %v", u)
	}
}

func TestParseStrictEmpty(t *testing.T) {
	rc, u := ParseStrict("")
	if u != nil {
		t.Errorf("empty config: expected nil unknowns, got %v", u)
	}
	if rc.DTWAlgorithm != "stripe" {
		t.Error("empty config should return defaults")
	}
}

func TestMFCCParams(t *testing.T) {
	rc := Default()
	p := rc.MFCCParams()
	if p.FilterBankSize != rc.MFCCFilters {
		t.Error("FilterBankSize mismatch")
	}
	if p.MFCCSize != rc.MFCCSize {
		t.Error("MFCCSize mismatch")
	}
}

func TestGlobalSettingsAndLevelOverrides(t *testing.T) {
	rc := Parse("dtw_margin_l2=3|dtw_margin=1|mfcc_window_length=0.2|mfcc_window_shift=0.08|mfcc_mask_nonspeech=true|mfcc_mask_nonspeech_l3=false")
	for _, level := range []int{1, 2, 3, 4, 1} {
		rc.SetGranularity(level)
		wantWidth := 25
		if level == 2 {
			wantWidth = 75
		}
		if rc.DTWStripeWidth() != wantWidth || rc.MFCCWindowLength != 0.2 || rc.MFCCWindowShift != 0.08 || rc.MFCCMaskNonspeech != (level < 3) {
			t.Errorf("level %d: width %d, window %g, shift %g, mask %t", level,
				rc.DTWStripeWidth(), rc.MFCCWindowLength, rc.MFCCWindowShift, rc.MFCCMaskNonspeech)
		}
	}
}

func TestExactDTWIgnoresStripeMargin(t *testing.T) {
	rc := Parse("dtw_algorithm=exact|dtw_margin=0.2")
	rc.SetGranularity(1)
	if width := rc.DTWStripeWidth(); width != 0 {
		t.Errorf("exact DTW requested a %d-frame stripe", width)
	}
}
