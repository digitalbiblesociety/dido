package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/digitalbiblesociety/dido/internal/userconfig"
)

// fieldKind labels each row in the setup form so the renderer and the
// keyboard handler know how to treat it without a string lookup.
type fieldKind int

const (
	kText fieldKind = iota
	kToggle
	kSubmit
)

// setupRow is one row in the form. Text rows own a textinput.Model;
// toggles flip a *bool the model lends them. Hidden rows are skipped
// during focus traversal so the user never lands on an invisible field.
type setupRow struct {
	kind   fieldKind
	label  string
	hint   string         // shown after the value in dim style
	input  textinput.Model // valid only when kind == kText
	target *bool           // valid only when kind == kToggle
	hidden func() bool     // optional; row is skipped when true
}

// setupModel is the first screen: a labelled column of text inputs and
// toggles that gathers everything `runBatch` / `runSingle` need before
// we transition to the plan preview.
type setupModel struct {
	rows    []setupRow
	focus   int
	batch   bool
	resume  bool
	include bool
	err     string
	notice  string
	width   int
	height  int

	// userCfg holds the persisted defaults (env + config.env). The
	// settings screen edits this in place; on return we refill any
	// fields the user left empty.
	userCfg *userconfig.Config

	submitted bool
	cfg       cliFlags
	posArgs   []string // [audio, usfm, out] for batch; same shape in single mode
}

func newSetupModel(initial cliFlags, args []string, userCfg *userconfig.Config) *setupModel {
	if userCfg == nil {
		userCfg = &userconfig.Config{NameStyle: "sab", OutputFolder: "./out"}
	}
	// Resolve each field's starting value with this precedence:
	//   1. positional arg / flag the user just passed on the CLI
	//   2. persisted userconfig (env + ~/.config/dido/config.env)
	//   3. hard-coded placeholder
	pick := func(values ...string) string {
		for _, v := range values {
			if strings.TrimSpace(v) != "" {
				return v
			}
		}
		return ""
	}
	lang := pick(initial.lang, userCfg.Lang)
	resume := initial.resume || userCfg.Resume
	include := initial.includeSections || userCfg.IncludeSectionHeaders
	batchDefault := initial.batch || (initial.book == "" && initial.bookSeq == 0)

	audio, usfm, out := "", "", ""
	if len(args) > 0 {
		audio = args[0]
	}
	if len(args) > 1 {
		usfm = args[1]
	}
	if len(args) > 2 {
		out = args[2]
	}
	audio = pick(audio, userCfg.AudioRoot)
	usfm = pick(usfm, userCfg.USFMRoot)
	out = pick(out, userCfg.OutputFolder, "./out")

	m := &setupModel{
		batch:   batchDefault,
		resume:  resume,
		include: include,
		userCfg: userCfg,
	}

	mk := func(placeholder, value string) textinput.Model {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = placeholder
		ti.SetValue(value)
		ti.CharLimit = 512
		ti.Width = 56
		return ti
	}

	// Discard the positional `out` we computed above — Output is no
	// longer a setup-form field; it lives in Settings only. We still
	// hold a copy on the model for submit() to validate.
	_ = out

	rows := []setupRow{
		{kind: kText, label: "Language",
			hint:  "auto-filled from conf.toml on USFM pick · espeak-ng voice (eng, tha, epo, …)",
			input: mk("tha", lang)},
		{kind: kToggle, label: "Mode", hint: "space toggles batch ↔ single",
			target: &m.batch},
		{kind: kText, label: "USFM",
			hint:  "Bible folder under DIDO_USFM_ROOT (use ctrl+o to fuzzy-pick)",
			input: mk("/path/to/usfm", usfm)},
		{kind: kText, label: "Audio",
			hint:  "Audio folder under DIDO_AUDIO_ROOT (use ctrl+o to fuzzy-pick)",
			input: mk("/path/to/audio", audio)},
		{kind: kText, label: "Book", hint: "single mode only — e.g. ISA",
			input: mk("ISA", initial.book),
			hidden: func() bool { return m.batch }},
		{kind: kText, label: "Book seq", hint: "single mode only — 1-based position",
			input: mk("23", func() string {
				if initial.bookSeq == 0 {
					return ""
				}
				return strconv.Itoa(initial.bookSeq)
			}()),
			hidden: func() bool { return m.batch }},
		{kind: kText, label: "Chapters", hint: "optional, e.g. 1-5",
			input: mk("", initial.chapters)},
		{kind: kToggle, label: "Resume", hint: "skip chapters whose timing file exists",
			target: &m.resume},
		{kind: kToggle, label: "Include section headers", hint: "append \\s heading text to verses",
			target: &m.include},
		{kind: kSubmit, label: "Start"},
	}

	// Set initial focus to the first visible row, with cursor on its input.
	m.rows = rows
	m.focus = m.firstVisible(0, +1)
	m.refreshFocus()
	return m
}

