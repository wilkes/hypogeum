package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// writeFixture lays down a small markdown directory and returns its root.
func writeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"index.md":          "# Index\n\nSee [first](notes/first.md) and [external](https://x.test).\n",
		"notes/first.md":    "# First\n\nHello.\n",
		"notes/sub/deep.md": "# Deep\n\nNested.\n",
	}
	for rel, body := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// sized returns a model that has received an initial size message, so that
// View() produces real output rather than the empty pre-resize string.
func sized(t *testing.T, root, initialFile string) Model {
	t.Helper()
	m, err := New(root, initialFile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model)
}

func TestModel_BootsAndRendersFirstFile(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	view := m.View()
	if view == "" {
		t.Fatal("View returned empty string after WindowSizeMsg")
	}
	// Tree pane should mention the markdown files we wrote.
	if !strings.Contains(view, "index.md") {
		t.Errorf("expected tree to contain index.md, got:\n%s", view)
	}
	if !strings.Contains(view, "first.md") {
		t.Errorf("expected tree to contain first.md, got:\n%s", view)
	}
	// Auto-opened first file should land us on Index content.
	if !strings.Contains(view, "Index") {
		t.Errorf("expected rendered content to contain 'Index', got:\n%s", view)
	}
}

func TestModel_RefreshPopulatesLinks(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Auto-open lands on index.md, which has two links per writeFixture.
	if got := len(m.links); got != 2 {
		t.Fatalf("len(m.links) = %d, want 2 (index.md has [first] and [external])", got)
	}
	if m.links[0].Href != "notes/first.md" {
		t.Errorf("links[0].Href = %q, want notes/first.md", m.links[0].Href)
	}
	if m.links[1].Href != "https://x.test" {
		t.Errorf("links[1].Href = %q, want https://x.test", m.links[1].Href)
	}
}

func TestModel_FooterShowsNoLinkSelectedByDefault(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Phase 1: nothing selected yet, so the footer should NOT contain the
	// link-selection marker.
	if strings.Contains(m.View(), linkFooterMarker) {
		t.Errorf("expected no link-selection footer marker before any link is picked, got view:\n%s", m.View())
	}
}

func TestModel_OpensInitialFile(t *testing.T) {
	root := writeFixture(t)
	target := filepath.Join(root, "notes", "first.md")
	m := sized(t, root, target)

	if got := m.history.Current(); got != target {
		t.Errorf("history.Current = %q, want %q", got, target)
	}
	if !strings.Contains(m.View(), "First") {
		t.Errorf("expected rendered content to contain 'First'")
	}
}

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
