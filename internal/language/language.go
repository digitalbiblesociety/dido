// Package language provides ISO 639-3 language codes and lookup
// helpers backed by the official SIL registry (iso-639-3.tab).
//
// The registry is loaded once at package init from an embedded copy of
// the TSV; see iso639.go for the data layout and refresh procedure.
package language

import "sort"

// Language is an ISO 639-3 language code string.
type Language = string

// IsAllowed reports whether code is a registered ISO 639-3 code.
func IsAllowed(code Language) bool {
	_, ok := registry[code]
	return ok
}

// HumanName returns the SIL reference name for the given code, or the
// code itself if it is not registered.
func HumanName(code Language) string {
	if v, ok := registry[code]; ok {
		return v.RefName
	}
	return code
}

// AllowedValues returns every registered 639-3 code, sorted.
// Computed once at init.
var AllowedValues = func() []string {
	codes := make([]string, 0, len(registry))
	for k := range registry {
		codes = append(codes, k)
	}
	sort.Strings(codes)
	return codes
}()

// CodeToHuman is a legacy view of the registry as `code → name`. New
// code should prefer HumanName / Lookup, which avoids materialising
// the whole map.
var CodeToHuman = func() map[Language]string {
	m := make(map[Language]string, len(registry))
	for k, v := range registry {
		m[k] = v.RefName
	}
	return m
}()
