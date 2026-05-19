package main

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/digitalbiblesociety/dido/internal/userconfig"
)

// settingsField mirrors the userconfig.Config layout but with the
// kind/label metadata the renderer and keyboard handler need. It's
// the same shape as setupRow, kept separate so the two screens can
// evolve independently.
type settingsField struct {
	kind   fieldKind
	label  string
	hint   string
	input  textinput.Model
	target *bool
}

// settingsModel edits the persistent userconfig file. Reachable from
// the setup screen via `s`; `ctrl+s` / enter on the Save row writes
// the file and pops back to setup with the new defaults applied.
type settingsModel struct {
	cfg     *userconfig.Config
	rows    []settingsField
	focus   int
	err     string
	notice  string
	width   int
	height  int
}

func newSettingsModel(cfg *userconfig.Config) *settingsModel {
	if cfg == nil {
		cfg = &userconfig.Config{}
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
	m := &settingsModel{cfg: cfg}
	m.rows = []settingsField{
		{kind: kText, label: "Audio root",
			hint:  "DIDO_AUDIO_ROOT — default audio root for batch mode",
			input: mk("/Volumes/.../audio", cfg.AudioRoot)},
		{kind: kText, label: "USFM root",
			hint:  "DIDO_USFM_ROOT — directory of *.usfm files",
			input: mk("/Volumes/.../usfm", cfg.USFMRoot)},
		{kind: kText, label: "Output folder",
			hint:  "DIDO_OUTPUT_FOLDER — written if missing on run",
			input: mk("./out", cfg.OutputFolder)},
		{kind: kText, label: "Language",
			hint:  "DIDO_LANG — espeak-ng voice code (tha, eng, epo, …)",
			input: mk("tha", cfg.Lang)},
		{kind: kText, label: "Name style",
			hint:  "DIDO_NAME_STYLE — \"sab\" or \"simple\"",
			input: mk("sab", cfg.NameStyle)},
		{kind: kText, label: "espeak-ng path",
			hint:  "DIDO_ESPEAK_NG_PATH — override the binary lookup",
			input: mk("/opt/homebrew/bin/espeak-ng", cfg.EspeakNGPath)},
		{kind: kText, label: "Workers",
			hint: "DIDO_BATCH_WORKERS — concurrent chapters · 0 = auto (2) · pipeline itself is already parallel; higher = oversubscribed",
			input: mk("0", func() string {
				if cfg.Workers == 0 {
					return ""
				}
				return strconv.Itoa(cfg.Workers)
			}())},
		{kind: kToggle, label: "Resume by default",
			hint:   "DIDO_RESUME — skip chapters whose timing file exists",
			target: &cfg.Resume},
		{kind: kToggle, label: "Include section headers",
			hint:   "DIDO_INCLUDE_SECTION_HEADERS — append \\s text to verses",
			target: &cfg.IncludeSectionHeaders},
		{kind: kSubmit, label: "Save"},
	}
	m.refreshFocus()
	return m
}

func (m *settingsModel) Init() tea.Cmd { return textinput.Blink }

// settingsSavedMsg signals the parent to pop back to setup with the
// updated defaults applied to the form. The cfg pointer is reused so
// later edits stay live.
type settingsSavedMsg struct {
	cfg *userconfig.Config
}

// settingsCancelledMsg pops back without persisting changes. We still
// hand back the (untouched) cfg so the parent can keep its pointer.
type settingsCancelledMsg struct {
	cfg *userconfig.Config
}

func (m *settingsModel) refreshFocus() {
	for i := range m.rows {
		if m.rows[i].kind == kText {
			m.rows[i].input.Blur()
		}
	}
	if m.focus >= 0 && m.focus < len(m.rows) && m.rows[m.focus].kind == kText {
		m.rows[m.focus].input.Focus()
	}
}

// commit copies every text input back into m.cfg, then writes the file.
func (m *settingsModel) commit() error {
	get := func(label string) string {
		for i := range m.rows {
			if m.rows[i].label == label {
				return strings.TrimSpace(m.rows[i].input.Value())
			}
		}
		return ""
	}
	m.cfg.AudioRoot = get("Audio root")
	m.cfg.USFMRoot = get("USFM root")
	m.cfg.OutputFolder = get("Output folder")
	m.cfg.Lang = get("Language")
	m.cfg.NameStyle = get("Name style")
	m.cfg.EspeakNGPath = get("espeak-ng path")
	// Workers: empty or unparseable resets to 0 ("auto / NumCPU").
	// Negative entries are clamped to 0 for the same reason.
	w := 0
	if s := get("Workers"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			w = n
		}
	}
	m.cfg.Workers = w
	if m.cfg.NameStyle == "" {
		m.cfg.NameStyle = "sab"
	}
	if m.cfg.OutputFolder == "" {
		m.cfg.OutputFolder = "./out"
	}
	return m.cfg.Save()
}

func (m *settingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m, func() tea.Msg { return settingsCancelledMsg{cfg: m.cfg} }
		case "tab", "down":
			m.focus = (m.focus + 1) % len(m.rows)
			m.refreshFocus()
			return m, nil
		case "shift+tab", "up":
			m.focus = (m.focus - 1 + len(m.rows)) % len(m.rows)
			m.refreshFocus()
			return m, nil
		case " ", "space":
			if m.focus < len(m.rows) {
				r := &m.rows[m.focus]
				if r.kind == kToggle && r.target != nil {
					*r.target = !*r.target
					return m, nil
				}
			}
		case "ctrl+s":
			if err := m.commit(); err != nil {
				m.err = err.Error()
				return m, nil
			}
			return m, func() tea.Msg { return settingsSavedMsg{cfg: m.cfg} }
		case "enter":
			if m.focus < len(m.rows) && m.rows[m.focus].kind == kSubmit {
				if err := m.commit(); err != nil {
					m.err = err.Error()
					return m, nil
				}
				return m, func() tea.Msg { return settingsSavedMsg{cfg: m.cfg} }
			}
			m.focus = (m.focus + 1) % len(m.rows)
			m.refreshFocus()
			return m, nil
		}
	}
	if m.focus >= 0 && m.focus < len(m.rows) && m.rows[m.focus].kind == kText {
		var cmd tea.Cmd
		m.rows[m.focus].input, cmd = m.rows[m.focus].input.Update(msg)
		return m, cmd
	}
	return m, nil
}

var settingsTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).MarginBottom(1)

func (m *settingsModel) View() string {
	var sb strings.Builder
	sb.WriteString(settingsTitleStyle.Render("dido-sab — settings"))
	sb.WriteByte('\n')
	if p, err := userconfig.Path(); err == nil {
		sb.WriteString(dimStyle.Render("config: " + p))
		sb.WriteByte('\n')
	}
	sb.WriteByte('\n')

	for i, r := range m.rows {
		focused := i == m.focus
		label := setupLabel.Render(r.label)
		if focused {
			label = setupLabelHot.Render("▸ " + r.label)
		}
		switch r.kind {
		case kText:
			sb.WriteString(label)
			sb.WriteString(r.input.View())
			if r.hint != "" {
				sb.WriteString("  ")
				sb.WriteString(setupHint.Render(r.hint))
			}
		case kToggle:
			state := "[ ]"
			if r.target != nil && *r.target {
				state = "[x]"
			}
			sb.WriteString(label)
			if r.target != nil && *r.target {
				sb.WriteString(toggleOn.Render(state))
			} else {
				sb.WriteString(toggleOff.Render(state))
			}
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
		"tab/shift-tab move · space toggle · ctrl+s save · esc cancel"))
	return sb.String()
}
