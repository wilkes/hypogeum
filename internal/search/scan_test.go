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

func TestScanFileLines_SingleMatch(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "first line\nsecond line with foo here\nthird line\n")

	lines, err := scanFileLines(context.Background(), p, "foo")
	if err != nil {
		t.Fatalf("scanFileLines: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %+v", len(lines), lines)
	}
	ln := lines[0]
	if ln.Num != 2 {
		t.Errorf("Num = %d, want 2", ln.Num)
	}
	// The snippet is built lazily from the Line's offset/length.
	if snip := RenderSnippet(ln, SnippetBudget); !strings.Contains(snip, "\x11foo\x12") {
		t.Errorf("RenderSnippet = %q, missing highlight markers", snip)
	}
}

func TestScanFileLines_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "Foo\nfoo\nFOO\n")

	lines, err := scanFileLines(context.Background(), p, "foo")
	if err != nil {
		t.Fatalf("scanFileLines: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	for i, ln := range lines {
		if ln.Num != i+1 {
			t.Errorf("lines[%d].Num = %d, want %d", i, ln.Num, i+1)
		}
	}
}

func TestScanFileLines_NoMatch(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", "nothing to see here\n")

	lines, err := scanFileLines(context.Background(), p, "missing")
	if err != nil {
		t.Fatalf("scanFileLines: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("got %d lines, want 0", len(lines))
	}
}

func TestScanFileLines_MissingFile(t *testing.T) {
	lines, err := scanFileLines(context.Background(), "/no/such/file.md", "foo")
	if err == nil {
		t.Errorf("expected error for missing file, got nil")
	}
	if lines != nil {
		t.Errorf("expected nil lines, got %+v", lines)
	}
}

func TestScanFileLines_CancelledContext(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.md", strings.Repeat("foo\n", 1000))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	lines, err := scanFileLines(ctx, p, "foo")
	if err != nil && err != context.Canceled {
		t.Fatalf("scanFileLines: %v", err)
	}
	_ = lines // we don't care about contents — cancellation may race the early-out
}
