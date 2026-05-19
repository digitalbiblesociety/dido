package main

import (
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/digitalbiblesociety/dido/internal/userconfig"
)

// aboutModel is a static, scroll-free help/about page. It pulls the
// build's Go version and resolved config path so the user has enough
// info to file a sensible bug report without leaving the TUI.
type aboutModel struct {
	width  int
	height int
}

func newAboutModel() *aboutModel { return &aboutModel{} }

func (m *aboutModel) Init() tea.Cmd { return nil }

// aboutBackMsg signals the parent to return to the main menu.
type aboutBackMsg struct{}

func (m *aboutModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "q", "Q", "enter":
			return m, func() tea.Msg { return aboutBackMsg{} }
		}
	}
	return m, nil
}

var aboutTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).MarginBottom(1)
var aboutHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250")).MarginTop(1)

func (m *aboutModel) View() string {
	cfgPath := "(unresolved)"
	if p, err := userconfig.Path(); err == nil {
		cfgPath = p
	}
	var sb strings.Builder
	sb.WriteString(aboutTitleStyle.Render("dido-sab — about"))
	sb.WriteByte('\n')

	sb.WriteString(aboutHeaderStyle.Render("What it does"))
	sb.WriteByte('\n')
	sb.WriteString(strings.Join([]string{
		"  Force-aligns scripture audio against USFM text and emits the",
		"  three SAB output files the Scripture App Builder reader",
		"  consumes (-aeneas.txt, -aeneas-original.txt, -timing.txt).",
		"  Auto-transliterates non-Roman scripts when no native eSpeak",
		"  voice exists so the English voice can read phonetically.",
	}, "\n"))
	sb.WriteString("\n")

	sb.WriteString(aboutHeaderStyle.Render("Requirements"))
	sb.WriteByte('\n')
	sb.WriteString(strings.Join([]string{
		"  · espeak-ng on PATH (override with DIDO_ESPEAK_NG_PATH)",
		"  · ffmpeg on PATH (for MP3 decoding)",
		"  · fzf on PATH (optional — enables the ctrl+o path picker)",
	}, "\n"))
	sb.WriteString("\n")

	sb.WriteString(aboutHeaderStyle.Render("Key bindings"))
	sb.WriteByte('\n')
	sb.WriteString(strings.Join([]string{
		"  Main menu     ↑/↓ or j/k navigate · enter / hotkey select",
		"  Setup form    tab/shift-tab move · space toggle",
		"                ctrl+o pick dir (fzf)  ·  ctrl+e settings",
		"                enter on Start ▶ to plan + run",
		"  Plan preview  enter run · b/esc back · ↑/↓ scroll",
		"  Settings      tab/shift-tab move · space toggle",
		"                ctrl+s save · esc cancel",
		"  Anywhere      ctrl+c quit",
	}, "\n"))
	sb.WriteString("\n")

	sb.WriteString(aboutHeaderStyle.Render("Batch performance"))
	sb.WriteByte('\n')
	sb.WriteString(strings.Join([]string{
		"  The alignment pipeline (MFCC + DTW) is already parallel across",
		"  every CPU core, so DIDO_BATCH_WORKERS sets *concurrent chapters*",
		"  on top of that — keep it low (default 2). Each batch worker is",
		"  given an inner CPU budget of NumCPU / workers so total compute",
		"  goroutines stay around NumCPU.",
		"  External / exFAT volumes (e.g. /Volumes/<UUID>) often slow with",
		"  parallel reads — try workers=1 if audio sits on one.",
	}, "\n"))
	sb.WriteString("\n")

	sb.WriteString(aboutHeaderStyle.Render("Configuration"))
	sb.WriteByte('\n')
	sb.WriteString("  Defaults: ")
	sb.WriteString(cfgPath)
	sb.WriteString("\n  Env vars: DIDO_AUDIO_ROOT, DIDO_USFM_ROOT, DIDO_OUTPUT_FOLDER,\n")
	sb.WriteString("            DIDO_LANG, DIDO_NAME_STYLE, DIDO_RESUME,\n")
	sb.WriteString("            DIDO_INCLUDE_SECTION_HEADERS, DIDO_ESPEAK_NG_PATH\n")

	sb.WriteString(aboutHeaderStyle.Render("Build"))
	sb.WriteByte('\n')
	sb.WriteString("  Go " + runtime.Version() + " · " + runtime.GOOS + "/" + runtime.GOARCH)
	sb.WriteString("\n")

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("esc / q / enter to return to the menu"))
	return sb.String()
}
