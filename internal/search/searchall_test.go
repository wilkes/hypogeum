package search

import (
	"context"
	"testing"
)

func TestSearchAll_DeterministicOrder(t *testing.T) {
	dir := t.TempDir()
	// a.md: matches on lines 1 and 3
	a := writeFile(t, dir, "a.md", "needle here\nno match\nneedle again\n")
	// b.md: match on line 2
	b := writeFile(t, dir, "b.md", "nothing\nneedle found\n")
	// c.md: no match
	c := writeFile(t, dir, "c.md", "nothing at all\n")

	hits, err := SearchAll(context.Background(), []string{a, b, c}, "needle")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}

	// Expect exactly 3 hits in deterministic (path, line) order.
	want := []struct {
		path string
		line int
	}{
		{a, 1},
		{a, 3},
		{b, 2},
	}
	if len(hits) != len(want) {
		t.Fatalf("got %d hits, want %d: %+v", len(hits), len(want), hits)
	}
	for i, w := range want {
		if hits[i].Path != w.path || hits[i].Line != w.line {
			t.Errorf("hits[%d] = {%s, %d}, want {%s, %d}",
				i, hits[i].Path, hits[i].Line, w.path, w.line)
		}
	}
}

func TestSearchAll_EmptyQuery(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "needle here\n")

	hits, err := SearchAll(context.Background(), []string{p}, "")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if hits != nil {
		t.Errorf("empty query: got %d hits, want nil", len(hits))
	}
}

func TestSearchAll_EmptyPaths(t *testing.T) {
	hits, err := SearchAll(context.Background(), nil, "needle")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if hits != nil {
		t.Errorf("nil paths: got %d hits, want nil", len(hits))
	}
}

func TestSearchAll_MissingFileSkipped(t *testing.T) {
	dir := t.TempDir()
	good := writeFile(t, dir, "good.md", "needle here\n")
	bad := "/no/such/file.md"

	hits, err := SearchAll(context.Background(), []string{good, bad}, "needle")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1: %+v", len(hits), hits)
	}
	if hits[0].Path != good {
		t.Errorf("hit path = %q, want %q", hits[0].Path, good)
	}
}

func TestSearchAll_NoCap(t *testing.T) {
	dir := t.TempDir()
	// 100 matching lines — SearchAll must return all of them.
	var body string
	for i := 0; i < 100; i++ {
		body += "needle line\n"
	}
	p := writeFile(t, dir, "big.md", body)

	hits, err := SearchAll(context.Background(), []string{p}, "needle")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if len(hits) != 100 {
		t.Errorf("got %d hits, want 100", len(hits))
	}
}
