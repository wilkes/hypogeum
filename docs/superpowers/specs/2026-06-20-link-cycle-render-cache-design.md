# Link-cycle render reuse — design

Status: approved, pre-implementation.

## Goal

Stop link cycling (`n` / `p`) from re-rendering the whole document through
Glamour. Today every cursor move calls `applyLinkHighlight`, which re-reads the
file and runs a full `RenderWithLinks` (~12.3k allocations, ~1.4 ms) just to
move a reverse-video highlight by one link. The expensive Glamour render
depends only on `(content, width)`; the highlight is applied afterward by a
cheap `stripSentinels` pass. We split those two steps so cycling re-applies the
highlight without re-rendering.

## Background: why this, not a render micro-optimization

A memory profile of `BenchmarkRenderWithLinks` showed ~95% of allocations live
inside Glamour / termenv ANSI styling (`termenv.Style.Styled`, `fmt.Sprintf`,
`Foreground`, `ANSI256Color.Sequence`), and under 2% in hypogeum's own code.
There is no allocation hot spot in our code to tighten. The only meaningful
lever is to **not render when we don't have to** — and link cycling re-renders
identical content, varying only which link is highlighted.

This is the first follow-up from the measure-only
[benchmarking foundation](2026-06-20-benchmarking-foundation-design.md).

## Scope

In scope: make `n` / `p` cycling reuse the current document's render. Narrow,
high-impact, confined to `internal/markdown` (the render/highlight split) and
`internal/tui` (`applyLinkHighlight` + one `contentUIState` field).

Out of scope (deliberately, YAGNI — see Future work): a multi-document
navigation cache, surviving the highlight across resize, and combining mouse
zone markers with the keyboard highlight.

## The render/highlight split (`internal/markdown`)

`RenderWithLinks`'s pipeline (`links_render.go`):

```
src = preprocessEmbeds(src, base)        // cheap
src = preprocessWikilinks(src)           // cheap
raw = r.instrumented.Render(src)         // EXPENSIVE (Glamour) — depends only on (src, width)
asts = ExtractLinks(src)
cleaned, spans = stripSentinels(raw, marker)   // CHEAP — applies marker by a pass over raw
```

`raw` (the sentinel-instrumented Glamour output) is marker-independent: the
sentinels are injected by the instrumented renderer; the marker only enters at
`stripSentinels`. So one `raw` can produce any marker variation cheaply.

### New API

```go
// RenderResult is a completed render plus the sentinel-instrumented output,
// so the highlighted link can be changed without re-running Glamour.
type RenderResult struct {
    Content   string   // rendered output with the marker passed to the call applied
    Links     []Link
    EmbedDeps []string
    raw       string   // Glamour output with sentinels intact — input to re-highlight
}

// WithHighlight re-derives the visible output with only link `selected`
// reverse-videoed (selected = -1 highlights nothing). Cheap: one
// stripSentinels pass over raw, no Glamour render.
func (rr *RenderResult) WithHighlight(selected int) string {
    cleaned, _ := stripSentinels(rr.raw, HighlightMarker(selected))
    return cleaned
}

// RenderDocument is the new primary entry point. It is today's
// RenderWithLinks body, but it keeps raw and returns the struct.
func (r *Renderer) RenderDocument(src, base string, marker LinkMarker) (*RenderResult, error)
```

`RenderWithLinks` keeps its existing signature and becomes a thin wrapper, so
its ~25 existing test call sites are untouched:

```go
func (r *Renderer) RenderWithLinks(src, base string, marker LinkMarker) (string, []Link, []string, error) {
    rr, err := r.RenderDocument(src, base, marker)
    if err != nil {
        return "", nil, nil, err
    }
    return rr.Content, rr.Links, rr.EmbedDeps, nil
}
```

All sentinel knowledge stays inside `internal/markdown`; the TUI never sees
`raw` or `stripSentinels`.

### Correctness invariant

For any document and any link index `i`:

