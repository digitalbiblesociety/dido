package format

import (
	"strings"
	"testing"

	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/timing"
)

// Tests for sync-map output formats that were not previously covered by
// format_test.go: SUB, SBV, TAB, EAF, XML (new), DFXP (== TTML), and the
// "human" / "machine" variants of the textual formats.
//
// Every golden file under testdata/syncmaps/sonnet001.* was produced by the
// upstream Python aeneas pipeline, so byte-for-byte equality with these
// files is the strongest possible parity signal.

func TestFormatSUB(t *testing.T) {
	sm := sonnet001()
	got := formatSUB(sm)
	want := golden(t, "sonnet001.sub")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("SUB mismatch\ngot[0:300]:\n%s\nwant[0:300]:\n%s",
			got[:min(300, len(got))], want[:min(300, len(want))])
	}
}

func TestFormatSBV(t *testing.T) {
	sm := sonnet001()
	got := formatSBV(sm)
	want := golden(t, "sonnet001.sbv")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("SBV mismatch\ngot[0:300]:\n%s\nwant[0:300]:\n%s",
			got[:min(300, len(got))], want[:min(300, len(want))])
	}
}

func TestFormatTAB(t *testing.T) {
	sm := sonnet001()
	got := formatTAB(sm, false)
	want := golden(t, "sonnet001.tab")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("TAB mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatTSVH(t *testing.T) {
	sm := sonnet001()
	got := formatTSV(sm, true)
	want := golden(t, "sonnet001.tsvh")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("TSVh mismatch\ngot[0:200]:\n%s\nwant[0:200]:\n%s",
			got[:min(200, len(got))], want[:min(200, len(want))])
	}
}

func TestFormatSSVH(t *testing.T) {
	sm := sonnet001()
	got := formatSSV(sm, true)
	want := golden(t, "sonnet001.ssvh")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("SSVh mismatch")
	}
}

func TestFormatTXTH(t *testing.T) {
	sm := sonnet001()
	got := formatTXT(sm, true)
	want := golden(t, "sonnet001.txth")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("TXTh mismatch")
	}
}

func TestFormatAUDH(t *testing.T) {
	sm := sonnet001()
	got := formatAUD(sm, true)
	want := golden(t, "sonnet001.audh")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("AUDh mismatch")
	}
}

func TestFormatEAF(t *testing.T) {
	sm := sonnet001()
	got := formatEAF(sm, EAFParams{})
	want := golden(t, "sonnet001.eaf")
	if trimTrailing(got) != trimTrailing(want) {
		// EAF includes a generated timestamp + UUID — accept structural match.
		if !containsAll(got, []string{
			"<ANNOTATION_DOCUMENT",
			"<TIER",
			"<ANNOTATION_VALUE>From fairest creatures we desire increase,</ANNOTATION_VALUE>",
			"<ANNOTATION_VALUE>To eat the world's due, by the grave and thee.</ANNOTATION_VALUE>",
			"TIME_VALUE=\"0\"",
			"TIME_VALUE=\"53240\"",
		}) {
			t.Errorf("EAF mismatch (structural)\ngot[0:400]:\n%s", got[:min(400, len(got))])
		}
	}
}

func TestFormatDFXP(t *testing.T) {
	sm := sonnet001()
	got, err := Write(sm, FormatDFXP, SMILParams{}, EAFParams{})
	if err != nil {
		t.Fatal(err)
	}
	want := golden(t, "sonnet001.dfxp")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("DFXP mismatch\ngot[0:300]:\n%s\nwant[0:300]:\n%s",
			got[:min(300, len(got))], want[:min(300, len(want))])
	}
}

func TestFormatSMILM(t *testing.T) {
	sm := sonnet001()
	p := SMILParams{PageRef: "sonnet001.xhtml", AudioRef: "sonnet001.mp3"}
	got := formatSMIL(sm, p, false)
	want := golden(t, "sonnet001.smilm")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("SMILm mismatch\ngot[0:400]:\n%s\nwant[0:400]:\n%s",
			got[:min(400, len(got))], want[:min(400, len(want))])
	}
}

func TestFormatTextGridDefault(t *testing.T) {
	sm := sonnet001()
	got, err := Write(sm, FormatTextGrid, SMILParams{}, EAFParams{})
	if err != nil {
		t.Fatal(err)
	}
	want := golden(t, "sonnet001.textgrid")
	if got != want {
		t.Errorf("TextGrid (default/long) mismatch\ngot[0:300]:\n%s\nwant[0:300]:\n%s",
			got[:min(300, len(got))], want[:min(300, len(want))])
	}
}

func TestFormatTextGridLong(t *testing.T) {
	sm := sonnet001()
	got, err := Write(sm, FormatTextGridLong, SMILParams{}, EAFParams{})
	if err != nil {
		t.Fatal(err)
	}
	want := golden(t, "sonnet001.textgrid_long")
	if got != want {
		t.Errorf("TextGrid long mismatch\ngot[0:300]:\n%s\nwant[0:300]:\n%s",
			got[:min(300, len(got))], want[:min(300, len(want))])
	}
}

