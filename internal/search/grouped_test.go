package search

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestSearchGrouped_CountsPerFile(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.md", "foo\nbar\nfoo again\nno match\nfoo three\n") // 3 matches
	b := writeFile(t, dir, "b.md", "nothing\nFOO here\n")                        // 1 match (case-insensitive)
	c := writeFile(t, dir, "c.md", "no hits at all\n")                           // 0 — excluded

	got, err := SearchGrouped(context.Background(), []string{a, b, c}, "foo", 100)
	if err != nil {
		t.Fatalf("SearchGrouped: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d files, want 2 (c.md has no matches): %+v", len(got), got)
	}

	byPath := map[string]FileMatches{}
	for _, fm := range got {
		byPath[fm.Path] = fm
		if fm.Count != len(fm.Lines) {
			t.Errorf("%s: Count %d != len(Lines) %d", fm.Path, fm.Count, len(fm.Lines))
		}
	}
	if byPath[a].Count != 3 {
		t.Errorf("a.md Count = %d, want 3", byPath[a].Count)
	}
	if byPath[b].Count != 1 {
		t.Errorf("b.md Count = %d, want 1", byPath[b].Count)
	}
	// Lines are in ascending line order.
	wantLinesA := []int{1, 3, 5}
	gotLinesA := []int{}
	for _, ln := range byPath[a].Lines {
		gotLinesA = append(gotLinesA, ln.Num)
	}
	if !reflect.DeepEqual(gotLinesA, wantLinesA) {
		t.Errorf("a.md line numbers = %v, want %v", gotLinesA, wantLinesA)
	}
}

func TestSearchGrouped_CapsOnFilesNotHits(t *testing.T) {
	dir := t.TempDir()
	// Three files, each with many matches. A maxFiles=2 cap must return 2
	// files (not, say, a single high-match file's worth of hits).
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		writeFile(t, dir, name, strings.Repeat("foo\n", 50))
	}
	paths := []string{}
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		paths = append(paths, dir+"/"+name)
	}

	got, err := SearchGrouped(context.Background(), paths, "foo", 2)
	if err != nil {
		t.Fatalf("SearchGrouped: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d files, want exactly maxFiles=2", len(got))
	}
	// Each returned file still has its full, accurate count — the cap bounds
	// files, never truncates a file's matches.
	for _, fm := range got {
		if fm.Count != 50 {
			t.Errorf("%s Count = %d, want 50 (counts are exhaustive within a file)", fm.Path, fm.Count)
		}
	}
}

func TestSearchGrouped_Empty(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "foo\n")
	if got, _ := SearchGrouped(context.Background(), []string{p}, "", 10); got != nil {
		t.Errorf("empty query: got %+v, want nil", got)
	}
	if got, _ := SearchGrouped(context.Background(), nil, "foo", 10); got != nil {
		t.Errorf("no paths: got %+v, want nil", got)
	}
	if got, _ := SearchGrouped(context.Background(), []string{p}, "foo", 0); got != nil {
		t.Errorf("maxFiles 0: got %+v, want nil", got)
	}
}

// TestRenderSnippet_MatchesEagerHitSnippet locks the lazy snippet to the eager
// one: the snippet RenderSnippet produces for a Line must be byte-identical to
// the Snippet the hit-oriented scan builds for the same match, so the grouped
// and flat paths can never drift.
func TestRenderSnippet_MatchesEagerHitSnippet(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "the quick brown foo jumps over the lazy dog and the foo runs\n")

	lines, err := scanFileLines(context.Background(), p, "foo")
	if err != nil {
		t.Fatalf("scanFileLines: %v", err)
	}
	hits, err := SearchAll(context.Background(), []string{p}, "foo")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if len(lines) != len(hits) {
		t.Fatalf("len mismatch: %d lines vs %d hits", len(lines), len(hits))
	}
	for i := range lines {
		if got, want := RenderSnippet(lines[i], snippetBudget), hits[i].Snippet; got != want {
			t.Errorf("snippet %d: RenderSnippet=%q, eager Hit.Snippet=%q", i, got, want)
		}
	}
}
