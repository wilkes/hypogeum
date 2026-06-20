// Command hypogeum is the terminal markdown browser entrypoint. It parses
// argv into a (root directory, optional initial file) pair and hands control
// to the Bubble Tea program.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

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
	if len(args) > 0 && isQueryVerb(args[0]) {
		return runQuery(args, os.Stdout)
	}
	root, initialFile, err := resolveTarget(args)
	if err != nil {
		return err
	}

	model, err := tui.New(root, initialFile)
	if err != nil {
		return err
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
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
