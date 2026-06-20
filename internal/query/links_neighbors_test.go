package query

import (
	"os"
	"path/filepath"
	"testing"
)

func writeVault(t *testing.T) (dir, foo string) {
	t.Helper()
	dir = t.TempDir()
	foo = filepath.Join(dir, "foo.md")
	files := map[string]string{
		"foo.md": "See [[bar]], [missing](./nope.md), [site](https://x.com)\n",
		"bar.md": "# Bar\n",
		// Standard link (not wikilink) so the backlink carries a real 1-indexed line; the link sits on line 3.
		"baz.md": "# Baz\n\nLink to [foo](./foo.md) here\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, foo
}

func TestLinks(t *testing.T) {
	dir, foo := writeVault(t)

	links, err := Links(dir, foo)
	if err != nil {
		t.Fatalf("Links: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("got %d links, want 3: %+v", len(links), links)
	}

	if links[0].Kind != "wikilink" || links[0].Target != "[[bar]]" || links[0].Broken {
		t.Errorf("links[0] = %+v, want resolved wikilink [[bar]]", links[0])
	}
	if links[0].Path != filepath.Join(dir, "bar.md") {
		t.Errorf("links[0].Path = %q, want bar.md", links[0].Path)
	}
	if links[1].Kind != "relative" || !links[1].Broken || links[1].Path != "" {
		t.Errorf("links[1] = %+v, want broken relative with empty path", links[1])
	}
	if links[2].Kind != "external" || links[2].Broken || links[2].Target != "https://x.com" || links[2].Path != "" {
		t.Errorf("links[2] = %+v, want external https://x.com with empty path", links[2])
	}
}

func TestLinksResolvedRelative(t *testing.T) {
	dir := t.TempDir()
	foo := filepath.Join(dir, "foo.md")
	if err := os.WriteFile(foo, []byte("[real](./bar.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.md"), []byte("# bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	links, err := Links(dir, foo)
	if err != nil {
		t.Fatalf("Links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1", len(links))
	}
	if links[0].Kind != "relative" || links[0].Broken || links[0].Path != filepath.Join(dir, "bar.md") {
		t.Errorf("links[0] = %+v, want resolved relative to bar.md", links[0])
	}
}

func TestNeighbors(t *testing.T) {
	dir, foo := writeVault(t)

	n, err := Neighbors(dir, foo)
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if n.File != foo {
		t.Errorf("File = %q, want %q", n.File, foo)
	}
	if len(n.Outbound) != 3 {
		t.Errorf("got %d outbound, want 3", len(n.Outbound))
	}
	// baz.md links to foo via [foo](./foo.md) on line 3.
	if len(n.Backlinks) != 1 {
		t.Fatalf("got %d backlinks, want 1: %+v", len(n.Backlinks), n.Backlinks)
	}
	if n.Backlinks[0].Path != filepath.Join(dir, "baz.md") {
		t.Errorf("backlink path = %q, want baz.md", n.Backlinks[0].Path)
	}
	if n.Backlinks[0].Line != 3 {
		t.Errorf("backlink line = %d, want 3 (surfaced verbatim)", n.Backlinks[0].Line)
	}
}

func TestLinksFileNotFound(t *testing.T) {
	dir := t.TempDir()
	if _, err := Links(dir, filepath.Join(dir, "ghost.md")); err == nil {
		t.Error("Links on missing file returned nil error, want non-nil")
	}
}

func TestNeighborsFileNotFound(t *testing.T) {
	dir := t.TempDir()
	if _, err := Neighbors(dir, filepath.Join(dir, "ghost.md")); err == nil {
		t.Error("Neighbors on missing file returned nil error, want non-nil")
	}
}
