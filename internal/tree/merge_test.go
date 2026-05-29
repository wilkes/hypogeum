package tree

import (
	"path/filepath"
	"strings"
	"testing"
)

// fileChildPaths returns the absolute paths of the file children of n, in order.
func fileChildPaths(n *Node) []string {
	var out []string
	for _, c := range n.Children {
		if !c.IsDir {
			out = append(out, c.Path)
		}
	}
	return out
}

func findChild(n *Node, name string) *Node {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestMerge_SingleRootMatchesWalk(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "a.md", "sub/b.md")

	m, err := Merge([]string{root})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	w, err := Walk(root)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if m.Path != w.Path || m.Name != w.Name {
		t.Fatalf("single-root Merge root = (%q,%q), want (%q,%q)", m.Path, m.Name, w.Path, w.Name)
	}
	if got, want := childNames(m), childNames(w); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("single-root Merge children = %v, want %v", got, want)
	}
}

func TestMerge_UnionsDirectoriesByRelativePath(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	writeFiles(t, a, "shared/from_a.md", "only_a.md")
	writeFiles(t, b, "shared/from_b.md", "only_b.md")

	root, err := Merge([]string{a, b})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Top level: one merged "shared" dir plus both unique files.
	if got := childNames(root); strings.Join(got, ",") != "shared,only_a.md,only_b.md" {
		t.Fatalf("top-level children = %v, want [shared only_a.md only_b.md]", got)
	}

	shared := findChild(root, "shared")
	if shared == nil || !shared.IsDir {
		t.Fatalf("expected merged 'shared' directory, got %+v", shared)
	}
	// The merged directory is keyed by its relative path, not an absolute one.
	if shared.Path != "shared" {
		t.Fatalf("merged dir Path = %q, want %q", shared.Path, "shared")
	}
	// It contains the union of both roots' files, each keeping its real path.
	if got := childNames(shared); strings.Join(got, ",") != "from_a.md,from_b.md" {
		t.Fatalf("merged dir children = %v, want [from_a.md from_b.md]", got)
	}
	for _, p := range fileChildPaths(shared) {
		if !filepath.IsAbs(p) {
			t.Fatalf("merged file path %q is not absolute", p)
		}
	}
}

func TestMerge_KeepsCollidingFilesDisambiguated(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	writeFiles(t, a, "index.md")
	writeFiles(t, b, "index.md")

	root, err := Merge([]string{a, b})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	if len(root.Children) != 2 {
		t.Fatalf("expected 2 colliding entries, got %d (%v)", len(root.Children), childNames(root))
	}
	wantA := "index.md (" + filepath.Base(a) + ")"
	wantB := "index.md (" + filepath.Base(b) + ")"
	names := childNames(root)
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, wantA) || !strings.Contains(joined, wantB) {
		t.Fatalf("disambiguated names = %v, want to contain %q and %q", names, wantA, wantB)
	}
	// Each kept node still points at its own real file.
	for _, c := range root.Children {
		if c.Path != filepath.Join(a, "index.md") && c.Path != filepath.Join(b, "index.md") {
			t.Fatalf("colliding node path = %q, want one of the two real files", c.Path)
		}
	}
}

func TestMerge_DedupesOverlappingRoots(t *testing.T) {
	a := t.TempDir()
	writeFiles(t, a, "note.md")

	// Same root twice: the file must not be double-listed or disambiguated.
	root, err := Merge([]string{a, a})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got := childNames(root); strings.Join(got, ",") != "note.md" {
		t.Fatalf("overlapping roots children = %v, want [note.md]", got)
	}
}

func TestMerge_NestedDirectoriesUseRelativeKeys(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	writeFiles(t, a, "x/y/deep_a.md")
	writeFiles(t, b, "x/y/deep_b.md")

	root, err := Merge([]string{a, b})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	x := findChild(root, "x")
	if x == nil || x.Path != "x" {
		t.Fatalf("dir x = %+v, want Path %q", x, "x")
	}
	y := findChild(x, "y")
	if y == nil || y.Path != "x/y" {
		t.Fatalf("dir x/y = %+v, want Path %q", y, "x/y")
	}
	if got := childNames(y); strings.Join(got, ",") != "deep_a.md,deep_b.md" {
		t.Fatalf("x/y children = %v, want [deep_a.md deep_b.md]", got)
	}
}

func TestMerge_VirtualRootNameJoinsBaseNames(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	writeFiles(t, a, "a.md")
	writeFiles(t, b, "b.md")

	root, err := Merge([]string{a, b})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	want := filepath.Base(a) + " + " + filepath.Base(b)
	if root.Name != want {
		t.Fatalf("virtual root Name = %q, want %q", root.Name, want)
	}
	if root.Path != "" {
		t.Fatalf("virtual root Path = %q, want empty", root.Path)
	}
}

func TestMerge_EmptyRootsSynthesized(t *testing.T) {
	root, err := Merge(nil)
	if err != nil {
		t.Fatalf("Merge(nil): %v", err)
	}
	if root == nil || !root.IsDir {
		t.Fatalf("Merge(nil) = %+v, want empty dir node", root)
	}
}
