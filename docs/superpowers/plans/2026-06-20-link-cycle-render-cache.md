# Link-cycle Render Reuse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `n`/`p` link cycling re-apply the reverse-video highlight via a cheap `stripSentinels` pass on a cached render, instead of re-reading the file and running a full Glamour render every keystroke.

**Architecture:** Split the markdown render into the expensive Glamour pass (`raw`, depends only on content+width) and the cheap highlight pass. A new `RenderDocument` returns a `*RenderResult` that keeps `raw`; `RenderWithLinks` becomes a thin wrapper over it (signature unchanged). The TUI stores the handle in `contentUIState.render` and `applyLinkHighlight` calls `RenderResult.WithHighlight(cursor)` — no I/O, no Glamour.

**Tech Stack:** Go 1.24 (`testing.B`, `for b.Loop()`), Bubble Tea TUI, Glamour markdown render, sentinel-instrumented link recovery (`internal/markdown`).

## Global Constraints

- **Go 1.24.5**, module `github.com/wilkes/hypogeum`. `for b.Loop()` is the benchmark idiom (setup before the loop is untimed).
- **`RenderWithLinks` keeps its exact current signature** `(string, []Link, []string, error)` and becomes a wrapper over `RenderDocument`. This is what keeps its ~25 existing test call sites untouched — do not change them.
- **Correctness invariant:** `RenderDocument(src,base,anyMarker).WithHighlight(i)` must equal `RenderWithLinks(src,base,HighlightMarker(i))`'s rendered string, byte-for-byte, for every `i` (and `-1`).
- **`applyLinkHighlight` must route through `m.setContent`** so `m.content.rendered` (the drag-select overlay base) stays in sync — a documented invariant.
- **`m.content.render` must be set to `nil` on every non-markdown-success path** in `refreshContent` (dir error, read error, code-file branch, markdown error) and to the handle only on the markdown success path.
- **Race-clean:** `go test -race ./...` must pass.
- Commit message bodies end with these two trailer lines verbatim:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
  `Claude-Session: https://claude.ai/code/session_01SozSUyuyW9fNBS1KeHd5ri`

---

### Task 1: Render/highlight split in `internal/markdown`

**Files:**
- Modify: `internal/markdown/links_render.go`
- Test: `internal/markdown/links_render_test.go` (add one test)

**Interfaces:**
- Consumes (existing, unchanged): `(*Renderer).preprocessEmbeds`, `preprocessWikilinks`, `instrumented.Render`, `ExtractLinks`, `stripSentinels`, `ResolveLink`, `HighlightMarker`.
- Produces:
  - `type RenderResult struct { Content string; Links []Link; EmbedDeps []string; raw string }`
  - `func (rr *RenderResult) WithHighlight(selected int) string`
  - `func (r *Renderer) RenderDocument(src, base string, marker LinkMarker) (*RenderResult, error)`
  - `RenderWithLinks` retains signature `(string, []Link, []string, error)`, now a wrapper.

- [ ] **Step 1: Write the failing test**

Add to `internal/markdown/links_render_test.go`:

```go
func TestRenderDocument_WithHighlightMatchesFullRender(t *testing.T) {
	r, err := NewRenderer(80)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	src := "See [one](a.md) and [two](b.md) and [three](c.md).\n"
	base := "/base/file.md"

	rr, err := r.RenderDocument(src, base, nil)
	if err != nil {
		t.Fatalf("RenderDocument: %v", err)
	}
	if len(rr.Links) < 3 {
		t.Fatalf("expected >=3 links, got %d", len(rr.Links))
	}

	for _, i := range []int{-1, 0, 1, 2} {
		want, _, _, err := r.RenderWithLinks(src, base, HighlightMarker(i))
		if err != nil {
			t.Fatalf("RenderWithLinks(%d): %v", i, err)
		}
		if got := rr.WithHighlight(i); got != want {
			t.Errorf("WithHighlight(%d) != full render with HighlightMarker(%d)\n got: %q\nwant: %q",
				i, i, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/markdown/ -run TestRenderDocument_WithHighlightMatchesFullRender`