func (m *setupModel) Init() tea.Cmd {
	return textinput.Blink
}

// firstVisible returns the next visible row starting at `from` and moving
// by `dir` (+1 or -1). It wraps. Used to advance focus past hidden rows.
func (m *setupModel) firstVisible(from, dir int) int {
	n := len(m.rows)
	i := from
	for range n {
		if i < 0 {
			i = n - 1
		}
		if i >= n {
			i = 0
		}
		if !m.isHidden(i) {
			return i
		}
		i += dir
	}
	return 0
}

func (m *setupModel) isHidden(i int) bool {
	if i < 0 || i >= len(m.rows) {
		return true
	}
	if m.rows[i].hidden == nil {
		return false
	}
	return m.rows[i].hidden()
}

// refreshFocus blurs every input then focuses the current row's input
// (if any). Toggle/submit rows have no cursor so they only highlight.
func (m *setupModel) refreshFocus() {
	for i := range m.rows {
		if m.rows[i].kind == kText {
			m.rows[i].input.Blur()
		}
	}
	if m.focus >= 0 && m.focus < len(m.rows) {
		r := &m.rows[m.focus]
		if r.kind == kText {
			r.input.Focus()
		}
	}
}

func (m *setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case pickerPickedMsg:
		// USFM Bibles in our layout live under <Bible>/usfm/. Resolve
		// the picked Bible root to that subdir when present so the
		// downstream aligner finds .usfm files immediately; fall back
		// to the picked path otherwise (some bibles have .usfm at the
		// top level). conf.toml stays on the Bible root, so we read
		// language before the path is normalised.
		fieldPath := msg.path
		if msg.kind == "usfm" {
			if sub := filepath.Join(msg.path, "usfm"); pathExistsAndIsDir(sub) {
				fieldPath = sub
			}
		}
		m.setFieldValue(msg.field, fieldPath)
		hint := fieldPath
		if msg.bibleID != "" {
			hint = msg.bibleID + " (" + fieldPath + ")"
		}
		m.notice = fmt.Sprintf("picked %s: %s", msg.field, hint)
		if msg.kind == "usfm" {
			if iso, ok := languageFromBibleFolder(msg.path); ok {
				m.setFieldValue("Language", iso)
				m.notice += "  ·  language=" + iso + " (from conf.toml)"
			}
		}
		return m, nil
	case pickerCancelledMsg:
		if msg.reason != "" {
			m.notice = fmt.Sprintf("picker cancelled: %s", msg.reason)
		} else {
			m.notice = "picker cancelled."
		}
		return m, nil
	case pickerMissingMsg:
		m.notice = "fzf not found on PATH — install it for inline pickers (brew install fzf)."
		return m, nil
	case settingsSavedMsg:
		// Settings cfg pointer is the same one we hold; refill any
		// fields the user left blank with the new defaults.
		m.userCfg = msg.cfg
		m.applyConfigToEmptyFields()
		m.notice = "settings saved."
		return m, nil
	case settingsCancelledMsg:
		m.notice = "settings cancelled."
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			// esc on an active text input blurs it first so users can
			// recover from a typo without losing the form; the second
			// esc bubbles up to the menu.
			if m.focus >= 0 && m.focus < len(m.rows) && m.rows[m.focus].kind == kText {
				m.rows[m.focus].input.Blur()
				m.notice = "press esc again to go back to the menu"
				return m, nil
			}
			return m, func() tea.Msg { return setupBackMsg{} }
		case "tab", "down":
			m.notice = ""
			m.focus = m.firstVisible(m.focus+1, +1)
			m.refreshFocus()
			return m, nil
		case "shift+tab", "up":
			m.notice = ""
			m.focus = m.firstVisible(m.focus-1, -1)
			m.refreshFocus()
			return m, nil
		case "ctrl+o":
			// Launch the picker for the focused row. USFM / Audio use
			// the Bible-style fuzzy picker rooted at the matching
			// DIDO_*_ROOT; any other path row falls back to the
			// generic directory tree picker.
			if m.focus >= 0 && m.focus < len(m.rows) {
				r := &m.rows[m.focus]
				if r.kind == kText {
					switch r.label {
					case "USFM":
						root := ""
						if m.userCfg != nil {
							root = m.userCfg.USFMRoot
						}
						return m, pickBible(r.label, root, "usfm")
					case "Audio":
						root := ""
						if m.userCfg != nil {
							root = m.userCfg.AudioRoot
						}
						return m, pickBible(r.label, root, "audio")
					}
				}
			}
		case "ctrl+e", "f2":
			// Hand off to the settings screen. The parent app model
			// catches openSettingsMsg and swaps screens.
			return m, func() tea.Msg {
				return openSettingsMsg{cfg: m.userCfg}
			}
		case " ", "space":
			if m.focus < len(m.rows) {
				r := &m.rows[m.focus]
				if r.kind == kToggle && r.target != nil {
					*r.target = !*r.target
					// Mode toggle may hide/show rows; if focus lands
					// on a hidden one, snap forward.
					if m.isHidden(m.focus) {
						m.focus = m.firstVisible(m.focus, +1)
					}
					m.refreshFocus()
					return m, nil
				}
			}
		case "enter":
			if m.focus < len(m.rows) && m.rows[m.focus].kind == kSubmit {
				return m, m.submit()
			}
			// Advance to next visible row from a text input.
			m.focus = m.firstVisible(m.focus+1, +1)
			m.refreshFocus()
			return m, nil
		}
	}

	// Forward the message to the focused text input.
	if m.focus >= 0 && m.focus < len(m.rows) && m.rows[m.focus].kind == kText {
		var cmd tea.Cmd
		m.rows[m.focus].input, cmd = m.rows[m.focus].input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// isPathLabel returns true for the rows the fzf picker can populate.
// Audio + USFM use the Bible-folder picker; the helper exists so the
// view hint and the picker dispatcher agree on the set.
func isPathLabel(label string) bool {
	switch label {
	case "Audio", "USFM":
		return true
	}
	return false
}

// languageFromBibleFolder returns the best ISO 639-3 voice code for a
// chosen Bible folder. Preference order matches what an aeneas pipeline
// expects: explicit `lang` key first (often set when the bible team
// overrides the eSpeak voice), then `iso`. The boolean is false when
// no conf.toml is present or neither key is filled in.
func languageFromBibleFolder(bibleDir string) (string, bool) {
	conf, ok := readBibleConf(filepath.Join(bibleDir, "conf.toml"))
	if !ok {
		return "", false
	}
	if conf.Lang != "" {
		return conf.Lang, true
	}
	if conf.ISO != "" {
		return conf.ISO, true
	}
	return "", false
}

// setFieldValue rewrites the textinput on the row matching label. Used
// by the picker result handler. Silently no-ops on an unknown label so
// label refactors don't crash the model.
func (m *setupModel) setFieldValue(label, value string) {
	for i := range m.rows {
		if m.rows[i].label == label && m.rows[i].kind == kText {
			m.rows[i].input.SetValue(value)
			m.rows[i].input.CursorEnd()
			return
		}
	}
}

// applyConfigToEmptyFields refills empty path/lang inputs from the
// (possibly just-saved) userconfig. It deliberately leaves filled
// fields alone so users don't lose what they've typed.
func (m *setupModel) applyConfigToEmptyFields() {
	if m.userCfg == nil {
		return
	}
	// Output is no longer a setup-form field; the mapping stops at
	// the columns the user can still see and edit.
	mapping := map[string]string{
		"Audio":    m.userCfg.AudioRoot,
		"USFM":     m.userCfg.USFMRoot,
		"Language": m.userCfg.Lang,
	}
	for i := range m.rows {
		if m.rows[i].kind != kText {
			continue
		}
		if v, ok := mapping[m.rows[i].label]; ok && v != "" &&
			strings.TrimSpace(m.rows[i].input.Value()) == "" {
			m.rows[i].input.SetValue(v)
		}
	}
}

// openSettingsMsg is dispatched when the user presses ctrl+e / F2 on
// the setup screen; the parent transitions to the settings sub-model.
type openSettingsMsg struct {
	cfg *userconfig.Config
}

// setupBackMsg pops the setup form off in favour of the main menu.
// Dispatched by a bare `esc` (no focused input) so users can browse
// menu items without ctrl+c'ing the whole program.
type setupBackMsg struct{}

// setupDoneMsg is dispatched once the form validates; the parent model
// transitions to the plan screen on receipt.
type setupDoneMsg struct {
	cfg     cliFlags
	posArgs []string // [audio, usfm, out]
}

// submit reads each field, runs the same validation main()'s flag
// parsing path used to enforce, and either records a setupDoneMsg or
// stashes an error string into m.err for the view to render.
func (m *setupModel) submit() tea.Cmd {
	get := func(label string) string {
		for i := range m.rows {
			if m.rows[i].label == label {
				return strings.TrimSpace(m.rows[i].input.Value())
			}
		}
		return ""
	}

	c := cliFlags{
		lang:            get("Language"),
		batch:           m.batch,
		chapters:        get("Chapters"),
		nameStyle:       "sab",
		resume:          m.resume,
		includeSections: m.include,
	}
	if c.lang == "" {
		m.err = "Language is required (e.g. tha, eng, epo)."
		return nil
	}
	if c.chapters != "" {
		min, max, err := parseChapterRange(c.chapters)
		if err != nil {
			m.err = fmt.Sprintf("Chapters: %v", err)
			return nil
		}
		c.chapterMin = min
		c.chapterMax = max
	}
	audio := get("Audio")
	usfm := get("USFM")
	// Output is sourced from the persisted user config now; the setup
	// form no longer exposes it. Empty here means the user hasn't
	// configured it yet — point them at the Settings screen rather
	// than silently falling back to the cwd.
	out := ""
	if m.userCfg != nil {
		out = strings.TrimSpace(m.userCfg.OutputFolder)
	}
	if audio == "" || usfm == "" {
		m.err = "Audio and USFM paths are both required."
		return nil
	}
	if out == "" {
		m.err = "Output folder is not configured. Press ctrl+e to set it in Settings."
		return nil
	}
	if !pathExists(audio) {
		m.err = fmt.Sprintf("Audio path does not exist: %s", audio)
		return nil
	}
	if !pathExists(usfm) {
		m.err = fmt.Sprintf("USFM path does not exist: %s", usfm)
		return nil
	}
	if !m.batch {
		c.book = get("Book")
		if c.book == "" {
			m.err = "Book code is required in single mode."
			return nil
		}
		c.book = strings.ToUpper(c.book)
		if seq := get("Book seq"); seq != "" {
			n, err := strconv.Atoi(seq)
			if err != nil || n <= 0 {
				m.err = fmt.Sprintf("Book seq must be a positive integer (got %q).", seq)
				return nil
			}
			c.bookSeq = n
		}
		// Single mode wants a *.usfm file, not a dir.
		if info, err := os.Stat(usfm); err == nil && info.IsDir() {
			m.err = "Single mode: USFM must be a file, not a directory."
			return nil
		}
		// Single mode wants an audio dir, not a file.
		if info, err := os.Stat(audio); err == nil && !info.IsDir() {
			m.err = "Single mode: Audio must be a directory containing *.mp3."
			return nil
		}
	} else {
		if info, err := os.Stat(audio); err == nil && !info.IsDir() {
			m.err = "Batch mode: Audio must be a directory."
			return nil
		}
		if info, err := os.Stat(usfm); err == nil && !info.IsDir() {
			m.err = "Batch mode: USFM must be a directory."
			return nil
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		m.err = fmt.Sprintf("mkdir %s: %v", out, err)
		return nil
	}
	m.err = ""
	m.submitted = true
	m.cfg = c
	m.posArgs = []string{audio, usfm, out}
	return func() tea.Msg { return setupDoneMsg{cfg: c, posArgs: m.posArgs} }
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// view styles shared with the rest of the TUI live in tui.go; only the
// setup-specific ones are declared here.
var (
	setupTitle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).MarginBottom(1)
	setupLabel     = lipgloss.NewStyle().Width(22).Foreground(lipgloss.Color("250"))
	setupLabelHot  = lipgloss.NewStyle().Width(22).Foreground(lipgloss.Color("212")).Bold(true)
	setupHint      = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	setupBtn       = lipgloss.NewStyle().Padding(0, 2).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("250"))
	setupBtnFocus  = lipgloss.NewStyle().Padding(0, 2).Background(lipgloss.Color("212")).Foreground(lipgloss.Color("0")).Bold(true)
	setupErr       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	toggleOn       = lipgloss.NewStyle().Foreground(lipgloss.Color("78")).Bold(true)
	toggleOff      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func (m *setupModel) View() string {
	var sb strings.Builder
	sb.WriteString(setupTitle.Render("dido-sab — setup"))
	sb.WriteByte('\n')

	// Read-only echo of the output folder so users can see where the
	// run will write without leaving the screen. Empty = configure in
	// Settings first; the submit() handler enforces this hard.
	out := ""
	if m.userCfg != nil {
		out = strings.TrimSpace(m.userCfg.OutputFolder)
	}
	if out == "" {
		sb.WriteString(setupErr.Render("Output: not configured — press ctrl+e to set it in Settings."))
	} else {
		sb.WriteString(dimStyle.Render("Output → " + out + "  (edit in Settings · ctrl+e)"))
	}
	sb.WriteString("\n\n")

	for i, r := range m.rows {
		if m.isHidden(i) {
			continue
		}
		focused := i == m.focus
		label := setupLabel.Render(r.label)
		if focused {
			label = setupLabelHot.Render("▸ " + r.label)
		}
		switch r.kind {
		case kText:
			val := r.input.View()
			sb.WriteString(label)
			sb.WriteString(val)
			if focused && isPathLabel(r.label) {
				sb.WriteString("  ")
				sb.WriteString(setupHint.Render("ctrl+o to browse"))
			} else if r.hint != "" {
				sb.WriteString("  ")
				sb.WriteString(setupHint.Render(r.hint))
			}
		case kToggle:
			state := "[ ]"
			if r.target != nil && *r.target {
				state = "[x]"
			}
			extra := ""
			if r.label == "Mode" {
				if m.batch {
					extra = "  batch"
				} else {
					extra = "  single"
				}
			}
			sb.WriteString(label)
			if r.target != nil && *r.target {
				sb.WriteString(toggleOn.Render(state))
			} else {
				sb.WriteString(toggleOff.Render(state))
			}
			sb.WriteString(extra)
			if r.hint != "" {
				sb.WriteString("  ")
				sb.WriteString(setupHint.Render(r.hint))
			}
		case kSubmit:
			btn := setupBtn.Render(" " + r.label + " ▶ ")
			if focused {
				btn = setupBtnFocus.Render(" " + r.label + " ▶ ")
			}
			sb.WriteString(strings.Repeat(" ", 22))
			sb.WriteString(btn)
		}
		sb.WriteByte('\n')
	}

	if m.err != "" {
		sb.WriteByte('\n')
		sb.WriteString(setupErr.Render("⚠ " + m.err))
		sb.WriteByte('\n')
	} else if m.notice != "" {
		sb.WriteByte('\n')
		sb.WriteString(dimStyle.Render(m.notice))
		sb.WriteByte('\n')
	}

	sb.WriteByte('\n')
	sb.WriteString(dimStyle.Render(
		"tab/shift-tab move · space toggle · ctrl+o pick dir · ctrl+e settings · enter on Start ▶ · ctrl+c quit"))
	return sb.String()
}
