// Package config loads hypogeum's user-config file from the
// OS-canonical user-config directory. The file is optional;
// missing or malformed configs degrade gracefully to defaults.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the parsed user config.
type Config struct {
	Dialect string // "pager" (default) | "modern"
}

// Default returns the zero-config defaults.
func Default() Config {
	return Config{Dialect: "pager"}
}

// DefaultPath returns the per-OS expected config location, using
// os.UserConfigDir as the base. On Linux that's $XDG_CONFIG_HOME (or
// ~/.config). On macOS, ~/Library/Application Support. On Windows,
// %AppData%. The hypogeum subdirectory is appended.
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(base, "hypogeum", "config.toml"), nil
}

// Load reads and validates a config file.
//
// Behavior contract:
//   - File missing → returns Default(), no warnings, nil error.
//   - File present and empty → returns Default(), no warnings.
//   - File present and valid TOML → parses dialect.
//     If dialect is not one of the recognized values, falls back to
//     "pager" and returns a warning naming the valid options.
//   - File present but malformed TOML or unreadable → returns Default()
//     with a non-nil error. The caller decides how to surface the error;
//     hypogeum's main.go logs it to stderr and continues with defaults.
func Load(path string) (Config, []string, error) {
	cfg := Default()

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil, nil
		}
		return Default(), nil, fmt.Errorf("read %s: %w", path, err)
	}

	var parsed struct {
		Dialect string `toml:"dialect"`
	}
	if _, err := toml.Decode(string(raw), &parsed); err != nil {
		return Default(), nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var warnings []string
	switch parsed.Dialect {
	case "":
		// Field omitted; keep default.
	case "pager", "modern":
		cfg.Dialect = parsed.Dialect
	default:
		warnings = append(warnings,
			fmt.Sprintf(`config: unknown dialect %q (valid options: "pager", "modern"); falling back to "pager"`, parsed.Dialect))
	}

	return cfg, warnings, nil
}
