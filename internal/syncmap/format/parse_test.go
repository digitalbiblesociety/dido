package format

import (
	"testing"

	"github.com/digitalbiblesociety/dido/internal/syncmap"
)

// These tests exercise the parse* helpers in this package. Prior to B8 they
// were dead code (only parseJSON and parseSubtitles via SRT had any test
// coverage). The intent here is twofold:
//   1. Catch silent parser bugs by loading each Python-generated golden file
//      under testdata/syncmaps/ and asserting the recovered fragment count
//      and end-points match the sonnet001 reference.
//   2. Smoke-test that subsequent refactors don't accidentally break a parser
//      that no production caller invokes today.
//
// Anything left unwired here is intentional: see B8 in IMPROVEMENT_PLANS.md
// for the rationale.

const (
	wantFrags       = 15
	wantFirstBegin  = "0.000"
	wantLastEnd     = "53.240"
	wantFirstID     = "f000001"
	wantLastID      = "f000015"
)

// assertSonnet001 verifies sm has the shape produced by sonnet001() (15
// fragments, known begin/end stamps, known ids). Used by every parse test.
func assertSonnet001(t *testing.T, sm *syncmap.SyncMap, requireID bool) {
	t.Helper()
	frags := sm.Fragments()
	if len(frags) != wantFrags {
		t.Fatalf("fragment count: got %d, want %d", len(frags), wantFrags)
	}
	if got := frags[0].Begin().String(); got != wantFirstBegin {
		t.Errorf("first begin: got %q, want %q", got, wantFirstBegin)
	}
	if got := frags[wantFrags-1].End().String(); got != wantLastEnd {
		t.Errorf("last end: got %q, want %q", got, wantLastEnd)
	}
	if requireID {
		if got := frags[0].Identifier(); got != wantFirstID {
			t.Errorf("first id: got %q, want %q", got, wantFirstID)
		}
		if got := frags[wantFrags-1].Identifier(); got != wantLastID {
			t.Errorf("last id: got %q, want %q", got, wantLastID)
		}
	}
}

// --- tabular family --------------------------------------------------------

func TestParseCSV(t *testing.T) {
	sm := syncmap.NewSyncMap()
	cfg := tabularConfig{
		delimiter:  ",",
		textDelim:  "\"",
		hasID:      true,
		hasText:    true,
		fieldOrder: []string{"id", "begin", "end", "text"},
	}
	if err := parseTabular(golden(t, "sonnet001.csv"), sm, cfg); err != nil {
		t.Fatal(err)
	}
	assertSonnet001(t, sm, true)
}

func TestParseTSV(t *testing.T) {
	sm := syncmap.NewSyncMap()
	cfg := tabularConfig{
		delimiter:  "\t",
		textDelim:  "",
		hasID:      true,
		hasText:    false,
		fieldOrder: []string{"begin", "end", "id"},
	}
	if err := parseTabular(golden(t, "sonnet001.tsv"), sm, cfg); err != nil {
		t.Fatal(err)
	}
	assertSonnet001(t, sm, true)
}

func TestParseAUD(t *testing.T) {
	sm := syncmap.NewSyncMap()
	cfg := tabularConfig{
		delimiter:  "\t",
		textDelim:  "",
		hasID:      false,
		hasText:    true,
		fieldOrder: []string{"begin", "end", "text"},
	}
	if err := parseTabular(golden(t, "sonnet001.aud"), sm, cfg); err != nil {
		t.Fatal(err)
	}
	// AUD has no id column; only check timestamps + fragment count.
	assertSonnet001(t, sm, false)
}

// --- subtitle family -------------------------------------------------------

func TestParseVTT(t *testing.T) {
	sm := syncmap.NewSyncMap()
	cfg := subtitlesConfig{
		header:          "WEBVTT",
		hasID:           false,
		optionalID:      true,
		timeSep:         " --> ",
		lineBreakSymbol: "\n",
		timeFmt:         toHHMMSSMMM,
		parseTimeFn:     parseHHMMSSMMM,
	}
	if err := parseSubtitles(golden(t, "sonnet001.vtt"), sm, cfg); err != nil {
		t.Fatal(err)
	}
	assertSonnet001(t, sm, false)
}

func TestParseSUB(t *testing.T) {
	sm := syncmap.NewSyncMap()
	cfg := subtitlesConfig{
		header:          "[SUBTITLE]",
		footer:          "[END SUBTITLE]",
		hasID:           false,
		timeSep:         ",",
		lineBreakSymbol: "[br]",
		timeFmt:         toHHMMSSMMM,
		parseTimeFn:     parseHHMMSSMMM,
	}
	if err := parseSubtitles(golden(t, "sonnet001.sub"), sm, cfg); err != nil {
		t.Fatal(err)
	}
	assertSonnet001(t, sm, false)
}

// --- JSON family -----------------------------------------------------------

func TestParseRBSE(t *testing.T) {
	sm := syncmap.NewSyncMap()
	if err := parseRBSE(golden(t, "sonnet001.rbse"), sm); err != nil {
		t.Fatal(err)
	}
	assertSonnet001(t, sm, true)
}

// --- XML family ------------------------------------------------------------

func TestParseXML(t *testing.T) {
	sm := syncmap.NewSyncMap()
	if err := parseXML(golden(t, "sonnet001.xml"), sm); err != nil {
		t.Fatal(err)
	}
	assertSonnet001(t, sm, true)
}

func TestParseXMLLegacy(t *testing.T) {
	sm := syncmap.NewSyncMap()
	if err := parseXMLLegacy(golden(t, "sonnet001.xml_legacy"), sm); err != nil {
		t.Fatal(err)
	}
	assertSonnet001(t, sm, true)
}

func TestParseTTML(t *testing.T) {
	sm := syncmap.NewSyncMap()
	if err := parseTTML(golden(t, "sonnet001.ttml"), sm); err != nil {
		t.Fatal(err)
	}
	assertSonnet001(t, sm, true)
}

func TestParseSMIL(t *testing.T) {
	sm := syncmap.NewSyncMap()
	if err := parseSMIL(golden(t, "sonnet001.smil"), sm); err != nil {
		t.Fatal(err)
	}
	// SMIL ids are derived from the <text> href; SMIL parsing rebuilds them
	// from the page reference, so don't require the exact f000001/f000015 names.
	assertSonnet001(t, sm, false)
}

func TestParseEAF(t *testing.T) {
	sm := syncmap.NewSyncMap()
	if err := parseEAF(golden(t, "sonnet001.eaf"), sm); err != nil {
		t.Fatal(err)
	}
	// EAF identifies annotations by a synthetic id (ANNOTATION_ID="a1");
	// only verify fragment shape and times.
	assertSonnet001(t, sm, false)
}
