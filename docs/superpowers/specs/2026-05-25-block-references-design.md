# Block references and heading anchors — design

**Status:** shipped.
**Scope:** resolve `#Heading` and `^block-id` anchors in both wikilinks (`[[Note#Heading]]`, `[[Note#^block]]`) and standard inline links (`[text](note.md#heading)`, `[text](note.md#^block)`). Following an anchored link navigates to the file and scrolls to the anchor's line. Anchors that don't resolve count as broken.
**Out of scope:** rename-and-rewrite of block ids, `![[Note#^block]]` markdown transclusion, configurable vault root.

See also: [wikilinks and backlinks](2026-05-07-wikilinks-and-backlinks-design.md), [broken-link tally](2026-05-25-broken-link-tally-design.md), [pre-select inline link](2026-05-09-pre-select-inline-link-design.md).

## Motivation

The wikilink parser already extracts `Heading` and `Block` components from `[[Name#Heading^Block]]`, but `vault.Resolve` ignores them — it returns the file path and the destination opens at the top. Standard inline links with anchors (`[text](note.md#heading)`) populate `ResolvedLink.Anchor` but the TUI prints `anchor navigation not implemented: #...` in the footer.

Both gaps share the same destination machinery (`pendingPreselectRange` + `scrollToLine`) that already powers full-text search Enter, range-link Enter, and backlink-follow. Shipping anchor resolution closes both gaps with one design.

## Architecture

