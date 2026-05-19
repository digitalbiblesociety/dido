// Package tts is the eSpeak subprocess wrapper. It synthesises one
// audio segment per text fragment and returns the concatenated PCM
// audio with per-fragment time intervals.
//
// Two engines are supported: EngineEspeakNG (default) and EngineEspeak.
// Both share the same `-v VOICE -w WAV stdin` CLI; ResolveBinary picks
// the binary via DIDO_ESPEAK_NG_PATH or DIDO_ESPEAK_PATH, or by looking
// up `espeak-ng` / `espeak` on PATH. The chosen binary must be
// installed; dido does not ship one.
//
// Concurrency
//
// SynthesizeMultiple spawns up to N parallel espeak-ng processes
// where N is, in priority order:
//   1. the workers argument to SynthesizeMultipleWith (if > 0);
//   2. the DIDO_TTS_WORKERS environment variable (positive int);
//   3. runtime.NumCPU().
//
// The minimum is always 1. The pipeline package wires tts_concurrency
// from the config string through to SynthesizeMultipleWith.
//
// Error reporting
//
// On per-fragment failure, the call returns a *SynthesisError carrying
// the fragment id, language, resolved voice, and a truncated snippet
// of the offending text. errors.As / errors.Is unwrap to the underlying
// espeak-ng exit-status error.
//
// Automatic transliteration
//
// When a fragment's Language has no native espeak-ng voice (so VoiceFor
// falls back to "en") AND the text is dominated by a non-Latin script,
// the text is romanised before synthesis via PrepareTextForSpeech.
// This avoids the English voice mispronouncing Greek / Hebrew /
// Devanagari / Syriac / etc. byte sequences while leaving the
// per-fragment time intervals (the actual deliverable) accurate.
//
// Disable globally by setting AutoTransliterateDisabled to 1, or from
// the user config string with `tts_auto_transliterate=false`.
//
// The transliteration tables are provided by the
// github.com/digitalbiblesociety/transliterate library — the only
// third-party dependency in the entire dido module.
package tts
