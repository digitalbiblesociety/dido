package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// pickerPickedMsg carries the path the user chose in fzf back to the
// setup model. `field` is the textinput label so the receiver knows
// which row to update without round-tripping focus state. `kind` is
// "usfm" / "audio" / "" so the receiver can run kind-specific follow-up
// (e.g. read conf.toml after a USFM pick); `bibleID` is the selected
// folder's basename so the receiver can show it without re-parsing
// the path.
type pickerPickedMsg struct {
	field   string
	path    string
	kind    string
	bibleID string
}

// pickerCancelledMsg is dispatched when the user dismissed fzf (esc /
// ctrl+c) or fzf itself failed. We render a one-line note instead of an
// error so the form stays usable.
type pickerCancelledMsg struct {
	field  string
	reason string
}

// pickerMissingMsg is dispatched when `fzf` isn't on PATH. The setup
// view turns this into a hint pointing at the install command rather
// than failing the picker silently.
type pickerMissingMsg struct {
	field string
}

// pickDir returns a tea.Cmd that suspends Bubble Tea, runs fzf over a
// directory listing rooted at startFrom (or its parent when startFrom
// is a file), and resumes with one of the picker*Msg types.
//
// fzf's output is captured via a temp file because tea.ExecProcess
// hands the terminal directly to the child — we can't pipe its stdout
// back through Go. The shell pipeline is intentionally simple so it
// works the same on macOS BSD find and GNU find.
func pickDir(field, startFrom string) tea.Cmd {
	if _, err := exec.LookPath("fzf"); err != nil {
		return func() tea.Msg { return pickerMissingMsg{field: field} }
	}

	base := resolveBase(startFrom)

	tmp, err := os.CreateTemp("", "dido-sab-pick-*.txt")
	if err != nil {
		return func() tea.Msg {
			return pickerCancelledMsg{field: field, reason: err.Error()}
		}
	}
	tmpPath := tmp.Name()
	tmp.Close()

	// -maxdepth 6 keeps the listing responsive on big repos / volumes;
	// the SAB layout never goes deeper than ~4. shellQuote is used on
	// the base path so volume names with spaces work.
	shCmd := fmt.Sprintf(
		`find %s -maxdepth 6 -type d 2>/dev/null | fzf --height ~80%% --border --prompt "%s ▸ " --header "↑↓ navigate · enter select · esc cancel" > %s`,
		shellQuote(base), field, shellQuote(tmpPath),
	)
	cmd := exec.Command("sh", "-c", shCmd)
	cmd.Stderr = os.Stderr // fzf draws its UI on stderr

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			// Exit code 130 = user cancelled (esc / ctrl-c); anything
			// else is reported so the user knows fzf misbehaved.
			if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 130 {
				return pickerCancelledMsg{field: field}
			}
			return pickerCancelledMsg{field: field, reason: err.Error()}
		}
		data, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return pickerCancelledMsg{field: field, reason: readErr.Error()}
		}
		picked := strings.TrimSpace(string(data))
		if picked == "" {
			return pickerCancelledMsg{field: field}
		}
		return pickerPickedMsg{field: field, path: picked}
	})
}

// resolveBase picks the right starting directory for the find/fzf
// pipeline. If startFrom is empty or doesn't exist, fall back to $HOME.
// If it's a file, walk up one level. Otherwise use it as-is.
func resolveBase(startFrom string) string {
	startFrom = strings.TrimSpace(startFrom)
	if startFrom == "" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return "."
	}
	info, err := os.Stat(startFrom)
	if err != nil {
		// Walk up until we find a dir that does exist — handles the
		// case where the user typed a not-yet-created output path.
		dir := filepath.Dir(startFrom)
		for dir != "" && dir != "/" && dir != "." {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return "."
	}
	if !info.IsDir() {
		return filepath.Dir(startFrom)
	}
	return startFrom
}

