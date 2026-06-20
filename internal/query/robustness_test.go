package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Finding #2: file path resolves against the vault root, not cwd ---

func TestLinksRelativeFileResolvesAgainstVaultRoot(t *testing.T) {
	// Vault lives at dir; cwd is somewhere else entirely.
	dir := t.TempDir()
	files := map[string]string{
		"foo.md": "Link to [bar](./bar.md)\n",
		"bar.md": "# Bar\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Run from an unrelated cwd so a cwd-relative resolution would miss.
	chdirTemp(t)

	links, err := Links(dir, "foo.md")
	if err != nil {
		t.Fatalf("Links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1: %+v", len(links), links)
	}
	if links[0].Kind != "relative" || links[0].Broken {
		t.Errorf("links[0] = %+v, want resolved (non-broken) relative", links[0])
	}
	if links[0].Path != filepath.Join(dir, "bar.md") {
		t.Errorf("links[0].Path = %q, want %q", links[0].Path, filepath.Join(dir, "bar.md"))
	}
}

func TestNeighborsRelativeFileResolvesAgainstVaultRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.md"), []byte("[bar](./bar.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.md"), []byte("# Bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdirTemp(t)

	n, err := Neighbors(dir, "foo.md")
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if n.File != filepath.Join(dir, "foo.md") {
		t.Errorf("File = %q, want %q", n.File, filepath.Join(dir, "foo.md"))
	}
	if len(n.Outbound) != 1 || n.Outbound[0].Broken {
		t.Errorf("Outbound = %+v, want one resolved link", n.Outbound)
	}
}

// chdirTemp moves the process into a fresh temp dir for the duration of the
// test, restoring the original cwd on cleanup.
func chdirTemp(t *testing.T) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// --- Finding #3: degraded store — both verbs degrade, neither hard-fails ---

// corruptStore points stateFileFn at a malformed state file so recent.New
// returns a usable-but-empty store alongside a non-nil error.
func corruptStore(t *testing.T) {
	t.Helper()
	sf := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(sf, []byte("{ this is not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := stateFileFn
	stateFileFn = func() (string, error) { return sf, nil }
	t.Cleanup(func() { stateFileFn = prev })
}

func TestSearchDegradesOnCorruptStore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"),
		[]byte("alpha needle beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	corruptStore(t)

	hits, err := Search(dir, "needle", 10)
	if err != nil {
		t.Fatalf("Search degraded path returned error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1 (degraded still returns results)", len(hits))
	}
}

func TestRecentDegradesOnCorruptStore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("# b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	corruptStore(t)

	// Recent now reports visited-only. A corrupt store degrades to a usable
	// (non-nil) store with an empty visit map, so the graceful-degradation
	// contract is: no error, and zero entries (nothing has a recorded visit).
	got, err := Recent(dir, 10)
	if err != nil {
		t.Fatalf("Recent degraded path returned error: %v, want graceful degradation", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d entries, want 0 (corrupt store has no visits)", len(got))
	}
}

// --- Findings #4 + #6: link taxonomy for non-http schemes and anchors ---

func TestOutboundLinkTaxonomy(t *testing.T) {
	dir := t.TempDir()
	foo := filepath.Join(dir, "foo.md")
	content := "[mail](mailto:a@b.com)\n\n" +
		"[ftp](ftp://host/file)\n\n" +
		"[anchor](#section)\n\n" +
		"[rel](./bar.md)\n"
	if err := os.WriteFile(foo, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.md"), []byte("# Bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	links, err := Links(dir, foo)
	if err != nil {
		t.Fatalf("Links: %v", err)
	}
	if len(links) != 4 {
		t.Fatalf("got %d links, want 4: %+v", len(links), links)
	}

	// mailto: → external, never broken, no path.
	if links[0].Kind != "external" || links[0].Broken || links[0].Path != "" {
		t.Errorf("mailto link = %+v, want external, not broken, empty path", links[0])
	}
	// ftp: → external.
	if links[1].Kind != "external" || links[1].Broken {
		t.Errorf("ftp link = %+v, want external, not broken", links[1])
	}
	// #section → anchor, never broken.
	if links[2].Kind != "anchor" || links[2].Broken {
		t.Errorf("anchor link = %+v, want anchor, not broken", links[2])
	}
	// ./bar.md → relative, resolved, not broken.
	if links[3].Kind != "relative" || links[3].Broken || links[3].Path != filepath.Join(dir, "bar.md") {
		t.Errorf("relative link = %+v, want resolved relative to bar.md", links[3])
	}
}

// --- Finding #10: Neighbors emits [] not null for empty slices ---

func TestNeighborsEmitsEmptyArraysNotNull(t *testing.T) {
	dir := t.TempDir()
	foo := filepath.Join(dir, "lonely.md")
	if err := os.WriteFile(foo, []byte("# Lonely, no links, no backlinks\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := Neighbors(dir, foo)
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if n.Outbound == nil {
		t.Error("Outbound is nil; want non-nil empty slice")
	}
	if n.Backlinks == nil {
		t.Error("Backlinks is nil; want non-nil empty slice")
	}

	blob, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	s := string(blob)
	if !strings.Contains(s, `"outbound":[]`) {
		t.Errorf("JSON missing outbound:[]: %s", s)
	}
	if !strings.Contains(s, `"backlinks":[]`) {
		t.Errorf("JSON missing backlinks:[]: %s", s)
	}
}
