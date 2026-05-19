package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/digitalbiblesociety/dido/internal/userconfig"
)

// TestSetupRenders confirms the setup form constructs cleanly and the
// initial render contains the expected labels (no panics on a fresh
// model, all required rows visible in batch mode).
func TestSetupRenders(t *testing.T) {
	m := newSetupModel(cliFlags{lang: "tha"}, []string{"", "", ""}, nil)
	out := m.View()
	for _, want := range []string{"Language", "Mode", "Audio", "USFM", "Output", "Start"} {
		if !strings.Contains(out, want) {
			t.Errorf("setup view missing %q\n---\n%s", want, out)
		}
	}
	// Book / Book seq rows are hidden in batch mode by default.
	if strings.Contains(out, "Book seq") {
		t.Error("batch mode should hide 'Book seq' row")
	}
}

// TestSetupSingleModeShowsBookRows confirms the single-mode heuristic
// (book code set in flags → start in single mode) reveals the
// single-only Book / Book seq rows in the form.
func TestSetupSingleModeShowsBookRows(t *testing.T) {
	m := newSetupModel(cliFlags{lang: "tha", book: "ISA", bookSeq: 23},
		[]string{"", "", ""}, nil)
	if m.batch {
		t.Fatal("expected single mode default when -book is set")
	}
	out := m.View()
	if !strings.Contains(out, "Book seq") {
		t.Errorf("single mode should show 'Book seq' row\n---\n%s", out)
	}
}

// TestPlanModelHandlesEmptyDir confirms the plan model surfaces an
// error string when there's nothing to align rather than panicking.
func TestPlanModelHandlesEmptyDir(t *testing.T) {
	tmp := t.TempDir()
	cfg := cliFlags{lang: "tha", batch: true, nameStyle: "sab"}
	m := newPlanModel(cfg, []string{tmp, tmp, tmp})
	if m.err == nil {
		t.Error("expected error for empty audio/USFM dirs")
	}
	_ = m.View() // should not panic
}

// TestResultsModelRenders confirms the post-run summary builds and
// surfaces both success and failure rows.
func TestResultsModelRenders(t *testing.T) {
	rows := []bookState{
		{book: book{Seq: 1, Code: "GEN", Name: "Genesis"}, NumChap: 50, DoneChap: 50,
			Status: "done", Elapsed: 12 * time.Minute},
		{book: book{Seq: 2, Code: "EXO", Name: "Exodus"}, NumChap: 40, DoneChap: 17,
			Status: "failed", Err: errString("audio decode: ffmpeg exited 1"),
			Elapsed: 3 * time.Minute},
	}
	m := newResultsModel(rows, "./out", 15*time.Minute)
	out := m.View()
	for _, want := range []string{"GEN", "EXO", "Genesis", "Exodus", "failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("results view missing %q\n---\n%s", want, out)
		}
	}
}

// errString is a tiny error type so the result-test fixture can hold
// a deterministic error message without pulling in stdlib errors.
type errString string

func (e errString) Error() string { return string(e) }

// TestAppRoutesMessages drives the app model through a setup→plan
// transition to make sure the routing logic doesn't drop the message
// or panic on missing sub-models.
func TestAppRoutesMessages(t *testing.T) {
	app := newAppModel(cliFlags{}, []string{"", "", ""}, nil)
	app.Init()
	if app.current != screenMenu {
		t.Fatalf("expected initial screen = screenMenu, got %d", app.current)
	}
	// Window-size to all screens.
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	// Fake a setupDoneMsg directly.
	tmp := t.TempDir()
	app.Update(setupDoneMsg{
		cfg:     cliFlags{lang: "tha", batch: true, nameStyle: "sab"},
		posArgs: []string{tmp, tmp, tmp},
	})
	if app.current != screenPlan {
		t.Fatalf("expected screenPlan after setupDoneMsg, got %d", app.current)
	}
	if app.plan == nil {
		t.Fatal("plan model not constructed")
	}
	// planBack returns to setup.
	app.Update(planBackMsg{})
	if app.current != screenSetup {
		t.Fatalf("expected screenSetup after planBackMsg, got %d", app.current)
	}
}

