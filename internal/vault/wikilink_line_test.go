package vault

import (
	"path/filepath"
	"testing"
)

// TestWikilinkBacklinkLine guards against a regression where wikilink
// references reported line 0 because lineForNode found no ast.Text child
// on the wikilink node. A standard-markdown-link backlink already reports
// the correct line, so both should agree on a wikilink/std link placed on
// the same source line.
func TestWikilinkBacklinkLine(t *testing.T) {
	dir := t.TempDir()
	// The wikilink sits on line 3 (1-indexed).
	writeFile(t, dir, "source.md", "# Source\n\nSee [[target]] for details.\n")
	writeFile(t, dir, "target.md", "# Target\n")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	bls := v.Backlinks(filepath.Join(dir, "target.md"))
	if len(bls) != 1 {
		t.Fatalf("Backlinks: got %d want 1: %+v", len(bls), bls)
	}
	if bls[0].Line != 3 {
		t.Errorf("wikilink backlink Line = %d, want 3", bls[0].Line)
	}
}

// TestWikilinkLineMultiline checks a wikilink deeper in the document so a
// constant offset (e.g. always 1) wouldn't accidentally pass.
func TestWikilinkLineMultiline(t *testing.T) {
	dir := t.TempDir()
	// Lines: 1 "# Notes", 2 "", 3 "alpha", 4 "", 5 "beta [[target]] gamma".
	writeFile(t, dir, "source.md", "# Notes\n\nalpha\n\nbeta [[target]] gamma\n")
	writeFile(t, dir, "target.md", "# Target\n")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	bls := v.Backlinks(filepath.Join(dir, "target.md"))
	if len(bls) != 1 {
		t.Fatalf("Backlinks: got %d want 1: %+v", len(bls), bls)
	}
	if bls[0].Line != 5 {
		t.Errorf("wikilink backlink Line = %d, want 5", bls[0].Line)
	}
}
