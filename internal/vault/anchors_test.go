package vault

import (
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
