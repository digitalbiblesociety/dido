package tts

import "github.com/digitalbiblesociety/transliterate/script"

// prepareTextForSpeech is the pre-flight transformation applied to a
// fragment's text before it is fed to eSpeak-ng. When the assigned voice
// is the English fallback (i.e. lang had no native voice mapping) AND
// the text is dominated by a non-Latin script with a known
// transliterator, the text is romanised so the English voice produces
// recognisable phonetics. In every other case the input is returned
// unchanged.
//
// Returns the (possibly transformed) text plus the ISO 15924 script tag
// of the source script when transliteration was performed (empty
// otherwise). The script tag is exposed for logging/diagnostics — it's
// what a future SynthesisError.SourceScript field would carry.
func (opts Options) prepareTextForSpeech(text, lang string) (out string, sourceScript string) {
	if opts.DisableTransliteration {
		return text, ""
	}
	if !IsFallbackVoice(lang) {
		return text, ""
	}
	if text == "" {
		return text, ""
	}
	eng := script.Detect(text)
	if eng == nil {
		// No non-Latin script detected (e.g. already-Latin text, or a
		// script outside the library's coverage). Leave it alone.
		return text, ""
	}
	return eng.Transliterate(text), eng.Name
}
