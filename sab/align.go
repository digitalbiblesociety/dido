package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/digitalbiblesociety/transliterate/script"

	"github.com/digitalbiblesociety/dido/internal/config"
	"github.com/digitalbiblesociety/dido/internal/pipeline"
	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/usfm"
)

// phrase is one verse-fragment with both its source-script form and a
// Latin transliteration. Original feeds the permanent -aeneas.txt; Latin
// feeds dido's aligner. For Roman-script bibles the two columns are the
// same string.
type phrase struct {
	ID       string
	Original string
	Latin    string
}

// chapterResult captures what the alignment produced for one chapter.
// Returned so callers (single-book runner, TUI) can render stats.
type chapterResult struct {
	Stem      string
	Chapter   int
	Fragments int
}

// alignChapter runs the full pipeline on one chapter: extract verses →
// split into phrases → transliterate (if needed) → align via dido →
// emit the two SAB output files.
func alignChapter(audioMP3 string, allVerses []usfm.Verse, chapter int, stem, outDir string,
	tc pipeline.TaskConfig, rc config.RuntimeConfig) (chapterResult, error) {

	chapVerses := usfm.Chapter(allVerses, chapter)
	if len(chapVerses) == 0 {
		return chapterResult{}, fmt.Errorf("USFM has no verses for chapter %d", chapter)
	}

	var phrases []phrase
	for _, v := range chapVerses {
		if v.Text == "" {
			continue
		}
		eng := script.Detect(v.Text)
		var fragments []string
		if eng != nil {
			fragments = eng.Split(v.Text)
		}
		if len(fragments) == 0 {
			fragments = splitVerse(v.Text)
		}
		labels := verseLabels(v.ID(), len(fragments))
		for i, frag := range fragments {
			orig := strings.ReplaceAll(frag, "|", "/")
			latin := orig
			if eng != nil {
				lat := strings.TrimSpace(eng.Transliterate(frag))
				lat = strings.ReplaceAll(lat, "|", "/")
				if lat == "" {
					continue
				}
				latin = lat
			}
			phrases = append(phrases, phrase{ID: labels[i], Original: orig, Latin: latin})
		}
	}
	if len(phrases) == 0 {
		return chapterResult{}, fmt.Errorf("no phrases extracted")
	}

	frags := make([]*text.Fragment, len(phrases))
	for i, p := range phrases {
		frags[i] = &text.Fragment{
			Identifier: p.ID,
			Language:   tc.Language,
			Lines:      []string{p.Latin},
		}
	}

	sm, err := pipeline.Execute(audioMP3, frags, tc, rc)
	if err != nil {
		return chapterResult{}, fmt.Errorf("align: %w", err)
	}

	if err := writeAeneasFile(filepath.Join(outDir, stem+"-aeneas.txt"), phrases, phraseLatin); err != nil {
		return chapterResult{}, fmt.Errorf("write -aeneas.txt: %w", err)
	}
	if anyTransliterated(phrases) {
		if err := writeAeneasFile(filepath.Join(outDir, stem+"-aeneas-original.txt"), phrases, phraseOriginal); err != nil {
			return chapterResult{}, fmt.Errorf("write -aeneas-original.txt: %w", err)
		}
	}

	bookCode := codeFromStem(stem)
	timingPath := filepath.Join(outDir, stem+"-timing.txt")
	if err := writeTimingFile(timingPath, bookCode, chapter, sm); err != nil {
		return chapterResult{}, fmt.Errorf("write -timing.txt: %w", err)
	}
	return chapterResult{Stem: stem, Chapter: chapter, Fragments: len(phrases)}, nil
}

type phraseField func(phrase) string

func phraseLatin(p phrase) string    { return p.Latin }
func phraseOriginal(p phrase) string { return p.Original }

func writeAeneasFile(path string, phrases []phrase, field phraseField) error {
	var b strings.Builder
	for _, p := range phrases {
		b.WriteString(p.ID)
		b.WriteByte('|')
		b.WriteString(field(p))
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func anyTransliterated(phrases []phrase) bool {
	for _, p := range phrases {
		if p.Latin != p.Original {
			return true
		}
	}
	return false
}

func writeTimingFile(path, bookCode string, chapter int, sm *syncmap.SyncMap) error {
	var b strings.Builder
	fmt.Fprintf(&b, "\\id %s\n\\c %d\n\\level phrase\n\\separators %s\n",
		bookCode, chapter, formatSeparators(phraseSeparators))
	for _, frag := range sm.Fragments() {
		if frag.Identifier() == "HEAD" || frag.Identifier() == "TAIL" {
			continue
		}
		fmt.Fprintf(&b, "%.3f\t%.3f\t%s\n",
			frag.Begin().Float64(), frag.End().Float64(), frag.Identifier())
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// splitVerse breaks a verse on phraseSeparators, mirroring the
// audio-sync helper of the same name. Each fragment keeps its
// terminating punctuation; a verse with no separator becomes a single
// fragment.
func splitVerse(textIn string) []string {
	var fragments []string
	var cur strings.Builder
	for _, r := range textIn {
		cur.WriteRune(r)
		if strings.ContainsRune(phraseSeparators, r) {
			f := strings.TrimSpace(cur.String())
			if f != "" {
				fragments = append(fragments, f)
			}
			cur.Reset()
		}
	}
	if rem := strings.TrimSpace(cur.String()); rem != "" {
		fragments = append(fragments, rem)
	}
	if len(fragments) == 0 {
		fragments = []string{strings.TrimSpace(textIn)}
	}
	return fragments
}

// verseLabels: one fragment → bare verseID; many → verseID + a, b, c, …
func verseLabels(verseID string, n int) []string {
	if n <= 1 {
		return []string{verseID}
	}
	out := make([]string, n)
	for i := range n {
		out[i] = verseID + string(rune('a'+i))
	}
	return out
}

// stemFor builds the per-chapter output stem.
//
//	sab    → C01-<NN>-<CODE>-<CC>   (matches audio-sync / SAB convention)
//	simple → <CODE>-<CCC>           (one-book, three-digit chapter)
func stemFor(style string, seq int, code string, chap int) string {
	switch style {
	case "simple":
		return fmt.Sprintf("%s-%03d", code, chap)
	default:
		bs := seq
		if bs == 0 {
			bs = 1
		}
		return fmt.Sprintf("C01-%02d-%s-%02d", bs, code, chap)
	}
}

// codeFromStem recovers the USFM book code from a stem produced by stemFor.
func codeFromStem(stem string) string {
	parts := strings.Split(stem, "-")
	if len(parts) >= 3 && parts[0] == "C01" {
		return parts[2]
	}
	if len(parts) >= 1 {
		return parts[0]
	}
	return stem
}

// formatSeparators spaces the chars in s for readability in the SAB
// header (".?!:;," → ". ? ! : ; ,"). Multibyte runes pass through unchanged.
func formatSeparators(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
