# `internal/tui`

Bubble Tea Model that wires the directory tree, the markdown viewport, and the navigation history into the two-pane UI. The only package that knows about the terminal.

See also: [architecture overview](../architecture.md), [`internal/tree`](tree.md), [`internal/markdown`](markdown.md), [`internal/nav`](nav.md), [link-following plan](../link-following.md).

## Purpose

Implement the `tea.Model` interface (`Init` / `Update` / `View`) on top of the lower-layer packages. Manage focus between two panes, dispatch keystrokes, render the styled output Lip Gloss assembles into a frame.

## Types

```go
type Model struct {
    root        string
    rootNode    *tree.Node
    flatTree    []treeRow
    treeCursor  int

    viewport    viewport.Model
    renderer    *markdown.Renderer

    history     *nav.History
    focus       focus            // focusTree or focusContent

    links       []markdown.Link
    linkCursor  int              // -1 when no link selected

    width, height int
    keys          keyMap
    status        string
}

type treeRow struct {
    node  *tree.Node
    depth int
}
```

## Public surface

- `New(root, initialFile string) (Model, error)` — only constructor. `cmd/hypogeum` is the only caller.

Everything else is package-private. The Bubble Tea runtime drives the model through the `tea.Model` interface.

## Key invariants

- **The tree is flattened once.** `New` builds `[]treeRow` in dependency-of-cursor order (depth-first, dirs first). Cursor moves are O(1) index updates, not tree walks. Don't re-walk on keystrokes.
- **Auto-open is top-level only.** When `initialFile == ""`, `firstTopLevelFile` picks the first non-directory child of the root. *Don't* descend into subdirectories — earlier versions did, and the result was landing on the deepest leaf alphabetically because directories sort first. ([model.go:319](../../internal/tui/model.go))
- **Resize rebuilds the renderer.** `WindowSizeMsg` recreates `markdown.Renderer` at the new wrap width and re-renders the current file. Anything that changes content width must do the same.
- **`refreshContent` resets the link cursor to `-1`.** History navigation, file open, and resize all go through it. The link cursor is per-document; it doesn't survive a navigation.
- **Link bindings are content-pane scoped.** `n`/`p`/`Esc` and link-aware `Enter` only fire when `focus == focusContent`. The tree pane's bindings are unaffected. Full state model: [[link-cursor]].

## Key dispatch shape

```
Update(KeyMsg)
  ├── global: Quit, FocusTog, Back, Forward
  └── per-focus:
       ├── focusTree    → handleTreeKey   (Up/Down/Enter)
       └── focusContent → handleContentKey
            ├── NextLink (n)   → cycleLink(+1) + scrollToLink
            ├── PrevLink (p)   → cycleLink(-1) + scrollToLink
            ├── ClearLink (Esc) → linkCursor = -1
            ├── Open (Enter, when a link is selected) → followLink
            └── otherwise      → viewport.Update(msg)  // scrolling
```

`followLink` switches on `Resolved.Kind`:

- **`LinkLocalFile`** — `openFile(target)` plus `selectInTree(target)`. Records history; moves the tree cursor if the path is in the tree.
- **`LinkExternal`** — Status bar: `"external link not opened: <href>"`. Phase 3 will hand off to `xdg-open` / `open` after a confirm flow.
- **`LinkAnchor`** — Status bar: `"anchor navigation not implemented"`. Phase 2 will resolve to a heading row.
- **`LinkInvalid`** — Status bar: `"unrecognized link"`.

## Why `Model` holds both `links` and `linkCursor`

`links` is the document's link list, refreshed every render. `linkCursor` is the user's selection within it. Resetting on refresh keeps the two consistent — a link list from a document the user is no longer viewing would create a footer pointing at a dead row. Pair them or accept stale UI.

## Footer rendering

`renderFooter` always shows the current file path (relative to the tree root) plus the help string. When a link is selected, it prepends `"→ "` and appends `[k/n] <target>`. The marker constant (`linkFooterMarker`) is package-public for tests to assert on.

## Backlinks and modal surfaces

The TUI hosts three additional surfaces beyond the two-pane core: the persistent backlinks pane (`b`), the backlinks modal (`B`), and the log viewer modal (`?`). They share input rules, geometry, and a single `prevFocus` slot. Each has its own concept doc:

- [[modal-geometry]] — single-modal invariant, layout recompute on open, auto-collapse below height 20, `Esc` priority chain.
- [[diagnostics]] — the warn/error stream that feeds the footer transient and the `?` modal.
- [[return-cursor]] — single-slot cursor restoration that survives `Enter`-follow → `h`-back round trips on backlinks.

The vault that powers backlinks is documented at [[vault-index]].
