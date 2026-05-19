package format

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/timing"
)

// golden reads testdata/syncmaps/{name} and returns its contents.
func golden(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "syncmaps")
	data, err := os.ReadFile(filepath.Join(base, name))
	if err != nil {
		t.Fatalf("golden %s: %v", name, err)
	}
	return string(data)
}

// sonnet001 builds the 15-fragment flat SyncMap matching the sonnet001 fixtures.
func sonnet001() *syncmap.SyncMap {
	type entry struct{ b, e, id, line string }
	rows := []entry{
		{"0.000", "2.680", "f000001", "1"},
		{"2.680", "5.880", "f000002", "From fairest creatures we desire increase,"},
		{"5.880", "9.240", "f000003", "That thereby beauty's rose might never die,"},
		{"9.240", "11.920", "f000004", "But as the riper should by time decease,"},
		{"11.920", "15.280", "f000005", "His tender heir might bear his memory:"},
		{"15.280", "18.600", "f000006", "But thou contracted to thine own bright eyes,"},
		{"18.600", "22.800", "f000007", "Feed'st thy light's flame with self-substantial fuel,"},
		{"22.800", "25.680", "f000008", "Making a famine where abundance lies,"},
		{"25.680", "31.240", "f000009", "Thy self thy foe, to thy sweet self too cruel:"},
		{"31.240", "34.280", "f000010", "Thou that art now the world's fresh ornament,"},
		{"34.280", "36.960", "f000011", "And only herald to the gaudy spring,"},
		{"36.960", "40.680", "f000012", "Within thine own bud buriest thy content,"},
		{"40.680", "44.560", "f000013", "And tender churl mak'st waste in niggarding:"},
		{"44.560", "48.080", "f000014", "Pity the world, or else this glutton be,"},
		{"48.080", "53.240", "f000015", "To eat the world's due, by the grave and thee."},
	}
	sm := syncmap.NewSyncMap()
	for _, r := range rows {
		b := timing.MustParseTimeValue(r.b)
		e := timing.MustParseTimeValue(r.e)
		tf := &text.Fragment{Identifier: r.id, Language: "en", Lines: []string{r.line}}
		sm.Add(syncmap.NewSyncMapFragment(tf, b, e, syncmap.Regular))
	}
	return sm
}

// trimTrailing strips trailing whitespace from each line and the final newline
// so golden-file comparisons are whitespace-insensitive at line ends.
func trimTrailing(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return strings.Join(lines, "\n")
}

// --- JSON / RBSE ---

func TestFormatJSON(t *testing.T) {
	sm := sonnet001()
	got := formatJSON(sm)
	want := golden(t, "sonnet001.json")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("JSON mismatch\ngot:\n%s\nwant:\n%s", got[:min(200, len(got))], want[:min(200, len(want))])
	}
}

func TestParseJSON(t *testing.T) {
	sm := syncmap.NewSyncMap()
	if err := parseJSON(golden(t, "sonnet001.json"), sm); err != nil {
		t.Fatal(err)
	}
	frags := sm.Fragments()
	if len(frags) != 15 {
		t.Fatalf("want 15 fragments, got %d", len(frags))
	}
	if frags[0].Identifier() != "f000001" {
		t.Errorf("first id: %q", frags[0].Identifier())
	}
	if frags[0].Begin().String() != "0.000" {
		t.Errorf("first begin: %q", frags[0].Begin().String())
	}
}

func TestRoundtripJSON(t *testing.T) {
	sm := sonnet001()
	out := formatJSON(sm)
	sm2 := syncmap.NewSyncMap()
	if err := parseJSON(out, sm2); err != nil {
		t.Fatal(err)
	}
	frags := sm2.Fragments()
	orig := sm.Fragments()
	if len(frags) != len(orig) {
		t.Fatalf("roundtrip: fragment count %d != %d", len(frags), len(orig))
	}
	for i := range orig {
		if frags[i].Identifier() != orig[i].Identifier() {
			t.Errorf("frag %d id mismatch", i)
		}
	}
}

func TestFormatRBSE(t *testing.T) {
	sm := sonnet001()
	got := formatRBSE(sm)
	want := golden(t, "sonnet001.rbse")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("RBSE mismatch\ngot[0:200]:\n%s", got[:min(200, len(got))])
	}
}

// --- SRT / VTT ---

func TestFormatSRT(t *testing.T) {
	sm := sonnet001()
	got := formatSRT(sm)
	want := golden(t, "sonnet001.srt")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("SRT mismatch\ngot[0:300]:\n%s\nwant[0:300]:\n%s",
			got[:min(300, len(got))], want[:min(300, len(want))])
	}
}

func TestParseAndRoundtripSRT(t *testing.T) {
	sm := syncmap.NewSyncMap()
	cfg := subtitlesConfig{
		hasID: true, timeSep: " --> ",
		lineBreakSymbol: "\n", timeFmt: toSRT, parseTimeFn: parseHHMMSSMMM,
	}
	if err := parseSubtitles(golden(t, "sonnet001.srt"), sm, cfg); err != nil {
		t.Fatal(err)
	}
	if len(sm.Fragments()) != 15 {
		t.Fatalf("SRT parse: want 15, got %d", len(sm.Fragments()))
	}
}

func TestFormatVTT(t *testing.T) {
	sm := sonnet001()
	got := formatVTT(sm)
	want := golden(t, "sonnet001.vtt")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("VTT mismatch\ngot[0:300]:\n%s\nwant[0:300]:\n%s",
			got[:min(300, len(got))], want[:min(300, len(want))])
	}
}

// --- Tabular ---