// TestSetupPrefillsFromUserConfig confirms that fields the user hasn't
// explicitly overridden via flags get their starting value from the
// persisted userconfig (env + config.env).
func TestSetupPrefillsFromUserConfig(t *testing.T) {
	cfg := &userconfig.Config{
		AudioRoot:    "/srv/audio",
		USFMRoot:     "/srv/usfm",
		OutputFolder: "/srv/out",
		Lang:         "tha",
		NameStyle:    "sab",
	}
	m := newSetupModel(cliFlags{}, []string{"", "", ""}, cfg)
	out := m.View()
	for _, want := range []string{"/srv/audio", "/srv/usfm", "/srv/out", "tha"} {
		if !strings.Contains(out, want) {
			t.Errorf("setup view missing prefilled %q\n---\n%s", want, out)
		}
	}
}

// TestSettingsRoundtrip drives the settings sub-model: change a field,
// hit ctrl+s, and confirm the cfg pointer reflects the change so the
// parent and setup screens see the same updated defaults.
func TestSettingsRoundtrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &userconfig.Config{Lang: "eng", NameStyle: "sab"}
	m := newSettingsModel(cfg)
	// Find the Language row and overwrite its value.
	for i := range m.rows {
		if m.rows[i].label == "Language" {
			m.rows[i].input.SetValue("tha")
		}
	}
	// Move focus to the Save button and press enter.
	for i := range m.rows {
		if m.rows[i].kind == kSubmit {
			m.focus = i
			break
		}
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd from Save (settingsSavedMsg)")
	}
	if cfg.Lang != "tha" {
		t.Errorf("cfg.Lang = %q after save, want %q", cfg.Lang, "tha")
	}
}

// TestPickerCancelledMsgIsHandledCleanly: when the picker says the
// user cancelled, the setup form should record a notice rather than
// crash or change any field values.
func TestPickerCancelledMsgIsHandledCleanly(t *testing.T) {
	m := newSetupModel(cliFlags{}, []string{"", "", ""}, nil)
	before := ""
	for _, r := range m.rows {
		if r.label == "Audio" {
			before = r.input.Value()
		}
	}
	m.Update(pickerCancelledMsg{field: "Audio"})
	for _, r := range m.rows {
		if r.label == "Audio" && r.input.Value() != before {
			t.Errorf("cancelled picker should not touch Audio (was %q, now %q)",
				before, r.input.Value())
		}
	}
	if m.notice == "" {
		t.Error("expected notice after picker cancellation")
	}
}

// TestMenuActions drives each menu action through the app to confirm
// the screen switches arrive at the expected sub-model.
func TestMenuActions(t *testing.T) {
	tests := []struct {
		action menuAction
		want   screen
		assert func(*testing.T, *appModel)
	}{
		{actAlign, screenSetup, func(t *testing.T, a *appModel) {
			if a.setup == nil {
				t.Error("setup model nil after actAlign")
			}
		}},
		{actSettings, screenSettings, func(t *testing.T, a *appModel) {
			if a.settings == nil {
				t.Error("settings model nil after actSettings")
			}
			if a.settingsReturnTo != screenMenu {
				t.Errorf("settingsReturnTo = %d, want screenMenu", a.settingsReturnTo)
			}
		}},
		{actAbout, screenAbout, func(t *testing.T, a *appModel) {
			if a.about == nil {
				t.Error("about model nil after actAbout")
			}
		}},
	}
	for _, tc := range tests {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		app := newAppModel(cliFlags{}, []string{"", "", ""}, nil)
		app.Init()
		app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
		app.Update(menuChoseMsg{action: tc.action})
		if app.current != tc.want {
			t.Errorf("action %d: screen = %d, want %d", tc.action, app.current, tc.want)
		}
		tc.assert(t, app)
	}
}

// TestAboutBackReturnsToMenu confirms the About screen pops back when
// the user presses esc/q/enter.
func TestAboutBackReturnsToMenu(t *testing.T) {
	app := newAppModel(cliFlags{}, []string{"", "", ""}, nil)
	app.Init()
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	app.Update(menuChoseMsg{action: actAbout})
	app.Update(aboutBackMsg{})
	if app.current != screenMenu {
		t.Errorf("after aboutBackMsg: screen = %d, want screenMenu", app.current)
	}
}

