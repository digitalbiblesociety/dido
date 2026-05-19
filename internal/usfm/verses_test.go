package usfm

import (
	"strings"
	"testing"
)

func TestParseSmall(t *testing.T) {
	src := `\id ISA
\h Isaiah
\c 1
\p
\v 1 Hello world.
\v 2-3 Line two and three.
\c 2
\p
\v 1 Chapter two opening.
`
	verses, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []Verse{
		{Chapter: 1, VerseStart: 1, VerseEnd: 1, Text: "Hello world."},
		{Chapter: 1, VerseStart: 2, VerseEnd: 3, Text: "Line two and three."},
		{Chapter: 2, VerseStart: 1, VerseEnd: 1, Text: "Chapter two opening."},
	}
	if len(verses) != len(want) {
		t.Fatalf("verse count: got %d, want %d", len(verses), len(want))
	}
	for i, v := range verses {
		if v != want[i] {
			t.Errorf("verse[%d]: got %+v, want %+v", i, v, want[i])
		}
	}
}

func TestVerseID(t *testing.T) {
	if got := (Verse{VerseStart: 5, VerseEnd: 5}).ID(); got != "5" {
		t.Errorf("got %q, want %q", got, "5")
	}
	if got := (Verse{VerseStart: 5, VerseEnd: 7}).ID(); got != "5-7" {
		t.Errorf("got %q, want %q", got, "5-7")
	}
}

func TestStripUSFMWordMarkers(t *testing.T) {
	in := `\w Blessed|strong="H0835"\w* is the man.`
	out := stripUSFM(in)
	if out != "Blessed is the man." {
		t.Errorf("got %q", out)
	}
}

func TestStripUSFMFootnotes(t *testing.T) {
	in := `He shall be like a tree.\f + \fr 1.3 \ft wither: Heb. fade\f*`
	out := stripUSFM(in)
	if out != "He shall be like a tree." {
		t.Errorf("got %q", out)
	}
}

func TestStripUSFMNestedND(t *testing.T) {
	in := `The \nd  \+w LORD|strong="H3068"\+w*\nd* is here.`
	out := stripUSFM(in)
	if !strings.Contains(out, "LORD") {
		t.Errorf("LORD lost from %q: got %q", in, out)
	}
	if strings.Contains(out, "\\") || strings.Contains(out, "strong=") {
		t.Errorf("unstripped markup remains: %q", out)
	}
}

func TestSectionHeadersDroppedByDefault(t *testing.T) {
	src := `\c 1
\v 9 First verse text
\s1 The section header
\p
\v 10 Next verse text
`
	verses, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(verses) != 2 {
		t.Fatalf("verse count: got %d, want 2", len(verses))
	}
	if verses[0].Text != "First verse text" {
		t.Errorf("v9 should NOT include section header by default: got %q", verses[0].Text)
	}
	if verses[1].Text != "Next verse text" {
		t.Errorf("v10: got %q", verses[1].Text)
	}
}

func TestSectionHeadersIncludedWithOption(t *testing.T) {
	src := `\c 1
\v 9 First verse text
\s1 The section header
\p
\v 10 Next verse text
`
	verses, err := ParseWithOptions(strings.NewReader(src), Options{IncludeSectionHeaders: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(verses) != 2 {
		t.Fatalf("verse count: got %d, want 2", len(verses))
	}
	if verses[0].Text != "First verse text The section header" {
		t.Errorf("v9 should include section header with option: got %q", verses[0].Text)
	}
}

// All \s variants (\s, \s1, \s2, \s3, \s4, \sr, \sp, \sd) must be
// recognised. Verify the most common ones from real-world USFM.
func TestSectionHeadersVariantsDropped(t *testing.T) {
	src := `\c 1
\v 1 Verse one
\s A bare \s marker
\v 2 Verse two
\sr A scripture reference
\v 3 Verse three
\sp A speaker label
`
	verses, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Verse one", "Verse two", "Verse three"}
	if len(verses) != len(want) {
		t.Fatalf("verse count: got %d, want %d", len(verses), len(want))
	}
	for i, v := range verses {
		if v.Text != want[i] {
			t.Errorf("verse[%d]: got %q, want %q", i, v.Text, want[i])
		}
	}
}

func TestParseContinuationLines(t *testing.T) {
	// Verse 1 continues onto a poetry line (\q1) — the text should be
	// joined with a single space, with the structural marker dropped.
	src := `\c 1
\q1
\v 1 Line one
\q1 and continues
\v 2 Next verse
`
	verses, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(verses) != 2 {
		t.Fatalf("verse count: got %d, want 2", len(verses))
	}
	if !strings.Contains(verses[0].Text, "Line one") {
		t.Errorf("first verse missing main text: %q", verses[0].Text)
	}
	if !strings.Contains(verses[0].Text, "continues") {
		t.Errorf("first verse should have continuation: %q", verses[0].Text)
	}
}

// TestParseThaiIsaiah is a smoke test against the real Thai Isaiah
// USFM file shipped at /Users/jon/Projects/dbs/bibles/source/. Skipped
// when the file isn't present (CI / fresh checkouts).
func TestParseThaiIsaiah(t *testing.T) {
	path := "/Users/jon/Projects/dbs/bibles/source/THAKJV/usfm/24-ISAthaKJV.usfm"
	verses, err := ParseFile(path)
	if err != nil {
		t.Skipf("Thai ISA fixture not available: %v", err)
	}
	if MaxChapter(verses) != 66 {
		t.Errorf("Isaiah has 66 chapters; got max chapter %d", MaxChapter(verses))
	}
	// Chapter 1 of Isaiah has 31 verses in the KJV numbering.
	c1 := Chapter(verses, 1)
	if len(c1) != 31 {
		t.Errorf("Isaiah 1 should have 31 verses; got %d", len(c1))
	}
	if len(c1) > 0 {
		first := c1[0].Text
		if len(first) == 0 {
			t.Errorf("Isaiah 1:1 text empty")
		}
		if strings.Contains(first, `\v`) || strings.Contains(first, `\w`) {
			t.Errorf("Isaiah 1:1 still has unstripped markers: %q", first)
		}
	}
}
