// Command hypogeum is the terminal markdown browser entrypoint. It parses
// argv into a (root directory, optional initial file) pair and hands control
// to the Bubble Tea program.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/config"
	"github.com/wilkes/hypogeum/internal/tui"
)

// Build-time injected metadata. Defaults are placeholders for local
// `go build`; release builds overwrite these via ldflags in
// .goreleaser.yaml so the binary reports its tag, commit, and build date.
var (
	version = "devel"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hypogeum:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	for _, a := range args {
		if a == "--version" || a == "-v" {
			fmt.Printf("hypogeum %s (commit %s, built %s)\n", version, commit, date)
			return nil
		}
	}
	root, initialFile, err := resolveTarget(args)
	if err != nil {
		return err
	}

	cfg, warnings := loadConfig()

	model, err := tui.New(root, initialFile, tui.Options{
		Dialect:         cfg.Dialect,
		StartupWarnings: warnings,
	})
	if err != nil {
		return err
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

// loadConfig reads the user config and translates any error into a
// startup warning the TUI will surface via the log modal. A parse error
// also goes to stderr (visible before the alt-screen takes over and again
// after exit). loadConfig never returns an error; hypogeum always starts.
func loadConfig() (config.Config, []string) {
	cfgPath, pathErr := config.DefaultPath()
	if pathErr != nil {
		return config.Default(), []string{"config: " + pathErr.Error() + "; using defaults"}
	}
	cfg, warnings, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hypogeum: %s: %v (using defaults)\n", cfgPath, err)
		warnings = append(warnings, fmt.Sprintf("config %s: %v; using defaults", cfgPath, err))
		return config.Default(), warnings
	}
	return cfg, warnings
}

// resolveTarget interprets the CLI args per the README:
//   - no args:       browse the current working directory
//   - one dir arg:   browse that directory
//   - one file arg:  open that file, root the tree at its parent directory
func resolveTarget(args []string) (root, initialFile string, err error) {
	switch len(args) {
	case 0:
		root, err = os.Getwd()
		return root, "", err
	case 1:
		target, err := filepath.Abs(args[0])
		if err != nil {
			return "", "", err
		}
		info, err := os.Stat(target)
		if err != nil {
			return "", "", err
		}
		if info.IsDir() {
			return target, "", nil
		}
		return filepath.Dir(target), target, nil
	default:
		return "", "", fmt.Errorf("usage: hypogeum [path]")
	}
}