Expected: compile failure — `r.RenderDocument undefined` and `rr.WithHighlight undefined`.

- [ ] **Step 3: Implement the split**

In `internal/markdown/links_render.go`, replace the existing `RenderWithLinks` function (currently the whole body that renders + strips + builds links) with the type, the two methods, and the wrapper. The new code:

```go
// RenderResult is a completed render plus the sentinel-instrumented Glamour
// output, so the highlighted link can be changed without re-running Glamour.
// raw is unexported: only WithHighlight (same package) re-strips it.
type RenderResult struct {
	Content   string   // rendered output with the marker passed to RenderDocument applied
	Links     []Link   // every followable link, document order
	EmbedDeps []string // absolute source paths sliced in by embeds
	raw       string   // Glamour output with sentinels intact — input to re-highlight
}

// WithHighlight re-derives the visible output with only link `selected`
// reverse-videoed (selected = -1 highlights nothing). Cheap: a single
// stripSentinels pass over raw, with no Glamour render.
func (rr *RenderResult) WithHighlight(selected int) string {
	cleaned, _ := stripSentinels(rr.raw, HighlightMarker(selected))
	return cleaned
}

// RenderDocument renders src and returns a reusable RenderResult. base is the
// path of the file the source came from; it resolves relative link targets.
// See RenderWithLinks for the marker semantics. The pipeline: preprocessEmbeds
// and preprocessWikilinks rewrite the source, the instrumented renderer injects
// sentinels, stripSentinels recovers link positions from the ANSI output.
func (r *Renderer) RenderDocument(src, base string, marker LinkMarker) (*RenderResult, error) {
	src, embedDeps, embedLinks := r.preprocessEmbeds(src, base)
	src = r.preprocessWikilinks(src)
	raw, err := r.instrumented.Render(src)
	if err != nil {
		return nil, fmt.Errorf("render markdown: %w", err)
	}

	asts := ExtractLinks(src)
	cleaned, spans := stripSentinels(raw, marker)
	links := make([]Link, 0, len(spans)+len(embedLinks))
	for i, s := range spans {
		l := Link{Row: s.row}
		if i < len(asts) {
			l.Text = asts[i].Text
			l.Href = asts[i].Href
			l.Resolved = ResolveLink(base, asts[i].Href)
		} else {
			l.Text = s.text
		}
		links = append(links, l)
	}
	links = append(links, embedLinks...)

	return &RenderResult{
		Content:   cleaned,
		Links:     links,
		EmbedDeps: embedDeps,
		raw:       raw,
	}, nil
}

// RenderWithLinks renders src and returns the rendered string, the links, and
// the embed dependency paths. It is a thin wrapper over RenderDocument kept for
// callers that don't need the reusable handle.
//
// If marker is non-nil, the open/close strings it returns for each link are
// spliced around that link's visible text. They flow through downstream styling
// without changing visible width (caller's responsibility — typically
// zero-width sentinel sequences).
func (r *Renderer) RenderWithLinks(src, base string, marker LinkMarker) (string, []Link, []string, error) {
	rr, err := r.RenderDocument(src, base, marker)
	if err != nil {
		return "", nil, nil, err
	}
	return rr.Content, rr.Links, rr.EmbedDeps, nil
}
```

Keep the existing package-level doc comment block above `RenderWithLinks` (the `Link` and `LinkMarker` type docs and `HighlightMarker` stay exactly as they are earlier in the file). The `import "fmt"` at the top of the file is still needed.

- [ ] **Step 4: Run the new test and the full markdown suite**

Run: `go test ./internal/markdown/`
Expected: PASS — `TestRenderDocument_WithHighlightMatchesFullRender` passes, and all ~25 existing `RenderWithLinks` tests still pass (the wrapper preserves behavior).

- [ ] **Step 5: Commit**

```bash
git add internal/markdown/links_render.go internal/markdown/links_render_test.go
git commit -m "feat(markdown): split RenderDocument from RenderWithLinks for reusable renders"
```

