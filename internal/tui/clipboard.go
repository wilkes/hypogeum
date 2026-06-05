package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// copyCurrentPath writes the absolute path of the currently-open document
// to the system clipboard, so it can be pasted into another tool (e.g. a
// Claude session). Feedback goes through the diagnostics stream: a footer
// transient confirms the copy (and auto-clears) while also landing in the
// ^l log. With no document open it's a no-op beyond an info message.
func (m *Model) copyCurrentPath() {
	path := m.history.Current()
	if path == "" {
		if m.diag != nil {
			m.diag.Info("no document open to copy")
		}
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if err := m.copyClipboard(abs); err != nil {
		if m.diag != nil {
			m.diag.Error("copy path failed: " + err.Error())
		}
		return
	}
	if m.diag != nil {
		m.diag.Info("copied path: " + abs)
	}
}

// clipboardWriter copies text to the system clipboard. Returning an error
// surfaces in the status bar. Injected on the Model so tests can swap in a
// fake that records the text instead of touching the real clipboard —
// mirrors externalOpener.
type clipboardWriter func(text string) error

// copyToClipboard is the default clipboardWriter. It picks the
// platform-appropriate clipboard command and feeds text to it on stdin.
// Unlike openExternalURL it *waits* for the command (Run, not Start): the
// helper has to consume stdin before it exits, and the copies are tiny, so
// the user is never blocked for a perceptible time.
//
// On Linux there's no single canonical tool, so we probe in preference
// order: Wayland's wl-copy first, then X11's xclip, then xsel. The first
// one found on PATH wins; if none are installed we return an actionable
// error rather than silently doing nothing.
func copyToClipboard(text string) error {
	cmd, err := clipboardCommand()
	if err != nil {
		return err
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// clipboardCommand resolves the clipboard helper for the current platform.
// Split out from copyToClipboard so the tool-probing logic is testable and
// the error message stays close to the lookup that produced it.
func clipboardCommand() (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("pbcopy"), nil
	case "windows":
		return exec.Command("clip"), nil
	default:
		// Linux, BSDs, anything else. No canonical tool — probe PATH.
		if path, err := exec.LookPath("wl-copy"); err == nil {
			return exec.Command(path), nil
		}
		if path, err := exec.LookPath("xclip"); err == nil {
			return exec.Command(path, "-selection", "clipboard"), nil
		}
		if path, err := exec.LookPath("xsel"); err == nil {
			return exec.Command(path, "--clipboard", "--input"), nil
		}
		return nil, fmt.Errorf("no clipboard tool found (install wl-copy, xclip, or xsel)")
	}
}
