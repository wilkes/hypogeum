# Architecture

Top-level map of how `hypogeum` fits together. Per-package detail lives in `packages/`; this doc stays at the level of "which package owns what" and "how a keystroke flows."

See also: [docs index](index.md), [link-following plan](link-following.md).

## Package layering

```
cmd/hypogeum               (entrypoint — argv → tui.New → tea.NewProgram)
        │
        ▼
internal/tui               (Bubble Tea Model, the only package that knows about the UI)
   │      │      │      │      │
   ▼      ▼      ▼      ▼      ▼
   tree   markdown   nav   watch   vault   (lower layers)
              │                       │
              └───────► wikilink ◄────┘    (shared parser, no other deps)
```

- [`internal/tree`](packages/tree.md) walks the filesystem and produces a `*Node` tree of markdown files.
- [`internal/markdown`](packages/markdown.md) renders markdown via Glamour and resolves links.
- [`internal/nav`](packages/nav.md) is a back/forward stack of opaque path strings.
- [`internal/watch`](packages/watch.md) wraps fsnotify and emits debounced, markdown-aware events.
- `internal/vault` builds the wikilink/backlink index over the markdown set.
- `internal/wikilink` parses `[[Name#Heading^Block|Alias]]` bodies; shared by `vault` and `markdown` so neither package re-implements it.
- [`internal/tui`](packages/tui.md) is the only package that imports the other layers.

The lower layers know nothing about Bubble Tea or terminals; they're testable as pure functions.

## Data flow on a keystroke

1. Bubble Tea delivers a `tea.KeyMsg` to `Model.Update`.
2. Global bindings (quit, focus toggle, back/forward) match first.
3. Modal-toggle keys (`^b`, `^p`, `B`, `^l`, `?`) route to `openModalWith(kind, prepare)`.
4. If a modal is open, the keystroke is dispatched to that modal's handler — e.g. `modalTree` updates `m.tree.cursor` or calls `openFile` (closing itself on a file Enter).
5. Otherwise, dispatch by focus:
   - `focusContent` → `handleContentKey` cycles `m.content.linkCursor`, follows a link, clears selection, or falls through to the viewport's own scrolling bindings.
   - `focusBacklinks` → `handleBacklinksKey` moves `m.backlinks.cursor` and follows backlinks via `Enter`.
6. `openFile(path)` records the visit in `nav.History` and calls `refreshContent`.
7. `refreshContent(path)` reads the file, calls `markdown.RenderWithLinks`, sets the viewport content, and stores the new link list. The link cursor resets to `-1`.
8. `View()` renders the content viewport and optional backlinks pane, then overlays any open modal — `zone.Scan` runs last so BubbleZone records the modal's row zones for mouse hit-testing.

The keystroke path is synchronous — no goroutines, no commands waited on — because every action is local I/O fast enough to inline. The one exception is the watcher: `Init()` returns a `tea.Cmd` that blocks on `internal/watch`'s event channel, surfacing each debounced event as `fsEventMsg`. `Update` rebuilds the tree (on `StructureChanged`) or refreshes the open file (on `FileModified`) while preserving cursor and scroll position, then re-issues the wait command to keep listening.

## Why this shape

Three trade-offs worth knowing because they look like accidents otherwise:

**Pre-flatten the tree.** `internal/tree` returns a recursive `*Node`, but `internal/tui` flattens it into a `[]treeRow` once in `New`. Cursor moves are then O(1) index updates, not tree walks. Don't add features that re-walk on every keystroke. ([model.go](../internal/tui/model.go), `flatten`)

**Re-render on resize.** Glamour's word-wrap width is baked into the renderer, not the call. `WindowSizeMsg` rebuilds *both* the plain and instrumented renderers at the new width and re-renders the current file. Anything that changes content width must do the same. ([render.go](../internal/markdown/render.go), `NewRenderer`)

**Pre-flattened tree means `selectInTree` is a linear scan.** It's fine — the tree is small, the user pressed a key, microseconds don't matter. We optimized for the *typing-speed* path (cursor up/down), not the *click-something-in-the-content-pane* path.

## Cross-cutting concerns

- **Style detection** (dark / light / no-tty) lives in `markdown`. It mirrors Glamour's auto-style so the instrumented renderer is byte-equivalent to the plain one.
- **Path resolution** lives in `markdown.ResolveLink`. Local files become absolute; anchors keep their fragment; URLs pass through with their original href.
- **History semantics** live in `nav`. Browser-style: visiting truncates forward history; visiting the same path is a no-op.
- **Hidden-entry filtering** lives in `tree`. Anything starting with `.` is skipped — `.git`, dotfile note dirs, etc.
- **Empty-directory pruning** lives in `tree`. A directory with no `.md` anywhere underneath doesn't appear in the tree at all.
- **Cross-cutting concepts** that span multiple packages or specs (the sentinel-render trick, the vault index, diagnostics, modal geometry, the return cursor, the link cursor) live in [`docs/concepts/`](concepts/). The docs index lists them; package docs and specs link to them by name.

When you add a new concern, decide its owner first. The packages are small enough that the right home is usually obvious; pick wrong and the layering inverts.

## Where to start reading

If you want to understand the whole codebase, read in dependency order — bottom up:

1. [`internal/nav`](packages/nav.md) — pure stack, sets the vocabulary for "history."
2. `internal/wikilink` — single file, the shared `[[...]]` body parser.
3. [`internal/tree`](packages/tree.md) — filesystem walker, no UI, easy to picture.
4. [`internal/markdown`](packages/markdown.md) — render + link resolution + the sentinel trick.
5. `internal/vault` — wikilink/backlink index, split across `vault.go`/`extract.go`/`backlink.go`/`resolver.go`.
6. [`internal/watch`](packages/watch.md) — fsnotify wrapper; `classify.go` is pure, `debounce.go` debounces, `watch.go` runs the loop.
7. [`internal/tui`](packages/tui.md) — biggest package; `Model` decomposes into four sub-structs, dispatch helpers in `dispatch.go`.
8. [`cmd/hypogeum/main.go`](../cmd/hypogeum/main.go) — argv parsing and a `tea.NewProgram` call.
