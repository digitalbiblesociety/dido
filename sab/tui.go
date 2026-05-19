package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// isTerminal reports whether f looks like an interactive TTY. The TUI is
// only useful with cursor positioning and ANSI styling.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// plainSink is the non-TTY progress consumer: one line per event, with
// just enough detail to follow along in CI logs or `tee` output.
type plainSink struct {
	mu sync.Mutex
	w  io.Writer
}

func newPlainSink(w io.Writer) progressSink { return &plainSink{w: w} }

func (p *plainSink) Send(e progressEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch e.Kind {
	case "batch_start":
		fmt.Fprintf(p.w, "Aligning %d books…\n", e.Total)
	case "book_start":
		fmt.Fprintf(p.w, "→ %s %-13s (%d chapters)\n", e.Book.Code, e.Book.Name, e.NumChap)
	case "chapter_done":
		fmt.Fprintf(p.w, "    ✓ ch %d (%d fragments, %s)\n", e.Chap, e.Frags, e.Elapsed.Truncate(time.Millisecond))
	case "chapter_resume":
		fmt.Fprintf(p.w, "    ↷ ch %d (resume)\n", e.Chap)
	case "chapter_err":
		fmt.Fprintf(p.w, "    ✗ ch %d: %v\n", e.Chap, e.Err)
	case "book_done":
		fmt.Fprintf(p.w, "  ✓ %s done in %s\n", e.Book.Code, e.Elapsed.Truncate(time.Second))
	case "book_failed":
		if e.Err != nil {
			fmt.Fprintf(p.w, "  ✗ %s failed: %v\n", e.Book.Code, e.Err)
		} else {
			fmt.Fprintf(p.w, "  ✗ %s failed (see chapter errors above)\n", e.Book.Code)
		}
	case "batch_done":
		fmt.Fprintf(p.w, "Done in %s.\n", e.Elapsed.Truncate(time.Second))
	}
}

func (p *plainSink) Close() {}

// tuiSink wraps a running tea.Program. Send marshals events into tea.Msg
// values so the model can update; Close signals batch completion.
type tuiSink struct {
	prog *tea.Program
}

func newTUISink(plans []bookPlan) (progressSink, <-chan struct{}) {
	m := newBatchModel(plans)
	prog := tea.NewProgram(m)
	done := make(chan struct{})
	go func() {
		_, _ = prog.Run()
		close(done)
	}()
	return &tuiSink{prog: prog}, done
}

func (t *tuiSink) Send(e progressEvent) {
	t.prog.Send(e)
}

func (t *tuiSink) Close() {
	// Give the model a beat to render the final batch_done before quit.
	time.Sleep(150 * time.Millisecond)
	t.prog.Send(tea.Quit())
}

// bookState tracks per-book progress for the TUI.
type bookState struct {
	book
	NumChap     int
	DoneChap    int
	ResumedChap int
	Status      string // "pending" "running" "done" "failed" "skipped"
	Elapsed     time.Duration
	Err         error
	StartedAt   time.Time
}

// batchModel is the Bubble Tea model for `-batch`. It owns one bookState
// per planned book and renders a windowed list. With chapter-level
// parallelism multiple books are typically "running" at the same time,
// so the window centres on the first running row rather than tracking
// a single active index.
type batchModel struct {
	plans     []bookState
	idx       map[int]int // seq → index in plans
	completed int
	failed    int
	total     int
	spin      spinner.Model
	bar       progress.Model
	width     int
	height    int
	startedAt time.Time
	elapsed   time.Duration
	done      bool
}

func newBatchModel(plans []bookPlan) *batchModel {
	rows := make([]bookState, len(plans))
	idx := make(map[int]int, len(plans))
	for i, p := range plans {
		rows[i] = bookState{book: p.book, NumChap: len(p.Chapters), Status: "pending"}
		idx[p.Seq] = i
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

	bar := progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage())
	bar.Width = 20

	return &batchModel{
		plans:     rows,
		idx:       idx,
		total:     len(plans),
		spin:      sp,
		bar:       bar,
		startedAt: time.Now(),
	}
}

