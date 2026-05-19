package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// planModel shows the discovered books/chapters before the user commits
// to a run. It's a thin shell around a viewport so long batch plans
// (the whole 66-book canon) stay navigable on small terminals.
type planModel struct {
	plans  []bookPlan
	err    error
	cfg    cliFlags
	args   []string // [audio, usfm, out]
	vp     viewport.Model
	width  int
	height int
	ready  bool
}

func newPlanModel(cfg cliFlags, args []string) *planModel {
	m := &planModel{cfg: cfg, args: args}
	plans, err := buildPlan(cfg, args)
	m.plans = plans
	m.err = err
	return m
}

// buildPlan unifies the two entry shapes: batch mode delegates to
// discoverBooks; single mode hand-builds a one-entry bookPlan so the
// rest of the TUI/runner can treat both uniformly.
func buildPlan(c cliFlags, args []string) ([]bookPlan, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("internal: expected 3 positional args, got %d", len(args))
	}
	audio, usfm := args[0], args[1]
	if c.batch {
		plans, err := discoverBooks(audio, usfm, c)
		if err != nil {
			return nil, err
		}
		if len(plans) == 0 {
			return nil, fmt.Errorf("no book pairings under audio=%s usfm=%s", audio, usfm)
		}
		return plans, nil
	}
	mp3s, err := filepath.Glob(filepath.Join(audio, "*.mp3"))
	if err != nil {
		return nil, fmt.Errorf("glob mp3: %w", err)
	}
	sort.Strings(mp3s)
	var chs []chapterTask
	for _, mp3 := range mp3s {
		chap := parseChapterNumber(filepath.Base(mp3))
		if chap < 1 {
			continue
		}
		if c.chapterMin > 0 && chap < c.chapterMin {
			continue
		}
		if c.chapterMax > 0 && chap > c.chapterMax {
			continue
		}
		chs = append(chs, chapterTask{
			Chapter:  chap,
			AudioMP3: mp3,
			Stem:     stemFor(c.nameStyle, c.bookSeq, c.book, chap),
		})
	}
	if len(chs) == 0 {
		return nil, fmt.Errorf("no chapters matched in %s (filter=%q)", audio, c.chapters)
	}
	// Look up the canonical name for nicer rendering when the code is known.
	name := c.book
	for _, b := range canonBooks {
		if b.Code == c.book {
			name = b.Name
			break
		}
	}
	return []bookPlan{{
		book:     book{Seq: c.bookSeq, Code: c.book, Name: name},
		USFM:     usfm,
		AudioDir: audio,
		Chapters: chs,
	}}, nil
}

// planConfirmedMsg is dispatched when the user presses enter on the
// plan screen; the parent transitions to the run screen on receipt.
type planConfirmedMsg struct {
	plans []bookPlan
	cfg   cliFlags
	args  []string
}

// planBackMsg is dispatched on `b`/`esc` to return to the setup form
// without losing field values.
type planBackMsg struct{}

func (m *planModel) Init() tea.Cmd { return nil }

func (m *planModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		h := msg.Height - 8
		if h < 5 {
			h = 5
		}
		m.vp = viewport.New(msg.Width, h)
		m.vp.SetContent(m.renderRows())
		m.ready = true
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc", "b":
			return m, func() tea.Msg { return planBackMsg{} }
		case "enter":
			if m.err != nil || len(m.plans) == 0 {
				return m, nil
			}
			return m, func() tea.Msg {
				return planConfirmedMsg{plans: m.plans, cfg: m.cfg, args: m.args}
			}
		}
	}
	if m.ready {
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *planModel) renderRows() string {
	if m.err != nil {
		return setupErr.Render("⚠ " + m.err.Error())
	}
	if len(m.plans) == 0 {
		return setupErr.Render("No books/chapters discovered.")
	}
	var sb strings.Builder
	for _, p := range m.plans {
		fmt.Fprintf(&sb, "  %s  %-16s  %s\n",
			planCode.Render(padRight(p.Code, 3)),
			padRight(p.Name, 16),
			dimStyle.Render(fmt.Sprintf("%2d chapters", len(p.Chapters))))
	}
	return sb.String()
}

var planCode = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)

func (m *planModel) View() string {
	totalChap := 0
	for _, p := range m.plans {
		totalChap += len(p.Chapters)
	}
	var sb strings.Builder
	sb.WriteString(setupTitle.Render("dido-sab — plan"))
	sb.WriteByte('\n')
	mode := "batch"
	if !m.cfg.batch {
		mode = "single"
	}
	fmt.Fprintf(&sb, "mode: %s · lang: %s · output: %s\n", mode, m.cfg.lang, m.args[2])
	fmt.Fprintf(&sb, "%d book(s), %d chapter(s) planned\n\n", len(m.plans), totalChap)

	if m.ready {
		sb.WriteString(m.vp.View())
	} else {
		sb.WriteString(m.renderRows())
	}

	sb.WriteString("\n\n")
	if m.err != nil {
		sb.WriteString(dimStyle.Render("b/esc to go back · q to quit"))
	} else {
		sb.WriteString(dimStyle.Render(
			"enter to start · b/esc to go back · ↑/↓ to scroll · q to quit"))
	}
	return sb.String()
}

