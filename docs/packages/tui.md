# `internal/tui`

Bubble Tea Model that wires the directory tree, the markdown viewport, and the navigation history into the content-first UI. The only package that knows about the terminal. The tree lives in a modal (`^b`); content fills the screen, with a persistent backlinks pane that can open below it (`b`).

See also: [architecture overview](../architecture.md), [`internal/tree`](tree.md), [`internal/markdown`](markdown.md), [`internal/nav`](nav.md), [link-following plan](../link-following.md).

## Purpose

Implement the `tea.Model` interface (`Init` / `Update` / `View`) on top of the lower-layer packages. Manage focus between content and the optional backlinks pane, dispatch keystrokes, render the styled output Lip Gloss assembles into a frame. The tree, file finder, backlinks list, log viewer, and help cheat sheet share a single modal surface — at most one is open at a time.

## Types

State is grouped into four named sub-structs so `Model` reads as a composition rather than a flat bag of fields. Each lives next to the file that owns its behavior:

```go
type Model struct {
    root     string
    rootNode *tree.Node

    tree      treeUIState      // flat, cursor, vp, expanded
    content   contentUIState   // viewport, renderer, links, linkCursor
    backlinks backlinksUIState // open, vp, cursor, items, returnCursor
    modals    modalUIState     // kind, vp, picker, prevFocus

    history *nav.History
    focus   focus            // focusContent, focusBacklinks
    width, height int
    keys          keyMap
    status        string

    watcher *watch.Watcher
    vault   *vault.Vault
    diag    *diagnostics
}

type treeRow struct {
    node  *tree.Node
    depth int
}
```

`treeUIState`, `contentUIState`, `backlinksUIState`, and `modalUIState` are package-private. Field access is `m.tree.cursor`, `m.content.linkCursor`, `m.backlinks.items`, `m.modals.kind`, etc.

## Public surface

- `New(root, initialFile string) (Model, error)` — only constructor. `cmd/hypogeum` is the only caller.

Everything else is package-private. The Bubble Tea runtime drives the model through the `tea.Model` interface.

## Key invariants

- **The tree is flattened once.** `New` builds `[]treeRow` in dependency-of-cursor order (depth-first, dirs first). Cursor moves are O(1) index updates, not tree walks. Don't re-walk on keystrokes.
- **Auto-open is top-level only.** When `initialFile == ""`, `firstTopLevelFile` picks the first non-directory child of the root. *Don't* descend into subdirectories — earlier versions did, and the result was landing on the deepest leaf alphabetically because directories sort first. ([model.go:319](../../internal/tui/model.go))
- **Resize rebuilds the renderer.** `WindowSizeMsg` recreates `markdown.Renderer` at the new wrap width and re-renders the current file. Anything that changes content width must do the same.
- **`refreshContent` resets the link cursor to `-1`, except when `m.pendingPreselectTarget` is set.** Most refreshes (file open, resize, watcher events) reset the cursor. But navigation sources — `followBacklink`, Back (`h`), Forward (`l`) — set `m.pendingPreselectTarget` to the path being left, and `refreshContent` consumes it: scans the new document's link list for the first `LinkLocalFile` whose `Resolved.Target` matches and selects it. The field is single-shot and is cleared on every `refreshContent`. Full rules: [[link-cursor]].
- **Link bindings are content-pane scoped.** `n`/`p`/`Esc` and link-aware `Enter` only fire when `focus == focusContent` and no modal is open. The tree-modal's own bindings (cursor / Space / Enter) are unaffected. Full state model: [[link-cursor]].

## Key dispatch shape

