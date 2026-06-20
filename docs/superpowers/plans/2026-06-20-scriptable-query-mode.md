# Scriptable Query Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a non-interactive query mode to the `hypogeum` binary that emits JSON vault-query results to stdout and exits, for use by scripts and LLM agents.

**Architecture:** A new pure `internal/query` package orchestrates the existing `tree`, `search`, `vault`, and `recent` packages and returns JSON-tagged result structs. `cmd/hypogeum` gains git-style dispatch: a reserved first-arg verb (`search`/`links`/`recent`/`neighbors`) routes to query mode; anything else falls through to the existing TUI path. One small accessor (`Vault.Outbound`) is added to surface already-indexed outbound links, symmetric with the existing `Backlinks`.

**Tech Stack:** Go 1.24, standard library (`encoding/json`, `flag`, `context`), existing internal packages.

## Global Constraints

- Go 1.24; standard Charm stack already vendored — no new third-party dependencies.
- Keep the test suite race-clean: `go test -race ./...` is a CI gate.
- TUI behavior must not change. Query mode is additive.
- Pure packages stay TUI-free (`tree`, `search`, `vault`, `recent`, `query` must not import `internal/tui`).
- Snippets emitted as JSON must have the `highlight.Open`/`highlight.Close` control chars (`\x11`/`\x12`) stripped.
- stdout carries only JSON on success; all error messages go to stderr.
- Exit `0` on success (including zero results); exit `1` on operational failure.

---

## File Structure

- `internal/tree/tree.go` (modify) — add `MarkdownFiles(root) ([]string, error)` flattening helper.
- `internal/vault/outbound.go` (create) — `Outbound` struct, `OutboundKind`, `(*Vault).Outbound`.
- `internal/search/rerank.go` (create) — extracted `RerankByRecency` shared by TUI and query.
- `internal/tui/search.go` (modify) — `rerankByRecency` delegates to `search.RerankByRecency`.
- `internal/query/query.go` (create) — result structs, `Search`, `Links`, `Recent`, `Neighbors`, snippet sanitizer, store seam.
- `cmd/hypogeum/query.go` (create) — reserved-verb dispatch, per-verb flag parsing, JSON encoding.
- `cmd/hypogeum/main.go` (modify) — call into the dispatcher before path resolution.
- `README.md` + `CLAUDE.md` (modify) — document the query verbs and gotchas.

---

### Task 1: `tree.MarkdownFiles` flattening helper

**Files:**
- Modify: `internal/tree/tree.go`
- Test: `internal/tree/tree_test.go`

**Interfaces:**
- Consumes: existing `tree.Walk(root) (*Node, error)` and `tree.Node{Path, IsDir, Children}`.
- Produces: `func MarkdownFiles(root string) ([]string, error)` — every markdown file under `root` as an absolute path, in tree (depth-first) order. Empty slice (not nil-error) when the tree has no files.

- [ ] **Step 1: Write the failing test**

Add to `internal/tree/tree_test.go`:

```go
func TestMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.md"), "# a")
	mustWrite(t, filepath.Join(dir, "sub", "b.md"), "# b")
	mustWrite(t, filepath.Join(dir, "ignore.txt"), "nope")

	got, err := MarkdownFiles(dir)
	if err != nil {
		t.Fatalf("MarkdownFiles: %v", err)
	}

	want := map[string]bool{
		filepath.Join(dir, "a.md"):        true,
		filepath.Join(dir, "sub", "b.md"): true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d paths %v, want %d", len(got), got, len(want))
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected path %q", p)
		}
	}
}

// mustWrite creates parent dirs and writes content; fail the test on error.
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

If `mustWrite` already exists in the test file, reuse it and delete the duplicate above. Ensure `os` and `path/filepath` are imported in the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tree/ -run TestMarkdownFiles -v`
Expected: FAIL — `undefined: MarkdownFiles`.

- [ ] **Step 3: Implement the helper**

Append to `internal/tree/tree.go`:

```go
// MarkdownFiles walks root and returns every markdown file as an
// absolute path, in depth-first tree order. The tree is already pruned
// to markdown-only by Walk, so this just flattens leaf nodes. Returns an
// empty slice (never nil-with-nil-error) when nothing matches.
func MarkdownFiles(root string) ([]string, error) {
	n, err := Walk(root)
	if err != nil {
		return nil, err
	}
	out := []string{}
	var walk func(*Node)
	walk = func(nd *Node) {
		if nd == nil {
			return
		}
		if !nd.IsDir {
			out = append(out, nd.Path)
			return
		}
		for _, c := range nd.Children {
			walk(c)
		}
	}
	walk(n)
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tree/ -run TestMarkdownFiles -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tree/tree.go internal/tree/tree_test.go
git commit -m "feat(tree): add MarkdownFiles flattening helper"
```

---

