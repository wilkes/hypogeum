package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// loadConfig is hard to test in isolation because DefaultPath uses
// os.UserConfigDir which can't be redirected by env vars on macOS.
// Instead, test the behavior via a temporary HOME and exercise the
// happy path on Linux where XDG_CONFIG_HOME is honored.
func TestLoadConfig_MissingFileDoesNotError(t *testing.T) {
	// Redirect to a temp dir where no config file exists.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))

	cfg, warnings := loadConfig()
	if cfg.Dialect != "pager" {
		t.Errorf("Dialect = %q, want %q", cfg.Dialect, "pager")
	}
	for _, w := range warnings {
		if strings.Contains(w, "using defaults") {
			t.Errorf("unexpected warning for missing file: %q", w)
		}
	}
}
