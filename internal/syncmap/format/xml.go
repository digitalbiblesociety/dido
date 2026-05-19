package format

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/timing"
)

// writeAttr appends ` name="value"` to sb. The value is written verbatim
// — callers are responsible for pre-escaping it via xmlEscape if necessary.
// This is the hot-path replacement for fmt.Sprintf("... name=%q ...", v),
// which allocated and copied for every fragment.
func writeAttr(sb *strings.Builder, name, value string) {
	sb.WriteByte(' ')
	sb.WriteString(name)
	sb.WriteString(`="`)
	sb.WriteString(value)
	sb.WriteByte('"')
}

// writeIntAttr appends ` name="N"` (decimal integer) without going through
// fmt.Sprintf. Used for EAF TIME_VALUE millisecond stamps.
func writeIntAttr(sb *strings.Builder, name string, v int64) {
	sb.WriteByte(' ')
	sb.WriteString(name)
	sb.WriteString(`="`)
	sb.Write(strconv.AppendInt(nil, v, 10))
	sb.WriteByte('"')
}

// writeParID writes `prefix"parNNNNNN"` (zero-padded 6-digit SMIL par id)
// without fmt.Sprintf allocations. prefix should include the attribute name
// and `=`, e.g. ` id=`.
func writeParID(sb *strings.Builder, prefix string, n int) {
	sb.WriteString(prefix)
	sb.WriteString(`"par`)
	// fixed 6-digit zero-padded decimal
	var digits [6]byte
	for i := 5; i >= 0; i-- {
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	sb.Write(digits[:])
	sb.WriteByte('"')
}

// ── XML escape helper ────────────────────────────────────────────────────────

// xmlEscape escapes a string for XML text content.
// Apostrophes (') are left unescaped — they are legal in element text content.
func xmlEscape(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		// Fallback: manual escape.
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		return s
	}
	// xml.EscapeText escapes ' as &#39;, but that's unnecessary in element text.
	return strings.ReplaceAll(buf.String(), "&#39;", "'")
}

// ── XML format ───────────────────────────────────────────────────────────────

// formatXML serialises sm to the aeneas XML format.
func formatXML(sm *syncmap.SyncMap) string {
	var sb strings.Builder
	sb.WriteString("<?xml version='1.0' encoding='UTF-8'?>\n")
	sb.WriteString("<map>\n")
	for _, child := range sm.Tree.ChildrenNotEmpty() {
		writeXMLNode(&sb, child, "  ")
	}
	sb.WriteString("</map>")
	return sb.String()
}

func writeXMLNode(sb *strings.Builder, node *syncmap.Tree, indent string) {
	if node.IsEmpty() {
		return
	}
	frag, ok := node.Value.(*syncmap.SyncMapFragment)
	if !ok {
		return
	}

	id := xmlEscape(frag.Identifier())
	begin := toSSMMM(frag.Begin())
	end := toSSMMM(frag.End())

	sb.WriteString(indent)
	sb.WriteString("<fragment")
	writeAttr(sb, "id", id)
	writeAttr(sb, "begin", begin)
	writeAttr(sb, "end", end)
	sb.WriteString(">\n")

	// Write text lines.
	lines := fragmentLines(frag)
	for _, l := range lines {
		sb.WriteString(indent)
		sb.WriteString("  <line>")
		sb.WriteString(xmlEscape(l))
		sb.WriteString("</line>\n")
	}

	// Write children or self-closing <children/>.
	children := node.ChildrenNotEmpty()
	if len(children) == 0 {
		sb.WriteString(indent)
		sb.WriteString("  <children/>\n")
	} else {
		sb.WriteString(indent)
		sb.WriteString("  <children>\n")
		for _, child := range children {
			writeXMLNode(sb, child, indent+"    ")
		}
		sb.WriteString(indent)
		sb.WriteString("  </children>\n")
	}

	sb.WriteString(indent)
	sb.WriteString("</fragment>\n")
}

// fragmentLines returns the text lines of a SyncMapFragment.
func fragmentLines(frag *syncmap.SyncMapFragment) []string {
	if lines := frag.Lines(); len(lines) > 0 {
		return lines
	}
	t := frag.Text()
	if t == "" {
		return nil
	}
	return []string{t}
}