### Task 2: `Vault.Outbound` accessor

**Files:**
- Create: `internal/vault/outbound.go`
- Test: `internal/vault/outbound_test.go`

**Interfaces:**
- Consumes: existing private `(*Vault).files map[string]*fileEntry`, `fileEntry.refs []reference`, `reference{kind, target, resolved, displayText, snippet, line}`, and the `refWikilink` constant. The `files` map is keyed by absolute path.
- Produces:
  - `type OutboundKind int` with `OutboundWikilink` (= 0) and `OutboundStdLink`.
  - `type Outbound struct { DisplayText, RawTarget, Resolved string; Line int; Snippet string; Kind OutboundKind }`. `RawTarget` is the bare target as indexed (the wikilink name, e.g. `bar`, or the std-link href). `Resolved` is the vault's already-computed target path: for wikilinks it is existence-based (`""` when no indexed file matches); for std links it is a pure path computation (non-empty even when the file is missing — `resolveStdLink` does not check existence). **The accessor surfaces `ref.resolved` verbatim — no filesystem I/O.** Determining whether a relative link is "broken" is the query layer's job (Task 5), not the accessor's.
  - `func (v *Vault) Outbound(path string) []Outbound` — outbound references from `path` in document order; empty when the file is not indexed.

- [ ] **Step 1: Write the failing test**

Create `internal/vault/outbound_test.go`:

```go
package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutbound(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	foo := write("foo.md", "Links to [[bar]] and [missing](./nope.md) and [site](https://x.com)\n")
	write("bar.md", "# Bar\n")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	out := v.Outbound(foo)
	if len(out) != 3 {
		t.Fatalf("got %d outbound, want 3: %+v", len(out), out)
	}

	// First: resolved wikilink to bar.md.
	if out[0].Kind != OutboundWikilink {
		t.Errorf("out[0].Kind = %v, want OutboundWikilink", out[0].Kind)
	}
	if out[0].Resolved != filepath.Join(dir, "bar.md") {
		t.Errorf("out[0].Resolved = %q, want bar.md", out[0].Resolved)
	}

	// Second: relative std link. The vault surfaces the COMPUTED path
	// as-is; it does not check existence at the vault layer (broken-ness
	// is a query-layer concern). So even though nope.md is missing,
	// Resolved is the computed absolute path.
	if out[1].Kind != OutboundStdLink {
		t.Errorf("out[1].Kind = %v, want OutboundStdLink", out[1].Kind)
	}
	if out[1].Resolved != filepath.Join(dir, "nope.md") {
		t.Errorf("out[1].Resolved = %q, want computed nope.md path", out[1].Resolved)
	}

	// Third: external std link — raw target preserved, never resolved
	// (resolveStdLink returns "" for http/https schemes).
	if out[2].RawTarget != "https://x.com" {
		t.Errorf("out[2].RawTarget = %q, want https://x.com", out[2].RawTarget)
	}
	if out[2].Resolved != "" {
		t.Errorf("out[2].Resolved = %q, want empty (external)", out[2].Resolved)
	}
}

func TestOutboundUnknownFile(t *testing.T) {
	v, err := Build(t.TempDir(), NopDiagnostics{})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Outbound("/no/such/file.md"); len(got) != 0 {
		t.Errorf("Outbound(unknown) = %v, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/vault/ -run TestOutbound -v`
Expected: FAIL — `undefined: Outbound` / `undefined: OutboundWikilink`.

- [ ] **Step 3: Implement the accessor**

Create `internal/vault/outbound.go`:

```go
package vault

import "path/filepath"

// OutboundKind distinguishes a wikilink reference from a standard
// markdown-link reference. It mirrors the internal referenceKind but is
// part of the exported API.
type OutboundKind int

const (
	OutboundWikilink OutboundKind = iota
	OutboundStdLink
)

// Outbound is one outgoing reference from a file. It is the symmetric
// counterpart of Backlink (which describes references *into* a file).
type Outbound struct {
	DisplayText string       // the visible link text
	RawTarget   string       // wikilink name (e.g. "bar") or std-link href
	Resolved    string       // absolute target path, or "" if unresolved
	Line        int          // 1-indexed source line
	Snippet     string       // surrounding-line context
	Kind        OutboundKind // wikilink vs standard markdown link
}

// Outbound returns every outgoing reference from path, in document
// order. Returns an empty slice when path is not indexed.
func (v *Vault) Outbound(path string) []Outbound {
	v.mu.RLock()
	defer v.mu.RUnlock()

	abs, _ := filepath.Abs(path)
	entry, ok := v.files[abs]
	if !ok {
		return nil
	}
	out := make([]Outbound, 0, len(entry.refs))
	for _, ref := range entry.refs {
		kind := OutboundStdLink
		if ref.kind == refWikilink {
			kind = OutboundWikilink
		}
		out = append(out, Outbound{
			DisplayText: ref.displayText,
			RawTarget:   ref.target,
			Resolved:    ref.resolved,
			Line:        ref.line,
			Snippet:     ref.snippet,
			Kind:        kind,
		})
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/vault/ -run TestOutbound -v`
Expected: PASS (both `TestOutbound` and `TestOutboundUnknownFile`).

