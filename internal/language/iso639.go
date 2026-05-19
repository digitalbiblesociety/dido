// SIL ISO 639-3 registry loader. The TSV download is cached under
// `data/iso-639-3-<YYYY-MM-DD>.tab` and refreshed at most once per day
// by `go generate`. Only one dated file is kept on disk at a time so
// the embed stays small.
//
// Refresh manually with:
//
//	go generate ./internal/language/
//
// The repo's Makefile target `make build` runs this automatically.

package language

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:generate ./fetch-iso639.sh

//go:embed data/iso-639-3-*.tab
var iso639fs embed.FS

// Info is one row of the ISO 639-3 registry.
type Info struct {
	ID      string // 3-letter code (always set)
	Part2B  string // 639-2 bibliographic code (may be empty)
	Part2T  string // 639-2 terminologic code (may be empty)
	Part1   string // 639-1 two-letter code (may be empty)
	Scope   string // I (Individual), M (Macrolanguage), S (Special)
	Type    string // A Ancient, C Constructed, E Extinct, H Historical, L Living, S Special
	RefName string // canonical English reference name
	Comment string
}

// registry is the parsed ISO 639-3 dataset, keyed by 3-letter ID.
var registry = mustLoadRegistry()

func mustLoadRegistry() map[string]Info {
	entries, err := fs.Glob(iso639fs, "data/iso-639-3-*.tab")
	if err != nil || len(entries) == 0 {
		panic(fmt.Sprintf("iso-639-3 data missing — run `go generate ./internal/language/` (err=%v, matches=%v)", err, entries))
	}
	if len(entries) > 1 {
		panic(fmt.Sprintf("multiple iso-639-3 cache files found (%v); the fetcher should keep only one — delete the stale ones", entries))
	}
	b, err := iso639fs.ReadFile(entries[0])
	if err != nil {
		panic(fmt.Sprintf("read %s: %v", entries[0], err))
	}
	return parseISO639(string(b))
}

// parseISO639 parses the SIL iso-639-3.tab TSV format. Columns:
//
//	Id  Part2B  Part2T  Part1  Scope  Language_Type  Ref_Name  Comment
//
// The header row is skipped; blank lines are tolerated.
func parseISO639(s string) map[string]Info {
	out := make(map[string]Info, 8000)
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 7 || f[0] == "" {
			continue
		}
		out[f[0]] = Info{
			ID:      f[0],
			Part2B:  f[1],
			Part2T:  f[2],
			Part1:   f[3],
			Scope:   f[4],
			Type:    f[5],
			RefName: f[6],
			Comment: fieldAt(f, 7),
		}
	}
	return out
}

func fieldAt(f []string, i int) string {
	if i < len(f) {
		return f[i]
	}
	return ""
}

// Lookup returns the Info for the given 3-letter code and reports
// whether it was found.
func Lookup(code Language) (Info, bool) {
	v, ok := registry[code]
	return v, ok
}
