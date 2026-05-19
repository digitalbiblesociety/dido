package text

import (
	"path/filepath"
	"runtime"
	"testing"
)

// testdataDir resolves a path inside testdata relative to this file.
func testdataDir(rel string) string {
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Dir(file)
	return filepath.Join(base, "..", "..", "testdata", rel)
}

// --- PLAIN ---

func TestReadPlain(t *testing.T) {
	path := testdataDir("inputtext/sonnet_plain.txt")
	tf, err := ReadFile(path, FormatPlain, Params{})
	if err != nil {
		t.Fatal(err)
	}
	frags := tf.Fragments()
	if len(frags) != 15 {
		t.Fatalf("plain: want 15 fragments, got %d", len(frags))
	}
	if frags[0].Identifier != "f000001" {
		t.Errorf("first id: want f000001, got %q", frags[0].Identifier)
	}
	if frags[0].Lines[0] != "1" {
		t.Errorf("first text: want '1', got %q", frags[0].Lines[0])
	}
}

func TestReadPlainCustomIDFormat(t *testing.T) {
	path := testdataDir("inputtext/sonnet_plain.txt")
	tf, err := ReadFile(path, FormatPlain, Params{IDFormat: "s%03d"})
	if err != nil {
		t.Fatal(err)
	}
	frags := tf.Fragments()
	if frags[0].Identifier != "s001" {
		t.Errorf("want s001, got %q", frags[0].Identifier)
	}
}

// --- PARSED ---

func TestReadParsed(t *testing.T) {
	path := testdataDir("inputtext/sonnet_parsed.txt")
	tf, err := ReadFile(path, FormatParsed, Params{})
	if err != nil {
		t.Fatal(err)
	}
	frags := tf.Fragments()
	if len(frags) != 15 {
		t.Fatalf("parsed: want 15 fragments, got %d", len(frags))
	}
	if frags[0].Identifier != "f000001" {
		t.Errorf("first id: want f000001, got %q", frags[0].Identifier)
	}
	if frags[1].Lines[0] != "From fairest creatures we desire increase," {
		t.Errorf("second text: got %q", frags[1].Lines[0])
	}
}

// --- SUBTITLES ---

func TestReadSubtitles(t *testing.T) {
	path := testdataDir("inputtext/sonnet_subtitles_multiple_blank.txt")
	tf, err := ReadFile(path, FormatSubtitles, Params{})
	if err != nil {
		t.Fatal(err)
	}
	frags := tf.Fragments()
	if len(frags) == 0 {
		t.Fatal("subtitles: expected at least one fragment")
	}
	// first fragment is "1 (Unicode: àèìòù)" — single line
	if len(frags[0].Lines) != 1 {
		t.Errorf("first subtitle should be 1 line, got %d", len(frags[0].Lines))
	}
}

// --- MPLAIN ---

func TestReadMPlain(t *testing.T) {
	path := testdataDir("inputtext/sonnet_mplain.txt")
	tf, err := ReadFile(path, FormatMPlain, Params{})
	if err != nil {
		t.Fatal(err)
	}
	// The mplain file has 5 paragraphs separated by blank lines.
	frags := tf.Fragments()
	if len(frags) != 5 {
		t.Fatalf("mplain: want 5 top-level paragraphs, got %d", len(frags))
	}
	if frags[0].Identifier != "p000001" {
		t.Errorf("first para id: want p000001, got %q", frags[0].Identifier)
	}
}

func TestReadMPlainTreeShape(t *testing.T) {
	path := testdataDir("inputtext/sonnet_mplain.txt")
	tf, err := ReadFile(path, FormatMPlain, Params{})
	if err != nil {
		t.Fatal(err)
	}
	// mplain expands to a 4-level tree:
	//   root → paragraph → sentence → phrase → word
	rootChildren := tf.Tree.ChildrenNotEmpty()
	if len(rootChildren) == 0 {
		t.Fatal("no paragraph nodes")
	}
	paragraph := rootChildren[0]
	sentenceNodes := paragraph.ChildrenNotEmpty()
	if len(sentenceNodes) == 0 {
		t.Fatal("no sentence nodes")
	}
	phraseNodes := sentenceNodes[0].ChildrenNotEmpty()
	if len(phraseNodes) == 0 {
		t.Fatal("no phrase nodes under sentence")
	}
	wordNodes := phraseNodes[0].ChildrenNotEmpty()
	if len(wordNodes) == 0 {
		t.Fatal("no word nodes under phrase")
	}
}