// parseXML reads the aeneas XML format into sm (flat, top-level fragments only).
func parseXML(input string, sm *syncmap.SyncMap) error {
	type xmlLine struct {
		Text string `xml:",chardata"`
	}
	type xmlFragment struct {
		ID       string    `xml:"id,attr"`
		Begin    string    `xml:"begin,attr"`
		End      string    `xml:"end,attr"`
		Lines    []xmlLine `xml:"line"`
	}
	type xmlMap struct {
		Fragments []xmlFragment `xml:"fragment"`
	}

	var m xmlMap
	if err := xml.Unmarshal([]byte(input), &m); err != nil {
		return fmt.Errorf("format: xml: %w", err)
	}
	for _, xf := range m.Fragments {
		begin, err := parseSSMMM(xf.Begin)
		if err != nil {
			return fmt.Errorf("format: xml: bad begin %q: %w", xf.Begin, err)
		}
		end, err := parseSSMMM(xf.End)
		if err != nil {
			return fmt.Errorf("format: xml: bad end %q: %w", xf.End, err)
		}
		var lines []string
		for _, l := range xf.Lines {
			lines = append(lines, l.Text)
		}
		tf := &text.Fragment{
			Identifier: xf.ID,
			Lines:      lines,
		}
		frag := syncmap.NewSyncMapFragment(tf, begin, end, syncmap.Regular)
		sm.Add(frag)
	}
	return nil
}

// ── XML Legacy format ────────────────────────────────────────────────────────

// formatXMLLegacy serialises sm to the legacy aeneas XML format. The
// legacy schema is intentionally flat; hierarchical sync maps get a
// row per leaf fragment.
func formatXMLLegacy(sm *syncmap.SyncMap) string {
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" ?>\n")
	sb.WriteString("<map>\n")
	for _, frag := range sm.Leaves() {
		sb.WriteString(" <fragment>\n")
		sb.WriteString("  <identifier>")
		sb.WriteString(xmlEscape(frag.Identifier()))
		sb.WriteString("</identifier>\n")
		sb.WriteString("  <start>")
		sb.WriteString(toSSMMM(frag.Begin()))
		sb.WriteString("</start>\n")
		sb.WriteString("  <end>")
		sb.WriteString(toSSMMM(frag.End()))
		sb.WriteString("</end>\n")
		sb.WriteString(" </fragment>\n")
	}
	sb.WriteString("</map>")
	return sb.String()
}

// parseXMLLegacy reads the legacy XML format into sm.
func parseXMLLegacy(input string, sm *syncmap.SyncMap) error {
	type xmlLegacyFrag struct {
		Identifier string `xml:"identifier"`
		Start      string `xml:"start"`
		End        string `xml:"end"`
	}
	type xmlLegacyMap struct {
		Fragments []xmlLegacyFrag `xml:"fragment"`
	}
	var m xmlLegacyMap
	if err := xml.Unmarshal([]byte(input), &m); err != nil {
		return fmt.Errorf("format: xml_legacy: %w", err)
	}
	for _, xf := range m.Fragments {
		begin, err := parseSSMMM(xf.Start)
		if err != nil {
			return fmt.Errorf("format: xml_legacy: bad start %q: %w", xf.Start, err)
		}
		end, err := parseSSMMM(xf.End)
		if err != nil {
			return fmt.Errorf("format: xml_legacy: bad end %q: %w", xf.End, err)
		}
		tf := &text.Fragment{
			Identifier: xf.Identifier,
			Lines:      []string{},
		}
		frag := syncmap.NewSyncMapFragment(tf, begin, end, syncmap.Regular)
		sm.Add(frag)
	}
	return nil
}

// ── SMIL format ──────────────────────────────────────────────────────────────

