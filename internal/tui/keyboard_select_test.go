package tui

import (
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