// A multi-clause sentence should produce one phrase per clause, with
// each phrase's words attached underneath it.
func TestMPlainPhraseSplitting(t *testing.T) {
	// Two paragraphs, one sentence each. The second sentence has three
	// clauses separated by commas/period — should yield 3 phrase nodes.
	src := []string{
		"Hello world.",
		"",
		"The quick brown fox jumps, lazy dog sleeps, all is well.",
	}
	tf, err := ReadLines(src, FormatMPlain, Params{})
	if err != nil {
		t.Fatal(err)
	}
	paragraphs := tf.Tree.ChildrenNotEmpty()
	if len(paragraphs) != 2 {
		t.Fatalf("paragraphs: got %d, want 2", len(paragraphs))
	}
	// Paragraph 1: one sentence, one phrase ("Hello world.").
	sentencesP1 := paragraphs[0].ChildrenNotEmpty()
	if len(sentencesP1) != 1 {
		t.Fatalf("paragraph 1 sentences: got %d, want 1", len(sentencesP1))
	}
	if got := len(sentencesP1[0].ChildrenNotEmpty()); got != 1 {
		t.Errorf("paragraph 1 phrase count: got %d, want 1", got)
	}
	// Paragraph 2: one sentence, three phrases.
	sentencesP2 := paragraphs[1].ChildrenNotEmpty()
	if len(sentencesP2) != 1 {
		t.Fatalf("paragraph 2 sentences: got %d, want 1", len(sentencesP2))
	}
	phrases := sentencesP2[0].ChildrenNotEmpty()
	if len(phrases) != 3 {
		t.Fatalf("paragraph 2 phrase count: got %d, want 3", len(phrases))
	}
	wantPhrases := []string{
		"The quick brown fox jumps,",
		"lazy dog sleeps,",
		"all is well.",
	}
	for i, want := range wantPhrases {
		frag, ok := phrases[i].Value.(*Fragment)
		if !ok || frag.Lines[0] != want {
			t.Errorf("phrase[%d]: got %q, want %q", i, frag.Lines[0], want)
		}
	}
}

// --- UNPARSED ---

func TestReadUnparsed(t *testing.T) {
	path := testdataDir("inputtext/sonnet_unparsed.xhtml")
	tf, err := ReadFile(path, FormatUnparsed, Params{
		UnparsedIDRegex: "f[0-9]+",
	})
	if err != nil {
		t.Fatal(err)
	}
	frags := tf.Fragments()
	if len(frags) != 15 {
		t.Fatalf("unparsed (id regex): want 15, got %d", len(frags))
	}
	if frags[0].Identifier != "f001" {
		t.Errorf("first id: want f001, got %q", frags[0].Identifier)
	}
}

func TestReadUnparsedLexSort(t *testing.T) {
	path := testdataDir("inputtext/sonnet_unparsed.xhtml")
	tf, err := ReadFile(path, FormatUnparsed, Params{
		UnparsedIDRegex: "f[0-9]+",
		UnparsedIDSort:  "lexicographic",
	})
	if err != nil {
		t.Fatal(err)
	}
	frags := tf.Fragments()
	if len(frags) == 0 {
		t.Fatal("expected fragments")
	}
	// lexicographic: f001 < f002 < ... < f015
	for i := 1; i < len(frags); i++ {
		if frags[i].Identifier < frags[i-1].Identifier {
			t.Errorf("not sorted: %q before %q", frags[i-1].Identifier, frags[i].Identifier)
		}
	}
}

// --- MUNPARSED ---

func TestReadMUnparsed(t *testing.T) {
	path := testdataDir("inputtext/sonnet_munparsed.xhtml")
	tf, err := ReadFile(path, FormatMUnparsed, Params{
		MUnparsedL1IDRegex: `p[0-9]+`,
		MUnparsedL2IDRegex: `p[0-9]+s[0-9]+`,
		MUnparsedL3IDRegex: `p[0-9]+s[0-9]+w[0-9]+`,
	})
	if err != nil {
		t.Fatal(err)
	}
	frags := tf.Fragments()
	if len(frags) == 0 {
		t.Fatal("munparsed: expected at least one paragraph")
	}
}

// --- Filter ---

func TestIgnoreRegexFilter(t *testing.T) {
	f, err := NewIgnoreRegexFilter(`\(.*?\)`)
	if err != nil {
		t.Fatal(err)
	}
	got := f.Apply([]string{"hello (world) there"})
	if got[0] != "hello there" {
		t.Errorf("got %q", got[0])
	}
}

func TestFilterChain(t *testing.T) {
	fc := &FilterChain{}
	f1, _ := NewIgnoreRegexFilter(`\d+`)
	fc.Add(f1)
	got := fc.Apply([]string{"abc123def"})
	if got[0] != "abcdef" {
		t.Errorf("got %q", got[0])
	}
}

// --- TransliterationMap ---

func TestTransliterationMap(t *testing.T) {
	path := testdataDir("transliteration/transliteration.map")
	tm, err := NewTransliterationMap(path)
	if err != nil {
		t.Fatal(err)
	}
	// From the map file: "a A" means 'a' → 'A'
	got := tm.Transliterate("abc")
	if got[0] != 'A' {
		t.Errorf("'a' should transliterate to 'A', got %q", got)
	}
	// 'e' → deleted
	got2 := tm.Transliterate("de")
	if got2 != "d" {
		t.Errorf("'e' should be deleted, got %q", got2)
	}
}
