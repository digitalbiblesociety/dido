// Package userconfig loads and persists user-level defaults for the
// dido command suite from a simple KEY=VALUE file at
// $XDG_CONFIG_HOME/dido/config.env (or ~/.config/dido/config.env when
// XDG_CONFIG_HOME isn't set). The same file feeds dido, dido-sab, and
// any future CLI in the suite, matching the .env-driven pattern used by
// the sibling usfm-tools project.
//
// Load applies the file's values to the process environment (without
// clobbering anything the user already exported), then reads the
// resulting environment into a Config struct. Empty fields keep the
// hard-coded defaults so the loader is safe to call from any tool — a
// missing file is not an error.
package userconfig

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds the user-tunable defaults for the dido tools. New fields
// should map 1:1 to a DIDO_* env var and be listed in Save() so the
// settings UI round-trips them.
type Config struct {
	// AudioRoot is the parent directory the dido-sab batch mode walks
	// to find <NN>_<EnglishName>/ chapter-audio folders. Single mode
	// uses it as the starting point for the directory picker only.
	AudioRoot string
	// USFMRoot is the parent directory of *.usfm/*.sfm files for batch
	// mode. Single mode reuses it as the picker's starting directory.
	USFMRoot string
	// OutputFolder is where -aeneas.txt / -timing.txt files are written.
	OutputFolder string
	// Lang is the default eSpeak-NG voice (task_language).
	Lang string
	// NameStyle is the SAB output stem style: "sab" or "simple".
	NameStyle string
	// Resume defaults the "skip already-aligned chapters" flag.
	Resume bool
	// IncludeSectionHeaders defaults the heading-passthrough flag.
	IncludeSectionHeaders bool
	// EspeakNGPath overrides the espeak-ng executable lookup. Kept
	// here so the settings UI can edit it; the alignment runtime
	// already honours the env var of the same name.
	EspeakNGPath string
	// Workers caps the number of concurrent chapter-alignment jobs in
	// batch mode. 0 means "auto" (resolved to runtime.NumCPU() at run
	// time). Set this lower if you're sharing the machine, higher if
	// you want to oversubscribe.
	Workers int
}

// Path returns the canonical config file location.
func Path() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "dido", "config.env"), nil
}

// Load reads the config file (if present) into a Config. The file is
// optional — a missing file yields a zero-value Config with the
// documented defaults filled in.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	pairs, err := readEnvFile(path)
	if err != nil {
		return nil, err
	}
	// Mirror file values into the process environment when the shell
	// hasn't already exported a non-empty value for the same key. An
	// explicit empty export is treated as "not set" so the test
	// harness (and users clearing a var to fall back to file defaults)
	// gets the expected behaviour.
	for k, v := range pairs {
		if cur, ok := os.LookupEnv(k); !ok || cur == "" {
			_ = os.Setenv(k, v)
		}
	}
	c := &Config{
		AudioRoot:             os.Getenv("DIDO_AUDIO_ROOT"),
		USFMRoot:              os.Getenv("DIDO_USFM_ROOT"),
		OutputFolder:          os.Getenv("DIDO_OUTPUT_FOLDER"),
		Lang:                  os.Getenv("DIDO_LANG"),
		NameStyle:             os.Getenv("DIDO_NAME_STYLE"),
		Resume:                parseBool(os.Getenv("DIDO_RESUME"), false),
		IncludeSectionHeaders: parseBool(os.Getenv("DIDO_INCLUDE_SECTION_HEADERS"), false),
		EspeakNGPath:          os.Getenv("DIDO_ESPEAK_NG_PATH"),
		Workers:               parseIntOrZero(os.Getenv("DIDO_BATCH_WORKERS")),
	}
	if c.NameStyle == "" {
		c.NameStyle = "sab"
	}
	if c.OutputFolder == "" {
		c.OutputFolder = "./out"
	}
	return c, nil
}

// Save writes c back to the config file, creating parent directories
// as needed. The format is one KEY=VALUE per line — readable by
// godotenv-compatible parsers but not requiring the dependency.
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	lines := []string{
		"# dido user defaults — auto-generated, hand-editable.",
		"# Any DIDO_* env exported in the shell takes precedence over the value here.",
		fmt.Sprintf("DIDO_AUDIO_ROOT=%s", c.AudioRoot),
		fmt.Sprintf("DIDO_USFM_ROOT=%s", c.USFMRoot),
		fmt.Sprintf("DIDO_OUTPUT_FOLDER=%s", c.OutputFolder),
		fmt.Sprintf("DIDO_LANG=%s", c.Lang),
		fmt.Sprintf("DIDO_NAME_STYLE=%s", c.NameStyle),
		fmt.Sprintf("DIDO_RESUME=%t", c.Resume),
		fmt.Sprintf("DIDO_INCLUDE_SECTION_HEADERS=%t", c.IncludeSectionHeaders),
		fmt.Sprintf("DIDO_ESPEAK_NG_PATH=%s", c.EspeakNGPath),
		fmt.Sprintf("DIDO_BATCH_WORKERS=%d", c.Workers),
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// readEnvFile parses a minimal subset of the dotenv format: blank
// lines, `# comments`, and `KEY=VALUE` pairs with optional surrounding
// quotes around VALUE. A missing file returns an empty map, not an
// error — callers treat the file as optional.
func readEnvFile(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// Strip a matched pair of surrounding quotes (single or double).
		if len(val) >= 2 {
			first, last := val[0], val[len(val)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		out[key] = val
	}
	return out, sc.Err()
}

// parseIntOrZero parses a non-negative integer, returning 0 on any
// failure. The settings UI distinguishes 0 ("auto / NumCPU") from a
// specific worker count, so we never want to surface negative values.
func parseIntOrZero(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func parseBool(s string, def bool) bool {
	if s == "" {
		return def
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return def
	}
	return b
}