- [ ] **Step 5: Commit**

```bash
git add internal/vault/outbound.go internal/vault/outbound_test.go
git commit -m "feat(vault): add Outbound accessor for outgoing links"
```

---

### Task 3: Extract `search.RerankByRecency`

**Files:**
- Create: `internal/search/rerank.go`
- Test: `internal/search/rerank_test.go`
- Modify: `internal/tui/search.go` (make `rerankByRecency` delegate)

**Interfaces:**
- Consumes: `search.Hit{Path, Line, Snippet}`.
- Produces: `func RerankByRecency(order func(paths []string) []string, hits []Hit) []Hit`. `order` receives the unique hit paths and returns them reordered most-recent-first; paths it omits sort last in input order. A nil `order` returns `hits` unchanged. This is the shared core the TUI's `rerankByRecency` and `query.Search` both call.

This refactor removes duplication: `query` needs *identical* recency ordering to the TUI search modal, so the logic lives in one place. The TUI keeps its `recentStore`-based wrapper and its existing tests.

- [ ] **Step 1: Write the failing test**

Create `internal/search/rerank_test.go`:

```go
package search

import (
	"reflect"
	"testing"
)

func TestRerankByRecency(t *testing.T) {
	hits := []Hit{
		{Path: "/a.md", Line: 1},
		{Path: "/b.md", Line: 1},
		{Path: "/a.md", Line: 5},
		{Path: "/c.md", Line: 1},
	}
	// Recency order puts b first, then a; c is omitted (never visited).
	order := func(paths []string) []string {
		return []string{"/b.md", "/a.md"}
	}

	got := RerankByRecency(order, hits)
	want := []Hit{
		{Path: "/b.md", Line: 1},
		{Path: "/a.md", Line: 1},
		{Path: "/a.md", Line: 5},
		{Path: "/c.md", Line: 1}, // omitted-from-order paths trail, input order
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RerankByRecency =\n%+v\nwant\n%+v", got, want)
	}
}

func TestRerankByRecencyNilOrder(t *testing.T) {
	hits := []Hit{{Path: "/a.md", Line: 1}}
	if got := RerankByRecency(nil, hits); !reflect.DeepEqual(got, hits) {
		t.Errorf("nil order changed hits: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/search/ -run TestRerankByRecency -v`
Expected: FAIL — `undefined: RerankByRecency`.

- [ ] **Step 3: Implement the shared function**

Create `internal/search/rerank.go`:

```go
package search

// RerankByRecency reorders hits so files the order function ranks higher
// come first. Hits from the same file keep their input (line) order.
// order receives the unique hit paths (in stable input order) and must
// return some subset reordered most-recent-first; any path it omits
// trails the result in input order. A nil order returns hits unchanged.
func RerankByRecency(order func(paths []string) []string, hits []Hit) []Hit {
	if order == nil || len(hits) == 0 {
		return hits
	}

	// Unique paths in stable input order.
	seen := map[string]bool{}
	var uniquePaths []string
	for _, h := range hits {
		if !seen[h.Path] {
			seen[h.Path] = true
			uniquePaths = append(uniquePaths, h.Path)
		}
	}

	byPath := map[string][]Hit{}
	for _, h := range hits {
		byPath[h.Path] = append(byPath[h.Path], h)
	}

	out := make([]Hit, 0, len(hits))
	emitted := map[string]bool{}
	for _, p := range order(uniquePaths) {
		out = append(out, byPath[p]...)
		emitted[p] = true
	}
	// Paths the order function dropped trail in input order.
	for _, p := range uniquePaths {
		if !emitted[p] {
			out = append(out, byPath[p]...)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/search/ -run TestRerankByRecency -v`
Expected: PASS.

- [ ] **Step 5: Make the TUI wrapper delegate**

In `internal/tui/search.go`, replace the body of `rerankByRecency` (keep its signature and the `recentStore` interface) with:

```go
func rerankByRecency(store recentStore, hits []search.Hit) []search.Hit {
	if store == nil {
		return hits
	}
	return search.RerankByRecency(func(paths []string) []string {
		ranked := store.Rank(paths)
		out := make([]string, len(ranked))
		for i, r := range ranked {
			out[i] = r.Path
		}
		return out
	}, hits)
}
```

- [ ] **Step 6: Run TUI + search tests to verify no regression**

Run: `go test ./internal/tui/ ./internal/search/ -run 'Rerank|Search' -v`
Expected: PASS — existing TUI rerank tests still green via delegation.

