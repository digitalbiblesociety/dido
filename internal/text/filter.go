package text

import (
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

var multiSpaceRE = regexp.MustCompile(` [ ]+`)

// Filter applies a transformation to a slice of strings.
type Filter interface {
	Apply(lines []string) []string
}

// FilterChain applies zero or more filters in sequence.
type FilterChain struct {
	filters []Filter
}

// Add appends f to the chain.
func (fc *FilterChain) Add(f Filter) { fc.filters = append(fc.filters, f) }

// Apply passes lines through each filter in order.
func (fc *FilterChain) Apply(lines []string) []string {
	result := lines
	for _, f := range fc.filters {
		result = f.Apply(result)
	}
	return result
}

// IgnoreRegexFilter removes text matching the compiled regex from each line.
type IgnoreRegexFilter struct {
	re *regexp.Regexp
}

// NewIgnoreRegexFilter returns a filter that deletes matches of pattern.
func NewIgnoreRegexFilter(pattern string) (*IgnoreRegexFilter, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &IgnoreRegexFilter{re: re}, nil
}

func (f *IgnoreRegexFilter) Apply(lines []string) []string {
	out := make([]string, len(lines))
	for i, s := range lines {
		r := f.re.ReplaceAllString(s, "")
		r = multiSpaceRE.ReplaceAllString(r, " ")
		out[i] = strings.TrimSpace(r)
	}
	return out
}

// TranslitFilter applies a TransliterationMap to each line.
type TranslitFilter struct {
	m *TransliterationMap
}

// NewTranslitFilter returns a filter using the given map.
func NewTranslitFilter(m *TransliterationMap) *TranslitFilter {
	return &TranslitFilter{m: m}
}

func (f *TranslitFilter) Apply(lines []string) []string {
	out := make([]string, len(lines))
	for i, s := range lines {
		r := f.m.Transliterate(s)
		r = multiSpaceRE.ReplaceAllString(r, " ")
		out[i] = strings.TrimSpace(r)
	}
	return out
}

// TransliterationMap maps Unicode code points to replacement strings.
type TransliterationMap struct {
	table map[rune]string
}

var cpRE = regexp.MustCompile(`U\+([0-9A-Fa-f]+)`)
var deleteRE = regexp.MustCompile(`^(\S+)$`)
var replaceRE = regexp.MustCompile(`^(\S+) (\S+)$`)

// NewTransliterationMap reads a map file and returns the map.
func NewTransliterationMap(path string) (*TransliterationMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	tm := &TransliterationMap{table: make(map[rune]string)}
	src := strings.ReplaceAll(string(data), "\t", " ")
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		tm.processRule(line)
	}
	return tm, nil
}

func (tm *TransliterationMap) processRule(line string) {
	if m := replaceRE.FindStringSubmatch(line); m != nil {
		chars := tm.parseFirst(m[1])
		rep := tm.parseSecond(m[2])
		for _, c := range chars {
			tm.table[c] = rep
		}
		return
	}
	if m := deleteRE.FindStringSubmatch(line); m != nil {
		chars := tm.parseFirst(m[1])
		for _, c := range chars {
			tm.table[c] = ""
		}
	}
}

func (tm *TransliterationMap) parseFirst(s string) []rune {
	if strings.Contains(s, "-") {
		parts := strings.SplitN(s, "-", 2)
		start := tm.parseCodepoint(parts[0])
		end := tm.parseCodepoint(parts[1])
		if start >= 0 && end >= start {
			out := make([]rune, 0, end-start+1)
			for r := rune(start); r <= rune(end); r++ {
				out = append(out, r)
			}
			return out
		}
		return nil
	}
	cp := tm.parseCodepoint(s)
	if cp < 0 {
		return nil
	}
	return []rune{rune(cp)}
}

func (tm *TransliterationMap) parseCodepoint(s string) int {
	if len(s) == 0 {
		return -1
	}
	if m := cpRE.FindStringSubmatch(s); m != nil {
		var v int
		for _, c := range m[1] {
			v = v*16 + hexDigit(c)
		}
		return v
	}
	if utf8.RuneCountInString(s) == 1 {
		r, _ := utf8.DecodeRuneInString(s)
		return int(r)
	}
	return -1
}

func hexDigit(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}

func (tm *TransliterationMap) parseSecond(s string) string {
	return cpRE.ReplaceAllStringFunc(s, func(match string) string {
		m := cpRE.FindStringSubmatch(match)
		if m == nil {
			return ""
		}
		var v int
		for _, c := range m[1] {
			v = v*16 + hexDigit(c)
		}
		return string(rune(v))
	})
}

// Transliterate maps each rune of s using the table.
func (tm *TransliterationMap) Transliterate(s string) string {
	var b strings.Builder
	for _, r := range s {
		if rep, ok := tm.table[r]; ok {
			b.WriteString(rep)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
