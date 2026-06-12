package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func stripANSItest(s string) string { return ansi.Strip(s) }

func TestModel_CopyToClipboard_DefaultIsSet(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	if m.copyToClipboard == nil {
		t.Fatal("copyToClipboard should default to a non-nil writer")
	}
}

func TestModel_RenderedBaseIsStored(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	if m.content.rendered == "" {
		t.Fatal("content.rendered should hold the rendered output after open")
	}
	if !strings.Contains(stripANSItest(m.content.rendered), "Index") {
		t.Errorf("rendered base should contain the heading text; got %q",
			stripANSItest(m.content.rendered))
	}
}

func TestModel_ScreenToContent_MapsAndClamps(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	// Top-left content cell (border at 0,0 → text at 1,1) maps to line 0,col 0.
	if got := m.screenToContent(1, 1); got != (cellPos{line: 0, col: 0}) {
		t.Errorf("screenToContent(1,1) = %+v, want {0,0}", got)
	}
	// Negative-ish coords clamp to 0.
	if got := m.screenToContent(0, 0); got != (cellPos{line: 0, col: 0}) {
		t.Errorf("screenToContent(0,0) = %+v, want {0,0} (clamped)", got)
	}
	// A y far past the end clamps to the last line.
	last := len(m.contentLines()) - 1
	if got := m.screenToContent(1, 10_000); got.line != last {
		t.Errorf("screenToContent y=10000 line = %d, want %d (clamped)", got.line, last)
	}
	// An x far past the end of the first line clamps to that line's width.
	firstLineW := ansi.StringWidth(m.contentLines()[0])
	if got := m.screenToContent(10_000, 1); got.col != firstLineW {
		t.Errorf("screenToContent x=10000 col = %d, want %d (clamped)", got.col, firstLineW)
	}
}