- [ ] **Step 7: Commit**

```bash
git add internal/search/rerank.go internal/search/rerank_test.go internal/tui/search.go
git commit -m "refactor(search): extract RerankByRecency for reuse by query mode"
```

---

### Task 4: `query.Search` and `query.Recent`

**Files:**
- Create: `internal/query/query.go`
- Test: `internal/query/search_recent_test.go`

**Interfaces:**
- Consumes: `tree.MarkdownFiles` (Task 1), `search.Search` + `search.RerankByRecency` (Task 3), `recent.New`, `recent.DefaultStateFile`, `recent.Store.Rank`, `recent.Ranked{Path, Score, MTime, Visit}`, `highlight.Open`/`highlight.Close`.
- Produces:
  - `type SearchHit struct { Path string; Line int; Snippet string }` (json: `path`,`line`,`snippet`).
  - `type RecentEntry struct { Path string; Score float64; MTime, Visited time.Time }` (json: `path`,`score`,`mtime`,`visited`).
  - `func Search(root, term string, max int) ([]SearchHit, error)`.
  - `func Recent(root string, max int) ([]RecentEntry, error)`.
  - Package var `stateFileFn = recent.DefaultStateFile` (overridable in tests) and unexported `loadStore() (*recent.Store, error)`.
  - Unexported `sanitizeSnippet(string) string`.

- [ ] **Step 1: Write the failing test**

Create `internal/query/search_recent_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/query/ -run 'TestSearch|TestRecent' -v`
Expected: FAIL — package `query` does not exist / undefined identifiers.

- [ ] **Step 3: Implement the package and the two functions**

Create `internal/query/query.go`:

```go
// Package query is the non-interactive backend for hypogeum's scripting
// verbs. It orchestrates the pure tree/search/vault/recent packages and
// returns JSON-tagged result structs. It has no TUI dependencies.
package query

import (
	"context"
	"strings"
	"time"

	"github.com/wilkes/hypogeum/internal/highlight"
	"github.com/wilkes/hypogeum/internal/recent"
	"github.com/wilkes/hypogeum/internal/search"
	"github.com/wilkes/hypogeum/internal/tree"
)

// stateFileFn resolves the recent-visit state file. Overridable in tests
// so they never touch the real on-disk state.
var stateFileFn = recent.DefaultStateFile

// loadStore opens the persisted recency store. Returns (nil, err) on a
// hard failure; callers decide whether to degrade or surface the error.
func loadStore() (*recent.Store, error) {
	sf, err := stateFileFn()
	if err != nil {
		return nil, err
	}
	return recent.New(sf)
}

// sanitizeSnippet strips the highlight control chars search embeds so the
// JSON output is clean text.
func sanitizeSnippet(s string) string {
	s = strings.ReplaceAll(s, highlight.Open, "")
	return strings.ReplaceAll(s, highlight.Close, "")
}

// SearchHit is one full-text match.
type SearchHit struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

// Search scans every markdown file under root for term, recency-reranks
// the hits (same ordering as the TUI search modal), and returns at most
// max of them. A nil store degrades to unranked order.
func Search(root, term string, max int) ([]SearchHit, error) {
	paths, err := tree.MarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	hits, err := search.Search(context.Background(), paths, term, max)
	if err != nil {
		return nil, err
	}

	var order func([]string) []string
	if store, serr := loadStore(); serr == nil && store != nil {
		order = func(ps []string) []string {
			ranked := store.Rank(ps)
			out := make([]string, len(ranked))
			for i, r := range ranked {
				out[i] = r.Path
			}
			return out
		}
	}
	hits = search.RerankByRecency(order, hits)

	out := make([]SearchHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, SearchHit{
			Path:    h.Path,
			Line:    h.Line,
			Snippet: sanitizeSnippet(h.Snippet),
		})
	}
	return out, nil
}

// RecentEntry is one recency-ranked note.
type RecentEntry struct {
	Path    string    `json:"path"`
	Score   float64   `json:"score"`
	MTime   time.Time `json:"mtime"`
	Visited time.Time `json:"visited"`
}

// Recent returns up to max markdown files under root, ranked by the
// persisted hybrid recency score.
func Recent(root string, max int) ([]RecentEntry, error) {
	paths, err := tree.MarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	store, err := loadStore()
	if err != nil {
		return nil, err
	}
	ranked := store.Rank(paths)
	if max > 0 && len(ranked) > max {
		ranked = ranked[:max]
	}
	out := make([]RecentEntry, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, RecentEntry{
			Path:    r.Path,
			Score:   r.Score,
			MTime:   r.MTime,
			Visited: r.Visit,
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/query/ -run 'TestSearch|TestRecent' -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add internal/query/query.go internal/query/search_recent_test.go
git commit -m "feat(query): add Search and Recent query functions"
```

---

