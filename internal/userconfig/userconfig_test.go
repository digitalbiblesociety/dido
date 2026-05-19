package userconfig

import (
	"os"
	"path/filepath"
	"testing"
)

// withConfigDir points XDG_CONFIG_HOME at a temp dir so the test
// doesn't touch the real config file.
func withConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// Clear DIDO_* env so the test starts from a known state.
	for _, k := range []string{
		"DIDO_AUDIO_ROOT", "DIDO_USFM_ROOT", "DIDO_OUTPUT_FOLDER",
		"DIDO_LANG", "DIDO_NAME_STYLE", "DIDO_RESUME",
		"DIDO_INCLUDE_SECTION_HEADERS", "DIDO_ESPEAK_NG_PATH",
	} {
		t.Setenv(k, "")
	}
	return dir
}

// TestLoadMissingFileReturnsDefaults: no config.env on disk yet, so
// the loader gives back the documented zero-value defaults.
func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	withConfigDir(t)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.NameStyle != "sab" {
		t.Errorf("NameStyle default = %q, want %q", c.NameStyle, "sab")
	}
	if c.OutputFolder != "./out" {
		t.Errorf("OutputFolder default = %q, want %q", c.OutputFolder, "./out")
	}
}

// TestSaveThenLoadRoundTrips writes a config, reloads it, and confirms
// every field survives the trip — including the bool fields whose
// strconv parsing is the most likely to silently drop values.
func TestSaveThenLoadRoundTrips(t *testing.T) {
	withConfigDir(t)
	want := &Config{
		AudioRoot:             "/srv/audio",
		USFMRoot:              "/srv/usfm",
		OutputFolder:          "/srv/out",
		Lang:                  "tha",
		NameStyle:             "simple",
		Resume:                true,
		IncludeSectionHeaders: true,
		EspeakNGPath:          "/opt/espeak-ng",
	}
	if err := want.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *got != *want {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

// TestExportedEnvWins confirms an env var the user already exported
// takes precedence over the value baked into the file. Critical for
// CI / one-off overrides.
func TestExportedEnvWins(t *testing.T) {
	withConfigDir(t)
	(&Config{Lang: "eng"}).Save()
	t.Setenv("DIDO_LANG", "tha")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Lang != "tha" {
		t.Errorf("Lang = %q, want %q (exported env should beat file)", c.Lang, "tha")
	}
}

// TestPathHonoursXDG points XDG_CONFIG_HOME explicitly and verifies
// Path() resolves to the matching subpath. Sanity check; cheap.
func TestPathHonoursXDG(t *testing.T) {
	dir := withConfigDir(t)
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(dir, "dido", "config.env")
	if p != want {
		t.Errorf("Path = %q, want %q", p, want)
	}
}

// TestSaveCreatesParentDir confirms Save mkdirs the dido/ subdir
// when it doesn't already exist (first-run case).
func TestSaveCreatesParentDir(t *testing.T) {
	dir := withConfigDir(t)
	if err := (&Config{Lang: "eng"}).Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dido", "config.env")); err != nil {
		t.Errorf("config.env not created: %v", err)
	}
}
