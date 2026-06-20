# `internal/tui`

Bubble Tea Model that wires the directory tree, the markdown viewport, and the navigation history into the content-first UI. The only package that knows about the terminal. Content fills the screen; every other surface (tree, file finder, full-text search, backlinks, recently-opened, logs, help) is a modal — at most one open at a time.

See also: [architecture overview](../architecture.md), [`internal/tree`](tree.md), [`internal/markdown`](markdown.md), [`internal/nav`](nav.md), [`internal/recent`](recent.md), [link-following plan](../link-following.md).

## Purpose

Implement the `tea.Model` interface (`Init` / `Update` / `View`) on top of the lower-layer packages. Dispatch keystrokes, render the styled output Lip Gloss assembles into a frame. The tree, file finder, full-text search, backlinks list, recently-opened list, log viewer, and help cheat sheet share a single modal surface — at most one is open at a time. Focus is content-only; the `focus` type is a single-value enum that exists to keep `modalUIState.prevFocus` save/restore symmetric.

## Types

State is grouped into four named sub-structs so `Model` reads as a composition rather than a flat bag of fields. Each lives next to the file that owns its behavior:

```go
type Model struct {
    root     string
    rootNode *tree.Node

    tree       treeUIState      // flat, cursor, vp, expanded
    content    contentUIState   // viewport, renderer, links, linkCursor, selection
    backlinks  backlinksUIState // cursor, items, returnCursor
    recentList recentUIState    // cursor, items (the `r` modal)
    modals     modalUIState     // kind, vp, picker, search, prevFocus

    history *nav.History
    focus   focus            // focusContent (single-value enum)

    width, height int
    keys          keyMap

    currentPath   string // absolute path of the displayed file/view
    footerMessage string // last error/transient; takes the footer slot when set

    watcher *watch.Watcher
    vault   *vault.Vault
    recent  *recent.Store    // persisted visit history
    diag    *diagnostics

    pending pendingNav        // in-flight nav intent (preselect + external URL)

    openExternal    externalOpener  // injected for tests
    copyToClipboard clipboardWriter // injected for tests
}

type treeRow struct {
    node  *tree.Node
    depth int
}
```

`treeUIState`, `contentUIState`, `backlinksUIState`, `recentUIState`, and `modalUIState` are package-private. Field access is `m.tree.cursor`, `m.content.linkCursor`, `m.backlinks.items`, `m.modals.kind`, `m.modals.search`, etc. `modalUIState` holds both the file picker (`picker`) and the full-text search modal state (`search searchState`).

## Public surface

- `New(root, initialFile string) (Model, error)` — only constructor. `cmd/hypogeum` is the only caller.

Everything else is package-private. The Bubble Tea runtime drives the model through the `tea.Model` interface.

## Key invariants

- **The tree is flattened once.** `New` builds `[]treeRow` in dependency-of-cursor order (depth-first, dirs first). Cursor moves are O(1) index updates, not tree walks. Don't re-walk on keystrokes.
- **Auto-open is top-level only.** When `initialFile == ""`, `firstTopLevelFile` ([tree.go](../../internal/tui/tree.go)) scans the root's direct children in three passes: the first whose basename stem is `index` (case-insensitive), else the first whose stem is `readme`, else the first non-directory child (the historical fallback). It *never* descends into subdirectories — earlier versions did, and the result was landing on the deepest leaf alphabetically because directories sort first.
- **Resize rebuilds the renderer.** `WindowSizeMsg` recreates `markdown.Renderer` at the new wrap width and re-renders the current file. Anything that changes content width must do the same.
- **`refreshContent` resets the link cursor to `-1`, except when `m.pending.preselectTarget` is set.** Most refreshes (file open, resize, watcher events) reset the cursor. But navigation sources — `followBacklink`, Back (`h`), Forward (`l`) — set `m.pending.preselectTarget` (a field of `pendingNav`) to the path being left, and `refreshContent` consumes it: scans the new document's link list for the first `LinkLocalFile` whose `Resolved.Target` matches and selects it. The field is single-shot and is cleared on every `refreshContent`. Full rules: [[link-cursor]].
- **Link bindings are content-scoped.** `n`/`N`/`Esc` and link-aware `Enter` only fire when no modal is open. Each modal's own bindings (tree's cursor/Space/Enter, picker's text-input + ^j/^k, search's text-input + ^j/^k, backlinks' j/k/Enter, recent's j/k/Enter) take precedence. Full state model: [[link-cursor]].