// formatSMIL serialises sm to EPUB3 SMIL format.
// human selects HH:MM:SS.mmm time format when true (smilh); false = SS.mmm (smilm).
func formatSMIL(sm *syncmap.SyncMap, params SMILParams, human bool) string {
	timeFn := toSSMMM
	if human {
		timeFn = toHHMMSSMMM
	}

	var sb strings.Builder
	sb.WriteString(`<smil xmlns:epub="http://www.idpf.org/2007/ops" xmlns="http://www.w3.org/ns/SMIL" version="3.0">`)
	sb.WriteByte('\n')
	sb.WriteString("  <body>\n")
	pageRef := xmlEscape(params.PageRef)
	audioRef := xmlEscape(params.AudioRef)
	sb.WriteString("    <seq")
	writeAttr(&sb, "id", "seq000001")
	writeAttr(&sb, "epub:textref", pageRef)
	sb.WriteString(">\n")

	// Emit one <par> per aligned span; mirrors what an EPUB Media Overlay
	// expects (one media-text pair per playable cue, not per paragraph).
	for i, frag := range sm.Leaves() {
		sb.WriteString("      <par")
		writeParID(&sb, " id=", i+1)
		sb.WriteString(">\n")
		sb.WriteString("        <text")
		sb.WriteString(` src="`)
		sb.WriteString(pageRef)
		sb.WriteByte('#')
		sb.WriteString(xmlEscape(frag.Identifier()))
		sb.WriteString("\"/>\n")
		sb.WriteString("        <audio")
		writeAttr(&sb, "src", audioRef)
		writeAttr(&sb, "clipBegin", timeFn(frag.Begin()))
		writeAttr(&sb, "clipEnd", timeFn(frag.End()))
		sb.WriteString("/>\n")
		sb.WriteString("      </par>\n")
	}

	sb.WriteString("    </seq>\n")
	sb.WriteString("  </body>\n")
	sb.WriteString("</smil>")
	return sb.String()
}

// parseSMIL reads a SMIL file into sm.
func parseSMIL(input string, sm *syncmap.SyncMap) error {
	type smilAudio struct {
		Src        string `xml:"src,attr"`
		ClipBegin  string `xml:"clipBegin,attr"`
		ClipEnd    string `xml:"clipEnd,attr"`
	}
	type smilText struct {
		Src string `xml:"src,attr"`
	}
	type smilPar struct {
		ID    string    `xml:"id,attr"`
		Text  smilText  `xml:"text"`
		Audio smilAudio `xml:"audio"`
	}
	type smilSeq struct {
		ID       string    `xml:"id,attr"`
		TextRef  string    `xml:"textref,attr"`
		Pars     []smilPar `xml:"par"`
	}
	type smilBody struct {
		Seq smilSeq `xml:"seq"`
	}
	type smilDoc struct {
		Body smilBody `xml:"body"`
	}

	var doc smilDoc
	if err := xml.Unmarshal([]byte(input), &doc); err != nil {
		return fmt.Errorf("format: smil: %w", err)
	}

	// Try both HH:MM:SS.mmm and SS.mmm parsing.
	parseFn := func(s string) (timing.TimeValue, error) {
		if strings.Contains(s, ":") {
			return parseHHMMSSMMM(s)
		}
		return parseSSMMM(s)
	}

	for _, par := range doc.Body.Seq.Pars {
		begin, err := parseFn(par.Audio.ClipBegin)
		if err != nil {
			return fmt.Errorf("format: smil: bad clipBegin %q: %w", par.Audio.ClipBegin, err)
		}
		end, err := parseFn(par.Audio.ClipEnd)
		if err != nil {
			return fmt.Errorf("format: smil: bad clipEnd %q: %w", par.Audio.ClipEnd, err)
		}
		// Extract fragment id from text src (everything after '#').
		fragID := ""
		if idx := strings.LastIndex(par.Text.Src, "#"); idx >= 0 {
			fragID = par.Text.Src[idx+1:]
		}
		tf := &text.Fragment{
			Identifier: fragID,
			Lines:      []string{},
		}
		frag := syncmap.NewSyncMapFragment(tf, begin, end, syncmap.Regular)
		sm.Add(frag)
	}
	return nil
}

// ── TTML / DFXP format ───────────────────────────────────────────────────────

// formatTTML serialises sm to TTML/DFXP format. One <p> element per
// aligned leaf — hierarchical sync maps produce word-level paragraphs.
func formatTTML(sm *syncmap.SyncMap) string {
	// Determine language from the first fragment in the tree. Fragments()
	// gives a representative root-level entry whose language is set even
	// when the leaves don't carry one redundantly.
	lang := ""
	if roots := sm.Fragments(); len(roots) > 0 {
		lang = roots[0].Language()
	}
	frags := sm.Leaves()

	var sb strings.Builder
	sb.WriteString("<?xml version='1.0' encoding='UTF-8'?>\n")
	sb.WriteString(`<tt xmlns="http://www.w3.org/ns/ttml"`)
	writeAttr(&sb, "xml:lang", xmlEscape(lang))
	sb.WriteString(">\n")
	sb.WriteString("  <body>\n")
	sb.WriteString("    <div>\n")

	for _, frag := range frags {
		lines := fragmentLines(frag)
		text := ""
		if len(lines) > 0 {
			// Escape each line and join with <br/>.
			escaped := make([]string, len(lines))
			for i, l := range lines {
				escaped[i] = xmlEscape(l)
			}
			text = strings.Join(escaped, "<br/>")
		}
		sb.WriteString("      <p")
		writeAttr(&sb, "xml:id", xmlEscape(frag.Identifier()))
		writeAttr(&sb, "begin", toTTML(frag.Begin()))
		writeAttr(&sb, "end", toTTML(frag.End()))
		sb.WriteByte('>')
		sb.WriteString(text)
		sb.WriteString("</p>\n")
	}

	sb.WriteString("    </div>\n")
	sb.WriteString("  </body>\n")
	sb.WriteString("</tt>")
	return sb.String()
}

