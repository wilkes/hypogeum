package mcp

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/wilkes/hypogeum/internal/query"
)

// writeVault lays down a small fixture vault and returns its root. It mirrors
// the shape used by internal/query's tests so the lockstep assertions below
// compare against a familiar graph.
func writeVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"foo.md": "See [[bar]], [missing](./nope.md), [site](https://x.com)\n",
		"bar.md": "# Bar\n",
		"baz.md": "# Baz\n\nLink to [foo](./foo.md) here\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// newTestServer builds a server over the fixture and registers Close cleanup so
// the watcher goroutine is torn down at the end of the test.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(writeVault(t), "test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestHandleSearch(t *testing.T) {
	s := newTestServer(t)
	hits, err := s.handleSearch(searchArgs{Term: "Bar"})
	if err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("got 0 hits for \"Bar\", want at least 1")
	}
	if _, err := s.handleSearch(searchArgs{Term: "   "}); err == nil {
		t.Error("blank term returned nil error, want non-nil")
	}
}

func TestHandleLinks(t *testing.T) {
	s := newTestServer(t)
	links, err := s.handleLinks(fileArgs{File: "foo.md"})
	if err != nil {
		t.Fatalf("handleLinks: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("got %d links, want 3: %+v", len(links), links)
	}
	if links[0].Kind != "wikilink" || links[0].Broken {
		t.Errorf("links[0] = %+v, want resolved wikilink", links[0])
	}
	if _, err := s.handleLinks(fileArgs{}); err == nil {
		t.Error("missing file returned nil error, want non-nil")
	}
}

// TestHandleNeighbors_MatchesColdQuery is the CLI/MCP lockstep guard: the
// warm-index handler must produce exactly what the cold query.Neighbors path
// produces for the same vault. If the *FromVault refactor ever drifts from the
// build-per-call path, this fails.
func TestHandleNeighbors_MatchesColdQuery(t *testing.T) {
	s := newTestServer(t)

	warm, err := s.handleNeighbors(fileArgs{File: "foo.md"})
	if err != nil {
		t.Fatalf("handleNeighbors: %v", err)
	}
	cold, err := query.Neighbors(s.root, "foo.md")
	if err != nil {
		t.Fatalf("query.Neighbors: %v", err)
	}
	if !reflect.DeepEqual(warm, cold) {
		t.Errorf("warm neighbors != cold neighbors\nwarm: %+v\ncold: %+v", warm, cold)
	}
	if _, err := s.handleNeighbors(fileArgs{}); err == nil {
		t.Error("missing file returned nil error, want non-nil")
	}
}

func TestHandleGraph_MatchesColdQuery(t *testing.T) {
	s := newTestServer(t)

	warm, err := s.handleGraph()
	if err != nil {
		t.Fatalf("handleGraph: %v", err)
	}
	cold, err := query.GraphFor(s.root)
	if err != nil {
		t.Fatalf("query.GraphFor: %v", err)
	}
	if !reflect.DeepEqual(warm, cold) {
		t.Errorf("warm graph != cold graph\nwarm: %+v\ncold: %+v", warm, cold)
	}
	// fixture has 3 markdown docs.
	if len(warm.Nodes) != 3 {
		t.Errorf("got %d nodes, want 3", len(warm.Nodes))
	}
}

func TestHandleReadNote(t *testing.T) {
	s := newTestServer(t)

	got, err := s.handleReadNote(fileArgs{File: "bar.md"})
	if err != nil {
		t.Fatalf("handleReadNote: %v", err)
	}
	if got.Content != "# Bar\n" {
		t.Errorf("content = %q, want %q", got.Content, "# Bar\n")
	}
	if got.Path != filepath.Join(s.root, "bar.md") {
		t.Errorf("path = %q, want %s/bar.md", got.Path, s.root)
	}

	// Path traversal outside the root is refused.
	if _, err := s.handleReadNote(fileArgs{File: "../escape.md"}); err == nil {
		t.Error("read_note on ../escape.md returned nil error, want refusal")
	}
	// Missing file is an error, not a panic.
	if _, err := s.handleReadNote(fileArgs{File: "ghost.md"}); err == nil {
		t.Error("read_note on missing file returned nil error, want non-nil")
	}
}

// TestHandleReadNote_RefusesSymlinkEscape covers a symlink that lives inside
// the vault but points outside it: a purely lexical containment check passes
// it, so resolveUnderRoot must also resolve symlinks. The secret file must
// exist (the escape only reads when the resolved target exists), so we create
// one outside the root and link to its directory from within.
func TestHandleReadNote_RefusesSymlinkEscape(t *testing.T) {
	s := newTestServer(t)

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(s.root, "escape") // s.root/escape -> outside
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}

	if _, err := s.handleReadNote(fileArgs{File: "escape/secret.txt"}); err == nil {
		t.Error("read_note followed an in-vault symlink out of the root, want refusal")
	}
}

// TestWarmIndexConcurrentRefresh exercises the one piece of genuinely new
// concurrency: tool-call readers (RLock) racing the watcher's refresh path
// (Lock). Run under -race; it asserts no data race and that reads keep
// returning a consistent graph while rebuild/refreshFile churn the cache.
func TestWarmIndexConcurrentRefresh(t *testing.T) {
	s := newTestServer(t)
	// Warm the cache so rebuild/refreshFile actually do work (they no-op while
	// the vault is nil).
	if _, err := s.handleGraph(); err != nil {
		t.Fatalf("warm: %v", err)
	}

	var wg sync.WaitGroup
	const goroutines = 8
	const iters = 50

	// Readers.
	for r := 0; r < goroutines; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				if _, err := s.handleNeighbors(fileArgs{File: "foo.md"}); err != nil {
					t.Errorf("handleNeighbors: %v", err)
					return
				}
				if _, err := s.handleGraph(); err != nil {
					t.Errorf("handleGraph: %v", err)
					return
				}
			}
		}()
	}
	// Writers: simulate the watcher's two refresh paths.
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				s.idx.refreshFile(filepath.Join(s.root, "bar.md"))
				s.idx.rebuild()
			}
		}()
	}
	wg.Wait()

	// After all the churn the graph is still the cold-equivalent result.
	warm, err := s.handleGraph()
	if err != nil {
		t.Fatalf("post-churn handleGraph: %v", err)
	}
	cold, err := query.GraphFor(s.root)
	if err != nil {
		t.Fatalf("query.GraphFor: %v", err)
	}
	if !reflect.DeepEqual(warm, cold) {
		t.Errorf("post-churn graph diverged from cold build")
	}
}

func TestNewRejectsNonDirectory(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.md")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(f, "test"); err == nil {
		t.Error("New on a file returned nil error, want non-nil")
	}
	if _, err := New(filepath.Join(dir, "missing"), "test"); err == nil {
		t.Error("New on a missing path returned nil error, want non-nil")
	}
}
