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

func TestModel_ExtractSelection(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	m.content.rendered = "hello world\nsecond line\nthird"

	cases := []struct {
		name           string
		anchor, cursor cellPos
		want           string
	}{
		{"single-line forward", cellPos{0, 0}, cellPos{0, 5}, "hello"},
		{"single-line backward", cellPos{0, 5}, cellPos{0, 0}, "hello"},
		{"mid-line span", cellPos{0, 6}, cellPos{0, 11}, "world"},
		{"multi-line", cellPos{0, 6}, cellPos{1, 6}, "world\nsecond"},
		{"multi-line backward", cellPos{1, 6}, cellPos{0, 6}, "world\nsecond"},
		{"empty (zero width)", cellPos{0, 3}, cellPos{0, 3}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m.content.selection.anchor = tc.anchor
			m.content.selection.cursor = tc.cursor
			if got := m.extractSelection(); got != tc.want {
				t.Errorf("extractSelection() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestModel_ExtractSelection_StripsANSI(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	// Bold "hello" + reset, then plain " world"; second line plain.
	m.content.rendered = "\x1b[1mhello\x1b[0m world\nline two"

	// Select "hello" (cols 0..5 on line 0) — must come back as plain text.
	m.content.selection.anchor = cellPos{0, 0}
	m.content.selection.cursor = cellPos{0, 5}
	if got := m.extractSelection(); got != "hello" {
		t.Errorf("styled single-line: got %q, want %q", got, "hello")
	}

	// Multi-line across the styled boundary: "world\nline" (line0 col6..11, line1 col0..4).
	m.content.selection.anchor = cellPos{0, 6}
	m.content.selection.cursor = cellPos{1, 4}
	if got := m.extractSelection(); got != "world\nline" {
		t.Errorf("styled multi-line: got %q, want %q", got, "world\nline")
	}
}

func TestModel_SelectionHighlightAppliesAndClears(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	m.content.rendered = "hello world"
	m.content.viewport.SetContent(m.content.rendered)
	before := m.content.viewport.View()

	m.content.selection.anchor = cellPos{0, 0}
	m.content.selection.cursor = cellPos{0, 5}
	m.applySelectionHighlight()
	highlighted := m.content.viewport.View()
	if highlighted == before {
		t.Errorf("applySelectionHighlight should change the rendered view; stayed %q", before)
	}

	// clearSelection only restores when moved||copied; emulate a finished drag.
	m.content.selection.moved = true
	m.clearSelection()
	cleared := m.content.viewport.View()
	if cleared != before {
		t.Errorf("clearSelection should restore the base view; got %q want %q", cleared, before)
	}
}
