package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_OpenPickerModal(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	if m.modals.kind != modalNone {
		t.Fatalf("precondition: no modal should be open")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(Model)

	if m.modals.kind != modalPicker {
		t.Errorf("modalOpen = %v, want modalPicker", m.modals.kind)
	}
	if len(m.modals.picker.flat) == 0 {
		t.Errorf("picker.flat should be populated on open, got empty")
	}
}

// TestModel_PickerEscClosesFromAnyDepth replaces the old "Esc at root
// closes" test. The vault-rooted picker has no concept of "deeper than
// root" — Esc closes from anywhere.
func TestModel_PickerEscClosesFromAnyDepth(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(Model)
	if m.modals.kind != modalPicker {
		t.Fatalf("precondition: picker should be open")
	}

	// Expand a folder so we're "deep" — the old picker would walk up here.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.modals.kind != modalNone {
		t.Errorf("Esc should close picker from any depth; modalOpen = %v", m.modals.kind)
	}
}

// TestModel_PickerSelectOpensFile drives the picker through Ctrl+P →
// expand notes/ → enter on first.md, asserting the selection becomes
// the new history entry.
func TestModel_PickerSelectOpensFile(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(Model)

	// On open, the picker shows: root/, then collapsed children.
	// Cursor is at row 0 (root). Expand it.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})

	// Now move down to find first.md (under notes/sub potentially); for
	// the writeFixture layout, root expands to {notes/, index.md}.
	// Move down to notes/, expand it.
	want := filepath.Join(root, "index.md")
	target := -1
	for i, row := range m.modals.picker.flat {
		if row.node.Path == want {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("index.md not visible at depth-1; flat=%v", debugFlat(m.modals.picker.flat))
	}
	for m.modals.picker.cursor != target {
		key := tea.KeyMsg{Type: tea.KeyDown}
		if m.modals.picker.cursor > target {
			key = tea.KeyMsg{Type: tea.KeyUp}
		}
		prev := m.modals.picker.cursor
		m = pressKey(t, m, key)
		if m.modals.picker.cursor == prev {
			t.Fatalf("picker cursor stuck at %d trying to reach %d", prev, target)
		}
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if got := m.history.Current(); got != want {
		t.Errorf("history.Current = %q, want %q", got, want)
	}
	if m.modals.kind != modalNone {
		t.Errorf("picker should close after selection; modalOpen = %v", m.modals.kind)
	}
}

// TestModel_PickerHidesEmptyDirectories checks the user's primary ask:
// directories that contain no markdown (anywhere in their subtree)
// don't appear in the picker. By construction (we render m.rootNode
// which tree.Walk has already pruned), this is automatic — the test
// locks the property in.
func TestModel_PickerHidesEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	mk := func(rel, body string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("real-notes/note.md", "# note\n")
	mk("just-pdfs/x.pdf", "binary")
	mk("just-binaries/x.bin", "binary")
	if err := os.MkdirAll(filepath.Join(root, "totally-empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := sized(t, root, "")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(Model)
	// Expand the root so subdirs are visible.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})

	flatPaths := make([]string, len(m.modals.picker.flat))
	for i, r := range m.modals.picker.flat {
		flatPaths[i] = r.node.Path
	}
	allFlat := strings.Join(flatPaths, "\n")

	if !strings.Contains(allFlat, "real-notes") {
		t.Errorf("real-notes/ should appear in picker (contains a .md): %s", allFlat)
	}
	for _, banned := range []string{"just-pdfs", "just-binaries", "totally-empty"} {
		if strings.Contains(allFlat, banned) {
			t.Errorf("%s/ should NOT appear in picker (no markdown): %s", banned, allFlat)
		}
	}
}

// TestModel_PickerExpansionIndependentFromTreePane verifies that
// expanding/collapsing folders in the picker doesn't affect the left
// pane's expansion state, and vice versa.
func TestModel_PickerExpansionIndependentFromTreePane(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	// Snapshot left-pane expansion state before opening picker.
	leftExpandedBefore := len(m.tree.expanded)

	// Open picker, expand a folder in it.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(Model)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace}) // expand root

	if len(m.modals.picker.expanded) == 0 {
		t.Fatalf("precondition: picker should have at least one expanded entry")
	}

	// Close the picker.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	// Left pane expansion should be unchanged.
	if len(m.tree.expanded) != leftExpandedBefore {
		t.Errorf("left-pane expanded changed: before=%d after=%d", leftExpandedBefore, len(m.tree.expanded))
	}
}

// debugFlat formats a picker's flat list for test failure messages.
func debugFlat(rows []treeRow) string {
	parts := make([]string, len(rows))
	for i, r := range rows {
		parts[i] = filepath.Base(r.node.Path)
	}
	return strings.Join(parts, ", ")
}
