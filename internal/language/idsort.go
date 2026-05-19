package language

import (
	"regexp"
	"sort"
	"strconv"
)

// IDSortAlgorithm identifies how to sort a list of XML id attributes.
type IDSortAlgorithm string

const (
	// IDSortLexicographic sorts ids as strings (e.g. "f020" < "f10" < "f2").
	IDSortLexicographic IDSortAlgorithm = "lexicographic"
	// IDSortNumeric sorts by the integer value embedded in the id string.
	IDSortNumeric IDSortAlgorithm = "numeric"
	// IDSortUnsorted returns ids in their original order.
	IDSortUnsorted IDSortAlgorithm = "unsorted"
)

var nonDigit = regexp.MustCompile(`[^0-9]`)

// SortIDs returns a new sorted copy of ids using the given algorithm.
// Port of IDSortingAlgorithm.sort() in idsortingalgorithm.py.
func SortIDs(ids []string, alg IDSortAlgorithm) []string {
	cp := make([]string, len(ids))
	copy(cp, ids)

	switch alg {
	case IDSortLexicographic:
		sort.Strings(cp)

	case IDSortNumeric:
		sort.SliceStable(cp, func(i, j int) bool {
			ni := extractInt(cp[i])
			nj := extractInt(cp[j])
			return ni < nj
		})
	// IDSortUnsorted: leave cp as-is
	}
	return cp
}

func extractInt(s string) int {
	digits := nonDigit.ReplaceAllString(s, "")
	if digits == "" {
		return 0
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return n
}
