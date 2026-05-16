# Full-text search implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `^s` full-text search modal that scans every vault markdown file for case-insensitive substring matches, ranks hits by recency, and on `Enter` opens the chosen file scrolled to the matched line.

**Architecture:** Two new units plus one extension. `internal/search` is a pure search package with no Bubble Tea / recency dependency. `internal/tui/search.go` integrates it into the existing modal infrastructure, reusing the picker's textinput pattern and the backlinks modal's two-row-per-entry result rendering. `internal/tui/content.go` gets a small extension so the markdown render path honors `pendingPreselectRange` the same way the code render path already does — that's the load-bearing piece that lets `Enter` scroll to the match.

**Tech Stack:** Go 1.22+. `github.com/charmbracelet/bubbles/textinput`, `github.com/charmbracelet/bubbles/viewport`, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`. `context` and stdlib `sync`/`os`/`strings` for the search package itself.

**Spec:** [docs/superpowers/specs/2026-05-14-full-text-search-design.md](../specs/2026-05-14-full-text-search-design.md).

---

## File structure

**Create:**
- `internal/search/search.go` — `Hit`, `Search`, worker fan-out, snippet generation.
- `internal/search/search_test.go` — full unit suite (12 cases).
- `internal/tui/search.go` — `searchState`, render functions, key handler, message types.
- `internal/tui/search_test.go` — model-level tests (15 cases).

**Modify:**
- `internal/tui/modal.go` — add `modalSearch` constant; add `search searchState` field to `modalUIState`.
- `internal/tui/keys.go` — add `OpenSearch`, `SearchCursorDown`, `SearchCursorUp` bindings.
- `internal/tui/input.go` — printable-key fast path for the search modal, modal-toggle case, Esc/Enter/cursor handling.
- `internal/tui/model.go` — `Update` cases for `searchTickMsg` and `searchResultsMsg`; `View` integration for `modalSearch`.
- `internal/tui/content.go` — extend markdown render path to honor `m.content.rangeHighlight`.
- `internal/tui/model.go` — `WindowSizeMsg` handler calls `m.resizeSearch()` when modal is search.
- `internal/tui/dispatch_test.go` — one integration smoke test.
- `CLAUDE.md` — add three new gotcha bullets.
- `docs/index.md` — bump the spec entry from "design approved" to "shipped" once the feature lands (final task).

Each commit keeps `go test -race ./...` green so CI never trips.

---

## Task 1: Scaffold `internal/search` package with `Hit` type

**Files:**
- Create: `internal/search/search.go`
- Test: (none yet — type-only commit)

- [ ] **Step 1: Create the package file with the public type**

Write `internal/search/search.go`:

```go
// Package search scans a set of files for case-insensitive substring
// matches of a query string. It returns Hits in (file, line) order;
// callers responsible for any further sorting (the TUI re-ranks by
// recency before display).
//
// The package has no Bubble Tea, recency, or modal dependencies — it
// imports only stdlib. Workers fan out across paths and respect
// context.Context cancellation between files and roughly every 256
// lines within a file.
package search

// Hit is one match: a path, a 1-indexed line number, and a display
// snippet. The snippet is the matched line, optionally trimmed with
// leading/trailing "…" to fit a ~60-char display budget, with the
// matched substring wrapped in highlight markers.
//
// Highlight markers are \x11 (DC1, open) and \x12 (DC2, close). The
// TUI's snippet renderer (internal/tui/backlinks.go applyHighlight)
// turns these into bold yellow SGR. Using ASCII control chars keeps
// the markers invisible to plain-text processing.
type Hit struct {
	Path    string // absolute path
	Line    int    // 1-indexed line number in the source file
	Snippet string // see package doc
}

// snippetHighlightOpen / Close mirror the convention defined in
// internal/vault/snippet.go. They are the data contract between this
// package and the TUI's snippet renderer.
const (
	snippetHighlightOpen  = "\x11" // DC1
	snippetHighlightClose = "\x12" // DC2
)
```

- [ ] **Step 2: Verify the package compiles**

Run: `go build ./internal/search/`
Expected: no output, exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/search/search.go
git commit -m "feat(search): scaffold internal/search with Hit type"
```

---

## Task 2: Add snippet builder

**Files:**
- Modify: `internal/search/search.go`
- Create: `internal/search/snippet_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/search/snippet_test.go`:

```go
package search

import "testing"

func TestBuildSnippet(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		matchAt   int // byte offset of match in line
		matchLen  int
		budget    int // total visible chars budget
		want      string
	}{
		{
			name:     "short line fits whole",
			line:     "the quick brown fox",
			matchAt:  4,
			matchLen: 5,
			budget:   60,
			want:     "the \x11quick\x12 brown fox",
		},
		{
			name:     "match at start, line too long",
			line:     "foo " + repeat("x", 200),
			matchAt:  0,
			matchLen: 3,
			budget:   30,
			want:     "\x11foo\x12 " + repeat("x", 24) + "…",
		},
		{
			name:     "match at end, line too long",
			line:     repeat("x", 200) + " end",
			matchAt:  201,
			matchLen: 3,
			budget:   30,
			want:     "…" + repeat("x", 24) + " \x11end\x12",
		},
		{
			name:     "match in middle, line too long, centered window",
			line:     repeat("a", 100) + "needle" + repeat("b", 100),
			matchAt:  100,
			matchLen: 6,
			budget:   30,
			want:     "…" + repeat("a", 10) + "\x11needle\x12" + repeat("b", 10) + "…",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildSnippet(c.line, c.matchAt, c.matchLen, c.budget)
			if got != c.want {
				t.Errorf("buildSnippet:\n got: %q\nwant: %q", got, c.want)
			}
		})
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/search/ -run TestBuildSnippet -v`
Expected: FAIL with `undefined: buildSnippet`.

- [ ] **Step 3: Implement buildSnippet**

Append to `internal/search/search.go`:

```go
import "strings"

// buildSnippet wraps the match in line at byte offset matchAt..matchAt+matchLen
// with snippetHighlightOpen/Close. If the resulting display would exceed
// budget runes (markers excluded), it trims with leading/trailing "…"
// biased so the match stays centered.
//
// budget must be >= matchLen+2 (room for the match plus two ellipses).
// Smaller budgets degrade gracefully — the match is preserved at the
// expense of any context.
func buildSnippet(line string, matchAt, matchLen, budget int) string {
	marked := line[:matchAt] +
		snippetHighlightOpen + line[matchAt:matchAt+matchLen] + snippetHighlightClose +
		line[matchAt+matchLen:]
	visibleLen := len(line) // markers add no visible width
	if visibleLen <= budget {
		return marked
	}

	// We need to trim. Compute how many chars of context fit on each
	// side of the match, then decide whether each side needs an ellipsis.
	contextBudget := budget - matchLen - 2 // reserve room for two ellipses
	if contextBudget < 0 {
		contextBudget = 0
	}
	leftBudget := contextBudget / 2
	rightBudget := contextBudget - leftBudget

	leftAvail := matchAt
	rightAvail := visibleLen - (matchAt + matchLen)

	leftTake := min(leftAvail, leftBudget)
	rightTake := min(rightAvail, rightBudget)

	// If one side has slack, give it to the other.
	if leftTake < leftBudget {
		rightTake = min(rightAvail, contextBudget-leftTake)
	}
	if rightTake < rightBudget {
		leftTake = min(leftAvail, contextBudget-rightTake)
	}

	var b strings.Builder
	if leftTake < leftAvail {
		b.WriteString("…")
	}
	b.WriteString(line[matchAt-leftTake : matchAt])
	b.WriteString(snippetHighlightOpen)
	b.WriteString(line[matchAt : matchAt+matchLen])
	b.WriteString(snippetHighlightClose)
	b.WriteString(line[matchAt+matchLen : matchAt+matchLen+rightTake])
	if rightTake < rightAvail {
		b.WriteString("…")
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/search/ -run TestBuildSnippet -v`
Expected: PASS for all four subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/search/search.go internal/search/snippet_test.go
git commit -m "feat(search): add buildSnippet with centered match window"
```

---

## Task 3: Add single-file line scanner

**Files:**
- Modify: `internal/search/search.go`
- Create: `internal/search/scan_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/search/scan_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/search/ -run TestScanFile -v`
Expected: FAIL with `undefined: scanFile`.

- [ ] **Step 3: Implement scanFile**

Add to `internal/search/search.go`:

```go
import (
	"bufio"
	"context"
	"os"
)

