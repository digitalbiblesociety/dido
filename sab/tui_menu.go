package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// menuItem is one row in the top-level menu. The title carries the
// single-letter hotkey ("A) Align scripture audio") so users can pick
// without arrowing; the description is the dim sub-line shown under
// the highlighted row.
type menuItem struct {
	hotkey      rune
	title       string
	description string
	action      menuAction
}

func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.description }
func (i menuItem) FilterValue() string { return i.title }

// menuAction enumerates the side effects each menu row triggers. The
// app model maps these to screen transitions in one place so the menu
// stays UI-only.
type menuAction int

const (
	actAlign menuAction = iota
	actSettings
	actAbout
	actQuit
)

// menuItemDelegate is a one-line renderer copied from the usfm-tools
// menu: a leading "> " on the selected row, two spaces otherwise.
// Description rows are rendered below the title separately in View().
type menuItemDelegate struct{}

func (d menuItemDelegate) Height() int                             { return 1 }
func (d menuItemDelegate) Spacing() int                            { return 0 }
func (d menuItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d menuItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(menuItem)
	if !ok {
		return
	}
	if index == m.Index() {
		fmt.Fprint(w, menuSelectedStyle.Render("▸ "+it.title))
	} else {
		fmt.Fprint(w, "  "+it.title)
	}
}

// menuModel is the top-level navigable menu. It wraps a bubbles/list
// so we get arrow-key + j/k navigation for free, and adds hotkey
// shortcuts on top.
type menuModel struct {
	list   list.Model
	width  int
	height int
	notice string
}

func newMenuModel() *menuModel {
	items := []list.Item{
		menuItem{hotkey: 'a',
			title:       "A) Align scripture audio",
			description: "Pick audio + USFM + output → run alignment → see results",
			action:      actAlign},
		menuItem{hotkey: 's',
			title:       "S) Settings",
			description: "Edit DIDO_* defaults written to ~/.config/dido/config.env",
			action:      actSettings},
		menuItem{hotkey: 'h',
			title:       "H) Help / About",
			description: "Version, key bindings, env vars · `?` works too",
			action:      actAbout},
		menuItem{hotkey: 'q',
			title:       "Q) Quit",
			description: "Exit the program",
			action:      actQuit},
	}
	l := list.New(items, menuItemDelegate{}, 0, 0)
	l.Title = "dido-sab"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212")).
		Padding(1, 0)
	return &menuModel{list: l}
}

// menuChoseMsg is emitted when the user picks an item (enter or hotkey)
// so the parent app model can flip screens. Carrying just the action
// keeps the menu unaware of the larger state machine.
type menuChoseMsg struct {
	action menuAction
}

func (m *menuModel) Init() tea.Cmd { return nil }

func (m *menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Leave space for the title + description footer.
		listH := msg.Height - 8
		if listH < 6 {
			listH = 6
		}
		m.list.SetSize(msg.Width, listH)
	case tea.KeyMsg:
		// Hotkeys mirror the bracketed letter in each menu item's
		// title ("A) Align …"), accepted in both cases like
		// usfm-tools so caps lock doesn't break the workflow. `?`
		// stays as the conventional alias for Help/About on top of
		// the alphabetic `h`/`H` shortcut.
		switch msg.String() {
		case "ctrl+c", "q", "Q":
			return m, func() tea.Msg { return menuChoseMsg{action: actQuit} }
		case "a", "A":
			return m, func() tea.Msg { return menuChoseMsg{action: actAlign} }
		case "s", "S":
			return m, func() tea.Msg { return menuChoseMsg{action: actSettings} }
		case "?", "h", "H":
			return m, func() tea.Msg { return menuChoseMsg{action: actAbout} }
		case "enter":
			if it, ok := m.list.SelectedItem().(menuItem); ok {
				return m, func() tea.Msg { return menuChoseMsg{action: it.action} }
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

var (
	menuSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("212")).
				Bold(true)
	menuDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true).
			MarginLeft(4)
	menuBannerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)
)

func (m *menuModel) View() string {
	var sb strings.Builder
	sb.WriteString(menuBannerStyle.Render("dido-sab — scripture audio aligner"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("Force-align audio against USFM text, emit SAB-ready timing files."))
	sb.WriteString("\n\n")
	sb.WriteString(m.list.View())

	// Description footer for the currently-selected item.
	if it, ok := m.list.SelectedItem().(menuItem); ok && it.description != "" {
		sb.WriteString("\n")
		sb.WriteString(menuDescStyle.Render(it.description))
		sb.WriteString("\n")
	}

	if m.notice != "" {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(m.notice))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(
		"↑/↓ or j/k navigate · enter select · letter hotkeys (A/S/H) · q quit"))
	return sb.String()
}
