// Package format serialises and deserialises SyncMap objects to/from various
// output formats.  It is a Go port of the aeneas smfg*.py family.
package format

import (
	"fmt"
	"sort"
	"strings"

	"github.com/digitalbiblesociety/dido/internal/syncmap"
)

// Format is a sync-map output-format identifier.
//
// Many tabular and SMIL variants share an implementation but ship under
// three constant names — the "default" form (e.g. FormatCSV), a "human"
// form with HH:MM:SS.mmm timestamps (FormatCSVH), and a "machine" form
// that's currently identical to the default (FormatCSVM). The grouping
// below documents which constants are aliases:
//
//	default = machine
//	------- = -------
//	FormatAUD  ≡ FormatAUDM
//	FormatCSV  ≡ FormatCSVM
//	FormatSSV  ≡ FormatSSVM
//	FormatTSV  ≡ FormatTSVM
//	FormatTXT  ≡ FormatTXTM
//	FormatTAB  ≡ FormatTSV (tab is a no-id alias of TSV)
//	FormatSMIL ≡ FormatSMILH (both emit HH:MM:SS.mmm)
//
// The "*H" variants (human-readable HH:MM:SS.mmm timestamps) are NOT
// aliases — they produce different output. The "*M" aliases exist for
// CLI parity with the Python tool's flag set; new Go code should prefer
// the unsuffixed form.
//
// FormatDFXP is an alias of FormatTTML (DFXP is the SMPTE-TT subset of
// TTML and is structurally identical for our purposes).
type Format string

const (
	// AUD family — Audacity label-track format (tab-separated, no id).
	FormatAUD  Format = "aud"  // default = machine
	FormatAUDM Format = "audm" // alias of FormatAUD
	FormatAUDH Format = "audh" // human (HH:MM:SS.mmm)

	// CSV family — comma-separated.
	FormatCSV  Format = "csv"  // default = machine
	FormatCSVM Format = "csvm" // alias of FormatCSV
	FormatCSVH Format = "csvh" // human (HH:MM:SS.mmm)

	// SSV family — space-separated, text in quotes.
	FormatSSV  Format = "ssv"  // default = machine
	FormatSSVM Format = "ssvm" // alias of FormatSSV
	FormatSSVH Format = "ssvh" // human

	// TSV family — tab-separated, id-text.
	FormatTSV  Format = "tsv"  // default = machine
	FormatTSVM Format = "tsvm" // alias of FormatTSV
	FormatTSVH Format = "tsvh" // human
	FormatTAB  Format = "tab"  // alias of FormatTSV (no-id mode kept for compat)

	// TXT family — space-separated, id-first-text-quoted.
	FormatTXT  Format = "txt"  // default = machine
	FormatTXTM Format = "txtm" // alias of FormatTXT
	FormatTXTH Format = "txth" // human

	// JSON / RBSE.
	FormatJSON Format = "json"
	FormatRBSE Format = "rbse"

	// XML family.
	FormatXML       Format = "xml"
	FormatXMLLegacy Format = "xml_legacy"

	// TTML / DFXP — same on-disk schema; DFXP is the SMPTE-TT subset name.
	FormatTTML Format = "ttml"
	FormatDFXP Format = "dfxp" // alias of FormatTTML

	// SMIL family — EPUB Media Overlay.
	FormatSMIL  Format = "smil"  // default = human (HH:MM:SS.mmm)
	FormatSMILH Format = "smilh" // alias of FormatSMIL
	FormatSMILM Format = "smilm" // machine (SS.mmm)

	// EAF — ELAN annotation format.
	FormatEAF Format = "eaf"

	// Subtitle families.
	FormatSRT Format = "srt"
	FormatVTT Format = "vtt"
	FormatSUB Format = "sub" // SubViewer; uses [br] between lines
	FormatSBV Format = "sbv" // SubViewer variant; uses \n between lines

	// Praat TextGrid family.
	FormatTextGrid      Format = "textgrid"       // long form (alias of FormatTextGridLong)
	FormatTextGridLong  Format = "textgrid_long"  // verbose form with xmin/xmax labels
	FormatTextGridShort Format = "textgrid_short" // compact form (bare numbers)
)