```
Update(KeyMsg)
  ├── global: Quit, FocusTog, Back, Forward
  ├── modal-toggle (B, ^l, ?, ^p, ^b) → openModalWith(kind, prepare)
  ├── modal-open: route to modalTree / modalPicker / modalBacklinks / modalLogs / modalHelp
  │    └── modalTree: Up/Down/Space/Enter/Esc — Enter on a file closes the modal
  └── per-focus (modal closed):
       ├── focusContent   → handleContentKey
       │    ├── NextLink (n)   → cycleLink(+1) + scrollToLink
       │    ├── PrevLink (p)   → cycleLink(-1) + scrollToLink
       │    ├── ClearLink (Esc) → content.linkCursor = -1
       │    ├── Open (Enter, when a link is selected) → followLink
       │    └── otherwise      → viewport.Update(msg)  // scrolling
       └── focusBacklinks → handleBacklinksKey
            ├── Up/Down → cursorMoveAndRefresh + viewportClamp
            └── Enter   → followBacklink (records returnCursor)
```

The cursor-move-and-refresh pattern and the viewport-clamp pattern are extracted into [`dispatch.go`](../../internal/tui/dispatch.go) and shared by the tree, picker, backlinks pane, and backlinks modal.

`followLink` switches on `Resolved.Kind`:

- **`LinkLocalFile`** — `openFile(target)` plus `selectInTree(target)`. Records history; moves the tree cursor if the path is in the tree.
- **`LinkExternal`** — Status bar: `"external link not opened: <href>"`. Phase 3 will hand off to `xdg-open` / `open` after a confirm flow.
- **`LinkAnchor`** — Status bar: `"anchor navigation not implemented"`. Phase 2 will resolve to a heading row.
- **`LinkInvalid`** — Status bar: `"unrecognized link"`.

## Why `contentUIState` holds both `links` and `linkCursor`

`content.links` is the document's link list, refreshed every render. `content.linkCursor` is the user's selection within it. Refreshing them together keeps the two consistent — a link list from a document the user is no longer viewing would create a footer pointing at a dead row. The default is `linkCursor = -1` after a refresh; the `pendingPreselectTarget` carry-over is the only way to land on a non-default cursor and lives outside `contentUIState` so that the consistency contract between `links` and `linkCursor` stays simple.

## Footer rendering

`renderFooter` always shows the current file path (relative to the tree root) plus the help string. When a link is selected, it prepends `"→ "` and appends `[k/n] <target>`. The marker constant (`linkFooterMarker`) is package-public for tests to assert on.

### Flat file finder (`^p`)

The `^p` finder is a modal (`modalKind == modalPicker`) over every markdown file in the vault. On open, the model calls `recent.Rank(allVaultMarkdownPaths())` to get a `[]recent.Ranked` ordered by the hybrid recency score; that slice is stored on `pickerState.all` and not refreshed mid-modal.

The textinput is focused immediately. Each keystroke flows through `textinput.Update`; on value change, `refilter` runs `sahilm/fuzzy.Find` over a lowercased copy of the paths and re-sorts by match score (descending) with the source-order index as a stable tiebreaker — the latter preserves the recency order within a score tier. Rendered rows highlight matched bytes in bold cyan; selected row is reverse-video.

`^j`/`^k` (and `↑`/`↓`) move the cursor; `j`/`k` are typed characters now. `Esc` clears a non-empty query first, closes on the second press. `Enter` opens the selected row through `m.openFile`, which records a visit through `recent.Record`.

Specs: [unified-finder-recency](../superpowers/specs/2026-05-12-unified-finder-recency-design.md), [finder-fuzzy-filter](../superpowers/specs/2026-05-12-finder-fuzzy-filter-design.md).

## Backlinks and modal surfaces

The TUI hosts these surfaces beyond the content viewport: the persistent backlinks pane (`b`), the directory tree modal (`^b`), the file finder modal (`^p`), the backlinks modal (`B`), the log viewer modal (`^l`), and the help modal (`?`). They share input rules, geometry, and a single `modals.prevFocus` slot. Each has its own concept doc:

- [[modal-geometry]] — single-modal invariant, layout recompute on open, auto-collapse below height 20, `Esc` priority chain. `?` is anchored (no swap); `B` and `^l` swap with each other.
- [[diagnostics]] — the warn/error stream that feeds the footer transient and the `^l` modal.
- [[return-cursor]] — single-slot cursor restoration that survives `Enter`-follow → `h`-back round trips on backlinks.

The vault that powers backlinks is documented at [[vault-index]].