- **`modalKind` is a closed enumeration** (`modal.go`): `modalNone`, `modalBacklinks`, `modalLogs`, `modalPicker`, `modalHelp`, `modalTree`, `modalSearch`, `modalRecent`. At most one is open at a time (the single-modal-swap invariant).

## Key dispatch shape

```
Update(KeyMsg)
  ├── visual-mode grab: if a keyboard selection is active → handleVisualKey
  ├── picker/search text-input grab: if open AND key is KeyRunes → handlePickerKey / handleSearchKey
  ├── modal-toggle (b, ^l, ?, ^p/o, t, /, r) → openModalWith(kind, prepare)
  ├── modal-open: route to modalTree / modalPicker / modalSearch / modalBacklinks / modalLogs / modalHelp / modalRecent
  │    └── modalTree: Up/Down/Space/Enter/Esc, ←/h/→/l collapse/expand
  ├── global: Quit, Back (h/←), Forward (l/→)
  └── modal closed → handleContentKey
       ├── NextLink (n)   → cycleLink(+1) + scrollToLink
       ├── PrevLink (N)   → cycleLink(-1) + scrollToLink
       ├── EnterVisual (v) → enterVisual (caret + selection mode)
       ├── CopyPath (y)   → copy current file's absolute path
       ├── ClearLink (Esc) → clear range-highlight, then content.linkCursor = -1
       ├── Open (Enter, when a link is selected) → followLink
       ├── Top/Bottom (g/G), HalfPageDown/Up (^d/^u) → viewport jumps
       └── otherwise      → viewport.Update(msg)  // scrolling
```

The cursor-move-and-refresh pattern and the viewport-clamp pattern are extracted into [`dispatch.go`](../../internal/tui/dispatch.go) and shared by the tree, picker, and backlinks modal.

### Content-pane keys and the render branch

Beyond link cycling, `handleContentKey` ([input.go](../../internal/tui/input.go)) handles:

- **`y` (`CopyPath`)** — copies the current file's absolute path to the clipboard via `m.copyToClipboard` and toasts the result.
- **`v` (`EnterVisual`)** — enters keyboard visual selection: a movable caret appears, `Space` (`BeginSelect`) drops the anchor, motion keys extend the span, `y` yanks it, `Esc` cancels. While active, `handleVisualKey` intercepts every keystroke before any modal toggle. The same `selection{anchor,cursor}` span machinery backs mouse drag-to-select (`handleMouse`): a left-press in the content pane arms a selection; the first motion turns it into a drag; release copies the span via OSC 52 (a no-motion release instead follows the link under the press).
- **Scroll/jump** — `g`/`G` (`Top`/`Bottom`) and `^d`/`^u` (`HalfPageDown`/`HalfPageUp`); everything else falls through to `viewport.Update`.

`refreshContent` ([content.go](../../internal/tui/content.go)) branches on file type: markdown goes through `markdown.Renderer` (`RenderWithLinks`), everything else through the [`internal/code`](../../internal/code) renderer (Chroma → 256-color ANSI → line-number gutter → soft-wrap). Code files carry no link slice, so `n`/`N`/`Enter` are natural no-ops there. Directories dispatch through a synthesized markdown listing instead.

`followLink` (`links.go`) switches on `Resolved.Kind`:

- **`LinkLocalFile`** — `navigateTo(target)` (which does `openFile` + `selectInTree`). Records history; moves the tree cursor if the path is in the tree. If `Resolved.Range` is non-nil (a `#L<n>-L<n>` fragment), `content.rangeHighlight` is set first so the destination reverse-videos those lines.
- **`LinkExternal`** — Implemented. Sets `pending.externalURL` and footer-prompts `"press Enter again to open: <href>"`. A second `Enter` exec's `m.openExternal` (default `openExternalURL` in [external.go](../../internal/tui/external.go): validates http/https only, then `open` / `xdg-open` / `cmd start` per platform via `exec.Cmd.Start()` so the browser detaches); any other keystroke cancels.
- **`LinkAnchor`** — Footer: `"anchor navigation not implemented: #<anchor>"`. Not yet built.
- **`LinkInvalid`** — Footer: `"unrecognized link: <href>"`.

## Why `contentUIState` holds both `links` and `linkCursor`

`content.links` is the document's link list, refreshed every render. `content.linkCursor` is the user's selection within it. Refreshing them together keeps the two consistent — a link list from a document the user is no longer viewing would create a footer pointing at a dead row. The default is `linkCursor = -1` after a refresh; the `pending.preselectTarget` carry-over is the only way to land on a non-default cursor and lives outside `contentUIState` (on `pendingNav`) so that the consistency contract between `links` and `linkCursor` stays simple.