// parseTTML reads a TTML/DFXP file into sm.
func parseTTML(input string, sm *syncmap.SyncMap) error {
	type ttmlP struct {
		ID    string `xml:"id,attr"`
		Begin string `xml:"begin,attr"`
		End   string `xml:"end,attr"`
		Text  string `xml:",chardata"`
		Inner string `xml:",innerxml"`
	}
	type ttmlDiv struct {
		Paragraphs []ttmlP `xml:"p"`
	}
	type ttmlBody struct {
		Div ttmlDiv `xml:"div"`
	}
	type ttmlDoc struct {
		Lang string   `xml:"lang,attr"`
		Body ttmlBody `xml:"body"`
	}

	var doc ttmlDoc
	if err := xml.Unmarshal([]byte(input), &doc); err != nil {
		return fmt.Errorf("format: ttml: %w", err)
	}

	parseFn := func(s string) (timing.TimeValue, error) {
		s = strings.TrimSuffix(s, "s")
		return parseSSMMM(s)
	}

	for _, p := range doc.Body.Div.Paragraphs {
		begin, err := parseFn(p.Begin)
		if err != nil {
			return fmt.Errorf("format: ttml: bad begin %q: %w", p.Begin, err)
		}
		end, err := parseFn(p.End)
		if err != nil {
			return fmt.Errorf("format: ttml: bad end %q: %w", p.End, err)
		}
		// Reconstruct lines: split on <br/>.
		rawLines := strings.Split(p.Inner, "<br/>")
		var lines []string
		for _, l := range rawLines {
			// Strip any remaining XML tags.
			dec := xml.NewDecoder(strings.NewReader("<x>" + l + "</x>"))
			var buf strings.Builder
			for {
				tok, err := dec.Token()
				if err != nil {
					break
				}
				if cd, ok := tok.(xml.CharData); ok {
					buf.Write(cd)
				}
			}
			lines = append(lines, buf.String())
		}
		tf := &text.Fragment{
			Identifier: p.ID,
			Language:   doc.Lang,
			Lines:      lines,
		}
		frag := syncmap.NewSyncMapFragment(tf, begin, end, syncmap.Regular)
		sm.Add(frag)
	}
	return nil
}

// ── EAF format ───────────────────────────────────────────────────────────────

// eafDate is the deterministic date used in EAF output for reproducible tests.
const eafDate = "2000-01-01T00:00:00+00:00"