### Task 5: `query.Links` and `query.Neighbors`

**Files:**
- Modify: `internal/query/query.go`
- Test: `internal/query/links_neighbors_test.go`

**Interfaces:**
- Consumes: `vault.Build`, `vault.NopDiagnostics`, `vault.Vault.Outbound` (Task 2) returning `vault.Outbound{DisplayText, RawTarget, Resolved, Line, Snippet, Kind}` with `vault.OutboundWikilink`, and `vault.Vault.Backlinks` returning `vault.Backlink{SourceFile, DisplayText, Snippet, Line, Kind}`.
- Produces:
  - `type Link struct { Text, Target, Path, Kind string; Broken bool }` (json: `text`,`target`,`path`,`kind`,`broken`). `Kind` ∈ `"wikilink"|"relative"|"external"`. For wikilinks, `Target` is rendered `[[name]]`; for std links it is the raw href.
  - `type BacklinkEntry struct { Path string; Line int; Snippet, Text string }` (json: `path`,`line`,`snippet`,`text`).
  - `type Neighborhood struct { File string; Outbound []Link; Backlinks []BacklinkEntry }` (json: `file`,`outbound`,`backlinks`). Named `Neighborhood` (not `Neighbors`) because a type and the `Neighbors` function cannot share a name in one Go package.
  - `func Links(root, file string) ([]Link, error)` — returns a "file not found" error when `file` does not exist on disk (spec: missing file → exit 1).
  - `func Neighbors(root, file string) (Neighborhood, error)` — same missing-file error contract.
  - Unexported `outboundLinks(v *vault.Vault, abs string) []Link` shared by both.

- [ ] **Step 1: Write the failing test**

Create `internal/query/links_neighbors_test.go`:

```go
package query

import (
	"os"
	"path/filepath"
	"testing"
)

func writeVault(t *testing.T) (dir, foo string) {
	t.Helper()
	dir = t.TempDir()
	foo = filepath.Join(dir, "foo.md")
	files := map[string]string{
		"foo.md": "See [[bar]], [missing](./nope.md), [site](https://x.com)\n",
		"bar.md": "# Bar\n",
		// Standard markdown link (not a wikilink) so the backlink carries a
		// real 1-indexed line — wikilink nodes have no ast.Text child, so
		// the vault reports line 0 for them. The link sits on line 3.
		"baz.md": "# Baz\n\nLink to [foo](./foo.md) here\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, foo
}

func TestLinks(t *testing.T) {
	dir, foo := writeVault(t)

	links, err := Links(dir, foo)
	if err != nil {
		t.Fatalf("Links: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("got %d links, want 3: %+v", len(links), links)
	}

	if links[0].Kind != "wikilink" || links[0].Target != "[[bar]]" || links[0].Broken {
		t.Errorf("links[0] = %+v, want resolved wikilink [[bar]]", links[0])
	}
	if links[0].Path != filepath.Join(dir, "bar.md") {
		t.Errorf("links[0].Path = %q, want bar.md", links[0].Path)
	}
	if links[1].Kind != "relative" || !links[1].Broken || links[1].Path != "" {
		t.Errorf("links[1] = %+v, want broken relative with empty path", links[1])
	}
	if links[2].Kind != "external" || links[2].Broken || links[2].Target != "https://x.com" || links[2].Path != "" {
		t.Errorf("links[2] = %+v, want external https://x.com with empty path", links[2])
	}
}

func TestLinksResolvedRelative(t *testing.T) {
	dir := t.TempDir()
	foo := filepath.Join(dir, "foo.md")
	if err := os.WriteFile(foo, []byte("[real](./bar.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.md"), []byte("# bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	links, err := Links(dir, foo)
	if err != nil {
		t.Fatalf("Links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1", len(links))
	}
	if links[0].Kind != "relative" || links[0].Broken || links[0].Path != filepath.Join(dir, "bar.md") {
		t.Errorf("links[0] = %+v, want resolved relative to bar.md", links[0])
	}
}

func TestNeighbors(t *testing.T) {
	dir, foo := writeVault(t)

	n, err := Neighbors(dir, foo)
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if n.File != foo {
		t.Errorf("File = %q, want %q", n.File, foo)
	}
	if len(n.Outbound) != 3 {
		t.Errorf("got %d outbound, want 3", len(n.Outbound))
	}
	// baz.md links to foo via [foo](./foo.md) on line 3.
	if len(n.Backlinks) != 1 {
		t.Fatalf("got %d backlinks, want 1: %+v", len(n.Backlinks), n.Backlinks)
	}
	if n.Backlinks[0].Path != filepath.Join(dir, "baz.md") {
		t.Errorf("backlink path = %q, want baz.md", n.Backlinks[0].Path)
	}
	if n.Backlinks[0].Line != 3 {
		t.Errorf("backlink line = %d, want 3 (surfaced verbatim)", n.Backlinks[0].Line)
	}
}

func TestLinksFileNotFound(t *testing.T) {
	dir := t.TempDir()
	if _, err := Links(dir, filepath.Join(dir, "ghost.md")); err == nil {
		t.Error("Links on missing file returned nil error, want non-nil")
	}
}

func TestNeighborsFileNotFound(t *testing.T) {
	dir := t.TempDir()
	if _, err := Neighbors(dir, filepath.Join(dir, "ghost.md")); err == nil {
		t.Error("Neighbors on missing file returned nil error, want non-nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/query/ -run 'TestLinks|TestNeighbors' -v`