// pickBible lists the immediate subdirectories of root and presents
// them via fzf, in the style of usfm-tools' Bible selector. Each row
// shows the folder name padded to a fixed width plus an unobtrusive
// hint pulled from conf.toml when present (language + ISO code). The
// callback returns the chosen folder's full path, not just its name.
//
// `kind` ("usfm" or "audio") rides along on the result message so the
// setup model can run kind-specific follow-up — e.g. reading conf.toml
// to auto-fill the Language field after a USFM pick.
func pickBible(field, root, kind string) tea.Cmd {
	if _, err := exec.LookPath("fzf"); err != nil {
		return func() tea.Msg { return pickerMissingMsg{field: field} }
	}
	root = strings.TrimSpace(root)
	if root == "" || !pathExistsAndIsDir(root) {
		// Without a root we have nothing useful to fuzzy-list; fall
		// back to the generic directory picker so the user can still
		// browse the filesystem.
		return pickDir(field, root)
	}

	entries, err := listBibleFolders(root)
	if err != nil || len(entries) == 0 {
		return pickDir(field, root)
	}

	tmp, err := os.CreateTemp("", "dido-sab-bible-*.txt")
	if err != nil {
		return func() tea.Msg {
			return pickerCancelledMsg{field: field, reason: err.Error()}
		}
	}
	tmpPath := tmp.Name()
	tmp.Close()

	var list strings.Builder
	idToPath := make(map[string]string, len(entries))
	for _, e := range entries {
		list.WriteString(e.display)
		list.WriteByte('\n')
		idToPath[e.id] = e.path
	}

	// --delimiter=tab + --with-nth=1,2 hides the trailing path column
	// from fzf's match window but keeps it in the displayed row.
	header := fmt.Sprintf("Select %s Bible · ↑↓ navigate · enter · esc cancel",
		strings.ToUpper(kind))
	shCmd := fmt.Sprintf(
		`printf %%s %s | fzf --height ~80%% --border --delimiter='\t' --with-nth=1,2 --prompt "%s ▸ " --header "%s" > %s`,
		shellQuote(list.String()),
		field,
		header,
		shellQuote(tmpPath),
	)
	cmd := exec.Command("sh", "-c", shCmd)
	cmd.Stderr = os.Stderr

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 130 {
				return pickerCancelledMsg{field: field}
			}
			return pickerCancelledMsg{field: field, reason: err.Error()}
		}
		data, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return pickerCancelledMsg{field: field, reason: readErr.Error()}
		}
		picked := strings.TrimSpace(string(data))
		if picked == "" {
			return pickerCancelledMsg{field: field}
		}
		id := picked
		if tab := strings.IndexByte(picked, '\t'); tab > 0 {
			id = strings.TrimSpace(picked[:tab])
		}
		full, ok := idToPath[id]
		if !ok {
			return pickerCancelledMsg{field: field, reason: "unknown selection: " + id}
		}
		return pickerPickedMsg{field: field, path: full, kind: kind, bibleID: id}
	})
}

// bibleEntry is one candidate row returned by listBibleFolders.
type bibleEntry struct {
	id      string
	path    string
	display string // tab-separated "ID\tHINT" for fzf
}

// listBibleFolders enumerates the immediate subdirectories of root and
// builds an fzf-friendly row for each. Hidden / underscore-prefixed
// folders are dropped because they're conventional staging dirs in the
// usfm-tools layout. Non-directories are skipped silently.
func listBibleFolders(root string) ([]bibleEntry, error) {
	dirEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []bibleEntry
	for _, e := range dirEntries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		full := filepath.Join(root, name)
		hint := bibleFolderHint(full)
		out = append(out, bibleEntry{
			id:      name,
			path:    full,
			display: fmt.Sprintf("%-22s\t%s", name, hint),
		})
	}
	return out, nil
}

// bibleFolderHint produces a short status line for fzf rendering. It
// surfaces the language + ISO code from conf.toml when available and
// otherwise reports the dominant file count so users know what kind of
// folder they're looking at.
func bibleFolderHint(dir string) string {
	if conf, ok := readBibleConf(filepath.Join(dir, "conf.toml")); ok {
		parts := []string{}
		if conf.Language != "" {
			parts = append(parts, conf.Language)
		}
		if conf.ISO != "" {
			parts = append(parts, "iso="+conf.ISO)
		}
		if len(parts) > 0 {
			return strings.Join(parts, " · ")
		}
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "usfm", "*.usfm")); len(matches) > 0 {
		return fmt.Sprintf("%d usfm files (under usfm/)", len(matches))
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.usfm")); len(matches) > 0 {
		return fmt.Sprintf("%d usfm files", len(matches))
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*", "*.mp3")); len(matches) > 0 {
		return fmt.Sprintf("%d mp3 files (nested)", len(matches))
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.mp3")); len(matches) > 0 {
		return fmt.Sprintf("%d mp3 files", len(matches))
	}
	return ""
}

// bibleConf is the tiny slice of conf.toml the picker + auto-language
// logic care about. readBibleConf only scrapes the keys it needs so
// we don't pull in a full TOML parser.
type bibleConf struct {
	ISO      string // ISO 639-3 code, e.g. "tha"
	Lang     string // explicit `lang =` key (sometimes differs from iso for transliteration setups)
	Language string // human-readable `language =` key (e.g. "Thai")
}

// readBibleConf scrapes a conf.toml for the three keys we need. It is
// deliberately permissive: a missing file or malformed line returns
// whatever was found so the caller can still produce a partial hint.
func readBibleConf(path string) (bibleConf, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return bibleConf{}, false
	}
	var c bibleConf
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := unquoteTOMLValue(strings.TrimSpace(line[eq+1:]))
		switch key {
		case "iso":
			c.ISO = val
		case "lang":
			c.Lang = val
		case "language":
			c.Language = val
		}
	}
	if c.ISO == "" && c.Lang == "" && c.Language == "" {
		return c, false
	}
	return c, true
}

// unquoteTOMLValue strips matching single or double quotes from a TOML
// scalar. We don't decode escapes — every field we read is plain ASCII
// so naive unquoting is correct.
func unquoteTOMLValue(v string) string {
	if len(v) >= 2 {
		first, last := v[0], v[len(v)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func pathExistsAndIsDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// shellQuote wraps s in single quotes, escaping embedded single quotes
// the safe POSIX way ('"'"'). Used only for paths we hand to `sh -c`.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
