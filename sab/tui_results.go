package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// resultsModel renders the post-run summary: aggregate counts on top,
// per-book rows below in a scrollable viewport. The book rows are taken
// directly from the run screen's bookState slice so we don't recompute
// anything — just present what the batchModel already tracked.
type resultsModel struct {
	rows      []bookState
	outDir    string
	totalChap int
	doneChap  int
	failChap  int
	elapsed   time.Duration
	vp        viewport.Model
	width     int
	height    int
	ready     bool
}

func newResultsModel(rows []bookState, outDir string, elapsed time.Duration) *resultsModel {
	r := &resultsModel{rows: rows, outDir: outDir, elapsed: elapsed}
	for _, b := range rows {
		r.totalChap += b.NumChap
		r.doneChap += b.DoneChap
		if b.Status == "failed" {
			r.failChap++
		}
	}
	return r
}

func (m *resultsModel) Init() tea.Cmd { return nil }

func (m *resultsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "ctrl+c", "q", "esc", "enter":
			return m, tea.Quit
		}
	}
	if m.ready {
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *resultsModel) renderRows() string {
	var sb strings.Builder
	for _, b := range m.rows {
		var icon, code string
		switch b.Status {
		case "done":
			icon = doneStyle.Render("✓")
			code = doneStyle.Render(padRight(b.Code, codeColWidth))
		case "failed":
			icon = failedStyle.Render("✗")
			code = failedStyle.Render(padRight(b.Code, codeColWidth))
		case "running":
			icon = runningStyle.Render("…")
			code = runningStyle.Render(padRight(b.Code, codeColWidth))
		default:
			icon = dimStyle.Render("·")
			code = dimStyle.Render(padRight(b.Code, codeColWidth))
		}
		fmt.Fprintf(&sb, " %s  %s  %s  %s  %s\n",
			icon, code, padRight(b.Name, nameColWidth),
			dimStyle.Render(fmt.Sprintf("%2d/%-2d ch", b.DoneChap, b.NumChap)),
			dimStyle.Render(b.Elapsed.Truncate(time.Second).String()))
		if b.Err != nil {
			fmt.Fprintf(&sb, "       %s\n",
				failedStyle.Render(truncate(b.Err.Error(), 80)))
		}
	}
	return sb.String()
}

var resultsTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("78")).MarginBottom(1)

func (m *resultsModel) View() string {
	var sb strings.Builder
	title := "dido-sab — done"
	if m.failChap > 0 {
		title = "dido-sab — done (with failures)"
		sb.WriteString(failedStyle.Bold(true).Render(title))
	} else {
		sb.WriteString(resultsTitle.Render(title))
	}
	sb.WriteByte('\n')

	fmt.Fprintf(&sb, "%d/%d chapters aligned across %d book(s)",
		m.doneChap, m.totalChap, len(m.rows))
	if m.failChap > 0 {
		sb.WriteString(failedStyle.Render(fmt.Sprintf("  ·  %d book(s) failed", m.failChap)))
	}
	sb.WriteString(dimStyle.Render(fmt.Sprintf("   total %s", m.elapsed.Truncate(time.Second))))
	sb.WriteByte('\n')
	fmt.Fprintf(&sb, "output: %s\n\n", m.outDir)

	if m.ready {
		sb.WriteString(m.vp.View())
	} else {
		sb.WriteString(m.renderRows())
	}

	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("↑/↓ to scroll · q/enter/esc to quit"))
	return sb.String()
}