func TestFormatTextGridShort(t *testing.T) {
	sm := sonnet001()
	got, err := Write(sm, FormatTextGridShort, SMILParams{}, EAFParams{})
	if err != nil {
		t.Fatal(err)
	}
	want := golden(t, "sonnet001.textgrid_short")
	if got != want {
		t.Errorf("TextGrid short mismatch\ngot[0:300]:\n%s\nwant[0:300]:\n%s",
			got[:min(300, len(got))], want[:min(300, len(want))])
	}
}

func TestFormatXMLNew(t *testing.T) {
	sm := sonnet001()
	got := formatXML(sm)
	want := golden(t, "sonnet001.xml")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("XML mismatch\ngot[0:500]:\n%s\nwant[0:500]:\n%s",
			got[:min(500, len(got))], want[:min(500, len(want))])
	}
}

// TestWriteDispatchAllFormats validates that every Format constant is wired up
// in Write and produces non-empty output for our standard sync map.
func TestWriteDispatchAllFormats(t *testing.T) {
	sm := sonnet001()
	smil := SMILParams{PageRef: "sonnet001.xhtml", AudioRef: "sonnet001.mp3"}
	eaf := EAFParams{}
	wantFormats := []Format{
		FormatAUD, FormatAUDH, FormatAUDM,
		FormatCSV, FormatCSVH, FormatCSVM,
		FormatDFXP,
		FormatEAF,
		FormatJSON,
		FormatRBSE,
		FormatSBV,
		FormatSMIL, FormatSMILH, FormatSMILM,
		FormatSRT,
		FormatSSV, FormatSSVH, FormatSSVM,
		FormatSUB,
		FormatTAB,
		FormatTSV, FormatTSVH, FormatTSVM,
		FormatTTML,
		FormatTXT, FormatTXTH, FormatTXTM,
		FormatVTT,
		FormatXML,
		FormatXMLLegacy,
	}
	for _, f := range wantFormats {
		out, err := Write(sm, f, smil, eaf)
		if err != nil {
			t.Errorf("Write(%s): %v", f, err)
			continue
		}
		if strings.TrimSpace(out) == "" {
			t.Errorf("Write(%s) produced empty output", f)
		}
	}
}

// containsAll returns true if s contains every substring in needles.
func containsAll(s string, needles []string) bool {
	for _, n := range needles {
		if !strings.Contains(s, n) {
			return false
		}
	}
	return true
}

// hierarchicalFixture builds a 3-level sync map (paragraph → sentence →
// word) used to verify that flat output formats walk the leaves rather
// than the top-level partition. The test asserts the SRT serialisation
// shows the word-level cues (and only those).
func hierarchicalFixture() *syncmap.SyncMap {
	sm := syncmap.NewSyncMap()
	// Paragraph 1, sentence 1, two words.
	p1 := syncmap.NewSyncMapFragment(
		&text.Fragment{Identifier: "p1", Language: "en", Lines: []string{"para 1"}},
		timing.MustParseTimeValue("0.000"),
		timing.MustParseTimeValue("4.000"),
		syncmap.Regular)
	p1Node := syncmap.NewTree(p1)
	sm.Tree.AddChild(p1Node, true)

	s1 := syncmap.NewSyncMapFragment(
		&text.Fragment{Identifier: "s1", Language: "en", Lines: []string{"sentence one"}},
		timing.MustParseTimeValue("0.000"),
		timing.MustParseTimeValue("4.000"),
		syncmap.Regular)
	s1Node := syncmap.NewTree(s1)
	p1Node.AddChild(s1Node, true)

	w1 := syncmap.NewSyncMapFragment(
		&text.Fragment{Identifier: "w1", Language: "en", Lines: []string{"hello"}},
		timing.MustParseTimeValue("0.000"),
		timing.MustParseTimeValue("2.000"),
		syncmap.Regular)
	s1Node.AddChild(syncmap.NewTree(w1), true)

	w2 := syncmap.NewSyncMapFragment(
		&text.Fragment{Identifier: "w2", Language: "en", Lines: []string{"world"}},
		timing.MustParseTimeValue("2.000"),
		timing.MustParseTimeValue("4.000"),
		syncmap.Regular)
	s1Node.AddChild(syncmap.NewTree(w2), true)
	return sm
}

// TestFlatFormatsWalkLeaves verifies that flat output formats serialise
// the leaf-level fragments (hello/world), not the paragraph rollup.
func TestFlatFormatsWalkLeaves(t *testing.T) {
	sm := hierarchicalFixture()

	got := formatSRT(sm)
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("SRT should contain leaf word fragments; got:\n%s", got)
	}
	// The paragraph and sentence rollups should NOT appear (they don't
	// represent playable cues).
	if strings.Contains(got, "para 1") || strings.Contains(got, "sentence one") {
		t.Errorf("SRT should not contain paragraph/sentence rollups; got:\n%s", got)
	}

	tab := formatCSV(sm, false)
	if !strings.Contains(tab, "hello") || !strings.Contains(tab, "world") {
		t.Errorf("CSV should walk leaves; got:\n%s", tab)
	}
	if strings.Contains(tab, "para 1") || strings.Contains(tab, "sentence one") {
		t.Errorf("CSV should not include rollup fragments; got:\n%s", tab)
	}
}

