package search

import (
	"context"
	"sort"
	"testing"
)

// TestSearch_SubsetOfSearchAll asserts that the capped Search returns a
// subset of SearchAll's hits for the same corpus: every Search hit must
// correspond to a (path, line) that SearchAll also reports. This pins the
// shared scan core — both must surface the same matches; Search just caps.
func TestSearch_SubsetOfSearchAll(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.md", "needle one\nplain\nneedle two\n")
	b := writeFile(t, dir, "b.md", "needle three\n")
	c := writeFile(t, dir, "c.md", "no match\nneedle four\n")
	paths := []string{a, b, c}

	all, err := SearchAll(context.Background(), paths, "needle")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("SearchAll got %d hits, want 4: %+v", len(all), all)
	}

	allKeys := make(map[string]bool, len(all))
	for _, h := range all {
		allKeys[key(h)] = true
	}

	// A generous cap returns every hit; assert each is also in SearchAll.
	capped, err := Search(context.Background(), paths, "needle", 100)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(capped) != len(all) {
		t.Fatalf("Search got %d hits, want %d (cap is generous)", len(capped), len(all))
	}
	for _, h := range capped {
		if !allKeys[key(h)] {
			t.Errorf("Search hit %+v not present in SearchAll results", h)
		}
	}
}

// TestSearchAll_SortedEvenWhenCancelled pins the bonus fix: a pre-cancelled
// context returns ctx.Err(), but any partial slice must still be in
// (path, line) order, matching the doc-comment promise of determinism.
func TestSearchAll_SortedEvenWhenCancelled(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.md", "needle\n")
	b := writeFile(t, dir, "b.md", "needle\n")
	c := writeFile(t, dir, "c.md", "needle\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	hits, err := SearchAll(ctx, []string{a, b, c}, "needle")
	if err != nil && err != context.Canceled {
		t.Fatalf("SearchAll: %v", err)
	}
	// Whatever partial slice came back must be sorted by (path, line).
	if !sort.SliceIsSorted(hits, func(i, j int) bool {
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Line < hits[j].Line
	}) {
		t.Errorf("SearchAll returned unsorted hits on cancellation: %+v", hits)
	}
}

func key(h Hit) string {
	return h.Path + "\x00" + string(rune(h.Line))
}
