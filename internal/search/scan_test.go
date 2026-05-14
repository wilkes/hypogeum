package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScanFile_SingleMatch(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "first line\nsecond line with foo here\nthird line\n")

	hits, err := scanFile(context.Background(), p, "foo")
	if err != nil {
		t.Fatalf("scanFile: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1: %+v", len(hits), hits)
	}
	h := hits[0]
	if h.Path != p {
		t.Errorf("Path = %q, want %q", h.Path, p)
	}
	if h.Line != 2 {
		t.Errorf("Line = %d, want 2", h.Line)
	}
	if !strings.Contains(h.Snippet, "\x11foo\x12") {
		t.Errorf("Snippet = %q, missing highlight markers", h.Snippet)
	}
}

func TestScanFile_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "Foo\nfoo\nFOO\n")

	hits, err := scanFile(context.Background(), p, "foo")
	if err != nil {
		t.Fatalf("scanFile: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("got %d hits, want 3", len(hits))
	}
	for i, h := range hits {
		if h.Line != i+1 {
			t.Errorf("hits[%d].Line = %d, want %d", i, h.Line, i+1)
		}
	}
}

func TestScanFile_NoMatch(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "nothing to see here\n")

	hits, err := scanFile(context.Background(), p, "missing")
	if err != nil {
		t.Fatalf("scanFile: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("got %d hits, want 0", len(hits))
	}
}

func TestScanFile_MissingFile(t *testing.T) {
	hits, err := scanFile(context.Background(), "/no/such/file.md", "foo")
	if err == nil {
		t.Errorf("expected error for missing file, got nil")
	}
	if hits != nil {
		t.Errorf("expected nil hits, got %+v", hits)
	}
}

func TestScanFile_BinaryNULIsSkipped(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "abc\x00def foo\n")

	hits, err := scanFile(context.Background(), p, "foo")
	if err != nil {
		t.Fatalf("scanFile: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("binary file should yield no hits, got %d", len(hits))
	}
}

func TestScanFile_CancelledContext(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", strings.Repeat("foo\n", 1000))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	hits, err := scanFile(ctx, p, "foo")
	if err != nil && err != context.Canceled {
		t.Fatalf("scanFile: %v", err)
	}
	_ = hits // we don't care about contents — cancellation may race the early-out
}