// formatEAF serialises sm to ELAN EAF format.
func formatEAF(sm *syncmap.SyncMap, params EAFParams) string {
	var sb strings.Builder

	sb.WriteString("<?xml version='1.0' encoding='UTF-8'?>\n")
	sb.WriteString(`<ANNOTATION_DOCUMENT xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="http://www.mpi.nl/tools/elan/EAFv2.8.xsd" AUTHOR="aeneas"`)
	writeAttr(&sb, "DATE", eafDate)
	sb.WriteString(` FORMAT="2.8" VERSION="2.8">` + "\n")
	sb.WriteString("  <HEADER MEDIA_FILE=\"\" TIME_UNITS=\"milliseconds\"/>\n")
	sb.WriteString("  <TIME_ORDER>\n")

	// One annotation per aligned leaf — ELAN typically wants the
	// deepest tier (e.g. word level), not paragraph rollups.
	frags := sm.Leaves()
	for _, frag := range frags {
		id := frag.Identifier()
		beginMS := int64(frag.Begin().Float64()*1000 + 0.5)
		endMS := int64(frag.End().Float64()*1000 + 0.5)
		// <TIME_SLOT TIME_SLOT_ID="tsIDb" TIME_VALUE="N"/>
		sb.WriteString("    <TIME_SLOT")
		writeAttr(&sb, "TIME_SLOT_ID", "ts"+id+"b")
		writeIntAttr(&sb, "TIME_VALUE", beginMS)
		sb.WriteString("/>\n")
		sb.WriteString("    <TIME_SLOT")
		writeAttr(&sb, "TIME_SLOT_ID", "ts"+id+"e")
		writeIntAttr(&sb, "TIME_VALUE", endMS)
		sb.WriteString("/>\n")
	}

	sb.WriteString("  </TIME_ORDER>\n")
	sb.WriteString("  <TIER LINGUISTIC_TYPE_REF=\"utterance\" TIER_ID=\"tier1\">\n")

	for _, frag := range frags {
		id := frag.Identifier()
		txt := frag.Text()
		sb.WriteString("    <ANNOTATION>\n")
		sb.WriteString("      <ALIGNABLE_ANNOTATION")
		writeAttr(&sb, "ANNOTATION_ID", id)
		writeAttr(&sb, "TIME_SLOT_REF1", "ts"+id+"b")
		writeAttr(&sb, "TIME_SLOT_REF2", "ts"+id+"e")
		sb.WriteString(">\n")
		sb.WriteString("        <ANNOTATION_VALUE>")
		sb.WriteString(xmlEscape(txt))
		sb.WriteString("</ANNOTATION_VALUE>\n")
		sb.WriteString("      </ALIGNABLE_ANNOTATION>\n")
		sb.WriteString("    </ANNOTATION>\n")
	}

	sb.WriteString("  </TIER>\n")
	sb.WriteString("  <LINGUISTIC_TYPE LINGUISTIC_TYPE_ID=\"utterance\" TIME_ALIGNABLE=\"true\"/>\n")
	sb.WriteString("</ANNOTATION_DOCUMENT>")
	return sb.String()
}

// parseEAF reads an EAF file into sm.
func parseEAF(input string, sm *syncmap.SyncMap) error {
	type eafTimeSlot struct {
		ID    string `xml:"TIME_SLOT_ID,attr"`
		Value string `xml:"TIME_VALUE,attr"`
	}
	type eafTimeOrder struct {
		Slots []eafTimeSlot `xml:"TIME_SLOT"`
	}
	type eafAnnotValue struct {
		Text string `xml:",chardata"`
	}
	type eafAlignableAnnot struct {
		ID         string        `xml:"ANNOTATION_ID,attr"`
		SlotRef1   string        `xml:"TIME_SLOT_REF1,attr"`
		SlotRef2   string        `xml:"TIME_SLOT_REF2,attr"`
		Value      eafAnnotValue `xml:"ANNOTATION_VALUE"`
	}
	type eafAnnotation struct {
		Alignable eafAlignableAnnot `xml:"ALIGNABLE_ANNOTATION"`
	}
	type eafTier struct {
		Annotations []eafAnnotation `xml:"ANNOTATION"`
	}
	type eafDoc struct {
		TimeOrder eafTimeOrder `xml:"TIME_ORDER"`
		Tier      eafTier      `xml:"TIER"`
	}

	var doc eafDoc
	if err := xml.Unmarshal([]byte(input), &doc); err != nil {
		return fmt.Errorf("format: eaf: %w", err)
	}

	// Build time slot id → milliseconds map.
	tsMap := make(map[string]int64, len(doc.TimeOrder.Slots)*2)
	for _, ts := range doc.TimeOrder.Slots {
		var ms int64
		fmt.Sscan(ts.Value, &ms)
		tsMap[ts.ID] = ms
	}

	for _, ann := range doc.Tier.Annotations {
		a := ann.Alignable
		beginMS, ok1 := tsMap[a.SlotRef1]
		endMS, ok2 := tsMap[a.SlotRef2]
		if !ok1 || !ok2 {
			return fmt.Errorf("format: eaf: missing time slot for annotation %q", a.ID)
		}
		// Convert milliseconds to seconds as rational.
		begin := timing.FromInt64(beginMS, 1000)
		end := timing.FromInt64(endMS, 1000)
		tf := &text.Fragment{
			Identifier: a.ID,
			Lines:      []string{a.Value.Text},
		}
		frag := syncmap.NewSyncMapFragment(tf, begin, end, syncmap.Regular)
		sm.Add(frag)
	}
	return nil
}