// runningCount reports how many books are currently in flight. Used by
// the header so the user can see the parallelism level.
func (m *batchModel) runningCount() int {
	n := 0
	for _, p := range m.plans {
		if p.Status == "running" {
			n++
		}
	}
	return n
}

// firstRunning returns the index of the lowest-seq book currently
// running, or -1 if none. Centring the viewport on this index keeps
// the most "in-order" running book in view as parallel workers chew
// through the canon.
func (m *batchModel) firstRunning() int {
	for i, p := range m.plans {
		if p.Status == "running" {
			return i
		}
	}
	return -1
}

// tickMsg is the per-second wallclock pulse used to keep the elapsed
// counter and the active-book elapsed in sync with real time.
type tickMsg time.Time

func (m *batchModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, tickEvery())
}

func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *batchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if msg.Width > 30 {
			m.bar.Width = msg.Width / 3
			if m.bar.Width > 40 {
				m.bar.Width = 40
			}
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tickMsg:
		m.elapsed = time.Since(m.startedAt)
		return m, tickEvery()
	case progressEvent:
		m.applyEvent(msg)
		if msg.Kind == "batch_done" {
			m.done = true
		}
	}
	return m, nil
}

func (m *batchModel) applyEvent(e progressEvent) {
	switch e.Kind {
	case "batch_start":
		// Total already known from constructor; refresh just in case.
		m.total = e.Total
	case "book_start":
		i, ok := m.idx[e.Book.Seq]
		if !ok {
			return
		}
		m.plans[i].Status = "running"
		m.plans[i].DoneChap = 0
		m.plans[i].ResumedChap = 0
		m.plans[i].NumChap = e.NumChap
		m.plans[i].StartedAt = time.Now()
	case "chapter_done":
		i, ok := m.idx[e.Book.Seq]
		if !ok {
			return
		}
		m.plans[i].DoneChap++
	case "chapter_resume":
		i, ok := m.idx[e.Book.Seq]
		if !ok {
			return
		}
		m.plans[i].DoneChap++
		m.plans[i].ResumedChap++
	case "chapter_err":
		i, ok := m.idx[e.Book.Seq]
		if !ok {
			return
		}
		m.plans[i].Err = e.Err
	case "book_done":
		i, ok := m.idx[e.Book.Seq]
		if !ok {
			return
		}
		m.plans[i].Status = "done"
		m.plans[i].Elapsed = e.Elapsed
		m.completed++
	case "book_failed":
		i, ok := m.idx[e.Book.Seq]
		if !ok {
			return
		}
		m.plans[i].Status = "failed"
		m.plans[i].Elapsed = e.Elapsed
		if e.Err != nil && m.plans[i].Err == nil {
			m.plans[i].Err = e.Err
		}
		m.failed++
	}
}

// view styles.
var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	doneStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	failedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	runningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	codeColWidth  = 3
	nameColWidth  = 16
)

func (m *batchModel) View() string {
	if m.total == 0 {
		return "No books in plan.\n"
	}

	var sb strings.Builder

	// Header.
	sb.WriteString(headerStyle.Render("dido-sab batch") + "  ")
	sb.WriteString(fmt.Sprintf("%d/%d done", m.completed, m.total))
	if r := m.runningCount(); r > 0 {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  %d running", r)))
	}
	if m.failed > 0 {
		sb.WriteString(failedStyle.Render(fmt.Sprintf("  %d failed", m.failed)))
	}
	sb.WriteString(dimStyle.Render(fmt.Sprintf("   elapsed %s", m.elapsed.Truncate(time.Second))))
	if eta := m.eta(); eta > 0 {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("   ETA %s", eta.Truncate(time.Second))))
	}
	sb.WriteString("\n\n")

	// Windowed book list (centred on active book).
	rowsToShow := m.rowBudget()
	start, end := m.window(rowsToShow)
	for i := start; i < end; i++ {
		sb.WriteString(m.renderRow(i))
		sb.WriteByte('\n')
	}

	if m.done {
		sb.WriteString("\n" + dimStyle.Render("Press q or ctrl+c to exit."))
	} else {
		sb.WriteString("\n" + dimStyle.Render("q to quit"))
	}
	return sb.String()
}