// TestSettingsFromMenuReturnsToMenu confirms the settings sub-model
// pops back to the menu (not setup) when entered from the menu.
func TestSettingsFromMenuReturnsToMenu(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	app := newAppModel(cliFlags{}, []string{"", "", ""}, nil)
	app.Init()
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	app.Update(menuChoseMsg{action: actSettings})
	app.Update(settingsCancelledMsg{cfg: app.userCfg})
	if app.current != screenMenu {
		t.Errorf("settings from menu should return to menu, got %d", app.current)
	}
}

// TestSetupBackReturnsToMenu verifies that setupBackMsg pops back to
// the main menu instead of quitting.
func TestSetupBackReturnsToMenu(t *testing.T) {
	app := newAppModel(cliFlags{}, []string{"", "", ""}, nil)
	app.Init()
	app.Update(menuChoseMsg{action: actAlign})
	app.Update(setupBackMsg{})
	if app.current != screenMenu {
		t.Errorf("after setupBackMsg: screen = %d, want screenMenu", app.current)
	}
}

// TestSetupOmitsOutputRow confirms the form no longer exposes an
// editable Output field after the move-to-Settings change.
func TestSetupOmitsOutputRow(t *testing.T) {
	m := newSetupModel(cliFlags{lang: "tha"}, []string{"", "", ""}, nil)
	for _, r := range m.rows {
		if r.label == "Output" {
			t.Error("setup form should no longer contain an editable Output row")
		}
	}
}

// TestSetupShowsOutputFromConfig confirms the read-only Output echo at
// the top of the view reflects userCfg.OutputFolder.
func TestSetupShowsOutputFromConfig(t *testing.T) {
	cfg := &userconfig.Config{OutputFolder: "/srv/dido-out"}
	m := newSetupModel(cliFlags{}, []string{"", "", ""}, cfg)
	out := m.View()
	if !strings.Contains(out, "/srv/dido-out") {
		t.Errorf("view should echo Output from config, got\n---\n%s", out)
	}
}

