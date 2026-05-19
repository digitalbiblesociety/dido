package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/digitalbiblesociety/dido/internal/config"
	"github.com/digitalbiblesociety/dido/internal/pipeline"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/userconfig"
)

// screen identifies which sub-model owns the foreground.
type screen int

const (
	screenMenu screen = iota
	screenAbout
	screenSetup
	screenSettings
	screenPlan
	screenRun
	screenResults
)

// appModel is the top-level Bubble Tea model. It owns one sub-model per
// screen and forwards messages to whichever is active. The flow is
// menu → setup → plan → run → results, with settings reachable from
// menu or setup. A shared *tea.Program reference lets the run-screen
// goroutine push `progressEvent`s through the same event loop.
type appModel struct {
	prog    *tea.Program
	current screen
	// settingsReturnTo remembers which screen invoked settings so the
	// Save / Cancel handlers pop back to the right place.
	settingsReturnTo screen

	menu     *menuModel
	about    *aboutModel
	setup    *setupModel
	settings *settingsModel
	plan     *planModel
	run      *batchModel
	results  *resultsModel

	userCfg *userconfig.Config
	cfg     cliFlags
	args    []string // [audio, usfm, out]
	plans   []bookPlan
	started time.Time

	// startScreen is the screen the model boots into. The sab/ entry
	// point uses screenMenu; the legacy flags-only entry point can
	// pass screenSetup to skip the menu when the user supplied flags.
	startScreen screen

	width  int
	height int
}

func newAppModel(initial cliFlags, posArgs []string, userCfg *userconfig.Config) *appModel {
	if userCfg == nil {
		userCfg = &userconfig.Config{NameStyle: "sab", OutputFolder: "./out"}
	}
	return &appModel{
		current:     screenMenu,
		startScreen: screenMenu,
		menu:        newMenuModel(),
		setup:       newSetupModel(initial, posArgs, userCfg),
		userCfg:     userCfg,
	}
}

func (m *appModel) Init() tea.Cmd {
	switch m.current {
	case screenMenu:
		return m.menu.Init()
	case screenSetup:
		return m.setup.Init()
	}
	return nil
}

// batchFinishedMsg is dispatched once `batch_done` has propagated to
// the run-screen model. The parent uses it to flip to results.
type batchFinishedMsg struct {
	rows    []bookState
	elapsed time.Duration
}

