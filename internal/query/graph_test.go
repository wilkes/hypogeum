package query

import (
	"os"
	"path/filepath"
	"testing"
)

// writeGraphVault builds a small vault exercising every edge kind plus an orphan.
func writeGraphVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		// index.md: a resolved wikilink, a broken wikilink, an external URL,
		// a self anchor, and a resolved relative link.
		"index.md": "[[arch]] [[ghost]] [site](https://charm.sh) [top](#intro) [a](./arch.md)\n",
		"arch.md":  "# Arch\n",
		"orphan.md": "# Orphan, no links\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestGraphNodes(t *testing.T) {
	dir := writeGraphVault(t)
	g, err := GraphFor(dir)
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	// Nodes are every md file, orphan included, sorted by path.
	want := []string{
		filepath.Join(dir, "arch.md"),
		filepath.Join(dir, "index.md"),
		filepath.Join(dir, "orphan.md"),
	}
	if len(g.Nodes) != len(want) {
		t.Fatalf("got %d nodes, want %d: %+v", len(g.Nodes), len(want), g.Nodes)
	}
	for i, w := range want {
		if g.Nodes[i].Path != w {
			t.Errorf("Nodes[%d].Path = %q, want %q", i, g.Nodes[i].Path, w)
		}
	}
}

func TestGraphEdges(t *testing.T) {
	dir := writeGraphVault(t)
	g, err := GraphFor(dir)
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	index := filepath.Join(dir, "index.md")
	arch := filepath.Join(dir, "arch.md")

	// All five edges originate from index.md, in document order.
	if len(g.Edges) != 5 {
		t.Fatalf("got %d edges, want 5: %+v", len(g.Edges), g.Edges)
	}
	for i, e := range g.Edges {
		if e.From != index {
			t.Errorf("Edges[%d].From = %q, want index.md", i, e.From)
		}
	}
	// e0: resolved wikilink [[arch]]
	if g.Edges[0].Kind != "wikilink" || g.Edges[0].To != arch || g.Edges[0].Broken {
		t.Errorf("Edges[0] = %+v, want resolved wikilink -> arch.md", g.Edges[0])
	}
	// e1: broken wikilink [[ghost]]
	if g.Edges[1].Kind != "wikilink" || g.Edges[1].To != "" || !g.Edges[1].Broken {
		t.Errorf("Edges[1] = %+v, want broken wikilink with empty To", g.Edges[1])
	}
	// e2: external URL
	if g.Edges[2].Kind != "external" || g.Edges[2].To != "https://charm.sh" || g.Edges[2].Broken {
		t.Errorf("Edges[2] = %+v, want external https://charm.sh", g.Edges[2])
	}
	// e3: self anchor
	if g.Edges[3].Kind != "anchor" || g.Edges[3].To != "#intro" || g.Edges[3].Broken {
		t.Errorf("Edges[3] = %+v, want anchor #intro", g.Edges[3])
	}
	// e4: resolved relative link
	if g.Edges[4].Kind != "relative" || g.Edges[4].To != arch || g.Edges[4].Broken {
		t.Errorf("Edges[4] = %+v, want resolved relative -> arch.md", g.Edges[4])
	}
}

func TestGraphEmptyVault(t *testing.T) {
	g, err := GraphFor(t.TempDir())
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	if g.Nodes == nil || g.Edges == nil {
		t.Errorf("Graph on empty vault must init slices, got %+v", g)
	}
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Errorf("empty vault should have no nodes/edges, got %+v", g)
	}
}
