package text

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"github.com/digitalbiblesociety/dido/internal/language"
	"github.com/digitalbiblesociety/dido/internal/syncmap"
)

// readUnparsed extracts fragments from UNPARSED XHTML by matching id/class attributes.
func (tf *TextFile) readUnparsed(lines []string) error {
	src := strings.Join(lines, "\n")
	dec := xml.NewDecoder(strings.NewReader(src))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	idRE := compileAttrRE(tf.params.UnparsedIDRegex)
	classRE := compileAttrRE(tf.params.UnparsedClassRegex)

	type entry struct {
		id   string
		text string
	}
	var entries []entry
	idSeen := map[string]bool{}

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		id, class := attrValues(se.Attr)
		if !matchAttr(idRE, id) || !matchAttr(classRE, class) {
			continue
		}
		if id == "" {
			continue
		}
		if idSeen[id] {
			continue
		}
		inner, err := innerText(dec, se.Name)
		if err != nil {
			break
		}
		entries = append(entries, entry{id: id, text: inner})
		idSeen[id] = true
	}

	ids := make([]string, len(entries))
	textByID := make(map[string]string, len(entries))
	for i, e := range entries {
		ids[i] = e.id
		textByID[e.id] = e.text
	}

	sortedIDs := sortIDsByParam(ids, tf.params.UnparsedIDSort)

	fc := tf.buildFilter()
	for _, id := range sortedIDs {
		text := textByID[id]
		tf.addFragment(&Fragment{
			Identifier:    id,
			Lines:         []string{text},
			FilteredLines: fc.Apply([]string{text}),
		})
	}
	return nil
}

// readMUnparsed extracts a four-level (paragraph/sentence/phrase/word) tree
// from MUNPARSED XHTML. The XHTML only carries L1/L2/L3 IDs (paragraph,
// sentence, word); the phrase layer is synthesised by grouping consecutive
// L3 words whose ending punctuation matches a PhraseSeparators rune. A
// sentence with no internal punctuation collapses to a single phrase
// containing every L3 word, keeping the tree depth uniform.
func (tf *TextFile) readMUnparsed(lines []string) error {
	l1RE := compileAttrRE(tf.params.MUnparsedL1IDRegex)
	l2RE := compileAttrRE(tf.params.MUnparsedL2IDRegex)
	l3RE := compileAttrRE(tf.params.MUnparsedL3IDRegex)

	if l1RE == nil || l2RE == nil || l3RE == nil {
		return fmt.Errorf("munparsed requires l1/l2/l3 id regexes")
	}

	src := strings.Join(lines, "\n")

	// Build a simplified element tree so we can walk it hierarchically.
	type elem struct {
		name     string
		id       string
		text     string // direct text only (not descendant text)
		children []*elem
	}

	var buildTree func(dec *xml.Decoder, parent *elem) error
	buildTree = func(dec *xml.Decoder, parent *elem) error {
		for {
			tok, err := dec.Token()
			if err != nil {
				return err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				id, _ := attrValues(t.Attr)
				child := &elem{name: t.Name.Local, id: id}
				if err := buildTree(dec, child); err != nil {
					return err
				}
				parent.children = append(parent.children, child)
			case xml.EndElement:
				return nil
			case xml.CharData:
				parent.text += string(t)
			}
		}
	}

	root := &elem{name: "root"}
	dec := xml.NewDecoder(strings.NewReader(src))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	_ = buildTree(dec, root)

	// Walk and find l1/l2/l3 nodes.
	var collectText func(e *elem) string
	collectText = func(e *elem) string {
		parts := []string{e.text}
		for _, c := range e.children {
			parts = append(parts, collectText(c))
		}
		return strings.Join(parts, "")
	}

	var findByRE func(e *elem, re *regexp.Regexp) []*elem
	findByRE = func(e *elem, re *regexp.Regexp) []*elem {
		var out []*elem
		if e.id != "" && re.MatchString(e.id) {
			out = append(out, e)
			return out // don't recurse into matched node
		}
		for _, c := range e.children {
			out = append(out, findByRE(c, re)...)
		}
		return out
	}

	tree := syncmap.NewTree(nil)

	for _, l1 := range findByRE(root, l1RE) {
		var paragraphParts []string
		paragraphNode := syncmap.NewTree(nil)
		hasWord := false

		for _, l2 := range findByRE(l1, l2RE) {
			var sentenceParts []string
			sentenceNode := syncmap.NewTree(nil)

			// Collect all L3 words for this sentence first, then group
			// them into phrases. Direct attachment to the sentence node
			// happens after the phrase layer is in place.
			type wordEntry struct {
				id   string
				text string
			}
			var words []wordEntry
			for _, l3 := range findByRE(l2, l3RE) {
				l3text := strings.TrimSpace(collectText(l3))
				words = append(words, wordEntry{id: l3.id, text: l3text})
				sentenceParts = append(sentenceParts, l3text)
				if l3text != "" {
					hasWord = true
				}
			}

			// Bucket words into phrases by walking each word and
			// closing the current bucket whenever the word's last rune
			// is in PhraseSeparators. The final partial bucket (no
			// trailing punctuation) closes at end-of-sentence.
			type phraseEntry struct {
				text  string
				words []wordEntry
			}
			var phrases []phraseEntry
			cur := phraseEntry{}
			for _, w := range words {
				cur.words = append(cur.words, w)
				if cur.text != "" {
					cur.text += " "
				}
				cur.text += w.text
				if w.text != "" {
					last := []rune(w.text)
					if strings.ContainsRune(PhraseSeparators, last[len(last)-1]) {
						phrases = append(phrases, cur)
						cur = phraseEntry{}
					}
				}
			}
			if len(cur.words) > 0 {
				phrases = append(phrases, cur)
			}

			r := 1
			for _, p := range phrases {
				phraseID := fmt.Sprintf("%sr%06d", l2.id, r)
				phraseFrag := &Fragment{
					Identifier:    phraseID,
					Lines:         []string{p.text},
					FilteredLines: []string{p.text},
				}
				phraseNode := syncmap.NewTree(phraseFrag)
				sentenceNode.AddChild(phraseNode, true)
				for _, w := range p.words {
					wordFrag := &Fragment{
						Identifier:    w.id,
						Language:      language.Language(""),
						Lines:         []string{w.text},
						FilteredLines: []string{w.text},
					}
					phraseNode.AddChild(syncmap.NewTree(wordFrag), true)
				}
				r++
			}

			sentenceText := strings.Join(sentenceParts, " ")
			sentenceNode.Value = &Fragment{
				Identifier:    l2.id,
				Lines:         []string{sentenceText},
				FilteredLines: []string{sentenceText},
			}
			paragraphNode.AddChild(sentenceNode, true)
			paragraphParts = append(paragraphParts, sentenceText)
		}

		if hasWord {
			paragraphText := strings.Join(paragraphParts, " ")
			paragraphNode.Value = &Fragment{
				Identifier:    l1.id,
				Lines:         []string{paragraphText},
				FilteredLines: []string{paragraphText},
			}
			tree.AddChild(paragraphNode, true)
		}
	}

	tf.Tree = tree
	return nil
}

