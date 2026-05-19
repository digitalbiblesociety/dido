package text

import (
	"strings"

	"github.com/digitalbiblesociety/transliterate/script"
)

// PhraseSeparators is the punctuation set used to split a sentence
// into phrase-level fragments. Includes ASCII end-of-clause marks plus
// Arabic, CJK, Devanagari, Tibetan/Burmese, Khmer, and Thai variants.
// Mirrors the constant audio-sync uses for SAB output so dido-sab and
// the in-pipeline multilevel splitter agree on boundaries.
const PhraseSeparators = ".?!:;,؟،。！？．，、।॥།။။።៕។ฯ๏๚๛"

// SplitPhrases breaks s into phrase-level fragments. Script-aware:
//
//   - Scripts without inter-word spacing (Thai, Lao, Khmer, Burmese,
//     Tibetan, …) use the transliterate package's per-script splitter,
//     which knows the script's native pause marks AND treats whitespace
//     as a phrase boundary (since these scripts use space only between
//     phrases, not between words).
//   - Roman / Latin scripts split on PhraseSeparators only — whitespace
//     in Latin text marks word boundaries, not phrase boundaries.
//
// Trailing punctuation is kept on each phrase ("Hello, world." →
// ["Hello,", "world."]). Empty fragments are dropped. A sentence with
// no internal phrase boundary returns a single fragment equal to the
// trimmed input.
func SplitPhrases(s string) []string {
	if eng := script.Detect(s); eng != nil {
		if out := eng.Split(s); len(out) > 0 {
			return out
		}
	}
	return splitOnSeparators(s, PhraseSeparators)
}

// splitOnSeparators is the punctuation-only splitter used for Latin
// script. Each fragment retains its terminating separator; runs of
// whitespace between fragments are trimmed.
func splitOnSeparators(s, seps string) []string {
	var fragments []string
	var cur strings.Builder
	for _, r := range s {
		cur.WriteRune(r)
		if strings.ContainsRune(seps, r) {
			if f := strings.TrimSpace(cur.String()); f != "" {
				fragments = append(fragments, f)
			}
			cur.Reset()
		}
	}
	if rem := strings.TrimSpace(cur.String()); rem != "" {
		fragments = append(fragments, rem)
	}
	if len(fragments) == 0 {
		fragments = []string{strings.TrimSpace(s)}
	}
	return fragments
}
