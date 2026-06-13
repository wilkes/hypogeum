package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

func stripANSItest(s string) string { return ansi.Strip(s) }

func mouseAt(action tea.MouseAction, x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: action, Button: tea.MouseButtonLeft}
}

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
	// Borderless pane: top-left screen cell (0,0) maps directly to line 0, col 0.
	if got := m.screenToContent(0, 0); got != (cellPos{line: 0, col: 0}) {
		t.Errorf("screenToContent(0,0) = %+v, want {0,0}", got)
	}
	// A y far past the end clamps to the last line.
	last := len(m.contentLines()) - 1
	if got := m.screenToContent(0, 10_000); got.line != last {
		t.Errorf("screenToContent y=10000 line = %d, want %d (clamped)", got.line, last)
	}
	// An x far past the end of the first line clamps to that line's width.
	firstLineW := ansi.StringWidth(m.contentLines()[0])
	if got := m.screenToContent(10_000, 0); got.col != firstLineW {
		t.Errorf("screenToContent x=10000 col = %d, want %d (clamped)", got.col, firstLineW)
	}
}

func TestModel_ExtractSelection(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	m.setContent("hello world\nsecond line\nthird")

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
	m.setContent("\x1b[1mhello\x1b[0m world\nline two")

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
	m.setContent("hello world")
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

func TestModel_DragSelectsAndCopies(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))

	var copied string
	m.copyToClipboard = func(s string) { copied = s }

	// Force a known base so column math is predictable.
	m.setContent("hello world")

	// Press at content (0,0) → doc (0,0); drag to (5,0) → doc (0,5); release.
	updated, _ := m.Update(mouseAt(tea.MouseActionPress, 0, 0))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionMotion, 5, 0))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionRelease, 5, 0))
	m = updated.(Model)

	if copied != "hello" {
		t.Errorf("clipboard got %q, want %q", copied, "hello")
	}
	if !m.content.selection.copied {
		t.Error("selection.copied should be true after release with text")
	}
}

func TestModel_ClickWithoutDragFollowsLink(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	var copied string
	m.copyToClipboard = func(s string) { copied = s }

	if len(m.content.links) == 0 {
		t.Fatal("expected at least one link in index.md")
	}
	renderAndScan(t, m, zoneContentPane)
	lz := zone.Get(linkZoneID(0))
	if lz.IsZero() {
		t.Fatal("link zone 0 not registered")
	}
	updated, _ := m.Update(mouseAt(tea.MouseActionPress, lz.StartX, lz.StartY))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionRelease, lz.StartX, lz.StartY))
	m = updated.(Model)

	if copied != "" {
		t.Errorf("a click should not copy; got %q", copied)
	}
	if got := m.history.Current(); filepath.Base(got) != "first.md" {
		t.Errorf("click should follow link to first.md; current=%q", got)
	}
}

func TestModel_EmptyDragDoesNotCopy(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	var calls int
	m.copyToClipboard = func(string) { calls++ }
	m.setContent("hello world")

	updated, _ := m.Update(mouseAt(tea.MouseActionPress, 3, 1))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionMotion, 3, 1)) // same cell
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionRelease, 3, 1))
	m = updated.(Model)

	if calls != 0 {
		t.Errorf("zero-width drag should not copy; got %d calls", calls)
	}
}

func TestModel_KeystrokeClearsFinalizedSelection(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	m.copyToClipboard = func(string) {}
	m.setContent("hello world")

	updated, _ := m.Update(mouseAt(tea.MouseActionPress, 0, 0))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionMotion, 5, 0))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionRelease, 5, 0))
	m = updated.(Model)
	if !m.content.selection.copied {
		t.Fatal("precondition: selection should be copied")
	}

	// Any key clears it (use 'j', a harmless scroll key).
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.content.selection.copied {
		t.Error("keystroke should clear the finalized selection")
	}
}

func TestModel_FooterShowsCopiedCount(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	m.copyToClipboard = func(string) {}
	m.setContent("hello world")

	updated, _ := m.Update(mouseAt(tea.MouseActionPress, 0, 0))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionMotion, 5, 0))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionRelease, 5, 0))
	m = updated.(Model)

	if !strings.Contains(m.renderFooter(), "Copied 5 chars") {
		t.Errorf("footer should show copied count; got %q", m.renderFooter())
	}
}

func TestModel_ExtractSelection_TrimsTrailingPad(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	// Two lines, each with trailing padding spaces (as Glamour emits).
	m.setContent("alpha     \nbravo  ")
	// Select both lines fully (line0 col0..width, line1 col0..width).
	m.content.selection.anchor = cellPos{0, 0}
	m.content.selection.cursor = cellPos{1, ansi.StringWidth("bravo  ")}
	if got := m.extractSelection(); got != "alpha\nbravo" {
		t.Errorf("trailing pad should be trimmed; got %q want %q", got, "alpha\nbravo")
	}
}

func TestModel_PressWhileModalOpenDoesNotArmSelection(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	// Open the tree modal.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if m.modals.kind != modalTree {
		t.Fatalf("precondition: t should open tree modal, got %v", m.modals.kind)
	}
	// A press somewhere in the content region must NOT arm a selection
	// while a modal is open (selection is content-pane only).
	updated, _ := m.Update(mouseAt(tea.MouseActionPress, 5, 5))
	m = updated.(Model)
	if m.content.selection.anchored {
		t.Error("press while modal open should not arm a content selection")
	}
}
