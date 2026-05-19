// Package text provides text fragment reading and filtering.
// Port of aeneas/textfile.py.
package text

import "strings"

// Format is the text file format identifier.
type Format string

const (
	FormatMPlain    Format = "mplain"
	FormatMUnparsed Format = "munparsed"
	FormatParsed    Format = "parsed"
	FormatPlain     Format = "plain"
	FormatSubtitles Format = "subtitles"
	FormatUnparsed  Format = "unparsed"
)

// DefaultIDFormat is the printf format for auto-generated fragment IDs.
const DefaultIDFormat = "f%06d"

// ParsedTextSeparator is the delimiter between id and text in PARSED format.
const ParsedTextSeparator = "|"

// Fragment is a single text fragment with its identifier, language, and lines.
type Fragment struct {
	Identifier    string
	Language      string
	Lines         []string
	FilteredLines []string
}

// Text returns the fragment's lines joined by spaces.
func (f *Fragment) Text() string {
	if len(f.Lines) == 0 {
		return ""
	}
	return strings.Join(f.Lines, " ")
}

// FilteredText returns the fragment's filtered lines joined by spaces.
func (f *Fragment) FilteredText() string {
	if len(f.FilteredLines) == 0 {
		return ""
	}
	return strings.Join(f.FilteredLines, " ")
}

// GetIdentifier satisfies syncmap.TextFragment.
func (f *Fragment) GetIdentifier() string { return f.Identifier }

// GetLines satisfies syncmap.TextFragment.
func (f *Fragment) GetLines() []string { return f.Lines }

// GetLanguage satisfies syncmap.TextFragment.
func (f *Fragment) GetLanguage() string { return f.Language }

// Chars returns the total character count across all lines (no separators).
func (f *Fragment) Chars() int {
	n := 0
	for _, l := range f.Lines {
		n += len([]rune(l))
	}
	return n
}