## Footer rendering

`renderFooter` ([view.go](../../internal/tui/view.go)) shows the location slot — `m.currentPath` (relative to the tree root), or `m.footerMessage` when set (errors, the external-URL confirm prompt, "opened: …") — plus the help string. When a link is selected it prepends `"→ "` and appends `[<n>/<total>] <label>`. A diagnostics transient overrides the location slot; a non-zero broken-link tally appends a faint `⚠ N broken`. The marker constant (`linkFooterMarker`) is package-public for tests to assert on.

### Flat file finder (`^p` / `o`)

The finder is a modal (`modalKind == modalPicker`) over every markdown file in the vault. On open (`input.go`, in the `OpenPicker` toggle), the model calls `recent.RankByMTime(m.allVaultMarkdownPaths())` to get a `[]recent.Ranked` ordered by pure edit-recency (filesystem mtime, newest first). This ranking is **stateless** — no `recent.Store` is consulted — and the result is handed to `pickerState.reset`, not refreshed mid-modal.

The textinput is focused immediately. Each keystroke flows through `textinput.Update`; on value change, `refilter` runs `sahilm/fuzzy.Find` over a lowercased copy of the paths and re-sorts by match score (descending) with the source-order index as a stable tiebreaker — the latter preserves the recency order within a score tier. Rendered rows highlight matched bytes in bold cyan; selected row is reverse-video.

`^j`/`^k` (and `↑`/`↓`) move the cursor; `j`/`k` are typed characters now. `Esc` clears a non-empty query first, closes on the second press. `Enter` closes the modal and calls `m.navigateTo(path)`; the visit itself is recorded by `recent.Store.Record` inside `openFile` ([content.go](../../internal/tui/content.go)), reached through navigation.

Specs: [unified-finder-recency](../superpowers/specs/2026-05-12-unified-finder-recency-design.md), [finder-fuzzy-filter](../superpowers/specs/2026-05-12-finder-fuzzy-filter-design.md).

### Recently-opened modal (`r`)

`modalRecent` (`OpenRecentModal` = `r`, [keys.go](../../internal/tui/keys.go)) is a flat, visited-only list — the files the user has actually opened in hypogeum, most-recently-visited first. Implemented in [recent_modal.go](../../internal/tui/recent_modal.go) (`recentUIState`):

- `refreshRecentModal` ranks once at open via `m.recent.RankByVisit(m.allVaultMarkdownPaths())` and renders. It re-queries the store, so it runs only on open.
- `renderRecentModal` is render-only — cursor moves repaint the cached list without re-walking the vault.
- `j`/`k` (and `↑`/`↓`) move the cursor, `Enter` (`followRecent`) closes the modal and `navigateTo`s the selection, `Esc` closes.

Unlike the finder (edit-recency / mtime over the whole vault), this lists only visited files in visit order. The two orderings live in `internal/recent` and are deliberately separate — see [`internal/recent`](recent.md).

### Full-text search modal (`/`)

`modalSearch` (`OpenSearch` = `/`, [keys.go](../../internal/tui/keys.go)) scans every vault markdown file for a case-insensitive substring of the query (`searchState`; modal integration in `search.go`). On open it snapshots `m.allVaultMarkdownPaths()` into `searchState`; scans run on a debounce, each keystroke cancelling the prior scan context. Results re-rank by edit-recency before display. `Enter` sets `m.pending.preselectRange = &markdown.LineRange{Start: hit.Line, End: hit.Line}` and calls `m.navigateTo(hit.Path)` — the same scroll-to-line plumbing range-link Enter uses. `^j`/`^k` (and `↑`/`↓`) move the cursor; printable keys filter the query; `Esc` clears a non-empty query first, closes on the second press.

## Backlinks and modal surfaces

The TUI hosts these modal surfaces beyond the content viewport: the directory tree (`t`), the file finder (`^p`/`o`), the full-text search (`/`), the backlinks list (`b`), the recently-opened list (`r`), the log viewer (`^l`), and the help cheat sheet (`?`). They share input rules, geometry, and a single `modals.prevFocus` slot. Each has its own concept doc:

- [[modal-geometry]] — single-modal invariant, layout recompute on open, `Esc` priority chain. `?` is anchored (no swap); the rest swap with each other.
- [[diagnostics]] — the warn/error stream that feeds the footer transient and the `^l` modal.
- [[return-cursor]] — single-slot cursor restoration that survives `Enter`-follow → `h`-back round trips on backlinks.

The vault that powers backlinks is documented at [[vault-index]].
