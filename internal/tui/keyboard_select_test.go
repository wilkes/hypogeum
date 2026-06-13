package tui

import (
	"strings"
	"testing"

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
