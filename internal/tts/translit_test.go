package tts

import (
	"strings"
	"testing"
)

func TestIsFallbackVoice(t *testing.T) {
	cases := []struct {
		lang     string
		fallback bool
	}{
		{"eng", false},
		{"en", false},
		{"fra", false},
		{"deu", false},
		{"hin", false},  // espeak-ng has a hi voice
		{"unknown_xx", true},
		{"syr", true}, // Syriac — no native voice in espeak-ng
		{"", true},    // empty defaults to English fallback
	}
	for _, c := range cases {
		got := IsFallbackVoice(c.lang)
		if got != c.fallback {
			t.Errorf("IsFallbackVoice(%q) = %v, want %v", c.lang, got, c.fallback)
		}
	}
}

func TestPrepareTextForSpeechNativeVoice(t *testing.T) {
	// Hindi has a native eSpeak-ng voice → no transliteration even though
	// the script is Devanagari.
	in := "नमस्ते"
	out, src := PrepareTextForSpeech(in, "hin")
	if out != in {
		t.Errorf("native voice should not transliterate; got %q, want %q", out, in)
	}
	if src != "" {
		t.Errorf("expected empty source script tag; got %q", src)
	}
}

func TestPrepareTextForSpeechFallbackGreek(t *testing.T) {
	// "grc" (Ancient Greek) actually has a native espeak-ng voice. Use a
	// fake language code to force the fallback path.
	in := "Ἰησοῦς Χριστός"
	out, src := PrepareTextForSpeech(in, "unknown_grc_xx")
	if out == in {
		t.Fatalf("fallback voice should transliterate non-Latin text; got unchanged %q", in)
	}
	if src != "Grek" {
		t.Errorf("source script tag: got %q, want %q", src, "Grek")
	}
	// Smoke-check the transliteration produced ASCII-ish output (no
	// Greek code points).
	for _, r := range out {
		if r >= 0x0370 && r <= 0x03FF {
			t.Errorf("transliterated text still contains Greek rune %U: %q", r, out)
			break
		}
	}
	if !strings.Contains(strings.ToLower(out), "iesous") &&
		!strings.Contains(out, "Iēsous") {
		t.Errorf("transliterated Greek doesn't look like 'Iesous Christos': %q", out)
	}
}

func TestPrepareTextForSpeechFallbackLatin(t *testing.T) {
	// Already-Latin text under an unknown language → no engine match →
	// returned unchanged.
	in := "Hello world."
	out, src := PrepareTextForSpeech(in, "unknown_xx")
	if out != in {
		t.Errorf("latin text under fallback voice should pass through; got %q", out)
	}
	if src != "" {
		t.Errorf("no source script tag expected; got %q", src)
	}
}

func TestPrepareTextForSpeechDisabled(t *testing.T) {
	AutoTransliterateDisabled.Store(1)
	t.Cleanup(func() { AutoTransliterateDisabled.Store(0) })

	in := "Ἰησοῦς Χριστός"
	out, src := PrepareTextForSpeech(in, "unknown_xx")
	if out != in {
		t.Errorf("when disabled, text should pass through; got %q", out)
	}
	if src != "" {
		t.Errorf("disabled flag should suppress script tag; got %q", src)
	}
}

func TestPrepareTextForSpeechEmpty(t *testing.T) {
	out, src := PrepareTextForSpeech("", "unknown_xx")
	if out != "" || src != "" {
		t.Errorf("empty input: got (%q, %q)", out, src)
	}
}
