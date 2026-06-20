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

// recordVisit opens the store behind stateFileFn and records a visit to
// path, then closes it (writing through to the temp state file). Recent then
// reads the same persisted state. Used to seed visit history in tests since
// Recent now reports visited-only.
func recordVisit(t *testing.T, path string) {
	t.Helper()
	store, err := loadStore()
	if err != nil || store == nil {
		t.Fatalf("loadStore: %v", err)
	}
	if err := store.Record(path); err != nil {
		t.Fatalf("Record: %v", err)
	}
}

func TestRecentVisitedOnly(t *testing.T) {
	dir := withTempStore(t)
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	unvisited := filepath.Join(dir, "c.md")
	for _, p := range []string{a, b, unvisited} {
		if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Visit a then b → b is most recent, c is never opened.
	recordVisit(t, a)
	recordVisit(t, b)

	got, err := Recent(dir, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (unvisited excluded): %+v", len(got), got)
	}
	if got[0].Path != b {
		t.Errorf("first entry %q, want %q (most recent visit)", got[0].Path, b)
	}
	if got[1].Path != a {
		t.Errorf("second entry %q, want %q", got[1].Path, a)
	}
	for _, e := range got {
		if e.Path == unvisited {
			t.Errorf("unvisited file %q should be excluded", unvisited)
		}
		if e.Visited.IsZero() {
			t.Errorf("entry %q has zero Visited time", e.Path)
		}
	}
}

func TestRecentNoVisitsIsEmpty(t *testing.T) {
	dir := withTempStore(t)
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Recent(dir, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want 0 (nothing visited)", len(got))
	}
}

func TestRecentRespectsMax(t *testing.T) {
	dir := withTempStore(t)
	for _, n := range []string{"a.md", "b.md", "c.md"} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		recordVisit(t, p)
	}
	got, err := Recent(dir, 2)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d entries, want 2 (capped)", len(got))
	}
}
