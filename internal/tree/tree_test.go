package tree

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFiles lays down each path (relative to root) with placeholder content
// and creates parent directories as needed.
func writeFiles(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, rel := range paths {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// childNames returns the basenames of n.Children in order.
func childNames(n *Node) []string {
	out := make([]string, len(n.Children))
	for i, c := range n.Children {
		out[i] = c.Name
	}
	return out
}

func TestWalk_IncludesMarkdownFiles(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "a.md", "b.markdown", "c.mdown", "d.mkd")

	n, err := Walk(root)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	got := childNames(n)
	want := []string{"a.md", "b.markdown", "c.mdown", "d.mkd"}
	if len(got) != len(want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("children[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWalk_SkipsNonMarkdown(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "doc.md", "image.png", "data.json", "README")

	n, err := Walk(root)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if got := childNames(n); len(got) != 1 || got[0] != "doc.md" {
		t.Errorf("expected only doc.md, got %v", got)
	}
}

func TestWalk_SkipsHiddenFilesAndDirs(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"visible.md",
		".hidden.md",
		".git/config",
		".notes/secret.md",
	)

	n, err := Walk(root)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if got := childNames(n); len(got) != 1 || got[0] != "visible.md" {
		t.Errorf("expected only visible.md, got %v", got)
	}
}

func TestWalk_PrunesDirsWithNoMarkdown(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"keep.md",
		"empty/placeholder.txt",
		"images/pic.png",
		"deep/sub/sub/data.json",
	)

	n, err := Walk(root)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if got := childNames(n); len(got) != 1 || got[0] != "keep.md" {
		t.Errorf("expected pruned tree to contain only keep.md, got %v", got)
	}
}

func TestWalk_KeepsDirsWithNestedMarkdown(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "deep/sub/leaf.md")

	n, err := Walk(root)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if got := childNames(n); len(got) != 1 || got[0] != "deep" {
		t.Fatalf("root children = %v, want [deep]", got)
	}
	subDir := n.Children[0]
	if got := childNames(subDir); len(got) != 1 || got[0] != "sub" {
		t.Fatalf("deep children = %v, want [sub]", got)
	}
	leaf := subDir.Children[0].Children
	if len(leaf) != 1 || leaf[0].Name != "leaf.md" {
		t.Errorf("expected leaf.md at deep/sub, got %+v", leaf)
	}
}

func TestWalk_DirectoriesBeforeFilesAlphabetical(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"zebra.md",
		"apple.md",
		"Bravo/x.md",
		"alpha/y.md",
	)

	n, err := Walk(root)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	got := childNames(n)
	// Directories first (case-insensitive sort), then files.
	want := []string{"alpha", "Bravo", "apple.md", "zebra.md"}
	if len(got) != len(want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("children[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWalk_EmptyRootSynthesized(t *testing.T) {
	root := t.TempDir() // no markdown anywhere

	n, err := Walk(root)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if n == nil {
		t.Fatal("Walk on empty dir returned nil; expected synthesized empty root")
	}
	if !n.IsDir {
		t.Errorf("synthesized root IsDir = false, want true")
	}
	if len(n.Children) != 0 {
		t.Errorf("synthesized root has children: %v", childNames(n))
	}
}

func TestWalk_OnlyNonMarkdownStillSynthesizesRoot(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "a.png", "sub/b.txt")

	n, err := Walk(root)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if n == nil {
		t.Fatal("expected synthesized empty root, got nil")
	}
	if len(n.Children) != 0 {
		t.Errorf("expected empty root, got %v", childNames(n))
	}
}

func TestWalk_NonexistentRootReturnsError(t *testing.T) {
	_, err := Walk(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for nonexistent root, got nil")
	}
}

func TestWalk_PathsAreAbsolute(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "doc.md")

	// Use a relative path to confirm Walk resolves it.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	n, err := Walk(".")
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if !filepath.IsAbs(n.Path) {
		t.Errorf("root.Path = %q, want absolute", n.Path)
	}
	if len(n.Children) != 1 || !filepath.IsAbs(n.Children[0].Path) {
		t.Errorf("child path not absolute: %+v", n.Children)
	}
}

func TestWalk_CaseInsensitiveExtension(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "UPPER.MD", "Mixed.Markdown")

	n, err := Walk(root)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if got := childNames(n); len(got) != 2 {
		t.Errorf("expected 2 markdown files (case-insensitive ext), got %v", got)
	}
}

func TestMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.md"), "# a")
	mustWrite(t, filepath.Join(dir, "sub", "b.md"), "# b")
	mustWrite(t, filepath.Join(dir, "ignore.txt"), "nope")

	got, err := MarkdownFiles(dir)
	if err != nil {
		t.Fatalf("MarkdownFiles: %v", err)
	}

	want := map[string]bool{
		filepath.Join(dir, "a.md"):        true,
		filepath.Join(dir, "sub", "b.md"): true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d paths %v, want %d", len(got), got, len(want))
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected path %q", p)
		}
	}
}

// mustWrite creates parent dirs and writes content; fail the test on error.
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
