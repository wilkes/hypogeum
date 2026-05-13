package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if got := len(m.content.links); got != 2 {
		t.Fatalf("len(m.content.links) = %d, want 2 (index.md has [first] and [external])", got)
	}
	if m.content.links[0].Href != "notes/first.md" {
		t.Errorf("links[0].Href = %q, want notes/first.md", m.content.links[0].Href)
	}
	if m.content.links[1].Href != "https://x.test" {
		t.Errorf("links[1].Href = %q, want https://x.test", m.content.links[1].Href)
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

func TestKeyBTogglesBacklinksOpen(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.backlinks.open {
		t.Fatalf("expected backlinksOpen=false initially")
	}
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if !out.(Model).backlinks.open {
		t.Fatalf("after b: expected backlinksOpen=true")
	}
	out2, _ := out.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if out2.(Model).backlinks.open {
		t.Fatalf("after second b: expected backlinksOpen=false")
	}
}

func TestNewInitializesRecentStore(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "n.md")
	if err := os.WriteFile(notePath, []byte("# N"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.recent == nil {
		t.Fatal("Model.recent is nil; want non-nil Store")
	}
}

func TestAllVaultMarkdownPaths(t *testing.T) {
	dir := t.TempDir()
	// Create:  dir/a.md, dir/sub/b.md, dir/sub/sub2/c.md, dir/d.txt (excluded)
	mustWrite := func(p string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(dir, "a.md"))
	mustWrite(filepath.Join(dir, "sub", "b.md"))
	mustWrite(filepath.Join(dir, "sub", "sub2", "c.md"))
	mustWrite(filepath.Join(dir, "d.txt"))

	m, err := New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	paths := m.allVaultMarkdownPaths()
	if len(paths) != 3 {
		t.Errorf("got %d paths, want 3: %v", len(paths), paths)
	}
	// All paths absolute and end in .md
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			t.Errorf("path not absolute: %q", p)
		}
		if filepath.Ext(p) != ".md" {
			t.Errorf("non-md path: %q", p)
		}
	}
}
