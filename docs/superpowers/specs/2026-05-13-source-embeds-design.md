# Source-file embeds and line-range links

**Status:** shipped on 2026-05-13 (PR #30). Follow-up fixes in [source-embeds-followups](2026-05-13-source-embeds-followups-design.md).

## Summary

Let a markdown file embed a slice of a source file inline (`![[main.go#L42-L58]]`) and link to a specific line range in a source file (`[parser](main.go#L42-L58)`). Embeds re-render automatically when the source changes on disk; `Enter` on an embed or a range link opens the source, scrolled to and highlighting the referenced range.

## Motivation

Hypogeum already renders source files via `internal/code`, but a markdown document referring to "the parser" can only link to the whole file. Authors want to surface the specific lines they're talking about right inside their notes — and to follow that reference into the source without losing their place. Common in design docs, code-reviews-as-notes, and engineering knowledge bases.

## Grammar

Two new tokens (one embed, one range-link). Both reuse the GitHub-style `#L<n>-L<n>` line-range fragment so it's the same vocabulary readers already know.

### Embed token
```
![[<path>]]                       — embed whole file
![[<path>#L<n>]]                  — embed single line
![[<path>#L<a>-L<b>]]             — embed line range, inclusive
![[<path>#L<a>-L<b>+<c>]]         — embed range with <c> context lines before and after
```

`<path>` is resolved relative to the markdown file (same rule as ordinary markdown links). Paths may escape the markdown root (`../../code/main.go`).

### Range-link token
```
[label](<path>#L<n>)              — link to single line
[label](<path>#L<a>-L<b>)         — link to line range
```

Renders as an ordinary link in prose; on `Enter` opens the source file scrolled to and highlighting the range. Without a `#L…` fragment, behavior is unchanged from today.

### Drift semantics

Line numbers are *literal*. If the user edits the source file and inserts five lines at the top, an embed of `#L42-L58` will silently shift to whatever those line numbers point at now. This matches GitHub's behavior for line-range URLs and keeps the grammar minimal. Named anchors (refactor-survivable embeds) are explicitly out of scope for v1.

## Architecture

A new `internal/embed` package owns parsing, file slicing, and fence formatting. Existing packages get small, additive changes:

```
internal/embed/                        (NEW)
├── parse.go                           // body string → Embed{Path, Range, ContextLines}
├── slice.go                           // SliceFile(absPath, range, ctx) → ([]string, startLine, err)
├── fence.go                           // RenderToFence(absPath, lines, startLine, displayRange) → string
└── lang.go                            // LanguageFromPath: ext → Glamour/Chroma language ID

internal/markdown/links.go             (CHANGE)
   ResolvedLink gains Range *LineRange; ResolveLink parses #L<n>-L<n>.

internal/markdown/links_render.go      (CHANGE)
   New preprocessEmbeds pass runs *before* preprocessWikilinks.
   Rewrites ![[…]] tokens into fenced code blocks; records the embed
   source paths into a per-render slice the caller can read.

internal/markdown/render.go            (CHANGE)
   RenderWithLinks returns the embed-dependency slice alongside the
   existing links slice. The embed-dep slice is distinct from the
   link slice: embeds appear in both (as a navigable link and as a
   live-sync dependency), but the link slice carries display state
   while the dep slice carries only paths.

internal/code/render.go                (CHANGE)
   Render takes a RenderOptions parameter; Options.Highlight *LineRange
   marks the gutter for those lines in reverse-video.

internal/watch/watch.go                (CHANGE)
   New AddPath(dir string) method; idempotent wrapper over fsw.Add.

internal/tui/                          (CHANGE)
   contentUIState gains embedDeps map[string]struct{}.
   refreshContent persists embed deps from the renderer return value
   and AddPath()s any new dependency directories.
   handleFSEvent FileModified branch checks the open file's embedDeps
   in addition to the open path.
```

The layering rule (lower packages know nothing about the TUI) is preserved: `internal/embed` is pure, `internal/markdown` orchestrates the preprocess, `internal/tui` is the only layer that talks to the watcher about dependency directories.

## Render pipeline

Inside `markdown.Renderer.RenderWithLinks`:

1. **`preprocessEmbeds(src, base)`** (new) — regex-scans for `![[…]]` outside code fences, parses each via `embed.ParseEmbedToken`, slices the source file, formats as a fenced code block with a provenance header, and substitutes back into the source. Tracks `[]EmbedRef{Path, Range}` for the caller.
2. **`preprocessWikilinks(src)`** — runs unchanged. Order matters: embeds are consumed first so the wikilink regex doesn't match them.
3. **`instrumented.Render(src)`** — Glamour renders the now-fully-markdown source, including the injected fenced blocks (Chroma-styled, dark/light palette already wired up).
4. **`stripSentinels`** — unchanged. Embeds have no link sentinels in their body.

### Fence body shape

For `![[main.go#L42-L58]]` (with `main.go` containing those lines):

````markdown
> `main.go:42–58`
```go
 42 │ func parse(s string) Tree {
 43 │     // build AST
  …
 58 │ }
```
````

- **Provenance header** is a blockquote line above the fence so Glamour styles it faintly. The separator between line numbers is the en-dash `–` (U+2013), not a hyphen.
- **Gutter** is literal text inside the fence body (`42 │ …`). Chroma styles it as source code, but the box-drawing character makes it read as a gutter to humans. This is the deliberate Approach-A simplification: we do *not* use `internal/code.Renderer`'s separate gutter pipeline. Trade-off: embeds don't get the same soft-wrap behavior as standalone code files (long lines truncate at Glamour's fence width). For most code excerpts this is acceptable.
- **Language tag** comes from `embed.LanguageFromPath`. Unknown extensions render as a plain (no-language) fence.
- **Context lines** (the `+<c>` form) appear in the fence body but with a faint marker (rendered with a leading `~` in place of the gutter line number) so the reader can see what's primary vs. surrounding context. The header always shows the *primary* range, not the context-expanded one.

### Error cases

Each renders as a single blockquote warning in place of the embed; the rest of the markdown still renders normally.

| Condition | Substitution |
|---|---|
| Source file missing | `> ⚠ main.go: file not found` |
| Source file binary | `> ⚠ main.go: binary file, not embedded` |
| Source file > 5 MB | `> ⚠ main.go: file too large to embed` |
| Range past EOF | end truncated to last line; header softened: `main.go:42–58 (file ends at line 50)`. Start past EOF emits the `invalid range` warning instead. |
| Empty / inverted range | `> ⚠ main.go#L…: invalid range` |
| Path unresolvable | `> ⚠ main.go: invalid path` |

The 5 MB cap mirrors `internal/code.Renderer.Render`'s existing limit. Binary detection uses the same "NUL in first 8 KB" heuristic.

## Live-sync

A markdown file's render produces a set of *embed dependencies* — absolute paths of every source file successfully sliced into its output. The TUI persists this set on `contentUIState`:

```go
embedDeps map[string]struct{}
```

The `FileModified` branch of `handleFSEvent` consults this set in addition to the existing "is this the open file?" check:

```go
case watch.FileModified:
    cur := m.history.Current()
    if cur == "" { return }
    for _, p := range ev.Paths {
        if p == cur {
            offset := m.content.viewport.YOffset
            m.refreshContent(cur)
            m.content.viewport.SetYOffset(offset)
            return
        }
        if _, ok := m.content.embedDeps[p]; ok {
            offset := m.content.viewport.YOffset
            m.refreshContent(cur)
            m.content.viewport.SetYOffset(offset)
            return
        }
    }
```

`refreshContent` rebuilds `embedDeps` as a side effect of the next render, so opening a different markdown clears the prior file's dependencies automatically.

### Watching out-of-tree sources

`internal/watch.Watcher` watches directories under the markdown root. An embed pointing at `../../code/main.go` may live outside this set. `refreshContent` walks the new dependency directories and calls `m.watcher.AddPath(dir)` for each new parent directory. `AddPath` is a thin wrapper:

```go
func (w *Watcher) AddPath(dir string) error {
    if w == nil || w.fsw == nil { return nil }
    return w.fsw.Add(dir)
}
```

Idempotent (fsnotify's `Add` no-ops on a duplicate). We do not unwatch on the next render — keeping a stale watch costs almost nothing and avoids churn when the user navigates among files that reference shared source directories.

### Cycles

A markdown file may embed another markdown file (`![[note.md#L1-L10]]`). The embedded content renders as plain text inside a markdown fenced block — it is *not* recursively parsed as markdown. So no cycles are possible.

## Navigation

### Embeds join the link cycler

Every embed token contributes one entry to the markdown link list returned by `RenderWithLinks`. The entry's `Resolved.Kind` is `LinkLocalFile`, `Target` is the absolute source path, `Range` is the embed range. The `Row` is the line of the rendered provenance header (visible, unambiguous).

`n` / `p` cycle through embeds the same as ordinary links. `Enter` follows them via the existing `followLink` path.

### Range links

`ResolvedLink.Range *LineRange` is populated for `[t](path#L10-L20)` style links during `ResolveLink`. The fragment parser checks for `L\d+(-L\d+)?` *before* falling back to anchor handling. Other anchors continue to work unchanged.

### Opening the source

When `Enter` follows a link whose `Range != nil`:

1. `openFile(path)` records history and calls `refreshContent`.
2. `refreshContent` detects the new file is non-markdown and dispatches to `code.Renderer.Render`. We pass `RenderOptions{Highlight: link.Resolved.Range}` so the gutter for those lines renders in reverse-video.
3. After the viewport is populated, `m.scrollToLine(range.Start)` positions the range about 25% from the top (matching existing backlink-follow behavior).

### Range highlight lifecycle

The highlight persists across scrolling so the user can use it as a visual anchor. It clears on:
- `Esc` (additional case in the existing Esc cascade)
- Opening any other file
- Following a different range link

### History and pre-select

Navigation goes through `openFile`, recording an ordinary history entry. Pressing `h` (back) returns to the markdown. The existing `pendingPreselectTarget` plumbing already restores the cursor to the link that originated the jump; we extend the match condition: a returning link is "the one we came from" if its `Resolved.Target` matches *and* its `Resolved.Range` matches (so two range links into the same file disambiguate correctly).

## Testing

Tests live next to the code they test, mirroring the existing pattern. No TTY needed for any test.

| File | Coverage |
|---|---|
| `internal/embed/parse_test.go` | All legal forms (`![[p]]`, `#L5`, `#L5-L10`, `#L5-L10+3`), illegal forms (inverted, non-numeric, missing path), whole-file form. |
| `internal/embed/slice_test.go` | Range past EOF, range starting at 0 (rejected), single-line, context-line padding clamped at file bounds, binary-file rejection, oversize rejection. |
| `internal/embed/fence_test.go` | Language inference, gutter line-number right-alignment, context-line marker distinct from primary, header text. |
| `internal/markdown/embed_render_test.go` | `RenderWithLinks` on source with an embed produces the fenced block in output, returns one Link entry per embed with Range populated, returns embed-dep slice. |
| `internal/markdown/links_test.go` (extended) | `ResolveLink` handles `path#L10-L20` and `path#L10`; distinguishes line-range from heading anchor. |
| `internal/code/render_test.go` (extended) | `RenderOptions.Highlight` reverse-videos the requested gutter lines and only those. Nil Highlight is a no-op. |
| `internal/watch/watch_test.go` (extended) | `AddPath` is idempotent and accepts out-of-root directories. |
| `internal/tui/model_test.go` (extended) | Three scenarios: opening a markdown with an embed populates `embedDeps`; `FileModified` on an embedded source triggers `refreshContent` for the open markdown; `Enter` on an embed link opens the source scrolled-to-line with highlight active. |

## Out of scope

- **Named-anchor embeds** (`![[file#parse-loop]]` driven by source-side comments). Worth considering after we see how line-range embeds actually get used.
- **Recursive markdown embedding** (an embedded `.md` slice rendered as markdown rather than as raw source). Adds cycle concerns and a different rendering mode.
- **Diff-style embed previews** when the source has changed since the embed was written. Requires us to track the embed's "expected content" — a different feature.
- **Multi-range embeds** (`![[file#L1-L3,L10-L12]]`). Easy to add later if requested; not in v1.
- **Source-side click-through to the embedder** (jumping from the source file back to the markdown that embeds it). The vault index already has the data; a reverse-lookup feature like backlinks would be additive.

## Rollout

Single PR onto `main`, merge with `gh pr merge --merge` per repo convention. No flags, no migrations. CLAUDE.md gets a Gotchas entry describing the embed grammar, drift semantics, and the watcher's out-of-tree AddPath behavior. `docs/index.md` learns a link to this design.
