package search

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSearch_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.md", "alpha foo bravo\n")
	b := writeFile(t, dir, "b.md", "charlie foo delta\nepsilon foo\n")
	c := writeFile(t, dir, "c.md", "no match here\n")

	hits, err := Search(context.Background(), []string{a, b, c}, "foo", 100)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("got %d hits, want 3: %+v", len(hits), hits)
	}
}

func TestSearch_EmptyPaths(t *testing.T) {
	hits, err := Search(context.Background(), nil, "foo", 100)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if hits != nil {
		t.Errorf("got %d hits, want nil", len(hits))
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "anything\n")
	hits, err := Search(context.Background(), []string{p}, "", 100)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if hits != nil {
		t.Errorf("empty query should return nil hits, got %d", len(hits))
	}
}

func TestSearch_MaxHitsCap(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("foo\n", 50)
	p := writeFile(t, dir, "a.md", body)

	hits, err := Search(context.Background(), []string{p}, "foo", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) > 10 {
		t.Errorf("expected at most 10 hits, got %d", len(hits))
	}
	if len(hits) < 1 {
		t.Errorf("expected at least 1 hit, got 0")
	}
}

func TestSearch_PreCancelledContext(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "foo\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	hits, err := Search(ctx, []string{p}, "foo", 100)
	if err != nil && err != context.Canceled {
		t.Fatalf("Search: %v", err)
	}
	_ = hits
}

func TestSearch_MissingFileSkipped(t *testing.T) {
	dir := t.TempDir()
	good := writeFile(t, dir, "good.md", "foo bar\n")
	bad := "/no/such/file.md"

	hits, err := Search(context.Background(), []string{good, bad}, "foo", 100)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Path != good {
		t.Errorf("hit path = %q, want %q", hits[0].Path, good)
	}
}

func TestSearch_CancellationStopsEarly(t *testing.T) {
	dir := t.TempDir()
	// Build a corpus large enough to take >50ms to scan.
	bigBody := strings.Repeat("foo bar baz\n", 50000)
	var paths []string
	for i := 0; i < 20; i++ {
		paths = append(paths, writeFile(t, dir, name(i), bigBody))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var done atomic.Bool
	start := time.Now()
	go func() {
		_, _ = Search(ctx, paths, "foo", 1_000_000)
		done.Store(true)
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Wait up to 200ms for the Search call to return.
	for i := 0; i < 200; i++ {
		if done.Load() {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	if !done.Load() {
		t.Fatal("Search did not return within 200ms after cancellation")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("Search took %v after cancellation, want well under 500ms", time.Since(start))
	}
}

func name(i int) string {
	return "f" + string(rune('a'+i)) + ".md"
}
