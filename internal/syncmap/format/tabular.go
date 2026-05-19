package format

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/timing"
)

// tabularConfig describes how a tabular format encodes its fields.
type tabularConfig struct {
	delimiter  string
	textDelim  string   // "" means no quoting
	human      bool     // use HH:MM:SS.mmm instead of SS.mmm
	hasID      bool
	hasText    bool
	fieldOrder []string // order of: "begin", "end", "id", "text"
}

// formatTabular serialises sm into a flat text table using cfg.
func formatTabular(sm *syncmap.SyncMap, cfg tabularConfig) string {
	timeFn := toSSMMM
	if cfg.human {
		timeFn = toHHMMSSMMM
	}

	var sb strings.Builder
	// Tabular formats emit one row per aligned span; walk the leaves so
	// hierarchical sync maps don't lose their innermost partition.
	for _, frag := range sm.Leaves() {
		var fields []string
		for _, f := range cfg.fieldOrder {
			switch f {
			case "begin":
				fields = append(fields, timeFn(frag.Begin()))
			case "end":
				fields = append(fields, timeFn(frag.End()))
			case "id":
				fields = append(fields, frag.Identifier())
			case "text":
				t := frag.Text()
				if cfg.textDelim != "" {
					t = cfg.textDelim + t + cfg.textDelim
				}
				fields = append(fields, t)
			}
		}
		sb.WriteString(strings.Join(fields, cfg.delimiter))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// parseTabular reads a tabular text into sm.
func parseTabular(input string, sm *syncmap.SyncMap, cfg tabularConfig) error {
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			continue
		}

		parts := strings.Split(line, cfg.delimiter)
		if len(parts) < len(cfg.fieldOrder) {
			return fmt.Errorf("format: tabular: too few fields in line %q", line)
		}

		var (
			beginStr, endStr, id, txt string
		)
		for i, f := range cfg.fieldOrder {
			val := parts[i]
			switch f {
			case "begin":
				beginStr = val
			case "end":
				endStr = val
			case "id":
				id = val
			case "text":
				if cfg.textDelim != "" {
					val = strings.Trim(val, cfg.textDelim)
				}
				txt = val
			}
		}

		var parseFn func(string) (timing.TimeValue, error)
		if cfg.human {
			parseFn = parseHHMMSSMMM
		} else {
			parseFn = parseSSMMM
		}

		begin, err := parseFn(beginStr)
		if err != nil {
			return fmt.Errorf("format: tabular: bad begin %q: %w", beginStr, err)
		}
		end, err := parseFn(endStr)
		if err != nil {
			return fmt.Errorf("format: tabular: bad end %q: %w", endStr, err)
		}

		tf := &text.Fragment{
			Identifier: id,
			Lines:      []string{txt},
		}
		frag := syncmap.NewSyncMapFragment(tf, begin, end, syncmap.Regular)
		sm.Add(frag)
	}
	return scanner.Err()
}

// ── exported dispatch helpers ────────────────────────────────────────────────

// formatCSV formats sm as CSV (with identifier and text columns).
// human selects HH:MM:SS.mmm time format when true.
func formatCSV(sm *syncmap.SyncMap, human bool) string {
	return formatTabular(sm, tabularConfig{
		delimiter:  ",",
		textDelim:  "\"",
		human:      human,
		hasID:      true,
		hasText:    true,
		fieldOrder: []string{"id", "begin", "end", "text"},
	})
}

// formatTSV formats sm as TSV (tab-separated, no text column).
func formatTSV(sm *syncmap.SyncMap, human bool) string {
	return formatTabular(sm, tabularConfig{
		delimiter:  "\t",
		textDelim:  "",
		human:      human,
		hasID:      true,
		hasText:    false,
		fieldOrder: []string{"begin", "end", "id"},
	})
}

// formatSSV formats sm as SSV (space-separated with text column).
func formatSSV(sm *syncmap.SyncMap, human bool) string {
	return formatTabular(sm, tabularConfig{
		delimiter:  " ",
		textDelim:  "\"",
		human:      human,
		hasID:      true,
		hasText:    true,
		fieldOrder: []string{"begin", "end", "id", "text"},
	})
}

// formatTXT formats sm as TXT (space-separated with text column, id first).
func formatTXT(sm *syncmap.SyncMap, human bool) string {
	return formatTabular(sm, tabularConfig{
		delimiter:  " ",
		textDelim:  "\"",
		human:      human,
		hasID:      true,
		hasText:    true,
		fieldOrder: []string{"id", "begin", "end", "text"},
	})
}

// formatAUD formats sm as Audacity label track (tab-separated, no id).
func formatAUD(sm *syncmap.SyncMap, human bool) string {
	return formatTabular(sm, tabularConfig{
		delimiter:  "\t",
		textDelim:  "",
		human:      human,
		hasID:      false,
		hasText:    true,
		fieldOrder: []string{"begin", "end", "text"},
	})
}

// formatTAB formats sm as TAB (same as TSV machine).
func formatTAB(sm *syncmap.SyncMap, human bool) string {
	return formatTSV(sm, human)
}
