package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_TreeNavigationAndOpen(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	// Locate first.md in the flattened tree, then drive the cursor toward it
	// with up/down keystrokes (direction depends on where auto-open landed).
	want := filepath.Join(root, "notes", "first.md")
	target := -1
	for i, row := range m.flatTree {
		if row.node.Path == want {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("first.md not found in flattened tree: %+v", m.flatTree)
	}

	for m.treeCursor != target {
		var key tea.KeyMsg
		if m.treeCursor < target {
			key = tea.KeyMsg{Type: tea.KeyDown}
		} else {
			key = tea.KeyMsg{Type: tea.KeyUp}
		}
		prev := m.treeCursor
		updated, _ := m.Update(key)
		m = updated.(Model)
		if m.treeCursor == prev {
			t.Fatalf("cursor stuck at %d trying to reach %d", prev, target)
		}
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if got := m.history.Current(); got != want {
		t.Errorf("after Enter, history.Current = %q, want %q", got, want)
	}
}