// compileAttrRE returns nil if pattern is empty, otherwise compiles the anchored word-boundary RE.
func compileAttrRE(pattern string) *regexp.Regexp {
	if pattern == "" {
		return nil
	}
	return regexp.MustCompile(`(?i).*\b` + pattern + `\b.*`)
}

// matchAttr returns true when re is nil (no filter) or re matches value.
func matchAttr(re *regexp.Regexp, value string) bool {
	if re == nil {
		return true
	}
	return value != "" && re.MatchString(value)
}

// attrValues extracts id and class from an element's attributes.
func attrValues(attrs []xml.Attr) (id, class string) {
	for _, a := range attrs {
		switch a.Name.Local {
		case "id":
			id = a.Value
		case "class":
			class = a.Value
		}
	}
	return
}

// innerText reads tokens from dec until the matching end element, returning concatenated CharData.
func innerText(dec *xml.Decoder, name xml.Name) (string, error) {
	depth := 1
	var sb strings.Builder
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return sb.String(), err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			sb.Write(t)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

func sortIDsByParam(ids []string, sortParam string) []string {
	// import cycle avoided by inlining the three sort algorithms here.
	switch sortParam {
	case "lexicographic":
		return lexSort(ids)
	case "numeric":
		return numSort(ids)
	default: // "unsorted" or ""
		cp := make([]string, len(ids))
		copy(cp, ids)
		return cp
	}
}

func lexSort(ids []string) []string {
	cp := make([]string, len(ids))
	copy(cp, ids)
	// simple insertion sort (small slices are typical)
	for i := 1; i < len(cp); i++ {
		for j := i; j > 0 && cp[j] < cp[j-1]; j-- {
			cp[j], cp[j-1] = cp[j-1], cp[j]
		}
	}
	return cp
}

func numSort(ids []string) []string {
	cp := make([]string, len(ids))
	copy(cp, ids)
	digitRE := regexp.MustCompile(`[^0-9]`)
	extractNum := func(s string) int {
		digits := digitRE.ReplaceAllString(s, "")
		if digits == "" {
			return 0
		}
		n := 0
		for _, c := range digits {
			n = n*10 + int(c-'0')
		}
		return n
	}
	// stable insertion sort
	for i := 1; i < len(cp); i++ {
		for j := i; j > 0 && extractNum(cp[j]) < extractNum(cp[j-1]); j-- {
			cp[j], cp[j-1] = cp[j-1], cp[j]
		}
	}
	return cp
}