// SMILParams holds parameters needed for SMIL output.
type SMILParams struct {
	PageRef  string // text file reference, e.g. "sonnet001.xhtml"
	AudioRef string // audio file reference, e.g. "sonnet001.mp3"
}

// EAFParams holds parameters for EAF output.
type EAFParams struct {
	AudioRef string // optional
}

// Write serialises sm to the requested format and returns the result string.
// smilParams is only used for SMIL variants; eafParams is only used for EAF.
// Returns ("", error) for unknown formats.
func Write(sm *syncmap.SyncMap, f Format, smilParams SMILParams, eafParams EAFParams) (string, error) {
	switch f {
	// ── tabular variants ────────────────────────────────────────────────────
	case FormatCSV:
		return formatCSV(sm, false), nil
	case FormatCSVH:
		return formatCSV(sm, true), nil
	case FormatCSVM:
		return formatCSV(sm, false), nil

	case FormatTSV:
		return formatTSV(sm, false), nil
	case FormatTSVH:
		return formatTSV(sm, true), nil
	case FormatTSVM:
		return formatTSV(sm, false), nil

	case FormatSSV:
		return formatSSV(sm, false), nil
	case FormatSSVH:
		return formatSSV(sm, true), nil
	case FormatSSVM:
		return formatSSV(sm, false), nil

	case FormatTXT:
		return formatTXT(sm, false), nil
	case FormatTXTH:
		return formatTXT(sm, true), nil
	case FormatTXTM:
		return formatTXT(sm, false), nil

	case FormatAUD:
		return formatAUD(sm, false), nil
	case FormatAUDH:
		return formatAUD(sm, true), nil
	case FormatAUDM:
		return formatAUD(sm, false), nil

	case FormatTAB:
		return formatTAB(sm, false), nil

	// ── subtitle variants ───────────────────────────────────────────────────
	case FormatSRT:
		return formatSRT(sm), nil
	case FormatVTT:
		return formatVTT(sm), nil
	case FormatSUB:
		return formatSUB(sm), nil
	case FormatSBV:
		return formatSBV(sm), nil

	// ── JSON / RBSE ─────────────────────────────────────────────────────────
	case FormatJSON:
		return formatJSON(sm), nil
	case FormatRBSE:
		return formatRBSE(sm), nil

	// ── XML family ──────────────────────────────────────────────────────────
	case FormatXML:
		return formatXML(sm), nil
	case FormatXMLLegacy:
		return formatXMLLegacy(sm), nil

	case FormatSMIL, FormatSMILH:
		return formatSMIL(sm, smilParams, true), nil
	case FormatSMILM:
		return formatSMIL(sm, smilParams, false), nil

	case FormatTTML, FormatDFXP:
		return formatTTML(sm), nil

	case FormatEAF:
		return formatEAF(sm, eafParams), nil

	case FormatTextGrid, FormatTextGridLong:
		return formatTextGrid(sm, true), nil
	case FormatTextGridShort:
		return formatTextGrid(sm, false), nil

	default:
		return "", fmt.Errorf("unsupported format %q — valid formats are: %s",
			string(f), strings.Join(supportedFormats(), ", "))
	}
}

// supportedFormats returns the sorted list of Format constants Write
// understands. The list is the source of truth for `--help` and friendly
// error messages.
func supportedFormats() []string {
	all := []Format{
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
		FormatTextGrid, FormatTextGridLong, FormatTextGridShort,
		FormatTSV, FormatTSVH, FormatTSVM,
		FormatTTML,
		FormatTXT, FormatTXTH, FormatTXTM,
		FormatVTT,
		FormatXML,
		FormatXMLLegacy,
	}
	out := make([]string, len(all))
	for i, f := range all {
		out[i] = string(f)
	}
	sort.Strings(out)
	return out
}

// SupportedFormats returns the sorted list of Format constants Write
// understands. Exported for `--help` text and integration tests.
func SupportedFormats() []string { return supportedFormats() }
