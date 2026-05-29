// Package usfm provides a minimal verse-text extractor for USFM 3.0
// scripture files.
//
// This is NOT a full USFM parser — there's no AST, no character-style
// rendering, no validation. It exists for one job: scan a USFM file,
// emit one record per verse with the chapter number, the verse range,
// and the plain-text content with inline character-style markers
// stripped to their bare text.
//
// For everything beyond that (USX conversion, output formats, marker
// validation, AST queries), see github.com/digitalbiblesociety/usfm-tools
// — a full Go USFM parser. The deliberate non-goal here is to keep
// dido's USFM surface tiny: ~250 LOC of regex-driven scanning instead
// of a 6 kLOC parser fork.
package usfm

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Verse is one verse of scripture text.
//
// VerseStart/VerseEnd carry the range so a `\v 3-5 text` USFM marker
// becomes a single Verse with Start=3, End=5. For an unranged marker
// (`\v 7 text`) Start == End == 7.
type Verse struct {
	Chapter    int
	VerseStart int
	VerseEnd   int
	Text       string
}

// ID returns "N" for a single-verse marker or "N-M" for a range.
// Suitable as a fragment identifier in downstream alignment pipelines.
func (v Verse) ID() string {
	if v.VerseEnd > v.VerseStart {
		return fmt.Sprintf("%d-%d", v.VerseStart, v.VerseEnd)
	}
	return strconv.Itoa(v.VerseStart)
}

// Options controls optional behaviours of the verse extractor.
//
// The zero value matches the canonical Scripture App Builder output
// convention: section headings (\s/\s1/.../\sd) are dropped, paragraph
// and poetry markers (\p/\q) are dropped, and only verse content
// survives.
type Options struct {
	// IncludeSectionHeaders, when true, appends section-heading text
	// (\s, \s1, \s2, \s3, \s4, \sr, \sp, \sd) to the verse that
	// precedes it. When false (default) the heading text is discarded
	// — useful when the downstream consumer wants pure verse content
	// without inline subtitling.
	IncludeSectionHeaders bool
}

// ParseFile reads usfmPath and returns one Verse per `\v` marker in
// canonical order. Uses default Options (section headings dropped).
func ParseFile(usfmPath string) ([]Verse, error) {
	return ParseFileWithOptions(usfmPath, Options{})
}

