package format

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/digitalbiblesociety/dido/internal/timing"
)

// toSSMMM formats a TimeValue as "SS.mmm".
func toSSMMM(t timing.TimeValue) string { return t.String() }

// toHHMMSSMMM formats a TimeValue as "HH:MM:SS.mmm".
func toHHMMSSMMM(t timing.TimeValue) string { return t.FormatHMSm() }

// toSRT formats a TimeValue as "HH:MM:SS,mmm" (SRT comma style).
func toSRT(t timing.TimeValue) string { return t.FormatHMSc() }

// toTTML formats a TimeValue as "SS.mmms", e.g. "12.345s".
func toTTML(t timing.TimeValue) string { return t.String() + "s" }

// toTextGrid formats a TimeValue the way Praat / the Python `tgt` library
// does: shortest fixed-point decimal with at least one fractional digit
// (e.g. 0 → "0.0", 18.6 → "18.6", 53.24 → "53.24"). Trailing zeros in
// the fractional part are stripped; integer values always render with
// ".0" appended.
func toTextGrid(t timing.TimeValue) string {
	v := t.Float64()
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.ContainsRune(s, '.') {
		s += ".0"
	}
	return s
}

// parseSSMMM parses a decimal seconds string like "12.345" into a TimeValue.
func parseSSMMM(s string) (timing.TimeValue, error) {
	return timing.ParseTimeValue(s)
}

// parseHHMMSSMMM parses "HH:MM:SS.mmm" or "HH:MM:SS,mmm" into a TimeValue
// expressed as total seconds.
func parseHHMMSSMMM(s string) (timing.TimeValue, error) {
	// Normalise comma to dot for the sub-second part.
	s = strings.Replace(s, ",", ".", 1)

	// Split on ":"
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return timing.Zero, fmt.Errorf("format: cannot parse time %q (expected HH:MM:SS.mmm)", s)
	}

	h, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return timing.Zero, fmt.Errorf("format: bad hours in %q: %w", s, err)
	}
	m, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return timing.Zero, fmt.Errorf("format: bad minutes in %q: %w", s, err)
	}

	// parts[2] may be "SS.mmm" — pass directly to ParseTimeValue for seconds.
	sec, err := timing.ParseTimeValue(parts[2])
	if err != nil {
		return timing.Zero, fmt.Errorf("format: bad seconds in %q: %w", s, err)
	}

	totalSec := timing.FromInt64(h*3600+m*60, 1).Add(sec)
	return totalSec, nil
}