// snippetBudget is the visible-char budget for snippet rendering.
// 60 chars matches the spec's recommended width; the TUI may re-trim
// smaller without re-reading the file.
const snippetBudget = 60

// maxFileBytes caps the read budget for any single file. Files larger
// than this are scanned up to the cap and the rest is silently dropped.
// tree.Walk filters non-markdown so we shouldn't see huge files in
// practice — this is defense-in-depth.
const maxFileBytes = 1 << 20 // 1 MiB

// binaryProbe is the byte count examined for a NUL byte. NUL in the
// first probeLen bytes means we treat the file as binary and skip.
const binaryProbe = 512

// scanFile reads path and returns one Hit per line containing
// case-insensitive substring matches of query. The query is assumed
// non-empty (caller's responsibility — Search filters short queries).
//
// Returns an error only for I/O failures opening the file. Cancellation
// returns (nil, ctx.Err()).
func scanFile(ctx context.Context, path, query string) ([]Hit, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Binary probe.
	probe := make([]byte, binaryProbe)
	n, _ := f.Read(probe)
	for i := 0; i < n; i++ {
		if probe[i] == 0 {
			return nil, nil // skip binary file silently
		}
	}
	// Rewind so the scanner sees the same bytes.
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	loweredQuery := strings.ToLower(query)
	queryLen := len(query)
	var hits []Hit

	scanner := bufio.NewScanner(io.LimitReader(f, maxFileBytes))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum&0xFF == 0 { // check ctx every 256 lines
			if ctx.Err() != nil {
				return hits, ctx.Err()
			}
		}
		line := scanner.Text()
		idx := strings.Index(strings.ToLower(line), loweredQuery)
		if idx < 0 {
			continue
		}
		hits = append(hits, Hit{
			Path:    path,
			Line:    lineNum,
			Snippet: buildSnippet(line, idx, queryLen, snippetBudget),
		})
	}
	if err := scanner.Err(); err != nil {
		return hits, err
	}
	return hits, nil
}
```

Note: the existing `import` block must now include `bufio`, `context`, `io`, `os`. Combine with the existing `strings` import.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/search/ -run TestScanFile -v`
Expected: PASS for all six subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/search/search.go internal/search/scan_test.go
git commit -m "feat(search): add scanFile with binary skip and ctx cancellation"
```

---

## Task 4: Add `Search` fan-out

**Files:**
- Modify: `internal/search/search.go`
- Create: `internal/search/search_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/search/search_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/search/ -run TestSearch -v`
Expected: FAIL with `undefined: Search`.

- [ ] **Step 3: Implement Search**

Append to `internal/search/search.go`:

```go
import "sync"

// numWorkers is the goroutine fan-out width. Four is enough to overlap
// disk reads on a typical SSD without over-subscribing the OS.
const numWorkers = 4

