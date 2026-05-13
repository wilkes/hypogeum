package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestRefreshContent_CodeFile_DispatchesToCodeRenderer verifies that
// refreshContent on a .go path produces non-empty viewport content,
// empty links, and links cursor cleared.
func TestRefreshContent_CodeFile_DispatchesToCodeRenderer(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "index.md")
	goPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mdPath, []byte("# index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goPath, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	m.refreshContent(goPath)

	view := m.content.viewport.View()
	if strings.TrimSpace(view) == "" {
		t.Error("viewport empty after refreshContent on .go file")
	}
	if len(m.content.links) != 0 {
		t.Errorf("expected no links for code file, got %d", len(m.content.links))
	}
	if m.content.linkCursor != -1 {
		t.Errorf("expected linkCursor == -1, got %d", m.content.linkCursor)
	}
	if m.status != goPath {
		t.Errorf("expected status to be %q, got %q", goPath, m.status)
	}
}

// TestRefreshContent_CodeFileReadError_ClearsLinksAndReportsStatus
// covers the error path: refreshContent on a missing code file should
// still leave the model in a consistent state.
func TestRefreshContent_CodeFileReadError_ClearsLinksAndReportsStatus(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "index.md")
	if err := os.WriteFile(mdPath, []byte("# index\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	m.refreshContent(filepath.Join(dir, "nonexistent.go"))

	if m.status == "" {
		t.Error("expected status to carry read error, got empty string")
	}
	if len(m.content.links) != 0 {
		t.Errorf("expected links cleared after read error, got %d", len(m.content.links))
	}
}
