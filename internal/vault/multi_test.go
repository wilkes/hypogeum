package vault

import (
	"path/filepath"
	"testing"
)

// TestBuildRoots_ResolvesWikilinksAcrossRoots verifies the index is unified:
// a wikilink in one root resolves to a file living in another root.
func TestBuildRoots_ResolvesWikilinksAcrossRoots(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	from := writeFile(t, a, "from.md", "see [[target]] for details")
	target := writeFile(t, b, "target.md", "I am the target")

	v, err := BuildRoots([]string{a, b}, NopDiagnostics{})
	if err != nil {
		t.Fatalf("BuildRoots: %v", err)
	}

	got, ok := v.Resolve(from, "target", "", "")
	if !ok {
		t.Fatalf("Resolve(target) across roots: not found")
	}
	if got != target {
		t.Fatalf("Resolve(target) = %q, want %q", got, target)
	}
}

// TestBuildRoots_BacklinksSpanRoots verifies the reverse index also crosses
// root boundaries.
func TestBuildRoots_BacklinksSpanRoots(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	src := writeFile(t, a, "src.md", "points at [[dest]]")
	dest := writeFile(t, b, "dest.md", "destination")

	v, err := BuildRoots([]string{a, b}, NopDiagnostics{})
	if err != nil {
		t.Fatalf("BuildRoots: %v", err)
	}

	bl := v.Backlinks(dest)
	if len(bl) != 1 {
		t.Fatalf("Backlinks(dest) = %d, want 1 (%+v)", len(bl), bl)
	}
	if bl[0].SourceFile != src {
		t.Fatalf("backlink source = %q, want %q", bl[0].SourceFile, src)
	}
}

// TestBuildRoots_IndexesEveryRoot is a sanity check that all roots' files
// land in the index.
func TestBuildRoots_IndexesEveryRoot(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	writeFile(t, a, "one.md", "x")
	writeFile(t, a, "sub/two.md", "x")
	writeFile(t, b, "three.md", "x")

	v, err := BuildRoots([]string{a, b}, NopDiagnostics{})
	if err != nil {
		t.Fatalf("BuildRoots: %v", err)
	}
	if got := v.fileCount(); got != 3 {
		t.Fatalf("fileCount = %d, want 3", got)
	}
}

// TestBuild_SingleRootWrapper confirms Build still behaves as a single-root
// BuildRoots.
func TestBuild_SingleRootWrapper(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "x")
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := v.fileCount(); got != 1 {
		t.Fatalf("fileCount = %d, want 1", got)
	}
	abs, _ := filepath.Abs(dir)
	if len(v.roots) != 1 || v.roots[0] != abs {
		t.Fatalf("roots = %v, want [%q]", v.roots, abs)
	}
}