// --- Multi-line fragment fixtures (m / mu variants) ------------------------

// sonnet001m builds the 15-fragment sync map where each fragment has 1-2 lines,
// matching the _m fixtures under testdata/syncmaps. Used to validate that
// formats which honour the multi-line structure (SRT, VTT, SUB, JSON,
// TextGrid-ish) produce the expected output. The exact line splits are taken
// from the upstream golden files — note that fragments 1, 11, and 13 are
// single-line because the upstream tokeniser left them intact.
func sonnet001m() *multilineFixture {
	rows := []multilineRow{
		{"0.000", "2.680", "f000001", []string{"1"}},
		{"2.680", "5.880", "f000002", []string{"From fairest creatures", "we desire increase,"}},
		{"5.880", "9.240", "f000003", []string{"That thereby beauty's rose", "might never die,"}},
		{"9.240", "11.920", "f000004", []string{"But as the riper", "should by time decease,"}},
		{"11.920", "15.280", "f000005", []string{"His tender heir", "might bear his memory:"}},
		{"15.280", "18.600", "f000006", []string{"But thou contracted", "to thine own bright eyes,"}},
		{"18.600", "22.800", "f000007", []string{"Feed'st thy light's flame", "with self-substantial fuel,"}},
		{"22.800", "25.680", "f000008", []string{"Making a famine", "where abundance lies,"}},
		{"25.680", "31.240", "f000009", []string{"Thy self thy foe,", "to thy sweet self too cruel:"}},
		{"31.240", "34.280", "f000010", []string{"Thou that art now", "the world's fresh ornament,"}},
		{"34.280", "36.960", "f000011", []string{"And only herald to the gaudy spring,"}},
		{"36.960", "40.680", "f000012", []string{"Within thine own bud", "buriest thy content,"}},
		{"40.680", "44.560", "f000013", []string{"And tender churl mak'st waste in niggarding:"}},
		{"44.560", "48.080", "f000014", []string{"Pity the world,", "or else this glutton be,"}},
		{"48.080", "53.240", "f000015", []string{"To eat the world's due,", "by the grave and thee."}},
	}
	return &multilineFixture{rows: rows}
}

type multilineRow struct {
	begin, end, id string
	lines          []string
}

type multilineFixture struct {
	rows []multilineRow
}

func (f *multilineFixture) toSyncMap() *syncmap.SyncMap {
	sm := syncmap.NewSyncMap()
	for _, r := range f.rows {
		b := timing.MustParseTimeValue(r.begin)
		e := timing.MustParseTimeValue(r.end)
		tf := &text.Fragment{Identifier: r.id, Language: "en", Lines: append([]string(nil), r.lines...)}
		sm.Add(syncmap.NewSyncMapFragment(tf, b, e, syncmap.Regular))
	}
	return sm
}

// TestFormatSRTMultiline verifies that SRT output reproduces the line breaks
// from multi-line fragments (matching the sonnet001_m.srt fixture).
func TestFormatSRTMultiline(t *testing.T) {
	fix := sonnet001m()
	sm := fix.toSyncMap()
	got := formatSRT(sm)
	want := golden(t, "sonnet001_m.srt")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("SRT-m mismatch\ngot[0:500]:\n%s\nwant[0:500]:\n%s",
			got[:min(500, len(got))], want[:min(500, len(want))])
	}
}

// TestFormatVTTMultiline verifies VTT against sonnet001_m.vtt.
func TestFormatVTTMultiline(t *testing.T) {
	fix := sonnet001m()
	sm := fix.toSyncMap()
	got := formatVTT(sm)
	want := golden(t, "sonnet001_m.vtt")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("VTT-m mismatch\ngot[0:500]:\n%s\nwant[0:500]:\n%s",
			got[:min(500, len(got))], want[:min(500, len(want))])
	}
}

// TestFormatSUBMultiline verifies SUB uses [br] between lines in
// multi-line fragments.
func TestFormatSUBMultiline(t *testing.T) {
	fix := sonnet001m()
	sm := fix.toSyncMap()
	got := formatSUB(sm)
	want := golden(t, "sonnet001_m.sub")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("SUB-m mismatch\ngot[0:500]:\n%s\nwant[0:500]:\n%s",
			got[:min(500, len(got))], want[:min(500, len(want))])
	}
}

// TestFormatSBVMultiline verifies SBV uses \n between lines.
func TestFormatSBVMultiline(t *testing.T) {
	fix := sonnet001m()
	sm := fix.toSyncMap()
	got := formatSBV(sm)
	want := golden(t, "sonnet001_m.sbv")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("SBV-m mismatch\ngot[0:500]:\n%s\nwant[0:500]:\n%s",
			got[:min(500, len(got))], want[:min(500, len(want))])
	}
}