// TestReadBibleConf round-trips a minimal conf.toml fixture and
// verifies the ISO + lang + language keys are all extracted.
func TestReadBibleConf(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conf.toml")
	body := `# leading comment
bible_abbr = 'THAKJV'
language = 'Thai'
iso = 'tha'
lang = "tha"
font = 'Thai-Light.ttf'
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c, ok := readBibleConf(path)
	if !ok {
		t.Fatal("expected ok=true for a populated conf.toml")
	}
	if c.ISO != "tha" || c.Lang != "tha" || c.Language != "Thai" {
		t.Errorf("conf = %+v; want iso=tha lang=tha language=Thai", c)
	}
}

// TestLanguageFromBibleFolderPicksISOWhenLangAbsent guards the order
// of preference inside the auto-fill helper: explicit lang first, ISO
// as fallback.
func TestLanguageFromBibleFolderPicksISOWhenLangAbsent(t *testing.T) {
	dir := t.TempDir()
	body := "language = 'Thai'\niso = 'tha'\n"
	if err := os.WriteFile(filepath.Join(dir, "conf.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := languageFromBibleFolder(dir)
	if !ok || got != "tha" {
		t.Errorf("language = %q, %v; want \"tha\", true", got, ok)
	}
}

// TestPickerNormalisesUSFMToUSFMSubdir verifies that picking a Bible
// root that has a usfm/ subdir collapses the field value to that
// subdir (so the aligner finds .usfm files immediately), while a flat
// Bible folder is left as-is.
func TestPickerNormalisesUSFMToUSFMSubdir(t *testing.T) {
	root := t.TempDir()
	usfmDir := filepath.Join(root, "usfm")
	if err := os.MkdirAll(usfmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "conf.toml"),
		[]byte("iso = 'tha'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newSetupModel(cliFlags{}, []string{"", "", ""}, nil)
	m.Update(pickerPickedMsg{field: "USFM", path: root, kind: "usfm", bibleID: "THAKJV"})

	var got string
	for _, r := range m.rows {
		if r.label == "USFM" {
			got = r.input.Value()
		}
	}
	if got != usfmDir {
		t.Errorf("USFM field = %q; want normalised to %q", got, usfmDir)
	}
	// Language auto-fill from conf.toml should have landed too.
	var langVal string
	for _, r := range m.rows {
		if r.label == "Language" {
			langVal = r.input.Value()
		}
	}
	if langVal != "tha" {
		t.Errorf("Language = %q; want \"tha\" (from conf.toml)", langVal)
	}
}

// TestPickerKeepsAudioPathAsPicked confirms the audio kind doesn't
// trigger the usfm/-subdir normalisation.
func TestPickerKeepsAudioPathAsPicked(t *testing.T) {
	root := t.TempDir()
	// Make a usfm/ subdir to ensure the normaliser isn't tempted to
	// rewrite the audio path.
	if err := os.MkdirAll(filepath.Join(root, "usfm"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newSetupModel(cliFlags{}, []string{"", "", ""}, nil)
	m.Update(pickerPickedMsg{field: "Audio", path: root, kind: "audio"})
	for _, r := range m.rows {
		if r.label == "Audio" && r.input.Value() != root {
			t.Errorf("Audio field = %q; want %q (no rewrite for audio kind)",
				r.input.Value(), root)
		}
	}
}

// TestListBibleFolders smoke-tests the directory enumerator: hidden
// + underscore-prefixed folders are dropped, real ones survive.
func TestListBibleFolders(t *testing.T) {
	root := t.TempDir()
	for _, sub := range []string{"THAKJV", "ENGKJV", "_staging", ".hidden"} {
		if err := os.Mkdir(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := listBibleFolders(root)
	if err != nil {
		t.Fatalf("listBibleFolders: %v", err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.id] = true
	}
	if !got["THAKJV"] || !got["ENGKJV"] {
		t.Errorf("expected THAKJV + ENGKJV, got %v", got)
	}
	if got["_staging"] || got[".hidden"] {
		t.Errorf("hidden / underscore folders should be filtered, got %v", got)
	}
}

// TestSubmitRequiresOutputInSettings verifies that hitting Start when
// userCfg.OutputFolder is empty produces an actionable error pointing
// the user at Settings, instead of silently writing to cwd.
func TestSubmitRequiresOutputInSettings(t *testing.T) {
	tmp := t.TempDir()
	cfg := &userconfig.Config{} // OutputFolder empty
	m := newSetupModel(cliFlags{}, []string{"", "", ""}, cfg)
	m.setFieldValue("Language", "tha")
	m.setFieldValue("Audio", tmp)
	m.setFieldValue("USFM", tmp)
	if cmd := m.submit(); cmd != nil {
		t.Fatal("expected submit() to return nil and stash err when output is empty")
	}
	if !strings.Contains(m.err, "Settings") {
		t.Errorf("err = %q; want it to mention Settings", m.err)
	}
}

// TestMenuRenders confirms the menu screen's view contains every
// menu item title.
func TestMenuRenders(t *testing.T) {
	m := newMenuModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	out := m.View()
	for _, want := range []string{
		"Align scripture audio", "Settings", "Help", "Quit",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("menu view missing %q\n---\n%s", want, out)
		}
	}
}

// TestMenuHotkeysAreCaseInsensitive mirrors the usfm-tools convention:
// the bracketed letter in each item's title works in both cases so
// caps lock doesn't break the workflow. The `?` legacy alias is also
// accepted for the About item.
func TestMenuHotkeysAreCaseInsensitive(t *testing.T) {
	cases := []struct {
		key  string
		want menuAction
	}{
		{"a", actAlign}, {"A", actAlign},
		{"s", actSettings}, {"S", actSettings},
		{"h", actAbout}, {"H", actAbout}, {"?", actAbout},
		{"q", actQuit}, {"Q", actQuit},
	}
	for _, tc := range cases {
		m := newMenuModel()
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)})
		if cmd == nil {
			t.Errorf("key %q produced no cmd", tc.key)
			continue
		}
		msg, ok := cmd().(menuChoseMsg)
		if !ok {
			t.Errorf("key %q: cmd did not produce menuChoseMsg", tc.key)
			continue
		}
		if msg.action != tc.want {
			t.Errorf("key %q: action %d, want %d", tc.key, msg.action, tc.want)
		}
	}
}
