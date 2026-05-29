package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/tree"
)

// allNodeNames returns the Name of every node in the tree, depth-first.
func allNodeNames(n *tree.Node) []string {
	if n == nil {
		return nil
	}
	out := []string{n.Name}
	for _, c := range n.Children {
		out = append(out, allNodeNames(c)...)
	}
	return out
}

// writeRoot lays down rel→content files under a fresh temp dir and returns it.
func writeRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
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

// sizedMulti is the multi-root analogue of sized().
func sizedMulti(t *testing.T, roots []string, initialFile string) Model {
	t.Helper()
	isolatedHome(t)
	m, err := NewMulti(roots, initialFile)
	if err != nil {
		t.Fatalf("NewMulti: %v", err)
	}
	closeWatcherOnCleanup(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model)
}

func TestNewMulti_MergedTreeSpansRoots(t *testing.T) {
	a := writeRoot(t, map[string]string{"a.md": "# A", "shared/from_a.md": "x"})
	b := writeRoot(t, map[string]string{"b.md": "# B", "shared/from_b.md": "x"})

	m := sizedMulti(t, []string{a, b}, "")

	joined := strings.Join(allNodeNames(m.rootNode), "|")
	for _, want := range []string{"a.md", "b.md", "shared", "from_a.md", "from_b.md"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("merged tree missing %q; got rows: %s", want, joined)
		}
	}
}

func TestNewMulti_SearchCorpusIncludesEveryRoot(t *testing.T) {
	a := writeRoot(t, map[string]string{"a.md": "# A"})
	b := writeRoot(t, map[string]string{"sub/b.md": "# B"})

	m := sizedMulti(t, []string{a, b}, "")

	paths := m.allVaultMarkdownPaths()
	want := map[string]bool{
		filepath.Join(a, "a.md"):        false,
		filepath.Join(b, "sub", "b.md"): false,
	}
	for _, p := range paths {
		if _, ok := want[p]; ok {
			want[p] = true
		}
	}
	for p, found := range want {
		if !found {
			t.Fatalf("search corpus missing %q (got %v)", p, paths)
		}
	}
}

func TestNewMulti_WikilinkResolvesAcrossRoots(t *testing.T) {
	a := writeRoot(t, map[string]string{"from.md": "jump to [[target]]"})
	b := writeRoot(t, map[string]string{"target.md": "# Target\n\nhere"})

	m := sizedMulti(t, []string{a, b}, filepath.Join(a, "from.md"))

	// The vault should resolve the cross-root wikilink so it is not counted
	// as broken.
	if m.content.brokenCount != 0 {
		t.Fatalf("brokenCount = %d, want 0 (wikilink should resolve across roots)", m.content.brokenCount)
	}
	if m.vault == nil {
		t.Fatal("vault is nil")
	}
	got, ok := m.vault.Resolve(filepath.Join(a, "from.md"), "target", "", "")
	if !ok || got != filepath.Join(b, "target.md") {
		t.Fatalf("Resolve(target) = (%q, %v), want %q", got, ok, filepath.Join(b, "target.md"))
	}
}

func TestNewMulti_CollidingFileSelectableInTree(t *testing.T) {
	a := writeRoot(t, map[string]string{"index.md": "# A index"})
	b := writeRoot(t, map[string]string{"index.md": "# B index"})

	m := sizedMulti(t, []string{a, b}, "")

	// Both colliding files keep their real absolute paths and are reachable
	// by path in the flattened tree.
	for _, p := range []string{filepath.Join(a, "index.md"), filepath.Join(b, "index.md")} {
		if i := m.rowIndexByPath(p); i < 0 {
			t.Fatalf("colliding file %q not found in tree rows", p)
		}
	}
}