// ParseFileWithOptions is the option-bearing form of ParseFile.
func ParseFileWithOptions(usfmPath string, opts Options) ([]Verse, error) {
	f, err := os.Open(usfmPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseWithOptions(f, opts)
}

// Parse reads USFM content from r and returns the extracted verses.
// Uses default Options (section headings dropped).
func Parse(r io.Reader) ([]Verse, error) {
	return ParseWithOptions(r, Options{})
}

// ParseWithOptions is the option-bearing form of Parse. On encoding
// errors the partial result so far is returned alongside the error.
func ParseWithOptions(r io.Reader, opts Options) ([]Verse, error) {
	sc := bufio.NewScanner(r)
	// Default bufio.Scanner has a 64 KiB line cap. Some USFM files
	// (footnote-heavy translations) emit single lines longer than that.
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	var out []Verse
	chapter := 0
	verseStart := 0
	verseEnd := 0
	var buf strings.Builder
	inVerse := false

	flush := func() {
		if !inVerse {
			return
		}
		out = append(out, Verse{
			Chapter:    chapter,
			VerseStart: verseStart,
			VerseEnd:   verseEnd,
			Text:       strings.TrimSpace(stripUSFM(buf.String())),
		})
		buf.Reset()
		inVerse = false
	}

	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, `\c `):
			flush()
			chapter = parseLeadingInt(trimmed[3:])

		case strings.HasPrefix(trimmed, `\v `):
			flush()
			vstart, vend, rest := parseVerseMarker(trimmed[3:])
			verseStart, verseEnd = vstart, vend
			inVerse = true
			if rest != "" {
				buf.WriteString(rest)
			}

		case strings.HasPrefix(trimmed, `\id `),
			strings.HasPrefix(trimmed, `\h `),
			strings.HasPrefix(trimmed, `\toc`),
			strings.HasPrefix(trimmed, `\mt`),
			strings.HasPrefix(trimmed, `\ide`),
			strings.HasPrefix(trimmed, `\ip`),
			strings.HasPrefix(trimmed, `\imt`),
			strings.HasPrefix(trimmed, `\rem`):
			// Identification + introduction markers — never part of a verse.
			flush()

		default:
			// Continuation lines (poetry \q, paragraph \p, section \s,
			// inline-only content). When we're inside a verse, append.
			// Otherwise ignore — the next \v will reset.
			if inVerse {
				// Structural-only lines (just `\q1`, `\p`, etc., no
				// trailing text on the same line) are dropped entirely.
				// When the marker is followed by text on the same line,
				// the stripping pass inside stripUSFM removes the marker
				// itself and keeps the text.
				if structuralEmptyLine(trimmed) {
					continue
				}
				// Section headings carry their own text on the same
				// line (`\s1 Heading text`). Drop them unless the
				// caller opted into preserving them.
				if !opts.IncludeSectionHeaders && isSectionHeaderLine(trimmed) {
					continue
				}
				if buf.Len() > 0 {
					buf.WriteByte(' ')
				}
				buf.WriteString(line)
			}
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// parseVerseMarker handles `\v <vnum> <text>` and `\v <vnum>-<vend> <text>`.
// Returns the start, end, and the text that came after the verse number.
func parseVerseMarker(s string) (start, end int, rest string) {
	// Trim leading whitespace then split on first whitespace.
	s = strings.TrimLeft(s, " \t")
	cut := strings.IndexAny(s, " \t")
	var token string
	if cut < 0 {
		token, rest = s, ""
	} else {
		token, rest = s[:cut], strings.TrimLeft(s[cut:], " \t")
	}
	if dash := strings.IndexByte(token, '-'); dash > 0 {
		start = parseLeadingInt(token[:dash])
		end = parseLeadingInt(token[dash+1:])
		if end < start {
			end = start
		}
	} else {
		start = parseLeadingInt(token)
		end = start
	}
	return start, end, rest
}

// parseLeadingInt reads the leading run of decimal digits in s.
// Returns 0 if s starts with a non-digit.
func parseLeadingInt(s string) int {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}

// structuralMarkers is the set of USFM markers that introduce a block-
// level structure (paragraph, poetry, section, list, etc.) and carry
// no verse-text content of their own. When such a marker appears on a
// line by itself, the line is dropped during scanning; when it's
// followed by text, the stripper in stripUSFM removes the marker and
// keeps the text.
var structuralMarkers = map[string]bool{
	"p": true, "m": true, "mi": true,
	"pi": true, "pi1": true, "pi2": true, "pi3": true,
	"q": true, "q1": true, "q2": true, "q3": true, "q4": true,
	"qc": true, "qr": true,
	"qm": true, "qm1": true, "qm2": true, "qm3": true,
	"b": true, "nb": true,
	"s": true, "s1": true, "s2": true, "s3": true, "s4": true,
	"sr": true, "sp": true, "sd": true,
	"li": true, "li1": true, "li2": true, "li3": true, "li4": true,
	"tr": true, "d": true,
}

// sectionHeaderMarkers is the subset of structuralMarkers that
// introduces a section heading. These markers (unlike \p / \q) carry
// their own text on the same line and are dropped from verse output
// unless Options.IncludeSectionHeaders is set.
var sectionHeaderMarkers = map[string]bool{
	"s": true, "s1": true, "s2": true, "s3": true, "s4": true,
	"sr": true, "sp": true, "sd": true,
}

// isSectionHeaderLine reports whether trimmed begins with a section-
// heading marker (\s, \s1, …, \sd), regardless of whether the line
// carries trailing text.
func isSectionHeaderLine(trimmed string) bool {
	if !strings.HasPrefix(trimmed, `\`) {
		return false
	}
	body := strings.TrimLeft(trimmed[1:], " \t")
	tok := body
	if i := strings.IndexAny(body, " \t"); i >= 0 {
		tok = body[:i]
	}
	return sectionHeaderMarkers[tok]
}

// structuralEmptyLine reports whether trimmed is just a structural
// marker on its own (e.g. "\q1", "\p", "\s1"), with no trailing text.
func structuralEmptyLine(trimmed string) bool {
	if trimmed == "" || !strings.HasPrefix(trimmed, `\`) {
		return false
	}
	body := strings.TrimLeft(trimmed[1:], " \t")
	tok, rest := body, ""
	if i := strings.IndexAny(body, " \t"); i >= 0 {
		tok, rest = body[:i], strings.TrimSpace(body[i:])
	}
	return rest == "" && structuralMarkers[tok]
}

// Pre-compiled regexes for the USFM stripper: reduce raw USFM to plain
// verse text (footnotes, cross-refs, and character markers removed).
var (
	// \f + ... \f* and \x + ... \x* — footnotes and cross-refs, dropped wholesale.
	reFootnote = regexp.MustCompile(`(?s)\\f\s.*?\\f\*`)
	reXref     = regexp.MustCompile(`(?s)\\x\s.*?\\x\*`)
	// \w word|attrs\w*  (and \+w … \+w* for nested)
	reWord = regexp.MustCompile(`\\\+?w\s+([^|\\]+?)(?:\|[^\\]*)?\\\+?w\*`)
	// \add ... \add* (translator's addition)
	reAdd = regexp.MustCompile(`\\add\s+([^\\]+?)\\add\*`)
	// \nd ... \nd* (name of deity); the contents may contain nested \+w tags.
	reND = regexp.MustCompile(`(?s)\\nd\s+(.*?)\\nd\*`)
	// Catch-all: any leftover marker including its closing form. Use
	// after the structured replacements above.
	reOtherMarker = regexp.MustCompile(`\\[a-zA-Z0-9]+\*?`)
	// Collapse runs of whitespace.
	reWhitespace = regexp.MustCompile(`\s+`)
)

// stripUSFM removes all USFM markup from a verse-text string, leaving
// just the readable scripture text. Order matters: footnotes/xrefs
// are dropped wholesale first, then nested character styles are
// resolved to their content, then leftover markers are removed.
func stripUSFM(s string) string {
	s = reFootnote.ReplaceAllString(s, "")
	s = reXref.ReplaceAllString(s, "")
	s = reWord.ReplaceAllString(s, "$1")
	s = reAdd.ReplaceAllString(s, "$1")
	// \nd can wrap \+w — recurse the word stripper through its content.
	s = reND.ReplaceAllStringFunc(s, func(match string) string {
		// The submatch grouping inside reND grabs everything between
		// \nd and \nd*; ReplaceAllString of \+w handles the inner words.
		inner := reND.ReplaceAllString(match, "$1")
		return reWord.ReplaceAllString(inner, "$1")
	})
	// Any markers still present (paragraph/poetry leftovers inside a
	// line that came in through a continuation path, or unmatched
	// char styles) are stripped without preserving their content.
	s = reOtherMarker.ReplaceAllString(s, "")
	s = reWhitespace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// Chapter returns the verses belonging to a single chapter number,
// preserving order. Returns nil when no verses match.
func Chapter(verses []Verse, n int) []Verse {
	var out []Verse
	for _, v := range verses {
		if v.Chapter == n {
			out = append(out, v)
		}
	}
	return out
}

// MaxChapter returns the largest chapter number observed in verses,
// or 0 if verses is empty.
func MaxChapter(verses []Verse) int {
	n := 0
	for _, v := range verses {
		if v.Chapter > n {
			n = v.Chapter
		}
	}
	return n
}