func TestFormatCSV(t *testing.T) {
	sm := sonnet001()
	got := formatCSV(sm, false)
	want := golden(t, "sonnet001.csv")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("CSV mismatch\ngot:\n%s\nwant:\n%s", got[:min(200, len(got))], want[:min(200, len(want))])
	}
}

func TestFormatCSVHuman(t *testing.T) {
	sm := sonnet001()
	got := formatCSV(sm, true)
	want := golden(t, "sonnet001.csvh")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("CSVh mismatch\ngot:\n%s\nwant:\n%s", got[:min(200, len(got))], want[:min(200, len(want))])
	}
}

func TestFormatTSV(t *testing.T) {
	sm := sonnet001()
	got := formatTSV(sm, false)
	want := golden(t, "sonnet001.tsv")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("TSV mismatch")
	}
}

func TestFormatSSV(t *testing.T) {
	sm := sonnet001()
	got := formatSSV(sm, false)
	want := golden(t, "sonnet001.ssv")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("SSV mismatch\ngot:\n%s\nwant:\n%s", got[:min(200, len(got))], want[:min(200, len(want))])
	}
}

func TestFormatTXT(t *testing.T) {
	sm := sonnet001()
	got := formatTXT(sm, false)
	want := golden(t, "sonnet001.txt")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("TXT mismatch\ngot:\n%s\nwant:\n%s", got[:min(200, len(got))], want[:min(200, len(want))])
	}
}

func TestFormatAUD(t *testing.T) {
	sm := sonnet001()
	got := formatAUD(sm, false)
	want := golden(t, "sonnet001.aud")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("AUD mismatch\ngot:\n%s\nwant:\n%s", got[:min(200, len(got))], want[:min(200, len(want))])
	}
}

// --- XML family ---

func TestFormatXMLLegacy(t *testing.T) {
	sm := sonnet001()
	got := formatXMLLegacy(sm)
	want := golden(t, "sonnet001.xml_legacy")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("XML_legacy mismatch\ngot:\n%s\nwant:\n%s", got[:min(300, len(got))], want[:min(300, len(want))])
	}
}

func TestFormatTTML(t *testing.T) {
	sm := sonnet001()
	got := formatTTML(sm)
	want := golden(t, "sonnet001.ttml")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("TTML mismatch\ngot[0:500]:\n%s\nwant[0:500]:\n%s",
			got[:min(500, len(got))], want[:min(500, len(want))])
	}
}

func TestFormatSMIL(t *testing.T) {
	sm := sonnet001()
	p := SMILParams{PageRef: "sonnet001.xhtml", AudioRef: "sonnet001.mp3"}
	got := formatSMIL(sm, p, true) // human variant
	want := golden(t, "sonnet001.smil")
	if trimTrailing(got) != trimTrailing(want) {
		t.Errorf("SMIL mismatch\ngot[0:500]:\n%s\nwant[0:500]:\n%s",
			got[:min(500, len(got))], want[:min(500, len(want))])
	}
}

// --- Write dispatcher ---

func TestWriteDispatch(t *testing.T) {
	sm := sonnet001()
	for _, f := range []Format{
		FormatCSV, FormatTSV, FormatSSV, FormatTXT, FormatAUD, FormatTAB,
		FormatSRT, FormatVTT,
		FormatJSON, FormatRBSE,
		FormatXMLLegacy, FormatTTML,
	} {
		_, err := Write(sm, f, SMILParams{}, EAFParams{})
		if err != nil {
			t.Errorf("Write(%s): %v", f, err)
		}
	}
}

func TestWriteUnknownFormat(t *testing.T) {
	sm := sonnet001()
	_, err := Write(sm, "zzz", SMILParams{}, EAFParams{})
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	msg := err.Error()
	if !strings.Contains(msg, "zzz") {
		t.Errorf("error should mention bad format name; got %q", msg)
	}
	// At least three known formats should appear so the user has options.
	for _, f := range []string{"json", "srt", "vtt"} {
		if !strings.Contains(msg, f) {
			t.Errorf("error should list %q as a valid format; got %q", f, msg)
		}
	}
}

func TestSupportedFormatsIncludesEverythingWriteSupports(t *testing.T) {
	all := SupportedFormats()
	if len(all) < 25 {
		t.Errorf("SupportedFormats() returned only %d entries; expected ≥25", len(all))
	}
	// Spot-check a few canonical formats.
	want := []string{"json", "srt", "vtt", "ttml", "smil", "eaf", "xml"}
	got := make(map[string]bool, len(all))
	for _, f := range all {
		got[f] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("SupportedFormats() missing %q", w)
		}
	}
}

// --- Timing helpers ---

func TestToSSMMM(t *testing.T) {
	tv := timing.MustParseTimeValue("12.345")
	if toSSMMM(tv) != "12.345" {
		t.Errorf("got %q", toSSMMM(tv))
	}
}

func TestToHHMMSSMMM(t *testing.T) {
	tv := timing.MustParseTimeValue("3612.345")
	if toHHMMSSMMM(tv) != "01:00:12.345" {
		t.Errorf("got %q", toHHMMSSMMM(tv))
	}
}

func TestToSRT(t *testing.T) {
	tv := timing.MustParseTimeValue("12.345")
	if toSRT(tv) != "00:00:12,345" {
		t.Errorf("got %q", toSRT(tv))
	}
}

func TestParseHHMMSSMMM(t *testing.T) {
	tv, err := parseHHMMSSMMM("01:00:12.345")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%.3f", tv.Float64()) != "3612.345" {
		t.Errorf("got %.3f", tv.Float64())
	}
}

func TestParseHHMMSSMMMComma(t *testing.T) {
	tv, err := parseHHMMSSMMM("00:00:12,345")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%.3f", tv.Float64()) != "12.345" {
		t.Errorf("got %.3f", tv.Float64())
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
