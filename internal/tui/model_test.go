package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/markdown"
	"github.com/wilkes/hypogeum/internal/watch"
)

func TestModel_BootsAndRendersFirstFile(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	view := m.View()
	if view == "" {
		t.Fatal("View returned empty string after WindowSizeMsg")
	}
	// Auto-opened first file should land us on Index content. (Tree is
	// hidden by default, so we only assert the content render.)
	if !strings.Contains(view, "Index") {
		t.Errorf("expected rendered content to contain 'Index', got:\n%s", view)
	}

	// After ^b, the tree modal shows the root level: index.md is at the
	// top level so it's visible; notes/ is a folder at the top level and
	// also visible (its children are hidden until the user expands).
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)
	view = m.View()
	if !strings.Contains(view, "index.md") {
		t.Errorf("expected tree to contain index.md after ^b, got:\n%s", view)
	}
	if !strings.Contains(view, "notes") {
		t.Errorf("expected tree to contain notes folder after ^b, got:\n%s", view)
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
	isolatedHome(t)
	dir := t.TempDir()
	m, err := New(dir, "", Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.vault == nil {
		t.Fatalf("expected vault to be constructed")
	}
}

func TestKeyBTogglesBacklinksModal(t *testing.T) {
	isolatedHome(t)
	dir := t.TempDir()
	m, err := New(dir, "", Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.modals.kind != modalNone {
		t.Fatalf("expected no modal initially, got %v", m.modals.kind)
	}
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if out.(Model).modals.kind != modalBacklinks {
		t.Fatalf("after b: expected modalBacklinks, got %v", out.(Model).modals.kind)
	}
	out2, _ := out.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if out2.(Model).modals.kind != modalNone {
		t.Fatalf("after second b: expected modalNone, got %v", out2.(Model).modals.kind)
	}
}

func TestNewInitializesRecentStore(t *testing.T) {
	isolatedHome(t)
	dir := t.TempDir()
	notePath := filepath.Join(dir, "n.md")
	if err := os.WriteFile(notePath, []byte("# N"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(dir, "", Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.recent == nil {
		t.Fatal("Model.recent is nil; want non-nil Store")
	}
}

func TestAllVaultMarkdownPaths(t *testing.T) {
	isolatedHome(t)
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

	m, err := New(dir, "", Options{})
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

func TestModel_EmbedDepsPopulatedOnOpen(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	mdPath := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(mdPath, []byte("# notes\n\n![[main.go#L1-L2]]\n"), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}

	m, err := New(dir, mdPath, Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := m.content.embedDeps[src]; !ok {
		t.Fatalf("embedDeps missing %q; got %v", src, m.content.embedDeps)
	}
}

func TestModel_FileModifiedOnEmbedDepRefreshesOpenMarkdown(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	mdPath := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(mdPath, []byte("![[main.go#L1-L2]]\n"), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}
	m := sized(t, dir, mdPath)

	if err := os.WriteFile(src, []byte("X\nY\nZ\n"), 0o644); err != nil {
		t.Fatalf("rewrite src: %v", err)
	}
	m.handleFSEvent(watch.Event{Kind: watch.FileModified, Paths: []string{src}})

	if !strings.Contains(m.content.viewport.View(), "X") {
		t.Fatalf("expected re-rendered content to contain new source line; viewport:\n%s",
			m.content.viewport.View())
	}
}

func TestModel_EnterOnRangeLinkOpensSourceWithHighlight(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("a\nb\nc\nd\ne\nf\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	mdPath := filepath.Join(dir, "notes.md")
	mdBody := "[the parser](main.go#L2-L3)\n"
	if err := os.WriteFile(mdPath, []byte(mdBody), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}

	m := sized(t, dir, mdPath)
	if len(m.content.links) == 0 {
		t.Fatalf("expected a link in the document, got none")
	}
	// Move link cursor onto the range link (the only link in the document).
	m.content.linkCursor = 0
	m.followCurrentLink()

	if m.history.Current() != src {
		t.Fatalf("expected current file = %q, got %q", src, m.history.Current())
	}
	if m.content.rangeHighlight == nil ||
		m.content.rangeHighlight.Start != 2 || m.content.rangeHighlight.End != 3 {
		t.Fatalf("rangeHighlight = %+v", m.content.rangeHighlight)
	}
	if !strings.Contains(m.content.viewport.View(), "\x1b[7m") {
		t.Fatalf("expected reverse-video SGR in viewport:\n%s",
			m.content.viewport.View())
	}
}

func TestModel_EscClearsRangeHighlight(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	m := sized(t, dir, src)
	m.content.rangeHighlight = &markdown.LineRange{Start: 1, End: 2}
	m.refreshContent(src)
	if !strings.Contains(m.content.viewport.View(), "\x1b[7m") {
		t.Fatalf("setup: expected reverse-video in viewport")
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.content.rangeHighlight != nil {
		t.Fatalf("Esc should clear rangeHighlight; got %+v", m.content.rangeHighlight)
	}
	if strings.Contains(m.content.viewport.View(), "\x1b[7m") {
		t.Fatalf("Esc should have re-rendered without highlight; viewport still has SGR:\n%s",
			m.content.viewport.View())
	}
}

func TestModel_EscClearingRangeHighlightPreservesScroll(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "big.go")
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("line content\n")
	}
	if err := os.WriteFile(src, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	m := sized(t, dir, src)
	m.content.rangeHighlight = &markdown.LineRange{Start: 1, End: 2}
	m.refreshContent(src)
	m.content.viewport.SetYOffset(60)
	want := m.content.viewport.YOffset

	m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})

	if m.content.rangeHighlight != nil {
		t.Fatalf("Esc should clear rangeHighlight; got %+v", m.content.rangeHighlight)
	}
	if m.content.viewport.YOffset != want {
		t.Fatalf("Esc should preserve scroll: YOffset %d -> %d",
			want, m.content.viewport.YOffset)
	}
}

func TestModel_CyclingOntoEmbedDoesNotScroll(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.go")
	if err := os.WriteFile(target, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	// A markdown file long enough that we can scroll, with an embed at
	// the end. The pad lines before the embed force the viewport to
	// have a non-zero YOffset when we land on the embed link.
	var pad strings.Builder
	for i := 0; i < 50; i++ {
		pad.WriteString("filler line\n\n")
	}
	doc := pad.String() + "![[target.go]]\n"
	md := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(md, []byte(doc), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	m := sized(t, dir, md)
	// Scroll well past the top.
	m.content.viewport.SetYOffset(40)
	want := m.content.viewport.YOffset

	// One cycleLink(+1) call: empty cursor -> first link (the embed).
	m.cycleLink(+1)

	if m.content.viewport.YOffset != want {
		t.Fatalf("cycling onto embed link must not scroll: YOffset %d -> %d",
			want, m.content.viewport.YOffset)
	}
	if m.content.linkCursor != 0 {
		t.Fatalf("expected linkCursor=0, got %d", m.content.linkCursor)
	}
}
