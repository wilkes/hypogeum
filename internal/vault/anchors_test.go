package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractAnchors_Headings(t *testing.T) {
	src := "# Top\n\nIntro paragraph.\n\n## Sub Section\n\nbody\n\n### Deep One\n"
	got := extractAnchors(src)

	want := map[string]int{
		"top":         1,
		"sub-section": 5,
		"deep-one":    9,
	}
	if len(got.headings) != len(want) {
		t.Fatalf("headings len = %d, want %d (%v)", len(got.headings), len(want), got.headings)
	}
	for slug, line := range want {
		if got.headings[slug] != line {
			t.Errorf("headings[%q] = %d, want %d", slug, got.headings[slug], line)
		}
	}
}

func TestExtractAnchors_Blocks(t *testing.T) {
	src := "First paragraph. ^p1\n\n- list item with id ^li\n- second item\n\n> quoted block ^q\n\n```\ncode ^notcounted\n```\n\nLast para. ^last\n"
	got := extractAnchors(src)

	cases := map[string]int{
		"p1":   1,
		"li":   3,
		"q":    6,
		"last": 12,
	}
	for id, line := range cases {
		if got.blocks[id] != line {
			t.Errorf("blocks[%q] = %d, want %d (got=%v)", id, got.blocks[id], line, got.blocks)
		}
	}
	if _, present := got.blocks["notcounted"]; present {
		t.Errorf("block marker inside fenced code should be ignored; got %v", got.blocks)
	}
}

func TestExtractAnchors_DuplicateBlockIDs_FirstWins(t *testing.T) {
	src := "First. ^dup\n\nSecond. ^dup\n"
	got := extractAnchors(src)
	if got.blocks["dup"] != 1 {
		t.Errorf("blocks[dup] = %d, want 1 (first wins)", got.blocks["dup"])
	}
}

func TestVault_BuildPopulatesAnchors(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "note.md")
	src := "# Top Heading\n\nA paragraph. ^para1\n"
	if err := os.WriteFile(notePath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatal(err)
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	entry := v.files[notePath]
	if entry == nil {
		t.Fatalf("file entry missing for %s", notePath)
	}
	if entry.anchors.headings["top-heading"] != 1 {
		t.Errorf("headings[top-heading] = %d, want 1", entry.anchors.headings["top-heading"])
	}
	if entry.anchors.blocks["para1"] != 3 {
		t.Errorf("blocks[para1] = %d, want 3", entry.anchors.blocks["para1"])
	}
}

func TestVault_ResolveAnchor(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "note.md")
	src := "# A Heading\n\nBody paragraph. ^bx\n"
	if err := os.WriteFile(notePath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatal(err)
	}

	if line, ok := v.ResolveAnchor(notePath, "A Heading", ""); !ok || line != 1 {
		t.Errorf("ResolveAnchor heading: got (%d, %v), want (1, true)", line, ok)
	}
	if line, ok := v.ResolveAnchor(notePath, "", "bx"); !ok || line != 3 {
		t.Errorf("ResolveAnchor block: got (%d, %v), want (3, true)", line, ok)
	}
	if line, ok := v.ResolveAnchor(notePath, "A Heading", "bx"); !ok || line != 3 {
		t.Errorf("ResolveAnchor both: got (%d, %v), want (3, true)", line, ok)
	}
	if _, ok := v.ResolveAnchor(notePath, "", "nope"); ok {
		t.Error("ResolveAnchor missing block: ok=true, want false")
	}
	if _, ok := v.ResolveAnchor("/nonexistent.md", "A Heading", ""); ok {
		t.Error("ResolveAnchor missing file: ok=true, want false")
	}
}
