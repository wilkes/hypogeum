package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// hasReverseVideo reports whether the rendered viewport contains any
// reverse-video (\x1b[7m) run — i.e. a caret or selection is painted.
func hasReverseVideo(s string) bool {
	return strings.Contains(s, "\x1b[7m")
}

func TestVisual_EnterShowsCaretAtTopLeft(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent("hello world\nsecond line")

	m = pressRune(t, m, 'v')

	if !m.content.selection.visual {
		t.Fatal("v should enter visual mode")
	}
	if m.content.selection.selecting {
		t.Error("entering visual should be in positioning phase (selecting=false)")
	}
	wantCaret := cellPos{line: m.content.viewport.YOffset, col: 0}
	if m.content.selection.cursor != wantCaret {
		t.Errorf("caret = %+v, want %+v", m.content.selection.cursor, wantCaret)
	}
	if m.content.selection.anchor != wantCaret {
		t.Errorf("anchor = %+v, want %+v", m.content.selection.anchor, wantCaret)
	}
	if !hasReverseVideo(m.content.viewport.View()) {
		t.Errorf("expected a reverse-video caret cell in view:\n%s", ansi.Strip(m.content.viewport.View()))
	}
	if got := m.extractSelection(); got != "" {
		t.Errorf("extractSelection at entry = %q, want empty", got)
	}
}

func TestVisual_PositioningMovesCaretNoSpan(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent("hello world\nsecond line")

	m = pressRune(t, m, 'v')         // caret at {0,0}
	m = pressRune(t, m, 'l')         // → {0,1}
	m = pressRune(t, m, 'l')         // → {0,2}
	m = pressRune(t, m, 'j')         // → {1,2}

	if got := m.content.selection.cursor; got != (cellPos{line: 1, col: 2}) {
		t.Fatalf("caret = %+v, want {1,2}", got)
	}
	if m.content.selection.anchor != m.content.selection.cursor {
		t.Errorf("anchor %+v should track cursor %+v in positioning phase",
			m.content.selection.anchor, m.content.selection.cursor)
	}
	if got := m.extractSelection(); got != "" {
		t.Errorf("extractSelection in positioning = %q, want empty", got)
	}
}

func TestVisual_EscCancelsFromPositioning(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent("hello world")

	m = pressRune(t, m, 'v')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.content.selection.visual {
		t.Error("Esc should exit visual mode")
	}
	if hasReverseVideo(m.content.viewport.View()) {
		t.Error("Esc should restore the clean (un-highlighted) render")
	}
}

func TestVisual_CaretClampsAtEdges(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent("ab\ncd")

	m = pressRune(t, m, 'v')         // {0,0}
	m = pressRune(t, m, 'h')         // clamp col → {0,0}
	m = pressRune(t, m, 'k')         // clamp line → {0,0}
	if got := m.content.selection.cursor; got != (cellPos{0, 0}) {
		t.Errorf("caret after clamp = %+v, want {0,0}", got)
	}
}

func TestVisual_SpaceAnchorsThenExtends(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent("hello world")

	m = pressRune(t, m, 'v')                            // caret {0,0}
	m = pressRune(t, m, 'l')                            // {0,1} (positioning, no span)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})  // drop anchor at {0,1}
	if !m.content.selection.selecting {
		t.Fatal("Space should enter the extend phase")
	}
	if got := m.content.selection.anchor; got != (cellPos{line: 0, col: 1}) {
		t.Errorf("anchor after Space = %+v, want {0,1}", got)
	}
	m = pressRune(t, m, 'l') // {0,2}
	m = pressRune(t, m, 'l') // {0,3}
	m = pressRune(t, m, 'l') // {0,4}

	// Anchor frozen at col 1; cursor at col 4 → "ell".
	if got := m.extractSelection(); got != "ell" {
		t.Errorf("extractSelection = %q, want %q", got, "ell")
	}
}

func TestVisual_YankBeforeAnchorExitsCleanly(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	var copied string
	m.copyToClipboard = func(s string) { copied = s }
	m.setContent("hello world")

	m = pressRune(t, m, 'v') // caret {0,0}, positioning (not selecting)
	m = pressRune(t, m, 'l') // {0,1}, still positioning → zero-width span
	m = pressRune(t, m, 'y') // yank with nothing selected

	if copied != "" {
		t.Errorf("zero-width yank should copy nothing; clipboard = %q", copied)
	}
	if m.content.selection.visual {
		t.Error("zero-width yank should exit visual mode")
	}
	if hasReverseVideo(m.content.viewport.View()) {
		t.Error("zero-width yank should restore the clean render (no caret left)")
	}
}