---

### Task 2: Benchmark the re-highlight win

**Files:**
- Modify: `internal/markdown/render_bench_test.go` (add one benchmark)

**Interfaces:**
- Consumes: `benchcorpus.Generate`, `Corpus.Target`; `markdown.NewRenderer`, `(*Renderer).RenderDocument`, `(*RenderResult).WithHighlight`, `markdown.HighlightMarker`.
- Produces: `BenchmarkWithHighlight` — measures the per-keystroke re-highlight cost, to compare against `BenchmarkRenderWithLinks` (the cost a keystroke used to incur).

- [ ] **Step 1: Add the benchmark**

Append to `internal/markdown/render_bench_test.go`:

```go
func BenchmarkWithHighlight(b *testing.B) {
	c := benchcorpus.Generate(b.TempDir(), 7, 50, 4)
	body, err := os.ReadFile(c.Target)
	if err != nil {
		b.Fatal(err)
	}
	// Prepend an inline link to a local file so there is a cyclable link for
	// the highlight to land on (the corpus body uses wikilinks, which don't
	// enter the link cycler).
	src := "See [anchor](other.md) for details.\n\n" + string(body)

	r, err := markdown.NewRenderer(80)
	if err != nil {
		b.Fatal(err)
	}
	rr, err := r.RenderDocument(src, c.Target, markdown.HighlightMarker(-1))
	if err != nil {
		b.Fatal(err)
	}
	if len(rr.Links) == 0 {
		b.Fatal("expected at least the prepended inline link")
	}

	for b.Loop() {
		_ = rr.WithHighlight(0)
	}
}
```

- [ ] **Step 2: Run both benchmarks and compare**

Run: `go test -run=^$ -bench='BenchmarkRenderWithLinks|BenchmarkWithHighlight' -benchmem ./internal/markdown/`
Expected: two lines. `BenchmarkRenderWithLinks` ~12k allocs/op (the old per-keystroke cost); `BenchmarkWithHighlight` a small constant (the new cost) — at least ~20× fewer allocs/op and far lower ns/op. If `WithHighlight` is not dramatically cheaper, STOP and report — the split isn't delivering the win.

- [ ] **Step 3: Commit**

```bash
git add internal/markdown/render_bench_test.go
git commit -m "test(bench): BenchmarkWithHighlight proves re-highlight avoids re-render"
```

---

### Task 3: Wire the handle into the TUI

**Files:**
- Modify: `internal/tui/content.go` (struct field + `refreshContent`)
- Modify: `internal/tui/links.go` (`applyLinkHighlight`, imports)
- Test: `internal/tui/model_test.go` (add one test)