func (m *appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Window-size and global-quit messages route to every screen so the
	// sub-models can lay themselves out.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Forward to all sub-models that exist so a transition lands on
		// an already-laid-out screen.
		if m.menu != nil {
			m.menu.Update(msg)
		}
		if m.about != nil {
			m.about.Update(msg)
		}
		if m.setup != nil {
			m.setup.Update(msg)
		}
		if m.settings != nil {
			m.settings.Update(msg)
		}
		if m.plan != nil {
			m.plan.Update(msg)
		}
		if m.run != nil {
			m.run.Update(msg)
		}
		if m.results != nil {
			m.results.Update(msg)
		}
		return m, nil

	case menuChoseMsg:
		switch msg.action {
		case actAlign:
			m.current = screenSetup
			return m, nil
		case actSettings:
			m.settings = newSettingsModel(m.userCfg)
			m.settingsReturnTo = screenMenu
			m.current = screenSettings
			if m.width > 0 {
				m.settings.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			}
			return m, m.settings.Init()
		case actAbout:
			m.about = newAboutModel()
			m.current = screenAbout
			if m.width > 0 {
				m.about.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			}
			return m, m.about.Init()
		case actQuit:
			return m, tea.Quit
		}

	case aboutBackMsg:
		m.current = screenMenu
		return m, nil

	case openSettingsMsg:
		m.settings = newSettingsModel(msg.cfg)
		m.settingsReturnTo = screenSetup
		m.current = screenSettings
		if m.width > 0 {
			m.settings.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		return m, m.settings.Init()

	case settingsSavedMsg:
		// Adopt the (possibly mutated) cfg pointer. If the user came
		// from the setup screen, forward the msg so it can refill any
		// empty fields. Either way, pop back to the screen we entered
		// settings from.
		m.userCfg = msg.cfg
		if m.settingsReturnTo == screenSetup && m.setup != nil {
			m.setup.Update(msg)
		}
		m.current = m.returnScreen()
		return m, nil

	case settingsCancelledMsg:
		if m.settingsReturnTo == screenSetup && m.setup != nil {
			m.setup.Update(msg)
		}
		m.current = m.returnScreen()
		return m, nil

	case setupBackMsg:
		// Setup esc-without-input now returns to the menu rather than
		// quitting, so the user can browse other options without losing
		// the program.
		m.current = screenMenu
		return m, nil

	case setupDoneMsg:
		m.cfg = msg.cfg
		m.args = msg.posArgs
		m.plan = newPlanModel(msg.cfg, msg.posArgs)
		m.current = screenPlan
		var cmd tea.Cmd
		// Resend the last window size so the viewport can size itself.
		if m.width > 0 {
			_, cmd = m.plan.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		return m, cmd

	case planBackMsg:
		m.current = screenSetup
		return m, nil

	case planConfirmedMsg:
		m.plans = msg.plans
		m.cfg = msg.cfg
		m.args = msg.args
		m.run = newBatchModel(msg.plans)
		m.current = screenRun
		m.started = time.Now()
		if m.width > 0 {
			m.run.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		// Kick off the alignment worker. It pushes progressEvent values
		// through prog.Send so they show up on this same Update loop.
		go m.runAlignment()
		return m, m.run.Init()

	case progressEvent:
		// Drive the run-screen counters first.
		var cmd tea.Cmd
		if m.run != nil {
			_, cmd = m.run.Update(msg)
		}
		// On batch_done, schedule the results transition. The brief
		// delay lets the user see the final 100%/✓ frame before the
		// screen swaps under them.
		if msg.Kind == "batch_done" && m.run != nil {
			finished := batchFinishedMsg{rows: m.run.plans, elapsed: msg.Elapsed}
			return m, tea.Batch(cmd, tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
				return finished
			}))
		}
		return m, cmd

	case batchFinishedMsg:
		outDir := ""
		if len(m.args) >= 3 {
			outDir = m.args[2]
		}
		m.results = newResultsModel(msg.rows, outDir, msg.elapsed)
		m.current = screenResults
		if m.width > 0 {
			m.results.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		return m, m.results.Init()
	}

	// Dispatch the message to the active screen.
	var cmd tea.Cmd
	switch m.current {
	case screenMenu:
		if m.menu != nil {
			_, cmd = m.menu.Update(msg)
		}
	case screenAbout:
		if m.about != nil {
			_, cmd = m.about.Update(msg)
		}
	case screenSetup:
		_, cmd = m.setup.Update(msg)
	case screenSettings:
		if m.settings != nil {
			_, cmd = m.settings.Update(msg)
		}
	case screenPlan:
		if m.plan != nil {
			_, cmd = m.plan.Update(msg)
		}
	case screenRun:
		if m.run != nil {
			_, cmd = m.run.Update(msg)
		}
	case screenResults:
		if m.results != nil {
			_, cmd = m.results.Update(msg)
		}
	}
	return m, cmd
}

func (m *appModel) View() string {
	switch m.current {
	case screenMenu:
		if m.menu != nil {
			return m.menu.View()
		}
	case screenAbout:
		if m.about != nil {
			return m.about.View()
		}
	case screenSetup:
		return m.setup.View()
	case screenSettings:
		if m.settings != nil {
			return m.settings.View()
		}
	case screenPlan:
		if m.plan != nil {
			return m.plan.View()
		}
	case screenRun:
		if m.run != nil {
			return m.run.View()
		}
	case screenResults:
		if m.results != nil {
			return m.results.View()
		}
	}
	return ""
}

// returnScreen picks the screen the settings sub-flow should pop back
// to. We remember the entry point in settingsReturnTo because settings
// is reachable from both the main menu and the setup form.
func (m *appModel) returnScreen() screen {
	if m.settingsReturnTo == screenMenu {
		return screenMenu
	}
	return screenSetup
}

// runAlignment is the worker goroutine for the run screen. It builds the
// pipeline/runtime config the same way runBatch() does, then drives
// runBatchWorker with a sink that funnels every event back through
// prog.Send so the parent Update loop animates the progress display.
//
// When the user hasn't passed -workers on the CLI, fall back to the
// persisted DIDO_BATCH_WORKERS / Settings value so the TUI honours the
// same parallelism preference batches launched from a shell would.
func (m *appModel) runAlignment() {
	cfg := m.cfg
	if cfg.workers == 0 && m.userCfg != nil {
		cfg.workers = m.userCfg.Workers
	}
	rc := config.Default()
	rc.SetGranularity(1)
	rc.SetTTS(1)
	tc := pipeline.TaskConfig{
		Language:   cfg.lang,
		TextFormat: text.FormatParsed,
	}
	sink := &programSink{prog: m.prog}
	runBatchWorker(m.plans, m.args[2], cfg, tc, rc, sink)
}

// programSink ferries progressEvents from the worker goroutine to the
// tea.Program so the parent Update loop sees them.
type programSink struct {
	prog *tea.Program
}

func (p *programSink) Send(e progressEvent) {
	if p.prog != nil {
		p.prog.Send(e)
	}
}

func (p *programSink) Close() {}

// runTUI is the entry point used by main() when no positional args are
// given on a TTY. It seeds the setup form with whatever flags the user
// did pass plus the persisted user config (env vars + config.env), then
// runs the alternate-screen Bubble Tea program.
func runTUI(initial cliFlags) error {
	userCfg, err := userconfig.Load()
	if err != nil {
		// Loading defaults is best-effort: a corrupt or unreadable
		// config shouldn't block the TUI. Surface the error to the
		// user via the settings notice once the screen mounts.
		userCfg = &userconfig.Config{NameStyle: "sab", OutputFolder: "./out"}
	}
	args := []string{"", "", ""}
	app := newAppModel(initial, args, userCfg)
	prog := tea.NewProgram(app, tea.WithAltScreen())
	app.prog = prog
	_, err = prog.Run()
	return err
}

