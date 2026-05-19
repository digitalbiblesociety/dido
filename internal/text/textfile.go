package text

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/digitalbiblesociety/dido/internal/language"
	"github.com/digitalbiblesociety/dido/internal/syncmap"
)

// Params holds the parsing parameters corresponding to Python's task parameters.
type Params struct {
	// ID format string for auto-generated IDs (printf-style, default "f%06d").
	IDFormat string

	// PLAIN/SUBTITLES/MPLAIN word separator.
	MPlainWordSeparator string

	// UNPARSED matching.
	UnparsedIDRegex    string
	UnparsedClassRegex string
	UnparsedIDSort     string // "unsorted", "lexicographic", "numeric"

	// MUNPARSED matching.
	MUnparsedL1IDRegex string
	MUnparsedL2IDRegex string
	MUnparsedL3IDRegex string

	// Text filters.
	IgnoreRegex    string
	TranslitMapPath string
}

// TextFile is a tree of TextFragments.
type TextFile struct {
	Tree   *syncmap.Tree
	params Params
}

// New returns an empty TextFile.
func New(p Params) *TextFile {
	if p.IDFormat == "" {
		p.IDFormat = DefaultIDFormat
	}
	return &TextFile{Tree: syncmap.NewTree(nil), params: p}
}

// ReadFile reads fragments from path in the given format.
func ReadFile(path string, format Format, p Params) (*TextFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return ReadLines(lines, format, p)
}

