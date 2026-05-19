package format

import (
	"strings"

	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/timing"
)

// formatTextGrid serialises sm to Praat TextGrid format. When `long` is
// true the verbose form is emitted (with `xmin =`, `intervals [N]:`
// labels and tab indentation); when false the short form is emitted
// (bare numbers and quoted strings, one per line). Default is long.
//
// Matches the output of the Python `tgt.io.export_to_long_textgrid` /
// `export_to_short_textgrid` helpers used by aeneas/syncmap/smftextgrid.py.
func formatTextGrid(sm *syncmap.SyncMap, long bool) string {
	// One interval per aligned leaf — hierarchical sync maps should
	// produce one TextGrid interval per word, not per paragraph.
	frags := sm.Leaves()

	xmin, xmax := timing.Zero, timing.Zero
	if len(frags) > 0 {
		xmin = frags[0].Begin()
		xmax = frags[len(frags)-1].End()
	}

	var sb strings.Builder
	sb.WriteString("File type = \"ooTextFile\"\n")
	sb.WriteString("Object class = \"TextGrid\"\n\n")

	if long {
		writeTextGridLong(&sb, frags, xmin, xmax)
	} else {
		writeTextGridShort(&sb, frags, xmin, xmax)
	}
	return sb.String()
}

func writeTextGridLong(sb *strings.Builder, frags []*syncmap.SyncMapFragment, xmin, xmax timing.TimeValue) {
	sb.WriteString("xmin = ")
	sb.WriteString(toTextGrid(xmin))
	sb.WriteByte('\n')
	sb.WriteString("xmax = ")
	sb.WriteString(toTextGrid(xmax))
	sb.WriteByte('\n')
	sb.WriteString("tiers? <exists>\n")
	sb.WriteString("size = 1\n")
	sb.WriteString("item []:\n")
	sb.WriteString("\titem [1]:\n")
	sb.WriteString("\t\tclass = \"IntervalTier\"\n")
	sb.WriteString("\t\tname = \"Token\"\n")
	sb.WriteString("\t\txmin = ")
	sb.WriteString(toTextGrid(xmin))
	sb.WriteByte('\n')
	sb.WriteString("\t\txmax = ")
	sb.WriteString(toTextGrid(xmax))
	sb.WriteByte('\n')
	sb.WriteString("\t\tintervals: size = ")
	sb.WriteString(itoa(len(frags)))
	sb.WriteByte('\n')

	for i, frag := range frags {
		sb.WriteString("\t\tintervals [")
		sb.WriteString(itoa(i + 1))
		sb.WriteString("]:\n")
		sb.WriteString("\t\t\txmin = ")
		sb.WriteString(toTextGrid(frag.Begin()))
		sb.WriteByte('\n')
		sb.WriteString("\t\t\txmax = ")
		sb.WriteString(toTextGrid(frag.End()))
		sb.WriteByte('\n')
		sb.WriteString("\t\t\ttext = \"")
		sb.WriteString(escapeTextGrid(textGridText(frag)))
		sb.WriteByte('"')
		// Last fragment has no trailing newline (matches Python output).
		if i != len(frags)-1 {
			sb.WriteByte('\n')
		}
	}
}

func writeTextGridShort(sb *strings.Builder, frags []*syncmap.SyncMapFragment, xmin, xmax timing.TimeValue) {
	sb.WriteString(toTextGrid(xmin))
	sb.WriteByte('\n')
	sb.WriteString(toTextGrid(xmax))
	sb.WriteByte('\n')
	sb.WriteString("<exists>\n")
	sb.WriteString("1\n")
	sb.WriteString("\"IntervalTier\"\n")
	sb.WriteString("\"Token\"\n")
	sb.WriteString(toTextGrid(xmin))
	sb.WriteByte('\n')
	sb.WriteString(toTextGrid(xmax))
	sb.WriteByte('\n')
	sb.WriteString(itoa(len(frags)))
	sb.WriteByte('\n')
	for i, frag := range frags {
		sb.WriteString(toTextGrid(frag.Begin()))
		sb.WriteByte('\n')
		sb.WriteString(toTextGrid(frag.End()))
		sb.WriteByte('\n')
		sb.WriteByte('"')
		sb.WriteString(escapeTextGrid(textGridText(frag)))
		sb.WriteByte('"')
		// No trailing newline after the last interval (matches Python).
		if i != len(frags)-1 {
			sb.WriteByte('\n')
		}
	}
}

// textGridText returns the fragment's text, substituting "SIL" for an
// empty string (matches the upstream behaviour for silence fragments).
func textGridText(f *syncmap.SyncMapFragment) string {
	t := f.Text()
	if t == "" {
		return "SIL"
	}
	return t
}

// escapeTextGrid escapes special characters inside a TextGrid quoted
// string. Praat uses doubled quotes ("") to represent a literal " inside
// a quoted string; we mirror that.
func escapeTextGrid(s string) string {
	return strings.ReplaceAll(s, `"`, `""`)
}

// itoa is a small helper to avoid pulling strconv into every call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
