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
	if got := m.content.selection.cursor.col; got != m.content.lineWidths[last] {
		t.Errorf("G caret col = %d, want end-of-line %d", got, m.content.lineWidths[last])
	}
	m = pressRune(t, m, 'y')
	if !strings.HasPrefix(copied, "line 00\n") {
		t.Errorf("yank should start at the top; got %q", copied[:min(20, len(copied))])
	}
	if !strings.Contains(copied, "line 99") {
		t.Errorf("G-to-end then yank should include the last line; got tail %q", copied[max(0, len(copied)-20):])
	}
}

func TestVisual_HalfPageDownScrolls(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent(tallContent(200))
	startOffset := m.content.viewport.YOffset

	m = pressRune(t, m, 'v')
	// Press ^d enough times that the caret must leave the initial window.
	for i := 0; i < 4; i++ {
		m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	}

	if m.content.viewport.YOffset <= startOffset {
		t.Errorf("repeated ^d should scroll the viewport; offset %d -> %d", startOffset, m.content.viewport.YOffset)
	}
	// The caret should remain within the (now scrolled) visible window.
	top := m.content.viewport.YOffset
	h := m.content.viewport.Height
	if line := m.content.selection.cursor.line; line < top || line >= top+h {
		t.Errorf("caret line %d should be within visible window [%d, %d)", line, top, top+h)
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

func TestVisual_GJumpsToTopFromMidDoc(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent(tallContent(100))

	m = pressRune(t, m, 'v')
	m = pressRune(t, m, 'G') // caret to end of last line (bottom)
	if m.content.selection.cursor.line == 0 {
		t.Fatal("precondition: G should have moved the caret off line 0")
	}
	m = pressRune(t, m, 'g') // jump back to top-left
	if got := m.content.selection.cursor; got != (cellPos{line: 0, col: 0}) {
		t.Errorf("g should jump the caret to {0,0}; got %+v", got)
	}
}

func TestVisual_BackwardSelectionYanks(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	var copied string
	m.copyToClipboard = func(s string) { copied = s }
	m.setContent("hello world")

	// Position the caret at col 5, drop the anchor, then extend BACKWARD to col 0.
	m = pressRune(t, m, 'v')
	for i := 0; i < 5; i++ {
		m = pressRune(t, m, 'l') // caret → {0,5}
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace}) // anchor at {0,5}
	for i := 0; i < 5; i++ {
		m = pressRune(t, m, 'h') // cursor → {0,0}, before the anchor
	}
	// extractSelection should normalize direction → "hello".
	if got := m.extractSelection(); got != "hello" {
		t.Errorf("backward selection extract = %q, want %q", got, "hello")
	}
	m = pressRune(t, m, 'y')
	if copied != "hello" {
		t.Errorf("backward selection yank = %q, want %q", copied, "hello")
	}
}
