package tui

import (
	"path/filepath"
	"strings"
	"testing"
)

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

// View() must fit within the reported terminal height. Overshooting causes
// the terminal to scroll and clips the top borders/title row off-screen.
func TestModel_ViewFitsTerminalHeight(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	lines := strings.Split(m.View(), "\n")
	if got, want := len(lines), m.height; got > want {
		t.Errorf("View() produced %d lines, exceeds terminal height %d (top will clip)", got, want)
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

func TestNewBuildsVault(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.vault == nil {
		t.Fatalf("expected vault to be constructed")
	}
}