Expected: FAIL — `undefined: Links` / `undefined: Neighbors`.

- [ ] **Step 3: Implement Links, Neighbors, and the shared helper**

Append to `internal/query/query.go` (and add `"fmt"`, `"net/url"`, `"os"`, `"path/filepath"`, and `"github.com/wilkes/hypogeum/internal/vault"` to its import block):

```go
// Link is one outbound edge from a file.
type Link struct {
	Text   string `json:"text"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Kind   string `json:"kind"` // wikilink | relative | external
	Broken bool   `json:"broken"`
}

// BacklinkEntry is one reference into a file.
type BacklinkEntry struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
	Text    string `json:"text"`
}

// Neighborhood is a file's 1-hop context bundle. Named Neighborhood, not
// Neighbors, because a type and the Neighbors function cannot share a
// name in the same Go package.
type Neighborhood struct {
	File      string          `json:"file"`
	Outbound  []Link          `json:"outbound"`
	Backlinks []BacklinkEntry `json:"backlinks"`
}

// isExternalURL reports whether target is an http/https URL.
func isExternalURL(target string) bool {
	u, err := url.Parse(target)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

// outboundLinks maps a vault file's outbound references into Link values.
//
// Broken-ness is determined here, not in the vault accessor:
//   - wikilink: broken when the vault left Resolved empty (existence-based
//     via the names index, so Resolved already implies the file exists).
//   - relative: the vault computes Resolved by pure path math without
//     checking existence, so we os.Stat it. A missing target is broken and
//     reports an empty Path (matching the spec's broken-relative example).
//   - external: never broken (we do not probe URLs).
func outboundLinks(v *vault.Vault, abs string) []Link {
	refs := v.Outbound(abs)
	out := make([]Link, 0, len(refs))
	for _, r := range refs {
		l := Link{
			Text: r.DisplayText,
			Path: r.Resolved,
		}
		switch {
		case r.Kind == vault.OutboundWikilink:
			l.Kind = "wikilink"
			l.Target = "[[" + r.RawTarget + "]]"
			l.Broken = r.Resolved == ""
		case isExternalURL(r.RawTarget):
			l.Kind = "external"
			l.Target = r.RawTarget
			l.Path = ""
			l.Broken = false
		default:
			l.Kind = "relative"
			l.Target = r.RawTarget
			if r.Resolved == "" || !fileExists(r.Resolved) {
				l.Path = ""
				l.Broken = true
			}
		}
		out = append(out, l)
	}
	return out
}

// fileExists reports whether path names an existing entry on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// mustExist returns an absolute path for file, or an error if file does
// not exist on disk. A missing file argument is an operational failure
// (exit 1), distinct from a file that simply has zero links.
func mustExist(file string) (string, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("file not found: %s", file)
	}
	return abs, nil
}

// Links returns the outbound edges from file within the vault at root.
func Links(root, file string) ([]Link, error) {
	abs, err := mustExist(file)
	if err != nil {
		return nil, err
	}
	v, err := vault.Build(root, vault.NopDiagnostics{})
	if err != nil {
		return nil, err
	}
	return outboundLinks(v, abs), nil
}