**Interfaces:**
- Consumes: `(*markdown.Renderer).RenderDocument` and `*markdown.RenderResult` (Task 1); `(*RenderResult).WithHighlight`, `RenderResult.Content`, `.Links`, `.EmbedDeps`.
- Produces: `contentUIState.render *markdown.RenderResult`; a rewritten `applyLinkHighlight` that re-highlights without I/O.

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/model_test.go`:

```go
func TestModel_LinkCycleReusesRenderHandle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mdPath := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(mdPath, []byte("See [alpha](a.md) and [bravo](b.md).\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := sized(t, dir, mdPath)
	if m.content.render == nil {
		t.Fatal("expected render handle set after refresh")
	}
	if len(m.content.links) < 2 {
		t.Fatalf("expected >=2 links, got %d", len(m.content.links))
	}

	m.cycleLink(+1) // linkCursor -> 0
	first := m.content.viewport.View()
	if !strings.Contains(first, "\x1b[7m") {
		t.Fatalf("expected reverse-video after cycling to first link:\n%s", first)
	}

	m.cycleLink(+1) // linkCursor -> 1
	second := m.content.viewport.View()
	if !strings.Contains(second, "\x1b[7m") {
		t.Fatalf("expected reverse-video after cycling to second link:\n%s", second)
	}
	if first == second {
		t.Fatal("expected highlight to move to a different link (views identical)")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_LinkCycleReusesRenderHandle`
Expected: compile failure — `m.content.render undefined` (the field doesn't exist yet).

- [ ] **Step 3: Add the `render` field**

In `internal/tui/content.go`, add to the `contentUIState` struct (right after the `links []markdown.Link` / `linkCursor int` fields):

```go
	// render is the current markdown document's reusable render. It holds the
	// sentinel-instrumented Glamour output so applyLinkHighlight can
	// re-highlight a different link without re-rendering. nil for code files
	// and error states.
	render *markdown.RenderResult
```

- [ ] **Step 4: Set `render = nil` on every non-markdown-success path in `refreshContent`**

In `internal/tui/content.go`, add `m.content.render = nil` to each early-return block that already nils `m.content.links`:

1. The directory-listing error block (alongside `m.content.links = nil`).
2. The file-read error block (alongside `m.content.links = nil`).
3. The code-file branch — alongside the `m.content.links = nil` / `m.content.linkCursor = -1` / `m.content.embedDeps = nil` cleanup that runs before its `return`.

Example, the file-read error block becomes:

```go
		src, err = os.ReadFile(path)
		if err != nil {
			m.footerMessage = err.Error()
			m.setContent(fmt.Sprintf("Error: %v", err))
			m.content.links = nil
			m.content.linkCursor = -1
			m.content.brokenCount = 0
			m.content.render = nil
			return
		}
```

Apply the same one-line `m.content.render = nil` addition to the directory error block and the code-file cleanup block.

- [ ] **Step 5: Switch the markdown success path to `RenderDocument` and store the handle**

In `internal/tui/content.go`, change the markdown render block. Replace:

```go
	m.content.renderer.SetFromFile(path)
	out, links, deps, err := m.content.renderer.RenderWithLinks(string(src), path, linkZoneMarker)
	if err != nil {
		m.footerMessage = err.Error()
		m.setContent(fmt.Sprintf("Error: %v", err))
		m.content.links = nil
		m.content.linkCursor = -1
		m.content.embedDeps = nil
		m.content.brokenCount = 0
		return
	}
	m.currentPath = path
	m.footerMessage = ""
	m.setContent(out)
	m.content.viewport.GotoTop()
	if pendingScrollLine > 0 {
		m.scrollToLine(pendingScrollLine)
	}
	m.content.links = links
	m.content.brokenCount = m.content.renderer.CountUnresolvedWikilinks(string(src))
	for _, l := range links {
		if l.Resolved.Kind != markdown.LinkLocalFile {
			continue
		}
		if markdown.IsBrokenLocalLink(l.Resolved.Target) {
			m.content.brokenCount++
		}
	}

	m.content.embedDeps = make(map[string]struct{}, len(deps))
	for _, p := range deps {
		m.content.embedDeps[p] = struct{}{}
		if m.watcher != nil {
			_ = m.watcher.AddPath(filepath.Dir(p))
		}
	}
```

with:

```go
	m.content.renderer.SetFromFile(path)
	rr, err := m.content.renderer.RenderDocument(string(src), path, linkZoneMarker)
	if err != nil {
		m.footerMessage = err.Error()
		m.setContent(fmt.Sprintf("Error: %v", err))
		m.content.links = nil
		m.content.linkCursor = -1
		m.content.embedDeps = nil
		m.content.brokenCount = 0
		m.content.render = nil
		return
	}
	m.content.render = rr
	m.currentPath = path
	m.footerMessage = ""
	m.setContent(rr.Content)
	m.content.viewport.GotoTop()
	if pendingScrollLine > 0 {
		m.scrollToLine(pendingScrollLine)
	}
	m.content.links = rr.Links
	m.content.brokenCount = m.content.renderer.CountUnresolvedWikilinks(string(src))
	for _, l := range rr.Links {
		if l.Resolved.Kind != markdown.LinkLocalFile {
			continue
		}
		if markdown.IsBrokenLocalLink(l.Resolved.Target) {
			m.content.brokenCount++
		}
	}

	m.content.embedDeps = make(map[string]struct{}, len(rr.EmbedDeps))
	for _, p := range rr.EmbedDeps {
		m.content.embedDeps[p] = struct{}{}
		if m.watcher != nil {
			_ = m.watcher.AddPath(filepath.Dir(p))
		}
	}
```

(The preselect block below this — `m.content.linkCursor = -1`, the target match loop, and the trailing `if m.content.linkCursor >= 0 { m.scrollToLink(...); m.applyLinkHighlight() }` — stays exactly as-is. It now relies on `m.content.render` being set just above, which it is.)

- [ ] **Step 6: Rewrite `applyLinkHighlight`**

In `internal/tui/links.go`, replace the entire `applyLinkHighlight` function body (the `os.Stat`/`renderDirListing`/`os.ReadFile`/`SetFromFile`/`RenderWithLinks` version) with:

```go
// applyLinkHighlight re-renders the current document's reverse-video highlight
// around the selected link by re-applying the highlight marker to the cached
// render — no file read, no Glamour render. The scroll position set by
// scrollToLink is preserved. A nil render handle (code file / error state)
// is a no-op; such documents have no cyclable links.
func (m *Model) applyLinkHighlight() {
	if m.content.render == nil {
		return
	}
	offset := m.content.viewport.YOffset
	m.setContent(m.content.render.WithHighlight(m.content.linkCursor))
	m.content.viewport.SetYOffset(offset)
}
```

Then remove the now-unused `"os"` import from `internal/tui/links.go` (it was used only by the old `applyLinkHighlight`; `followLink` and the others use `path/filepath` and `markdown`, not `os`).

- [ ] **Step 7: Build, run the new test, and the focused regressions**

Run: `go build ./... && go test ./internal/tui/ -run 'TestModel_LinkCycleReusesRenderHandle|TestModel_'`
Expected: PASS — the new test passes, and existing model tests (range-link highlight, Esc-clears-highlight, link preselect on navigation) stay green. If `go build` flags an unused import, remove it.

- [ ] **Step 8: Full race-clean suite**

Run: `go test -race ./...`
Expected: PASS across all packages.

- [ ] **Step 9: Commit**

```bash
git add internal/tui/content.go internal/tui/links.go internal/tui/model_test.go
git commit -m "perf(tui): reuse cached render for link-cycle highlight"
```

---

## Self-Review

**Spec coverage:**
- Render/highlight split (`RenderResult`, `RenderDocument`, `WithHighlight`, `RenderWithLinks` wrapper) → Task 1. ✓
- Correctness invariant (cached == full render) → Task 1 Step 1 test. ✓
- The win, measured → Task 2 `BenchmarkWithHighlight` vs `BenchmarkRenderWithLinks`. ✓
- TUI handle field + `refreshContent` (success sets handle, all other paths nil) + `applyLinkHighlight` rewrite → Task 3. ✓
- `setContent` invariant preserved → Task 3 Step 6 uses `m.setContent`. ✓
- Race-clean → Task 3 Step 8. ✓
- Edge cases: code files (`render == nil` guard, Task 3 Step 6); resize-drops-highlight (unchanged behavior — `refreshContent` rebuilds handle, no task needed). ✓
- Future work (nav cache, composite markers) → explicitly not built; no task, by design. ✓

**Placeholder scan:** No TBD/TODO/"handle errors"/"similar to Task N". Every code step shows complete code. The "keep the existing block as-is" notes in Task 3 reference concrete, quoted surrounding code.

**Type consistency:** `RenderResult{Content, Links, EmbedDeps, raw}`, `RenderDocument(src, base, marker) (*RenderResult, error)`, `WithHighlight(selected int) string`, and `contentUIState.render *markdown.RenderResult` are used identically across Tasks 1–3. `RenderWithLinks` signature unchanged, verified against the 25 call sites. The `refreshContent` rewrite renames local `out`→`rr.Content`, `links`→`rr.Links`, `deps`→`rr.EmbedDeps` consistently within the block.