A per-file anchor index is added to `internal/vault`, populated during the same goldmark walk that gathers references. `internal/markdown` gains a preprocess pass that strips trailing `^block-id` markers from the rendered source (the vault sees the raw markers, the renderer doesn't). Renderer branches that produce link hrefs grow a `ResolveAnchor` check so anchor-broken links style as broken. TUI follow paths look up the anchor's line and set `pendingPreselectRange` before `navigateTo`.

No new package. The existing `markdown → vault` interface edge (via `Resolver`) grows one method.

## Components

### `internal/vault` — anchor index

Each indexed file's record gains:

```go
headings map[string]int   // slug → 1-based line of the '#' marker
blocks   map[string]int   // id   → 1-based line of the FIRST line of the enclosing block
```

Slug rule reuses `slugify` from `internal/markdown/links_render.go`. To avoid a `vault → markdown` import cycle, `slugify` is copied (it's ~15 lines and stable); a comment on each copy points at the other.

Extraction runs inside the existing goldmark AST walk used by `internal/vault/extract.go`. For headings, the line number comes from the heading node's source position. For blocks, the walker tracks the enclosing block-level node (paragraph, list item, blockquote, fenced code block, etc.) and when it sees a trailing `^id` token on the last text line of that block, records `blocks[id] = blockStartLine`. Markers inside fenced code blocks are ignored — the regex is gated on the block kind.

The `Resolver` interface (in `internal/markdown/resolver.go`) grows one method:

```go
type Resolver interface {
    Resolve(fromFile, name, heading, block string) (path string, ok bool)
    ResolveAnchor(path, heading, block string) (line int, ok bool)
}
```

`Vault.ResolveAnchor` semantics:

- `block != ""`: return `blocks[block]`. (Block wins over heading when both are present — Obsidian allows `#Heading^block`; the block is more specific.)
- `heading != ""`: return `headings[slugify(heading)]`.
- Both empty: `(0, false)`.

`RefreshFile` rebuilds the anchor maps for the refreshed file alongside the existing references. `Rebuild` rebuilds for the whole vault.

### `internal/markdown` — preprocess + render-time check

A new preprocess pass, `preprocessBlockMarkers`, runs *before* `preprocessEmbeds` and `preprocessWikilinks` in `RenderWithLinks`. It strips a trailing ` ^[a-zA-Z0-9-]+$` from non-code lines, using a small state machine to skip lines inside fenced code blocks (the same fence-tracking used by `preprocessEmbeds`).

Both the wikilink render branch (in `links_render.go`) and the inline-link render branch grow an anchor-resolution check whenever an anchor component is present:

- File resolves *and* anchor resolves → render as normal link. Href carries the anchor so the follow path can recover it.
- File resolves *but* anchor doesn't → render with the existing broken-style SGR and `?` suffix; the broken count increments.
- File doesn't resolve → existing broken handling (unchanged).

Broken-link counting reuses the same counter the renderer already feeds to the footer tally. No new field.

### `internal/tui` — follow path

The wikilink follow path and the inline-link follow path both look up the anchor's destination line before `navigateTo`:

```go
if w.Heading != "" || w.Block != "" {
    if line, ok := m.vault.ResolveAnchor(path, w.Heading, w.Block); ok {
        m.pendingPreselectRange = &markdown.LineRange{Start: line, End: line}
    }
}
m.navigateTo(path)
```

For standard inline links, `l.Resolved.Anchor` is parsed: a leading `^` means block, otherwise heading. The existing markdown scroll-to-line plumbing (added for full-text search Enter) handles the rest of the navigation.

The `anchor navigation not implemented: #...` footer status is removed; anchored navigation now works.

## Data flow on follow

```
Enter on [[Note#^foo]]
  → wikilink follow path
  → vault.Resolve(fromFile, "Note", "", "foo")    → path
  → vault.ResolveAnchor(path, "", "foo")          → line
  → m.pendingPreselectRange = {line, line}
  → m.navigateTo(path)
  → refreshContent renders → GotoTop → scrollToLine(line)
```

Standard inline link `[text](note.md#heading)`:

```
Enter on the link
  → followLink
  → l.Resolved.Anchor = "heading"
  → vault.ResolveAnchor(l.Resolved.Target, "heading", "")  → line
  → m.pendingPreselectRange = {line, line}
  → m.navigateTo(target)
```

## Error handling and edge cases

- **Block marker inside fenced code block** — skipped. The preprocess state machine and the AST walker both track fence state. Tested.
- **Duplicate block ids in the same file** — first occurrence wins. A `warn` diagnostic is emitted via the existing `Diagnostics` stream with file + both line numbers. Surfaced in the footer transient and the `^l` log modal.
- **Anchor that looks like a line-range fragment (`#L10`, `#L10-L20`)** — `parseLineFragment` already claims these and returns a `LineRange`. The anchor branch only runs when `LineRange == nil`. No change.
- **Heading text with special chars** — `slugify` already handles this for existing `path + "#" + slugify(heading)` href construction in `links_render.go`. Same rule applies on both sides.
- **CLI-opened code file with `^foo` in its text** — code files don't go through the markdown renderer, so the marker renders literally. Block refs are a markdown-vault feature; this is acceptable.
- **Empty anchor (`[[Note#]]` or `[[Note#^]]`)** — parser treats the trailing component as empty; `ResolveAnchor("", "")` returns `(0, false)`. Renders as if no anchor was present (file resolves → normal link, no scroll).
- **Anchor on a self-link (`[[#^foo]]`)** — out of scope. Same-document anchors don't currently follow even for `#heading`; treating block ids in the same-document case is a separate concern (would need `LinkAnchor` follow support, which is also `not implemented` today).

## Testing

- `internal/vault/anchors_test.go` (new):
  - Heading extraction with various slug-affecting characters.
  - Block extraction at end of paragraph, list item, blockquote, fenced code block (the last is ignored — code-fenced markers don't count).
  - Duplicate block id → first wins + diagnostic emitted.
  - `ResolveAnchor` with block-and-heading both set → block wins.
- `internal/vault/extract_test.go` (extend): refresh path keeps anchor maps consistent after `RefreshFile`.
- `internal/markdown/links_render_test.go` (extend):
  - Anchor-broken wikilink renders broken-style and bumps broken count.
  - Anchor-resolved wikilink renders normal style.
  - `preprocessBlockMarkers` strips trailing `^id` outside fences but not inside.
- `internal/tui/model_test.go` (extend):
  - Following `[[Note#Heading]]` sets `pendingPreselectRange` to the heading line.
  - Following `[[Note#^foo]]` sets it to the block line.
  - Following with broken anchor still navigates (file exists) and source page shows broken count incremented; no preselect set.
  - Standard inline link `[text](note.md#heading)` scrolls to the heading line.

## Rollout

Single PR off `block-references`. No flag-gating — the existing anchor footer message is replaced wholesale by working navigation, and broken-anchor counting strictly increases the broken tally (it doesn't reclassify previously-good links). Tests cover both the resolution path and the broken-anchor render path.

## Open questions / accepted risks

- **Slug duplication.** Copying `slugify` to `vault` means two source-of-truth-ish definitions. Mitigation: a tiny `internal/anchor` package could host it later if a third caller appears; for now the cost of the cycle break is one duplicated 15-line function. Acceptable.
- **Block-marker syntax is a strict regex.** Obsidian accepts `^block-id` with alphanumerics + hyphens. We don't accept underscores or unicode. If a real-world vault has fancier ids, we'll extend the regex; not worth speculating now.
- **Anchor extraction reads the raw source, not the rendered output.** A `^foo` inside an HTML block, a setext heading underline, or other markdown corners may behave subtly differently than the goldmark AST suggests. Mitigation: the walker is AST-driven (not regex-on-source), so block kind is authoritative; only the trailing-marker recognition is regex-based, and it runs over the AST node's text content.
