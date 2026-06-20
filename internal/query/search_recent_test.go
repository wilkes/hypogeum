package query

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempStore points the recent store at a temp file so tests never
// touch the real on-disk state. Returns the vault dir.
func withTempStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sf := filepath.Join(t.TempDir(), "state.json")
	prev := stateFileFn
	stateFileFn = func() (string, error) { return sf, nil }
	t.Cleanup(func() { stateFileFn = prev })
	return dir
}

func TestSearch(t *testing.T) {
	dir := withTempStore(t)
	if err := os.WriteFile(filepath.Join(dir, "a.md"),
		[]byte("alpha needle beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	hits, err := Search(dir, "needle", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1: %+v", len(hits), hits)
	}
	if hits[0].Line != 1 {
		t.Errorf("Line = %d, want 1", hits[0].Line)
	}
	// Snippet must not contain highlight control chars.
	for _, b := range hits[0].Snippet {
		if b == '\x11' || b == '\x12' {
			t.Errorf("snippet retains control char: %q", hits[0].Snippet)
		}
	}
}

func TestSearchNoResults(t *testing.T) {
	dir := withTempStore(t)
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("nothing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hits, err := Search(dir, "needle", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("got %d hits, want 0", len(hits))
	}
}

func TestRecent(t *testing.T) {
	dir := withTempStore(t)
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("# b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Recent(dir, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
}

func TestRecentRespectsMax(t *testing.T) {
	dir := withTempStore(t)
	for _, n := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Recent(dir, 2)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d entries, want 2 (capped)", len(got))
	}
}