func TestVisual_YankCopiesAndPersists(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	var copied string
	m.copyToClipboard = func(s string) { copied = s }
	m.setContent("hello world")

	m = pressRune(t, m, 'v')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace}) // anchor at {0,0}
	m = pressRune(t, m, 'l')
	m = pressRune(t, m, 'l')
	m = pressRune(t, m, 'l')
	m = pressRune(t, m, 'l')
	m = pressRune(t, m, 'l')                            // cursor {0,5} → "hello"
	m = pressRune(t, m, 'y')                            // yank

	if copied != "hello" {
		t.Errorf("clipboard = %q, want %q", copied, "hello")
	}
	if m.content.selection.visual || m.content.selection.selecting {
		t.Error("yank should exit visual mode")
	}
	if !m.content.selection.copied {
		t.Error("yank should leave the selection in the copied state")
	}
	if !hasReverseVideo(m.content.viewport.View()) {
		t.Error("highlight should persist after yank")
	}
	if !strings.Contains(m.renderFooter(), "Copied 5 chars") {
		t.Errorf("footer should toast the count; got %q", m.renderFooter())
	}
}

// tallContent builds N numbered lines so jumps/scrolling have room.
func tallContent(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "line %02d\n", i)
	}
	return strings.TrimRight(b.String(), "\n")
}

func TestVisual_GJumpsToBottomAndYanksToEnd(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	var copied string
	m.copyToClipboard = func(s string) { copied = s }
	m.setContent(tallContent(100))

	m = pressRune(t, m, 'v')                           // caret {0,0}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace}) // anchor {0,0}
	m = pressRune(t, m, 'G')                           // caret → last line

	last := len(m.contentLines()) - 1
	if got := m.content.selection.cursor.line; got != last {
		t.Fatalf("G caret line = %d, want %d", got, last)
	}
	m = pressRune(t, m, 'y')
	// Anchor is {0,0} and cursor is {last,0}: the selection runs from line 0
	// up to (but not past) col 0 of the last line, so "line 98" is the last
	// substantive content. "line 99" has hi=0 and contributes an empty segment.
	if !strings.HasPrefix(copied, "line 00\n") || !strings.Contains(copied, "line 98") {
		t.Errorf("G-then-yank should copy from top toward end; got %q", copied)
	}
}

func TestVisual_HalfPageDownScrolls(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent(tallContent(100))

	// Enter visual mode (caret at viewport top) then press ^d.
	m = pressRune(t, m, 'v')
	startLine := m.content.selection.cursor.line
	half := m.content.viewport.Height / 2
	if half < 1 {
		half = 1
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD}) // ^d

	wantLine := startLine + half
	if got := m.content.selection.cursor.line; got != wantLine {
		t.Errorf("^d caret line: got %d, want %d (startLine %d + half %d)",
			got, wantLine, startLine, half)
	}
	// The viewport scrolls only when the caret leaves the visible window;
	// with Height≈38 and a starting offset of 0, line 19 is still on screen.
	// We verify caret-in-view invariant: the caret must be within the viewport.
	vp := m.content.viewport
	if m.content.selection.cursor.line < vp.YOffset || m.content.selection.cursor.line >= vp.YOffset+vp.Height {
		t.Errorf("caret line %d not visible in viewport [%d, %d)",
			m.content.selection.cursor.line, vp.YOffset, vp.YOffset+vp.Height)
	}
}

func TestVisual_ModernDialectEntersAndYanks(t *testing.T) {
	root := writeFixture(t)
	isolatedHome(t)
	m, err := New(root, "", Options{Dialect: "modern"})
	if err != nil {
		t.Fatal(err)
	}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)
	var copied string
	m.copyToClipboard = func(s string) { copied = s }
	m.setContent("hello world")

	m = pressRune(t, m, 'v')                           // enter (v, both dialects)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace}) // anchor
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})    // cursor {0,5} → "hello"
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})    // modern yank = ^y

	if copied != "hello" {
		t.Errorf("modern dialect yank: clipboard = %q, want %q", copied, "hello")
	}
}

func TestVisual_NoOpWhileModalOpen(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = pressRune(t, m, 't') // open tree modal (pager)
	if m.modals.kind != modalTree {
		t.Fatalf("precondition: tree modal should be open, got %v", m.modals.kind)
	}
	m = pressRune(t, m, 'v')
	if m.content.selection.visual {
		t.Error("v should be a no-op while a modal is open")
	}
}

func TestVisual_RegressionCopyPathOutsideVisual(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	var copied string
	m.copyToClipboard = func(s string) { copied = s }

	m = pressRune(t, m, 'y') // y outside visual mode → copy current path
	if !strings.HasPrefix(copied, root) {
		t.Errorf("y outside visual should copy the path; got %q", copied)
	}
}