// Neighbors returns file's outbound links and its backlinks.
func Neighbors(root, file string) (Neighborhood, error) {
	abs, err := mustExist(file)
	if err != nil {
		return Neighborhood{}, err
	}
	v, err := vault.Build(root, vault.NopDiagnostics{})
	if err != nil {
		return Neighborhood{}, err
	}
	n := Neighborhood{
		File:     abs,
		Outbound: outboundLinks(v, abs),
	}
	for _, b := range v.Backlinks(abs) {
		n.Backlinks = append(n.Backlinks, BacklinkEntry{
			Path:    b.SourceFile,
			Line:    b.Line,
			Snippet: sanitizeSnippet(b.Snippet),
			Text:    b.DisplayText,
		})
	}
	if n.Backlinks == nil {
		n.Backlinks = []BacklinkEntry{}
	}
	if n.Outbound == nil {
		n.Outbound = []Link{}
	}
	return n, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/query/ -run 'TestLinks|TestNeighbors' -v`
Expected: PASS.

- [ ] **Step 5: Run the whole query package race-clean**

Run: `go test -race ./internal/query/`
Expected: PASS, no race warnings.

- [ ] **Step 6: Commit**

```bash
git add internal/query/query.go internal/query/links_neighbors_test.go
git commit -m "feat(query): add Links and Neighbors query functions"
```

---

### Task 6: CLI dispatch and JSON output

**Files:**
- Create: `cmd/hypogeum/query.go`
- Test: `cmd/hypogeum/query_test.go`
- Modify: `cmd/hypogeum/main.go`

**Interfaces:**
- Consumes: `query.Search`, `query.Links`, `query.Recent`, `query.Neighbors` (Tasks 4–5).
- Produces:
  - `func isQueryVerb(s string) bool` — true for `search`/`links`/`recent`/`neighbors`.
  - `func runQuery(args []string, stdout io.Writer) error` — parses `args` (verb + flags + positional), runs the verb, encodes the result as JSON (compact, trailing newline) to `stdout`. Returns a non-nil error on bad usage / operational failure; the caller routes that to stderr + exit 1.
- `main.go`'s `run` calls `runQuery(args, os.Stdout)` when `isQueryVerb(args[0])`, before path resolution.

- [ ] **Step 1: Write the failing test**

Create `cmd/hypogeum/query_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIsQueryVerb(t *testing.T) {
	for _, v := range []string{"search", "links", "recent", "neighbors"} {
		if !isQueryVerb(v) {
			t.Errorf("isQueryVerb(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"foo.md", "", "Search", "help"} {
		if isQueryVerb(v) {
			t.Errorf("isQueryVerb(%q) = true, want false", v)
		}
	}
}

func TestRunQueryLinks(t *testing.T) {
	dir := t.TempDir()
	foo := filepath.Join(dir, "foo.md")
	if err := os.WriteFile(foo, []byte("See [[bar]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.md"), []byte("# bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := runQuery([]string{"links", "--vault", dir, foo}, &out)
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0]["kind"] != "wikilink" {
		t.Errorf("unexpected links output: %s", out.String())
	}
}

func TestRunQueryUnknownFlag(t *testing.T) {
	var out bytes.Buffer
	if err := runQuery([]string{"links", "--bogus"}, &out); err == nil {
		t.Error("runQuery with unknown flag returned nil error, want non-nil")
	}
}

func TestRunQueryMissingArg(t *testing.T) {
	var out bytes.Buffer
	if err := runQuery([]string{"links"}, &out); err == nil {
		t.Error("runQuery links with no file returned nil error, want non-nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/hypogeum/ -run 'TestIsQueryVerb|TestRunQuery' -v`
Expected: FAIL — `undefined: isQueryVerb` / `undefined: runQuery`.

- [ ] **Step 3: Implement the dispatcher**

Create `cmd/hypogeum/query.go`:

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/wilkes/hypogeum/internal/query"
)

// queryVerbs is the set of reserved first-arg verbs that route to
// non-interactive query mode instead of launching the TUI.
var queryVerbs = map[string]bool{
	"search":    true,
	"links":     true,
	"recent":    true,
	"neighbors": true,
}

func isQueryVerb(s string) bool { return queryVerbs[s] }

// runQuery parses args (verb + flags + positional) and writes the verb's
// result as JSON to stdout. The first arg must be a query verb.
func runQuery(args []string, stdout io.Writer) error {
	verb := args[0]
	fs := flag.NewFlagSet(verb, flag.ContinueOnError)
	fs.SetOutput(io.Discard) // we surface flag errors ourselves
	vault := fs.String("vault", "", "vault root (default: current directory)")
	n := fs.Int("n", defaultLimit(verb), "max results")
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}

	root := *vault
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		root = cwd
	}

	var result any
	switch verb {
	case "search":
		term := fs.Arg(0)
		if term == "" {
			return fmt.Errorf("search: missing query term")
		}
		hits, err := query.Search(root, term, *n)
		if err != nil {
			return err
		}
		result = hits
	case "recent":
		entries, err := query.Recent(root, *n)
		if err != nil {
			return err
		}
		result = entries
	case "links":
		file := fs.Arg(0)
		if file == "" {
			return fmt.Errorf("links: missing file argument")
		}
		links, err := query.Links(root, file)
		if err != nil {
			return err
		}
		result = links
	case "neighbors":
		file := fs.Arg(0)
		if file == "" {
			return fmt.Errorf("neighbors: missing file argument")
		}
		nb, err := query.Neighbors(root, file)
		if err != nil {
			return err
		}
		result = nb
	default:
		return fmt.Errorf("unknown query verb: %s", verb)
	}

	enc := json.NewEncoder(stdout)
	return enc.Encode(result)
}

// defaultLimit is the default -n cap per verb.
func defaultLimit(verb string) int {
	switch verb {
	case "search":
		return 50
	case "recent":
		return 20
	default:
		return 0 // links/neighbors ignore -n
	}
}
```

- [ ] **Step 4: Wire dispatch into main.go**

In `cmd/hypogeum/main.go`, inside `run`, after the `--version` loop and before `resolveTarget`, add:

```go
	if len(args) > 0 && isQueryVerb(args[0]) {
		return runQuery(args, os.Stdout)
	}
```

(`os` is already imported in `main.go`.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/hypogeum/ -run 'TestIsQueryVerb|TestRunQuery' -v`
Expected: PASS (all four tests).

- [ ] **Step 6: Manual smoke check**

Run:
```bash
go build -o /tmp/hypo ./cmd/hypogeum
mkdir -p /tmp/v && printf 'See [[b]]\n' > /tmp/v/a.md && printf '# b\n' > /tmp/v/b.md
/tmp/hypo links --vault /tmp/v /tmp/v/a.md
/tmp/hypo recent --vault /tmp/v
/tmp/hypo search --vault /tmp/v See
/tmp/hypo neighbors --vault /tmp/v /tmp/v/b.md
```
Expected: each prints a single line of JSON; `neighbors` on `b.md` shows `a.md` as a backlink.

- [ ] **Step 7: Commit**

```bash
git add cmd/hypogeum/query.go cmd/hypogeum/query_test.go cmd/hypogeum/main.go
git commit -m "feat(cmd): add scriptable query-mode dispatch"
```

---

### Task 7: Documentation

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: the verb surface from Tasks 4–6. No code.

- [ ] **Step 1: Add a "Scripting / query mode" section to README.md**

Add a section documenting the four verbs with one example each and a note that JSON goes to stdout, errors to stderr, exit 0 on success (including empty results) / 1 on failure. Use the smoke-check commands from Task 6 as examples. Place it after the existing usage/keybindings section.

- [ ] **Step 2: Add a gotcha to CLAUDE.md**

Add a bullet under the Gotchas list:

```markdown
- **Reserved query verbs shadow file paths.** `cmd/hypogeum` routes a first arg of `search`/`links`/`recent`/`neighbors` to non-interactive JSON query mode (`internal/query`) *before* path resolution — so `hypogeum search` never opens a file literally named `search`; pass `./search` to reach the TUI. Query logic lives in the pure `internal/query` package (no TUI deps); the recency ordering is shared with the TUI search modal via `search.RerankByRecency`. JSON only on stdout, errors on stderr.
```

- [ ] **Step 3: Verify the full suite is green and race-clean**

Run: `go build ./... && go vet ./... && go test -race ./...`
Expected: all PASS — this is the CI gate.

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: document scriptable query mode"
```

---

## Self-Review

**Spec coverage:**
- Search verb → Task 4. ✓
- Links verb (kind/broken derivation, external detection) → Task 5. ✓
- Recent verb (persisted store, `-n` cap) → Tasks 4. ✓
- Neighbors verb (outbound + backlinks with line/snippet) → Task 5. ✓
- `Vault.Outbound` accessor → Task 2. ✓
- Git-style dispatch + verb-collision rule → Task 6. ✓
- cwd-default root + `--vault` → Task 6. ✓
- JSON-by-default, snippet sanitizing → Tasks 4–6. ✓
- Exit codes (stdout JSON-only, stderr errors, 1 on failure) → Task 6 (`runQuery` returns error) + existing `main` error path (prints to stderr, `os.Exit(1)`). ✓
- Missing-file → exit 1 (spec lists "file not found" as a failure): `Links`/`Neighbors` guard with `mustExist` (Task 5), covered by `TestLinksFileNotFound`/`TestNeighborsFileNotFound`. `vault.Outbound` returning empty for an *indexed-but-linkless* file remains a valid zero-result (exit 0). ✓
- Stable ordering → search recency (Task 3), backlinks source+line (existing `Backlinks`). ✓
- Tests without a TTY → Tasks 4–6 all use plain function/`io.Writer` tests. ✓

**Placeholder scan:** No TBD/TODO; every code step shows full code; every command has expected output. ✓

**Type consistency:**
- `Outbound{DisplayText, RawTarget, Resolved, Line, Snippet, Kind}` / `OutboundWikilink` defined Task 2, consumed Task 5. ✓
- `RerankByRecency(order func([]string) []string, hits []Hit)` defined Task 3, consumed Task 4. ✓
- `query.Search/Recent/Links/Neighbors` signatures defined Tasks 4–5, consumed Task 6. ✓
- `stateFileFn` / `sanitizeSnippet` defined Task 4, reused Task 5. ✓

**Note on neighbors `outbound` shape:** the design's neighbors example omitted the `target` field; the plan reuses the full `Link` struct (superset, includes `target`) for DRY. This is an intentional, additive deviation — more information, same parser-friendly shape.