// ReadLines parses already-read lines in the given format.
func ReadLines(lines []string, format Format, p Params) (*TextFile, error) {
	tf := New(p)
	switch format {
	case FormatPlain:
		tf.readPlain(lines)
	case FormatParsed:
		tf.readParsed(lines)
	case FormatSubtitles:
		tf.readSubtitles(lines)
	case FormatMPlain:
		tf.readMPlain(lines)
	case FormatUnparsed:
		if err := tf.readUnparsed(lines); err != nil {
			return nil, err
		}
	case FormatMUnparsed:
		if err := tf.readMUnparsed(lines); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
	return tf, nil
}

// Fragments returns the non-empty direct children of the root as *Fragment values.
func (tf *TextFile) Fragments() []*Fragment {
	vals := tf.Tree.VChildrenNotEmpty()
	frags := make([]*Fragment, len(vals))
	for i, v := range vals {
		frags[i] = v.(*Fragment)
	}
	return frags
}

// SetLanguage sets the language on all direct fragments.
func (tf *TextFile) SetLanguage(lang language.Language) {
	for _, f := range tf.Fragments() {
		f.Language = lang
	}
}

func (tf *TextFile) addFragment(frag *Fragment) {
	tf.Tree.AddChild(syncmap.NewTree(frag), true)
}

func (tf *TextFile) buildFilter() *FilterChain {
	fc := &FilterChain{}
	if tf.params.IgnoreRegex != "" {
		if f, err := NewIgnoreRegexFilter(tf.params.IgnoreRegex); err == nil {
			fc.Add(f)
		}
	}
	if tf.params.TranslitMapPath != "" {
		if tm, err := NewTransliterationMap(tf.params.TranslitMapPath); err == nil {
			fc.Add(NewTranslitFilter(tm))
		}
	}
	return fc
}

func (tf *TextFile) idFormat() string {
	if tf.params.IDFormat != "" {
		return tf.params.IDFormat
	}
	return DefaultIDFormat
}

func (tf *TextFile) createFragments(pairs [][2][]string) {
	fc := tf.buildFilter()
	for _, pair := range pairs {
		id := pair[0][0]
		lines := pair[1]
		tf.addFragment(&Fragment{
			Identifier:    id,
			Lines:         lines,
			FilteredLines: fc.Apply(lines),
		})
	}
}

func (tf *TextFile) readPlain(lines []string) {
	idfmt := tf.idFormat()
	fc := tf.buildFilter()
	i := 1
	for _, line := range lines {
		text := strings.TrimSpace(line)
		id := fmt.Sprintf(idfmt, i)
		tf.addFragment(&Fragment{
			Identifier:    id,
			Lines:         []string{text},
			FilteredLines: fc.Apply([]string{text}),
		})
		i++
	}
}

func (tf *TextFile) readParsed(lines []string) {
	fc := tf.buildFilter()
	for _, line := range lines {
		idx := strings.Index(line, ParsedTextSeparator)
		if idx < 0 {
			continue
		}
		id := strings.TrimSpace(line[:idx])
		text := strings.TrimSpace(line[idx+1:])
		if id == "" {
			continue
		}
		tf.addFragment(&Fragment{
			Identifier:    id,
			Lines:         []string{text},
			FilteredLines: fc.Apply([]string{text}),
		})
	}
}

func (tf *TextFile) readSubtitles(lines []string) {
	idfmt := tf.idFormat()
	fc := tf.buildFilter()
	stripped := make([]string, len(lines))
	for i, l := range lines {
		stripped[i] = strings.TrimSpace(l)
	}
	i := 1
	current := 0
	for current < len(stripped) {
		if stripped[current] == "" {
			current++
			continue
		}
		fragLines := []string{stripped[current]}
		following := current + 1
		for following < len(stripped) && stripped[following] != "" {
			fragLines = append(fragLines, stripped[following])
			following++
		}
		id := fmt.Sprintf(idfmt, i)
		tf.addFragment(&Fragment{
			Identifier:    id,
			Lines:         fragLines,
			FilteredLines: fc.Apply(fragLines),
		})
		current = following
		i++
	}
}

func (tf *TextFile) readMPlain(lines []string) {
	wordSep := tf.params.MPlainWordSeparator
	switch wordSep {
	case "", "space":
		wordSep = " "
	case "equal":
		wordSep = "="
	case "pipe":
		wordSep = "|"
	case "tab":
		wordSep = "\t"
	}

	stripped := make([]string, len(lines))
	for i, l := range lines {
		stripped[i] = strings.TrimSpace(l)
	}

	tree := syncmap.NewTree(nil)
	i := 1
	current := 0
	for current < len(stripped) {
		if stripped[current] == "" {
			current++
			continue
		}
		// Collect consecutive non-blank lines as sentences of this paragraph.
		var sentences []string
		following := current
		for following < len(stripped) && stripped[following] != "" {
			sentences = append(sentences, stripped[following])
			following++
		}

		paragraphID := fmt.Sprintf("p%06d", i)
		paragraphText := strings.Join(sentences, " ")
		paragraphFrag := &Fragment{
			Identifier:    paragraphID,
			Lines:         []string{paragraphText},
			FilteredLines: []string{paragraphText},
		}
		paragraphNode := syncmap.NewTree(paragraphFrag)
		tree.AddChild(paragraphNode, true)

		j := 1
		for _, s := range sentences {
			sentenceID := fmt.Sprintf("%ss%06d", paragraphID, j)
			sentenceFrag := &Fragment{
				Identifier:    sentenceID,
				Lines:         []string{s},
				FilteredLines: []string{s},
			}
			sentenceNode := syncmap.NewTree(sentenceFrag)
			paragraphNode.AddChild(sentenceNode, true)
			j++

			// Phrase layer: split the sentence on punctuation /
			// script-specific separators. A sentence with no internal
			// boundary yields one phrase equal to the sentence text;
			// the layer is still created so the tree depth is uniform.
			phrases := SplitPhrases(s)
			r := 1
			for _, p := range phrases {
				if p == "" {
					continue
				}
				phraseID := fmt.Sprintf("%sr%06d", sentenceID, r)
				phraseFrag := &Fragment{
					Identifier:    phraseID,
					Lines:         []string{p},
					FilteredLines: []string{p},
				}
				phraseNode := syncmap.NewTree(phraseFrag)
				sentenceNode.AddChild(phraseNode, true)
				r++

				k := 1
				for _, w := range strings.Split(p, wordSep) {
					if w == "" {
						continue
					}
					wordID := fmt.Sprintf("%sw%06d", phraseID, k)
					wordFrag := &Fragment{
						Identifier:    wordID,
						Lines:         []string{w},
						FilteredLines: []string{w},
					}
					phraseNode.AddChild(syncmap.NewTree(wordFrag), true)
					k++
				}
			}
		}
		current = following
		i++
	}
	tf.Tree = tree
}