// rowBudget returns the number of book rows we can fit given the current
// terminal height, with a sensible floor for very small windows.
func (m *batchModel) rowBudget() int {
	if m.height <= 0 {
		return 12
	}
	// Header (2 lines) + footer (2 lines) reserved.
	budget := m.height - 4
	if budget < 5 {
		budget = 5
	}
	if budget > len(m.plans) {
		budget = len(m.plans)
	}
	return budget
}

// window returns [start, end) indices into m.plans to render, centred
// on the first running book when one exists, otherwise anchored on
// the first pending row (or the last completed when everything's done).
// With chapter-level parallelism multiple books are typically running
// at once; centring on the lowest-seq running book keeps the canonical
// order legible.
func (m *batchModel) window(budget int) (int, int) {
	anchor := m.firstRunning()
	if anchor < 0 {
		anchor = 0
		for i, p := range m.plans {
			if p.Status == "pending" {
				anchor = i
				break
			}
			if p.Status == "done" || p.Status == "failed" {
				anchor = i
			}
		}
	}
	half := budget / 2
	start := anchor - half
	if start < 0 {
		start = 0
	}
	end := start + budget
	if end > len(m.plans) {
		end = len(m.plans)
		start = end - budget
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func (m *batchModel) renderRow(i int) string {
	p := m.plans[i]
	var icon, codeRendered, nameRendered, status string
	switch p.Status {
	case "done":
		icon = doneStyle.Render("✓")
		codeRendered = doneStyle.Render(padRight(p.Code, codeColWidth))
		nameRendered = padRight(p.Name, nameColWidth)
		status = dimStyle.Render(fmt.Sprintf("%d ch · %s", p.NumChap, p.Elapsed.Truncate(time.Second)))
	case "failed":
		icon = failedStyle.Render("✗")
		codeRendered = failedStyle.Render(padRight(p.Code, codeColWidth))
		nameRendered = padRight(p.Name, nameColWidth)
		if p.Err != nil {
			status = failedStyle.Render(truncate(p.Err.Error(), 60))
		} else {
			status = failedStyle.Render(fmt.Sprintf("%d/%d", p.DoneChap, p.NumChap))
		}
	case "running":
		icon = runningStyle.Render(m.spin.View())
		codeRendered = runningStyle.Render(padRight(p.Code, codeColWidth))
		nameRendered = runningStyle.Render(padRight(p.Name, nameColWidth))
		ratio := 0.0
		if p.NumChap > 0 {
			ratio = float64(p.DoneChap) / float64(p.NumChap)
		}
		status = fmt.Sprintf("ch %d/%d  %s  %s",
			p.DoneChap, p.NumChap,
			m.bar.ViewAs(ratio),
			dimStyle.Render(time.Since(p.StartedAt).Truncate(time.Second).String()))
	default: // pending
		icon = dimStyle.Render(" ")
		codeRendered = dimStyle.Render(padRight(p.Code, codeColWidth))
		nameRendered = dimStyle.Render(padRight(p.Name, nameColWidth))
		status = dimStyle.Render(fmt.Sprintf("%d ch", p.NumChap))
	}
	return fmt.Sprintf(" %s  %s  %s  %s", icon, codeRendered, nameRendered, status)
}

// eta returns a rough estimate of the time remaining based on average
// per-book time so far. Returns 0 before any book has finished.
func (m *batchModel) eta() time.Duration {
	if m.completed == 0 {
		return 0
	}
	per := m.elapsed / time.Duration(m.completed)
	remaining := m.total - m.completed - m.failed
	if remaining <= 0 {
		return 0
	}
	return per * time.Duration(remaining)
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

func truncate(s string, w int) string {
	if len(s) <= w {
		return s
	}
	return s[:w-1] + "…"
}