```
RenderDocument(src, base, anyMarker).WithHighlight(i)
    == RenderWithLinks(src, base, HighlightMarker(i))   // the Content value
```

The cached re-highlight path must be byte-identical to a full render with the
same highlight marker. This is the central guarantee the tests enforce.

## TUI wiring (`internal/tui`)

### Store the handle

`contentUIState` gains one field:

```go
render *markdown.RenderResult // current document's reusable render; nil for code files / errors
```

### refreshContent

The markdown branch (`content.go`, around line 250) switches from
`RenderWithLinks` to `RenderDocument`, stores the handle in
`m.content.render`, and reads `Content` / `Links` / `EmbedDeps` off it.
The non-markdown (code) branch and **every** error / early-return path set
`m.content.render = nil`. Render cost here is unchanged — this path already ran
the render.

### applyLinkHighlight

Collapses from "re-read file + full render" to a cheap re-strip:

```go
func (m *Model) applyLinkHighlight() {
    if m.content.render == nil {
        return // code file / error state — no links to cycle anyway
    }
    offset := m.content.viewport.YOffset
    m.setContent(m.content.render.WithHighlight(m.content.linkCursor))
    m.content.viewport.SetYOffset(offset)
}
```

This deletes the existing `os.Stat` / `renderDirListing` / `os.ReadFile` /
`SetFromFile` / `RenderWithLinks` block: the handle already encapsulates the
rendered output whether the source was a file or a synthesized directory
listing.

### Data flow per keystroke

`cycleLink` → update index → `scrollToLink` → `applyLinkHighlight` → one
`stripSentinels` pass → `setContent`. No file I/O, no Glamour render.

### Invariants and edge cases

- **`setContent` is mandatory.** Routing through `m.setContent` keeps
  `m.content.rendered` (the drag-select overlay's base) in sync — a documented
  invariant. The cheap path must not bypass it.
- **Correctness bonus:** re-highlighting the cached render means the highlight
  always matches the displayed document. The watcher's `FileModified` branch
  still produces a fresh handle (via `refreshContent`) when the file actually
  changes.
- **Code files:** no markdown render → `render == nil`; `cycleLink` already
  returns early on zero links, and the guard is belt-and-suspenders.
- **Resize mid-highlight:** `WindowSizeMsg` → `refreshContent` rebuilds the
  handle at the new width with zone markers (highlight drops), exactly as
  today. Faithful to current behavior; not changed here.

## Testing & verification

- **Correctness (markdown):** `TestRenderDocument_WithHighlightMatchesFullRender`
  — for a document with ≥2 links, assert `rr.WithHighlight(i)` is byte-identical
  to `RenderWithLinks(src, base, HighlightMarker(i))`'s output for each `i` and
  for `-1` (no highlight).
- **The win (benchmark):** add `BenchmarkWithHighlight` (re-highlight from a
  prebuilt handle) beside the existing `BenchmarkRenderWithLinks`. Expected:
  allocs/op drops from ~12.3k to a small constant. Capture the
  `benchstat`-able before/after.
- **TUI behavior:** a model-level test that after `cycleLink`,
  `m.content.rendered` contains the reverse-video SGR (`\x1b[7m`) around the
  selected link's text and `m.content.render` is non-nil. Existing
  link-cycling tests stay green (no behavior change).
- **Race:** `go test -race ./...` clean.

**Success criterion:** `n` / `p` cycling performs zero Glamour renders and zero
file reads, proven by the benchmark delta and the equality test, with no change
to what the user sees.

## Future work (not built now)

The design leaves these seams open, but builds none of them:

- **Navigation cache:** `RenderResult` is the reusable unit. A small LRU keyed
  by `(path, width, mtime)` would let Back / Forward to a visited file at the
  same width skip rendering. The handle type does not change.
- **Composite markers:** `WithHighlight` is one of a family. A `WithZones()` or
  `WithZonesAndHighlight(i)` (so mouse zones survive cycling) is a new cheap
  method on `RenderResult`, not a re-architecture.