// Search scans every path for case-insensitive substring matches of
// query. Returns hits in unspecified order (the TUI sorts them). An
// empty query returns nil immediately. Cancellation may return partial
// results; callers should check ctx.Err().
func Search(ctx context.Context, paths []string, query string, maxHits int) ([]Hit, error) {
	if query == "" || len(paths) == 0 || maxHits <= 0 {
		return nil, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	workCh := make(chan string, len(paths))
	for _, p := range paths {
		workCh <- p
	}
	close(workCh)

	hitsCh := make(chan Hit, maxHits)
	stopCtx, stopAll := context.WithCancel(ctx)
	defer stopAll()

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range workCh {
				if stopCtx.Err() != nil {
					return
				}
				hits, err := scanFile(stopCtx, path, query)
				if err != nil {
					continue // skip the file; Search itself stays "best-effort"
				}
				for _, h := range hits {
					select {
					case hitsCh <- h:
					case <-stopCtx.Done():
						return
					}
				}
			}
		}()
	}

	// Closer goroutine: when all workers finish, close hitsCh so the
	// collector loop terminates.
	go func() {
		wg.Wait()
		close(hitsCh)
	}()

	var out []Hit
	for h := range hitsCh {
		out = append(out, h)
		if len(out) >= maxHits {
			stopAll() // tell workers to stop producing
			// Drain remaining hits without appending so workers don't
			// block on hitsCh.
			go func() {
				for range hitsCh {
				}
			}()
			break
		}
	}
	if stopCtx.Err() != nil && stopCtx.Err() != context.Canceled {
		return out, stopCtx.Err()
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/search/ -v`
Expected: PASS for all subtests (including TestSearch_CancellationStopsEarly).

- [ ] **Step 5: Commit**

```bash
git add internal/search/search.go internal/search/search_test.go
git commit -m "feat(search): add Search with goroutine fan-out and cancellation"
```

---

## Task 5: Extend `refreshContent` to honor `rangeHighlight` on markdown render path

**Files:**
- Modify: `internal/tui/content.go`
- Test: `internal/tui/content_test.go` (add new test case)

This is the load-bearing extension for the search-Enter scroll-to-line behavior. Today's markdown render path **clears** `m.content.rangeHighlight` at content.go:150 before rendering, so the existing range-highlight feature is code-files-only. The search feature needs the markdown path to **honor** a non-nil `rangeHighlight` by calling `scrollToLine` after `GotoTop`, then clear it. The clear stays for the "no-op on subsequent renders" semantic.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/content_test.go`:

```go
// TestRefreshContent_MarkdownHonorsRangeHighlight pins that a non-nil
// m.content.rangeHighlight set before refreshContent on a markdown
// file causes the viewport to scroll to that line. Search-Enter uses
// pendingPreselectRange → rangeHighlight as the scroll-to-line carrier.
func TestRefreshContent_MarkdownHonorsRangeHighlight(t *testing.T) {
	dir := t.TempDir()
	// Build a markdown file that renders tall enough to require scrolling.
	var sb strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&sb, "line %d filler text to fill the viewport\n\n", i)
	}
	p := filepath.Join(dir, "tall.md")
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(t, dir)
	// Resize so the viewport is bounded.
	m.width = 80
	m.height = 24
	m.resizeContent()

	m.pendingPreselectRange = &markdown.LineRange{Start: 50, End: 50}
	m.openFile(p)

	if m.content.viewport.YOffset == 0 {
		t.Errorf("YOffset = 0, expected non-zero scroll to line 50")
	}
}
```

You will need to add these imports to `content_test.go` if not already present: `"fmt"`, `"os"`, `"path/filepath"`, `"strings"`, `"github.com/wilkes/hypogeum/internal/markdown"`.

Check first whether `newTestModel` exists in `internal/tui/`:

Run: `grep -n "func newTestModel" internal/tui/*.go`
If it exists, use it. If not, replace the `m := newTestModel(t, dir)` line with a copy of how other tests construct a `Model` (look at `internal/tui/content_test.go` for the established pattern and copy verbatim — DO NOT invent a helper).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestRefreshContent_MarkdownHonorsRangeHighlight -v`
Expected: FAIL with `YOffset = 0, expected non-zero scroll to line 50`. The current code clears `rangeHighlight` *before* rendering, so the scroll never fires.

- [ ] **Step 3: Modify `refreshContent` to honor rangeHighlight on the markdown path**

Open `internal/tui/content.go`. Find this block (starts around line 148):

```go
	// Opening a markdown file always clears any prior source-range
	// highlight; the renderer doesn't carry it across non-source files.
	m.content.rangeHighlight = nil
	m.content.renderer.SetFromFile(path)
```

Replace it with:

```go
	// Capture rangeHighlight (if any) BEFORE clearing — search-Enter
	// and any future caller can set this to ask the markdown render
	// path to scroll to a specific line after rendering. Then clear so
	// subsequent re-renders (e.g. on resize) don't keep re-scrolling.
	pendingScrollLine := 0
	if m.content.rangeHighlight != nil {
		pendingScrollLine = m.content.rangeHighlight.Start
	}
	m.content.rangeHighlight = nil
	m.content.renderer.SetFromFile(path)
```

Then find the block (around line 161-163) that looks like:

```go
	m.status = path
	m.content.viewport.SetContent(out)
	m.content.viewport.GotoTop()
	m.content.links = links
```

Insert after `GotoTop()`:

```go
	m.status = path
	m.content.viewport.SetContent(out)
	m.content.viewport.GotoTop()
	if pendingScrollLine > 0 {
		m.scrollToLine(pendingScrollLine)
	}
	m.content.links = links
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestRefreshContent_MarkdownHonorsRangeHighlight -v`
Expected: PASS.

- [ ] **Step 5: Run the full TUI test suite to confirm no regression**

Run: `go test -race ./internal/tui/`
Expected: all tests pass. The existing code-file rangeHighlight test at content_test.go (search for "rangeHighlight" in that file) must still pass.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/content.go internal/tui/content_test.go
git commit -m "feat(tui): markdown render path honors pendingPreselectRange scroll"
```

---

## Task 6: Add `modalSearch` constant and `searchState` field

**Files:**
- Modify: `internal/tui/modal.go`

- [ ] **Step 1: Add the modal kind constant**

Open `internal/tui/modal.go`. Find the `modalKind` enum (around line 17):

```go
const (
	modalNone modalKind = iota
	modalBacklinks
	modalLogs
	modalPicker
	modalHelp
	modalTree
)
```

Add `modalSearch` after `modalTree`:

```go
const (
	modalNone modalKind = iota
	modalBacklinks
	modalLogs
	modalPicker
	modalHelp
	modalTree
	modalSearch
)
```

- [ ] **Step 2: Add the `search` field to `modalUIState`**

Find the `modalUIState` struct (around line 31):

```go
type modalUIState struct {
	kind      modalKind
	vp        viewport.Model
	picker    pickerState
	prevFocus focus
}
```

Add the `search` field:

```go
type modalUIState struct {
	kind      modalKind
	vp        viewport.Model
	picker    pickerState
	search    searchState
	prevFocus focus
}
```

- [ ] **Step 3: Verify it compiles (it will fail — searchState not yet defined)**

Run: `go build ./internal/tui/`
Expected: FAIL with `undefined: searchState`. This is fine — we wire the type up in the next task.

- [ ] **Step 4: Don't commit yet**

Task 7 introduces `searchState`. We'll commit them together so each commit compiles.

---

## Task 7: Define `searchState` and the message types

**Files:**
- Create: `internal/tui/search.go`

- [ ] **Step 1: Create `internal/tui/search.go` with the state struct**

Write `internal/tui/search.go`:

```go
package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wilkes/hypogeum/internal/search"
)

// searchMaxHits caps how many full-text hits the modal will hold.
// Matches pickerMaxVisible's choice for the same reason: a runaway
// short-query result list shouldn't lag rendering.
const searchMaxHits = 200

// searchDebounce is how long after the latest keystroke the scan fires.
// 150ms is the established UX threshold for "the system feels responsive
// without firing on every character."
const searchDebounce = 150 * time.Millisecond

// searchMinQuery is the minimum query length below which no scan fires.
// 1 char is too noisy on a multi-hundred-file vault; 3 is frustrating
// on short words like "go". 2 is the established sweet spot.
const searchMinQuery = 2

// searchSnippetWindow is the displayed budget per snippet, after which
// the row gets truncated by the renderer.
const searchSnippetWindow = 60

// searchState bundles full-text search modal state.
//
// paths is a snapshot taken at modal-open time so a mid-search watcher
// event doesn't yank files out from under in-flight workers. New files
// appear only on the next ^s open — this is deliberate; rescanning on
// every fsnotify event would be expensive AND yank cursor focus.
//
// scanStop is the CancelFunc for the currently-running (or most-recently
// scheduled) scan. Each keystroke that fires a new tick calls scanStop
// first so workers from the prior scan return early.
type searchState struct {
	input    textinput.Model
	paths    []string
	hits     []search.Hit
	cursor   int
	vp       viewport.Model
	scanCtx  context.Context
	scanStop context.CancelFunc
	// inFlight is true between the moment a scan is dispatched and the
	// moment its searchResultsMsg lands (or is discarded as stale).
	// Drives the "(searching…)" placeholder.
	inFlight bool
}

// newSearch returns a zero-valued search state with a fresh textinput.
func newSearch() searchState {
	ti := textinput.New()
	ti.Prompt = ""      // we render our own "> " prefix
	ti.Placeholder = ""
	ti.CharLimit = 256
	return searchState{
		vp:    viewport.New(0, 0),
		input: ti,
	}
}

// reset clears every transient field and re-focuses the textinput.
// Called on every modal-open. paths is the snapshot of vault files
// captured at open time.
func (s *searchState) reset(paths []string) {
	if s.scanStop != nil {
		s.scanStop()
	}
	s.paths = paths
	s.hits = nil
	s.cursor = 0
	s.scanCtx = nil
	s.scanStop = nil
	s.inFlight = false
	s.input.SetValue("")
	s.input.Focus()
}

// searchTickMsg is delivered searchDebounce after each keystroke.
// query is the input value at the moment the tick was scheduled; the
// handler compares it to the current input to decide whether to honor
// or drop the tick (later keystrokes mean this one is stale).
type searchTickMsg struct {
	query string
}

// searchResultsMsg carries the output of a scan back into Update.
// query lets the handler discard results from a stale scan.
type searchResultsMsg struct {
	query string
	hits  []search.Hit
	err   error
}

// scheduleSearchTick returns a Cmd that fires a searchTickMsg with
// query after searchDebounce.
func scheduleSearchTick(query string) tea.Cmd {
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg {
		return searchTickMsg{query: query}
	})
}

// runSearchCmd returns a Cmd that runs the scan in a goroutine and
// emits searchResultsMsg with the result. paths is captured by value;
// the scan reads it without further synchronization.
func runSearchCmd(ctx context.Context, paths []string, query string) tea.Cmd {
	return func() tea.Msg {
		hits, err := search.Search(ctx, paths, query, searchMaxHits)
		return searchResultsMsg{query: query, hits: hits, err: err}
	}
}
```

- [ ] **Step 2: Verify the package compiles**

Run: `go build ./internal/tui/`
Expected: no output, exit 0. (Task 6's `searchState` reference now resolves.)

- [ ] **Step 3: Commit Tasks 6 + 7 together**

```bash
git add internal/tui/modal.go internal/tui/search.go
git commit -m "feat(tui): scaffold searchState, modalSearch, message types"
```

---

## Task 8: Add `OpenSearch` keybinding and dispatch routing

**Files:**
- Modify: `internal/tui/keys.go`
- Modify: `internal/tui/input.go`

- [ ] **Step 1: Add the bindings to keyMap**

Open `internal/tui/keys.go`. Add three fields to the `keyMap` struct (after the picker entries around line 26-28):

```go
type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Open    key.Binding
	Back    key.Binding
	Forward key.Binding
	Quit    key.Binding

	NextLink  key.Binding
	PrevLink  key.Binding
	ClearLink key.Binding

	OpenBacklinksModal key.Binding
	OpenLogsModal      key.Binding
	OpenHelpModal      key.Binding

	ToggleTree   key.Binding
	ToggleFolder key.Binding

	OpenPicker       key.Binding
	PickerCursorDown key.Binding
	PickerCursorUp   key.Binding

	OpenSearch       key.Binding
	SearchCursorDown key.Binding
	SearchCursorUp   key.Binding
}
```

Then in `defaultKeys()`, add three lines at the end of the struct literal (after the picker entries):

```go
		OpenPicker:       key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("^p", "open file…")),
		PickerCursorDown: key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("^j", "picker: next")),
		PickerCursorUp:   key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("^k", "picker: prev")),

		OpenSearch:       key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("^s", "search…")),
		SearchCursorDown: key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("^j", "search: next")),
		SearchCursorUp:   key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("^k", "search: prev")),
	}
```

(The `^j`/`^k` bindings duplicate the picker's; that's intentional — only one modal is open at a time, so the same physical key serves both modals' cursor movement.)

- [ ] **Step 2: Add the modal-toggle dispatch case**

Open `internal/tui/input.go`. Find the `handleKey` switch (around line 135) that has cases for `OpenBacklinksModal`, `OpenPicker`, etc. Add a new case right after the `OpenPicker` case (after the existing `m.openModalWith(modalPicker, ...)` block):

```go
	case key.Matches(msg, m.keys.OpenSearch):
		return *m, m.openModalWith(modalSearch, func() {
			m.modals.search.reset(m.allVaultMarkdownPaths())
		})
```

- [ ] **Step 3: Verify the package compiles**

Run: `go build ./internal/tui/`
Expected: no output. (We haven't wired the per-modal key handling yet — that's Task 9 — but the toggle plus the keymap should compile.)

- [ ] **Step 4: Quick smoke test the binding via keymap**

Run a sanity check: write a tiny test that constructs a Model and presses `^s`, asserts `m.modals.kind == modalSearch`:

Create `internal/tui/search_test.go`:

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// minimal smoke test that ^s opens the modal. Fuller behavior covered in later tasks.
func TestSearch_CtrlSOpensModal(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, dir)
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	mm := updated.(Model)
	if mm.modals.kind != modalSearch {
		t.Errorf("modals.kind = %v, want modalSearch", mm.modals.kind)
	}
}
```

(If `newTestModel` doesn't exist, look at existing test files in `internal/tui/` for the established Model-construction pattern and copy.)

Run: `go test ./internal/tui/ -run TestSearch_CtrlSOpensModal -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/keys.go internal/tui/input.go internal/tui/search_test.go
git commit -m "feat(tui): bind ^s to open the search modal"
```

---

## Task 9: Implement `handleSearchKey` and debounce dispatch

**Files:**
- Modify: `internal/tui/input.go`
- Modify: `internal/tui/search.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/search_test.go`:

```go
import (
	"strings"
	"time"
)

func TestSearch_TypingShortQueryDoesNotFire(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("foobar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, dir)
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	mm := updated.(Model)

	// Type one character.
	updated, cmd := mm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	mm = updated.(Model)
	if mm.modals.search.input.Value() != "a" {
		t.Errorf("input = %q, want %q", mm.modals.search.input.Value(), "a")
	}
	if cmd != nil {
		// One character is below the minimum, so no tick should be scheduled.
		t.Errorf("expected nil cmd for 1-char query, got non-nil")
	}
	if mm.modals.search.inFlight {
		t.Errorf("inFlight should be false for 1-char query")
	}
}

func TestSearch_TypingTwoCharsSchedulesTick(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("foobar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, dir)
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	mm := updated.(Model)

	updated, _ = mm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	mm = updated.(Model)
	updated, cmd := mm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	mm = updated.(Model)
	if mm.modals.search.input.Value() != "fo" {
		t.Fatalf("input = %q, want %q", mm.modals.search.input.Value(), "fo")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd (tick scheduled) for 2-char query")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run TestSearch_Typing -v`
Expected: FAIL — `handleKey` doesn't yet route printable keys to a search-specific handler, and there's no debounce.

- [ ] **Step 3: Add the printable-key fast path**

Open `internal/tui/input.go`. Find the printable-key fast path for the picker (around line 127):

```go
	if m.modals.kind == modalPicker && msg.Type == tea.KeyRunes {
		return m.handlePickerKey(msg)
	}
```

Add a parallel fast path right below it:

```go
	if m.modals.kind == modalPicker && msg.Type == tea.KeyRunes {
		return m.handlePickerKey(msg)
	}
	if m.modals.kind == modalSearch && msg.Type == tea.KeyRunes {
		return m.handleSearchKey(msg)
	}
```

- [ ] **Step 4: Implement `handleSearchKey`**

Append to `internal/tui/search.go`:

```go
// handleSearchKey forwards printable runes to the textinput, then
// decides whether to schedule a debounced scan tick. Called only when
// modalSearch is open and msg.Type is tea.KeyRunes.
func (m *Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.modals.search.input, cmd = m.modals.search.input.Update(msg)
	query := m.modals.search.input.Value()
	if len(query) < searchMinQuery {
		// Below the minimum — clear any prior results and don't fire a
		// scan. A previous tick may still be in flight from a longer
		// query; its result will be discarded by the stale-query check.
		m.modals.search.hits = nil
		m.modals.search.cursor = 0
		m.modals.search.inFlight = false
		m.refreshSearchVP()
		return *m, cmd
	}
	// Cancel any prior in-flight scan immediately. The tick may not
	// have fired yet, but if a scan is mid-flight cancelling now lets
	// workers return early.
	if m.modals.search.scanStop != nil {
		m.modals.search.scanStop()
		m.modals.search.scanStop = nil
		m.modals.search.scanCtx = nil
	}
	tick := scheduleSearchTick(query)
	if cmd != nil {
		return *m, tea.Batch(cmd, tick)
	}
	return *m, tick
}

// refreshSearchVP regenerates the search modal's viewport content
// from the current hits / cursor / input value. Called whenever any
// of those change.
func (m *Model) refreshSearchVP() {
	m.modals.search.vp.SetContent(m.renderSearchRows())
	viewportClamp(&m.modals.search.vp, m.modals.search.cursor, 2)
}

// renderSearchRows is a placeholder — Task 10 implements full hit
// rendering. For now it returns the empty/loading/no-match placeholders
// so handleSearchKey can call refreshSearchVP safely.
func (m *Model) renderSearchRows() string {
	q := m.modals.search.input.Value()
	faint := lipgloss.NewStyle().Faint(true)
	switch {
	case len(m.modals.search.paths) == 0:
		return faint.Render("(no markdown files in vault)")
	case len(q) < searchMinQuery:
		return faint.Render("(type 2+ chars to search)")
	case m.modals.search.inFlight && len(m.modals.search.hits) == 0:
		return faint.Render("(searching…)")
	case !m.modals.search.inFlight && len(m.modals.search.hits) == 0:
		return faint.Render(`(no match for "` + q + `")`)
	default:
		return strings.Join(formatHitsPlaceholder(m.modals.search.hits), "\n")
	}
}

// formatHitsPlaceholder is the minimal "render one row per hit" used by
// Task 9; Task 10 replaces it with the proper two-row-per-entry path/
// snippet layout.
func formatHitsPlaceholder(hits []search.Hit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Path)
	}
	return out
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestSearch_Typing -v`
Expected: PASS for both.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/input.go internal/tui/search.go internal/tui/search_test.go
git commit -m "feat(tui): debounce-schedule scans on search keystrokes"
```

---

## Task 10: Implement the tick + results message handlers

**Files:**
- Modify: `internal/tui/model.go`

- [ ] **Step 1: Find the Update method's message switch**

Open `internal/tui/model.go`. Find the `Update` method's `case` blocks (around line 240-285). They look like:

```go
case tea.WindowSizeMsg:
	...
case tea.KeyMsg:
	...
case tea.MouseMsg:
	...
case fsEventMsg:
	...
case transientClearMsg:
	...
```

- [ ] **Step 2: Add two new cases**

After the `transientClearMsg` case, add:

```go
	case searchTickMsg:
		return m.handleSearchTick(msg)
	case searchResultsMsg:
		return m.handleSearchResults(msg)
```

- [ ] **Step 3: Implement the two handlers in `internal/tui/search.go`**

Append to `internal/tui/search.go`:

```go
// handleSearchTick fires when a debounce tick lands. If the modal has
// closed, the tick is dropped. If the user has typed more characters
// since this tick was scheduled, the tick's query won't match the
// current input value and we drop it (the latest keystroke scheduled
// its own tick).
func (m *Model) handleSearchTick(msg searchTickMsg) (tea.Model, tea.Cmd) {
	if m.modals.kind != modalSearch {
		return *m, nil
	}
	if msg.query != m.modals.search.input.Value() {
		return *m, nil
	}
	if len(msg.query) < searchMinQuery {
		return *m, nil
	}
	// Allocate a new ctx for this scan. Previous ctxs (if any) have
	// been cancelled by handleSearchKey.
	ctx, cancel := context.WithCancel(context.Background())
	m.modals.search.scanCtx = ctx
	m.modals.search.scanStop = cancel
	m.modals.search.inFlight = true
	m.refreshSearchVP()
	return *m, runSearchCmd(ctx, m.modals.search.paths, msg.query)
}

// handleSearchResults consumes the scan's output. Stale results — from
// a cancelled scan whose query no longer matches the input — are
// discarded. Otherwise hits are recency-ranked and stored.
func (m *Model) handleSearchResults(msg searchResultsMsg) (tea.Model, tea.Cmd) {
	if m.modals.kind != modalSearch {
		return *m, nil
	}
	if msg.query != m.modals.search.input.Value() {
		// Stale: user has typed more since this scan started.
		return *m, nil
	}
	m.modals.search.inFlight = false
	m.modals.search.scanCtx = nil
	m.modals.search.scanStop = nil
	if msg.err != nil && msg.err != context.Canceled {
		if m.diag != nil {
			m.diag.Info("search %q: %v", msg.query, msg.err)
		}
	}
	m.modals.search.hits = rerankByRecency(m.recent, msg.hits)
	m.modals.search.cursor = 0
	m.refreshSearchVP()
	if m.diag != nil {
		m.diag.Info("search %q: %d hits", msg.query, len(msg.hits))
	}
	return *m, nil
}

// rerankByRecency reorders hits so files visited more recently come
// first. Hits from the same file keep their (line) order. Hits whose
// path doesn't appear in any recent.Ranked entry sort last in file-
// alphabetical order.
//
// store may be nil — happens in tests; we degrade to file-then-line
// order (input order).
func rerankByRecency(store recentStore, hits []search.Hit) []search.Hit {
	if store == nil || len(hits) == 0 {
		return hits
	}
	// Unique paths in stable input order.
	seen := map[string]int{}
	var uniquePaths []string
	for _, h := range hits {
		if _, ok := seen[h.Path]; !ok {
			seen[h.Path] = len(uniquePaths)
			uniquePaths = append(uniquePaths, h.Path)
		}
	}
	ranked := store.Rank(uniquePaths)
	// Build a path → priority map (lower = earlier).
	priority := make(map[string]int, len(ranked))
	for i, r := range ranked {
		priority[r.Path] = i
	}
	// Group hits by path, then emit groups in priority order.
	byPath := map[string][]search.Hit{}
	for _, h := range hits {
		byPath[h.Path] = append(byPath[h.Path], h)
	}
	out := make([]search.Hit, 0, len(hits))
	for _, r := range ranked {
		out = append(out, byPath[r.Path]...)
	}
	return out
}

// recentStore is the subset of *recent.Store that rerankByRecency uses.
// Defined as an interface so tests can swap in a nil-tolerant fake.
type recentStore interface {
	Rank(paths []string) []recent.Ranked
}
```

Add the import for `recent` at the top of the file:

```go
import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wilkes/hypogeum/internal/recent"
	"github.com/wilkes/hypogeum/internal/search"
)
```

- [ ] **Step 4: Verify the package compiles**

Run: `go build ./internal/tui/`
Expected: success.

- [ ] **Step 5: Run the existing TUI test suite to confirm no regression**

Run: `go test -race ./internal/tui/`
Expected: all existing tests pass. Our new search tests should still pass (they only exercise the printable-key path and don't depend on the message-handler additions yet).

- [ ] **Step 6: Commit**

```bash
git add internal/tui/model.go internal/tui/search.go
git commit -m "feat(tui): wire searchTickMsg and searchResultsMsg into Update"
```

---

## Task 11: Implement final hit rendering (path + snippet rows)

**Files:**
- Modify: `internal/tui/search.go`
- Modify: `internal/tui/search_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/search_test.go`:

```go
func TestSearch_HitsRenderAsPathPlusSnippet(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "note.md")
	if err := os.WriteFile(p, []byte("line one\nline with foo here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, dir)
	m.width = 100
	m.height = 30
	m.modals.kind = modalSearch
	m.modals.search.input.SetValue("foo")
	m.modals.search.hits = []search.Hit{
		{Path: p, Line: 2, Snippet: "line with \x11foo\x12 here"},
	}
	m.modals.search.cursor = 0
	m.resizeSearch()

	out := m.renderSearchRows()
	if !strings.Contains(out, "note.md:2") {
		t.Errorf("expected path:line in output, got: %q", out)
	}
	if !strings.Contains(out, "line with foo here") && !strings.Contains(out, "foo") {
		t.Errorf("expected snippet text in output, got: %q", out)
	}
}
```

You will need `"github.com/wilkes/hypogeum/internal/search"` imported in the test file.

- [ ] **Step 2: Run test to verify it fails (renders only the path)**

Run: `go test ./internal/tui/ -run TestSearch_HitsRenderAsPathPlusSnippet -v`
Expected: FAIL — `formatHitsPlaceholder` currently emits only `h.Path`, with no `:line` suffix and no snippet row.

- [ ] **Step 3: Replace the placeholder formatter with the real renderer**

In `internal/tui/search.go`, delete `formatHitsPlaceholder` and update `renderSearchRows` to call a new `formatSearchHits`:

Replace:

```go
func formatHitsPlaceholder(hits []search.Hit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Path)
	}
	return out
}
```

With:

```go
// formatSearchHits renders each hit as a two-row entry:
//   ▌ <relative-path>:<line>
//     <snippet with \x11/\x12 → bold yellow>
// The cursor marker appears only on the selected hit. width is the
// viewport's visible width; snippets are truncated to width-4.
//
// applyHighlight (internal/tui/backlinks.go) handles the \x11/\x12 →
// SGR conversion; this function delegates to it for the snippet row.
func formatSearchHits(hits []search.Hit, root string, width, cursor int) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	for i, h := range hits {
		marker := "  "
		if i == cursor {
			marker = cursorMarkerStyle.Render("▌") + " "
		}
		rel, err := filepathRel(root, h.Path)
		if err != nil {
			rel = h.Path
		}
		header := marker + rel + ":" + itoa(h.Line)
		snippet := "  " + truncateOneLine(applyHighlight(h.Snippet), width-4)
		if i == cursor {
			header = lipgloss.NewStyle().Reverse(true).Render(header)
			snippet = lipgloss.NewStyle().Reverse(true).Render(snippet)
		}
		b.WriteString(header)
		b.WriteByte('\n')
		b.WriteString(snippet)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
```

Then update `renderSearchRows`'s final `default` branch to call `formatSearchHits`:

```go
	default:
		return formatSearchHits(m.modals.search.hits, m.root, m.modals.search.vp.Width, m.modals.search.cursor)
	}
```

Add helpers at the bottom of `search.go`:

```go
// filepathRel is a stable indirection used here and in the picker —
// extracted so the test can use the same path-relative semantics.
func filepathRel(root, p string) (string, error) {
	return relativeTo(root, p), nil
}

// itoa converts a positive integer to a string without pulling in fmt
// for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
```

Wait — `relativeTo` is the helper in `picker.go` that already does what we need. Drop the `filepathRel` wrapper and call `relativeTo(root, h.Path)` directly. And drop `itoa` — `strconv` is already imported elsewhere; use `strconv.Itoa`. Update the imports at the top of `search.go`:

```go
import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wilkes/hypogeum/internal/recent"
	"github.com/wilkes/hypogeum/internal/search"
)
```

And replace `formatSearchHits` to use the existing helpers:

```go
func formatSearchHits(hits []search.Hit, root string, width, cursor int) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	for i, h := range hits {
		marker := "  "
		if i == cursor {
			marker = cursorMarkerStyle.Render("▌") + " "
		}
		header := marker + relativeTo(root, h.Path) + ":" + strconv.Itoa(h.Line)
		snippet := "  " + truncateOneLine(applyHighlight(h.Snippet), width-4)
		if i == cursor {
			header = lipgloss.NewStyle().Reverse(true).Render(header)
			snippet = lipgloss.NewStyle().Reverse(true).Render(snippet)
		}
		b.WriteString(header)
		b.WriteByte('\n')
		b.WriteString(snippet)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
```

(`cursorMarkerStyle`, `relativeTo`, `truncateOneLine`, and `applyHighlight` are all already defined in the `tui` package — `backlinks.go` and `picker.go`.)

- [ ] **Step 4: Implement `resizeSearch` and overflow-footer rendering**

Append to `search.go`:

```go
// resizeSearch fits the search modal's viewport into the modal interior,
// reserving rows for the query prompt and separator on top.
func (m *Model) resizeSearch() {
	_, _, w, h := modalGeometry(m.width, m.height)
	pw := w - 2
	ph := h - 2 - 2 // border (2) + prompt+separator (2)
	if pw < 1 {
		pw = 1
	}
	if ph < 1 {
		ph = 1
	}
	m.modals.search.vp.Width = pw
	m.modals.search.vp.Height = ph
	m.modals.search.input.Width = pw - 2 // leave room for "> " prefix
	m.refreshSearchVP()
}

// searchView returns the modal's renderable body — prompt, separator,
// viewport, and an optional overflow footer.
func (m *Model) searchView() string {
	p := &m.modals.search
	prompt := "> " + p.input.View()
	sepW := p.vp.Width
	if sepW < 1 {
		sepW = 1
	}
	sep := strings.Repeat("─", sepW)
	body := prompt + "\n" + sep + "\n" + p.vp.View()
	if len(p.hits) >= searchMaxHits {
		body += "\n" + lipgloss.NewStyle().Faint(true).
			Render("… results truncated at "+strconv.Itoa(searchMaxHits)+", refine the query")
	}
	return body
}
```

- [ ] **Step 5: Run the test**

Run: `go test ./internal/tui/ -run TestSearch_HitsRenderAsPathPlusSnippet -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/search.go internal/tui/search_test.go
git commit -m "feat(tui): render search hits as path:line + snippet two-row entries"
```

---

## Task 12: Wire `searchView` into `View()` and resize handling

**Files:**
- Modify: `internal/tui/view.go` (or wherever the modal switch in View lives)
- Modify: `internal/tui/model.go` (the WindowSizeMsg handler)

- [ ] **Step 1: Find where View dispatches by modal kind**

Run: `grep -n "modalKind\|modalPicker\|modals.kind" internal/tui/view.go`
Expected: a switch or if-chain that picks the modal body to render.

- [ ] **Step 2: Add the `modalSearch` case**

Find the switch (it'll have cases for `modalPicker`, `modalTree`, etc.). Add:

```go
	case modalSearch:
		body = m.searchView()
```

(The exact context depends on the existing structure — match the pattern of `modalPicker`. The variable name may be `body`, `content`, or similar; check the function and use whatever the existing branches use.)

- [ ] **Step 3: Find the WindowSizeMsg handler**

Run: `grep -n "WindowSizeMsg\|resizePicker\|resizeContent" internal/tui/model.go`

Find the `case tea.WindowSizeMsg:` block. It will call several `resizeFoo` methods. Add a call:

```go
		m.resizeSearch()
```

Place it adjacent to the existing `m.resizePicker()` call.

- [ ] **Step 4: Build and run the whole test suite**

Run: `go build ./... && go test -race ./...`
Expected: all green. No new test for this task — it's plumbing that the next tasks' tests exercise transitively.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/view.go internal/tui/model.go
git commit -m "feat(tui): render search modal in View, resize on WindowSizeMsg"
```

---

## Task 13: Implement cursor movement and Esc/Enter

**Files:**
- Modify: `internal/tui/input.go`
- Modify: `internal/tui/search_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/search_test.go`:

```go
func TestSearch_CursorDownAndUp(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, dir)
	m.modals.kind = modalSearch
	m.modals.search.hits = []search.Hit{
		{Path: "/x/a.md", Line: 1, Snippet: "a"},
		{Path: "/x/b.md", Line: 1, Snippet: "b"},
		{Path: "/x/c.md", Line: 1, Snippet: "c"},
	}

	// ^j moves down
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlJ})
	mm := updated.(Model)
	if mm.modals.search.cursor != 1 {
		t.Errorf("cursor = %d after ^j, want 1", mm.modals.search.cursor)
	}
	// ^k moves up
	updated, _ = mm.handleKey(tea.KeyMsg{Type: tea.KeyCtrlK})
	mm = updated.(Model)
	if mm.modals.search.cursor != 0 {
		t.Errorf("cursor = %d after ^k, want 0", mm.modals.search.cursor)
	}
	// Don't overshoot at boundaries
	updated, _ = mm.handleKey(tea.KeyMsg{Type: tea.KeyCtrlK})
	mm = updated.(Model)
	if mm.modals.search.cursor != 0 {
		t.Errorf("cursor = %d after ^k at top, want 0", mm.modals.search.cursor)
	}
}

func TestSearch_EscClearsQueryThenCloses(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, dir)
	m.modals.kind = modalSearch
	m.modals.search.input.SetValue("foo")

	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(Model)
	if mm.modals.kind != modalSearch {
		t.Errorf("first Esc should not close, kind = %v", mm.modals.kind)
	}
	if mm.modals.search.input.Value() != "" {
		t.Errorf("first Esc should clear query, got %q", mm.modals.search.input.Value())
	}

	updated, _ = mm.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	mm = updated.(Model)
	if mm.modals.kind != modalNone {
		t.Errorf("second Esc should close modal, kind = %v", mm.modals.kind)
	}
}

func TestSearch_EnterNavigatesAndScrolls(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.md")
	var sb strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&sb, "line %d\n\n", i)
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, dir)
	m.width = 80
	m.height = 24
	m.resizeContent()

	m.modals.kind = modalSearch
	m.modals.search.hits = []search.Hit{
		{Path: p, Line: 50, Snippet: "line 50"},
	}
	m.modals.search.cursor = 0

	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.modals.kind != modalNone {
		t.Errorf("Enter should close modal, kind = %v", mm.modals.kind)
	}
	if mm.history.Current() != p {
		t.Errorf("Current = %q, want %q", mm.history.Current(), p)
	}
	if mm.content.viewport.YOffset == 0 {
		t.Errorf("Expected viewport scrolled, YOffset = 0")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestSearch_CursorDownAndUp|TestSearch_EscClearsQueryThenCloses|TestSearch_EnterNavigatesAndScrolls' -v`
Expected: all three FAIL — handlers don't exist yet.

- [ ] **Step 3: Add the search-modal key handling block in input.go**

In `internal/tui/input.go`, find the existing block (around line 184) that starts with `if m.modals.kind != modalNone {` and dispatches per-modal key handling. Inside it, the picker has its own block. Mirror it for search.

Find the existing picker block and immediately after it (still inside the `if m.modals.kind != modalNone {` body), add:

```go
		if m.modals.kind == modalSearch {
			switch {
			case key.Matches(msg, m.keys.ClearLink): // Esc
				if m.modals.search.input.Value() != "" {
					m.modals.search.input.SetValue("")
					m.modals.search.hits = nil
					m.modals.search.cursor = 0
					if m.modals.search.scanStop != nil {
						m.modals.search.scanStop()
						m.modals.search.scanStop = nil
					}
					m.modals.search.inFlight = false
					m.refreshSearchVP()
					return *m, nil
				}
				m.closeModal()
				return *m, nil
			case key.Matches(msg, m.keys.Open): // Enter
				if 0 <= m.modals.search.cursor && m.modals.search.cursor < len(m.modals.search.hits) {
					h := m.modals.search.hits[m.modals.search.cursor]
					m.closeModal()
					m.pendingPreselectRange = &markdown.LineRange{Start: h.Line, End: h.Line}
					m.navigateTo(h.Path)
				}
				return *m, nil
			case key.Matches(msg, m.keys.SearchCursorDown):
				cursorMoveAndRefresh(&m.modals.search.cursor, len(m.modals.search.hits), 1, m.refreshSearchVP)
				return *m, nil
			case key.Matches(msg, m.keys.SearchCursorUp):
				cursorMoveAndRefresh(&m.modals.search.cursor, len(m.modals.search.hits), -1, m.refreshSearchVP)
				return *m, nil
			case key.Matches(msg, m.keys.Up):
				cursorMoveAndRefresh(&m.modals.search.cursor, len(m.modals.search.hits), -1, m.refreshSearchVP)
				return *m, nil
			case key.Matches(msg, m.keys.Down):
				cursorMoveAndRefresh(&m.modals.search.cursor, len(m.modals.search.hits), 1, m.refreshSearchVP)
				return *m, nil
			}
		}
```

If `markdown` isn't already imported in `input.go`, add `"github.com/wilkes/hypogeum/internal/markdown"` to the import block.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/tui/ -run 'TestSearch_CursorDownAndUp|TestSearch_EscClearsQueryThenCloses|TestSearch_EnterNavigatesAndScrolls' -v`
Expected: all three PASS.

- [ ] **Step 5: Run the full TUI suite to confirm no regression**

Run: `go test -race ./internal/tui/`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/input.go internal/tui/search_test.go
git commit -m "feat(tui): handle cursor, Esc, Enter inside the search modal"
```

---

## Task 14: Recency rerank integration test

**Files:**
- Modify: `internal/tui/search_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/search_test.go`:

```go
func TestSearch_RecencyRerank(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	if err := os.WriteFile(a, []byte("alpha needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("bravo needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, dir)
	// Visit b first so it scores higher in recency than a, then a so a
	// is the most-recent — final order should put a first.
	m.openFile(b)
	time.Sleep(2 * time.Millisecond)
	m.openFile(a)

	// Synthesize search results in alphabetical input order: a, b.
	hits := []search.Hit{
		{Path: a, Line: 1, Snippet: "alpha \x11needle\x12"},
		{Path: b, Line: 1, Snippet: "bravo \x11needle\x12"},
	}
	reranked := rerankByRecency(m.recent, hits)
	if len(reranked) != 2 {
		t.Fatalf("got %d hits, want 2", len(reranked))
	}
	if reranked[0].Path != a {
		t.Errorf("reranked[0].Path = %q, want %q (most recent)", reranked[0].Path, a)
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/tui/ -run TestSearch_RecencyRerank -v`
Expected: PASS. (If it fails, the test is the regression guard — fix `rerankByRecency`.)

- [ ] **Step 3: Commit**

```bash
git add internal/tui/search_test.go
git commit -m "test(tui): pin recency rerank order for search hits"
```

---

## Task 15: Update CLAUDE.md gotchas and docs/index.md

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/index.md`

- [ ] **Step 1: Add three gotchas to CLAUDE.md**

Open `CLAUDE.md`. Find the "Gotchas" section (look for the `- **URL-suppress preserves column width...**` bullet — the most recent gotcha addition).

Append these three new bullets to the end of the gotchas list (just before the `## What's not built yet` heading):

```markdown
- **`^s` opens the full-text search modal**, scanning every vault markdown file for a case-insensitive substring of the query. Lives in `internal/search` (pure, no TUI deps) + `internal/tui/search.go` (modal integration). Scans run on a 150ms debounce; each keystroke cancels the prior `scanCtx`. Results re-rank by `recent.Rank` before display. Enter sets `m.pendingPreselectRange = &markdown.LineRange{Start: hit.Line, End: hit.Line}` and calls `m.navigateTo(hit.Path)`. The destination scroll-to-line is the same plumbing range-link Enter and `followBacklink` use.
- **`searchState.paths` is a snapshot taken at modal-open time.** Files added/removed by the watcher during the modal's lifetime won't change the search corpus — closing and reopening `^s` refreshes. Deliberate: re-running every search on every fsnotify event would yank the user's cursor and burn CPU.
- **The markdown render path now honors `m.content.rangeHighlight` for scroll-to-line.** Previously this field was code-files-only (Chroma gutter highlight). `refreshContent` captures the line into `pendingScrollLine` before clearing, and after `GotoTop()` calls `scrollToLine(pendingScrollLine)`. The field still gets cleared so a re-render (e.g. WindowSizeMsg) doesn't keep re-scrolling. Search-Enter is the first caller, but any future caller that wants markdown-destination scroll can set `pendingPreselectRange` before `navigateTo`.
```

- [ ] **Step 2: Update docs/index.md**

Open `docs/index.md`. Find the line added by the spec:

```markdown
- [Full-text search](superpowers/specs/2026-05-14-full-text-search-design.md) — design approved 2026-05-14 — ...
```

Change `design approved 2026-05-14` to `shipped` and add a plan link:

```markdown
- [Full-text search](superpowers/specs/2026-05-14-full-text-search-design.md) — shipped — `^s` opens a modal that scans every vault markdown file for the query, renders hits as `path:line` + highlighted snippet, recency-ranks the result list, and on `Enter` opens the file scrolled to the matched line. [Plan](superpowers/plans/2026-05-14-full-text-search.md).
```

- [ ] **Step 3: Verify build is clean**

Run: `go build ./... && go vet ./... && go test -race ./...`
Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md docs/index.md
git commit -m "docs: document ^s full-text search gotchas and ship status"
```

---

## Task 16: Integration smoke test

**Files:**
- Modify: `internal/tui/dispatch_test.go`

- [ ] **Step 1: Find the existing dispatch tests for the established pattern**

Run: `grep -n "^func Test" internal/tui/dispatch_test.go | head -5`
Read those tests to see how they construct a Model and drive it. Use that pattern.

- [ ] **Step 2: Add the smoke test**

Append to `internal/tui/dispatch_test.go`:

```go
// TestSearch_EndToEnd opens the search modal via ^s, types a query that
// matches a single file, presses Enter, and verifies the destination
// renders scrolled to the matched line. This is the wire-it-all-up
// sanity check — fine-grained behavior covered in search_test.go.
func TestSearch_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "target.md")
	var sb strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&sb, "line %d\n\n", i)
	}
	sb.WriteString("the magic phrase appears here\n")
	for i := 1; i <= 10; i++ {
		sb.WriteString("trailing line\n")
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, dir)
	m.width = 80
	m.height = 24
	m.resizeContent()

	// ^s opens the modal
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	mm := updated.(Model)
	if mm.modals.kind != modalSearch {
		t.Fatalf("modal not opened")
	}

	// Type "magic"
	for _, r := range "magic" {
		updated, _ = mm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		mm = updated.(Model)
	}

	// Wait for debounce + scan. 250ms is generous.
	time.Sleep(250 * time.Millisecond)

	// Pump Update to deliver any tick that landed during the sleep.
	// The tea.Tick is delivered via the program loop in real use; tests
	// need to advance manually. The test infrastructure may already
	// handle this via the message loop helper — check existing tests
	// for the pattern.

	// Synthesize the tick directly since we're not running a tea program.
	updated, cmd := mm.Update(searchTickMsg{query: "magic"})
	mm = updated.(Model)
	if cmd != nil {
		// Run the cmd to get the searchResultsMsg.
		msg := cmd()
		updated, _ = mm.Update(msg)
		mm = updated.(Model)
	}

	if len(mm.modals.search.hits) == 0 {
		t.Fatalf("expected hits for 'magic', got 0")
	}

	// Enter on the hit
	updated, _ = mm.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)

	if mm.modals.kind != modalNone {
		t.Errorf("Enter should close modal, kind = %v", mm.modals.kind)
	}
	if mm.history.Current() != p {
		t.Errorf("Current = %q, want %q", mm.history.Current(), p)
	}
	if mm.content.viewport.YOffset == 0 {
		t.Errorf("expected viewport scrolled after Enter, YOffset = 0")
	}
}
```

Add `"fmt"`, `"os"`, `"path/filepath"`, `"strings"`, `"time"` to the imports if not present.

- [ ] **Step 3: Run the smoke test**

Run: `go test ./internal/tui/ -run TestSearch_EndToEnd -v`
Expected: PASS.

- [ ] **Step 4: Run the entire test suite a final time**

Run: `go test -race ./...`
Expected: every package green.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/dispatch_test.go
git commit -m "test(tui): end-to-end smoke test for ^s search modal"
```

---

## Task 17: Open the PR

- [ ] **Step 1: Push the branch**

```bash
git push -u origin full-text-search
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "feat(search): ^s full-text search modal" --body "$(cat <<'EOF'
## Summary
- New keystroke **^s** opens a full-text search modal that scans every vault markdown file for a case-insensitive substring of the query.
- Results render as two-row entries (`path:line` header + highlighted snippet) and rank by the picker's recency score. `Enter` opens the file scrolled to the matched line via the existing `pendingPreselectRange` plumbing.
- New **`internal/search`** package is pure (no Bubble Tea) and has full unit coverage. TUI integration lives in **`internal/tui/search.go`**.

## Design
[docs/superpowers/specs/2026-05-14-full-text-search-design.md](docs/superpowers/specs/2026-05-14-full-text-search-design.md) — design approved 2026-05-14.

## Test plan
- [x] `go test -race ./...` — all packages green
- [x] Unit tests in `internal/search/` cover scan, snippet, fan-out, cancellation, binary skip, missing file
- [x] Model-level tests in `internal/tui/search_test.go` cover ^s open/close, debounce, cursor, Esc, Enter, recency rerank
- [x] End-to-end smoke test in `internal/tui/dispatch_test.go` drives the full ^s → type → Enter → scrolled-content flow
- [ ] Manual smoke: `go run ./cmd/hypogeum docs/`, ^s, type a known phrase, Enter, confirm the destination file lands scrolled to the matched line

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Watch CI**

```bash
gh run watch
```
Expected: CI green.

---

## Self-review

### Spec coverage check

- ✅ Scope (markdown only) — Task 8's `m.allVaultMarkdownPaths()` snapshot
- ✅ Recency ranking — Task 10's `rerankByRecency` + Task 14's test
- ✅ 150ms debounce — Task 7's `searchDebounce` + Task 9's `scheduleSearchTick`
- ✅ Navigate-and-scroll — Task 5 (markdown rangeHighlight) + Task 13's Enter handler
- ✅ Case-insensitive — Task 3's `scanFile` lowercases
- ✅ 200-hit cap — Task 7's `searchMaxHits` + Task 4's Search cap
- ✅ 2-char minimum — Task 7's `searchMinQuery` + Task 9's guard
- ✅ `internal/search` package boundary — Task 1 (pure stdlib imports)
- ✅ `internal/tui/search.go` integration — Tasks 6-13
- ✅ `content.go` rangeHighlight extension — Task 5
- ✅ `modalSearch` kind + keymap — Tasks 6, 8
- ✅ Single-modal-swap invariant — `openModalWith` (Task 8) is the shared open path
- ✅ `^j`/`^k` cursor / `j`/`k` typing — Tasks 8, 13
- ✅ Esc two-press semantics — Task 13
- ✅ Empty/loading/no-match placeholders — Task 9's `renderSearchRows`
- ✅ Overflow footer — Task 11's `searchView`
- ✅ Help integration — `^s` in keymap (Task 8) auto-appears in help
- ✅ Watcher snapshot semantics — Task 7's `searchState.paths` doc comment
- ✅ Diagnostics logging — Task 10's `m.diag.Info` calls

### Placeholder scan

Searched the plan for "TBD", "TODO", "implement later", "appropriate error handling", "similar to Task" — none found. Every code step contains the actual code an engineer needs.

### Type consistency

- `Hit` struct: `Path string`, `Line int`, `Snippet string` — used consistently in Tasks 1, 2, 3, 4, 10, 11, 13, 14, 16.
- `searchState` fields: `input`, `paths`, `hits`, `cursor`, `vp`, `scanCtx`, `scanStop`, `inFlight` — defined Task 7, accessed unchanged in Tasks 9, 10, 11, 12, 13, 14.
- `searchTickMsg{query}` and `searchResultsMsg{query, hits, err}` — defined Task 7, used unchanged Tasks 9, 10, 16.
- `searchMaxHits`, `searchDebounce`, `searchMinQuery`, `searchSnippetWindow` — defined Task 7, referenced unchanged elsewhere.
- `rerankByRecency` signature `(store recentStore, hits []search.Hit) []search.Hit` — defined Task 10, used Task 14.
- `relativeTo`, `truncateOneLine`, `applyHighlight`, `cursorMarkerStyle` — pre-existing helpers in the `tui` package; Task 11 calls them without redefining.

No drift detected.
