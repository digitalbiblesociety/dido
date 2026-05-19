package format

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/timing"
)

// subtitlesConfig describes how a subtitle format encodes its blocks.
type subtitlesConfig struct {
	header          string // "" means no header (e.g. "WEBVTT")
	footer          string // "" means no footer
	hasID           bool
	optionalID      bool
	timeSep         string // e.g. " --> "
	lineBreakSymbol string // "\n" or "[br]"
	timeFmt         func(timing.TimeValue) string
	parseTimeFn     func(string) (timing.TimeValue, error)
}

// formatSubtitles serialises sm to a subtitle-style text format.
func formatSubtitles(sm *syncmap.SyncMap, cfg subtitlesConfig) string {
	var sb strings.Builder

	if cfg.header != "" {
		sb.WriteString(cfg.header)
		sb.WriteByte('\n')
		sb.WriteByte('\n')
	}

	// Walk the deepest fragments so hierarchical sync maps (e.g. mplain
	// input → paragraph → sentence → phrase → word tree) still emit one
	// cue per aligned span. For flat sync maps Leaves() == Fragments().
	frags := sm.Leaves()
	for i, frag := range frags {
		idx := i + 1 // 1-based

		if cfg.hasID || cfg.optionalID {
			sb.WriteString(strconv.Itoa(idx))
			sb.WriteByte('\n')
		}

		// Time line
		sb.WriteString(cfg.timeFmt(frag.Begin()))
		sb.WriteString(cfg.timeSep)
		sb.WriteString(cfg.timeFmt(frag.End()))
		sb.WriteByte('\n')

		// Text lines — join by lineBreakSymbol
		// frag.Text() returns lines joined by spaces; we want per-line output.
		// Reconstruct line-by-line from the underlying TextFragment if possible.
		lines := linesOf(frag)
		if cfg.lineBreakSymbol == "\n" {
			for _, l := range lines {
				sb.WriteString(l)
				sb.WriteByte('\n')
			}
		} else {
			sb.WriteString(strings.Join(lines, cfg.lineBreakSymbol))
			sb.WriteByte('\n')
		}

		sb.WriteByte('\n')
	}

	if cfg.footer != "" {
		sb.WriteString(cfg.footer)
		sb.WriteByte('\n')
	}

	return sb.String()
}

// linesOf extracts the Lines slice from a SyncMapFragment's TextFragment.
// Falls back to a single-element slice containing frag.Text().
func linesOf(frag *syncmap.SyncMapFragment) []string {
	if lines := frag.Lines(); len(lines) > 0 {
		return lines
	}
	t := frag.Text()
	if t == "" {
		return []string{""}
	}
	return []string{t}
}

// parseSubtitles reads a subtitle-format text into sm.
func parseSubtitles(input string, sm *syncmap.SyncMap, cfg subtitlesConfig) error {
	scanner := bufio.NewScanner(strings.NewReader(input))

	// Skip header block (everything up to and including the first blank line).
	if cfg.header != "" {
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), "\r")
			if line == "" {
				break
			}
		}
	}

	// State machine: collect blocks separated by blank lines.
	// A block is: optional index line, time line, text lines.
	type block struct {
		lines []string
	}
	var blocks []block
	var cur []string

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			if len(cur) > 0 {
				blocks = append(blocks, block{cur})
				cur = nil
			}
		} else {
			cur = append(cur, line)
		}
	}
	if len(cur) > 0 {
		blocks = append(blocks, block{cur})
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	for _, b := range blocks {
		if len(b.lines) < 2 {
			continue
		}
		lines := b.lines

		// Determine if the first line is a numeric index.
		startIdx := 0
		if cfg.hasID || cfg.optionalID {
			if _, err := strconv.Atoi(lines[0]); err == nil {
				startIdx = 1
			}
		}
		if startIdx >= len(lines) {
			continue
		}

		// Parse time line.
		timeLine := lines[startIdx]
		timeParts := strings.SplitN(timeLine, cfg.timeSep, 2)
		if len(timeParts) != 2 {
			return fmt.Errorf("format: subtitles: bad time line %q", timeLine)
		}
		parseFn := cfg.parseTimeFn
		if parseFn == nil {
			parseFn = parseHHMMSSMMM
		}
		begin, err := parseFn(strings.TrimSpace(timeParts[0]))
		if err != nil {
			return fmt.Errorf("format: subtitles: bad begin %q: %w", timeParts[0], err)
		}
		end, err := parseFn(strings.TrimSpace(timeParts[1]))
		if err != nil {
			return fmt.Errorf("format: subtitles: bad end %q: %w", timeParts[1], err)
		}

		// Remaining lines are text.
		textLines := lines[startIdx+1:]
		var flatLines []string
		for _, tl := range textLines {
			if cfg.lineBreakSymbol != "\n" {
				// [br]-joined: split on the symbol.
				parts := strings.Split(tl, cfg.lineBreakSymbol)
				flatLines = append(flatLines, parts...)
			} else {
				flatLines = append(flatLines, tl)
			}
		}

		tf := &text.Fragment{
			Lines: flatLines,
		}
		frag := syncmap.NewSyncMapFragment(tf, begin, end, syncmap.Regular)
		sm.Add(frag)
	}
	return nil
}

// ── exported dispatch helpers ────────────────────────────────────────────────

func formatSRT(sm *syncmap.SyncMap) string {
	return formatSubtitles(sm, subtitlesConfig{
		header:          "",
		hasID:           true,
		timeSep:         " --> ",
		lineBreakSymbol: "\n",
		timeFmt:         toSRT,
		parseTimeFn:     parseHHMMSSMMM,
	})
}

func formatVTT(sm *syncmap.SyncMap) string {
	return formatSubtitles(sm, subtitlesConfig{
		header:          "WEBVTT",
		hasID:           false,
		optionalID:      true,
		timeSep:         " --> ",
		lineBreakSymbol: "\n",
		timeFmt:         toHHMMSSMMM,
		parseTimeFn:     parseHHMMSSMMM,
	})
}

func formatSUB(sm *syncmap.SyncMap) string {
	return formatSubtitles(sm, subtitlesConfig{
		header:          "[SUBTITLE]",
		footer:          "[END SUBTITLE]",
		hasID:           false,
		timeSep:         ",",
		lineBreakSymbol: "[br]",
		timeFmt:         toHHMMSSMMM,
		parseTimeFn:     parseHHMMSSMMM,
	})
}

func formatSBV(sm *syncmap.SyncMap) string {
	// SBV is a variant of the SubViewer (SUB) format, sharing the
	// "[SUBTITLE]"/"[END SUBTITLE]" envelope. The only practical difference
	// is the inter-line separator inside a multi-line cue: SUB uses "[br]",
	// SBV uses a real newline. The Python aeneas implementation inherits the
	// header/footer from SyncMapFormatSUB; mirror that here.
	return formatSubtitles(sm, subtitlesConfig{
		header:          "[SUBTITLE]",
		footer:          "[END SUBTITLE]",
		hasID:           false,
		timeSep:         ",",
		lineBreakSymbol: "\n",
		timeFmt:         toHHMMSSMMM,
		parseTimeFn:     parseHHMMSSMMM,
	})
}
