// Package config loads hypogeum's user-config file from the
// OS-canonical user-config directory. The file is optional;
// missing or malformed configs degrade gracefully to defaults.
package config

import (
	"fmt"
	"os"
	"path/filepath"
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
