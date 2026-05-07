# Vault-rooted picker — design

**Status:** spec — not yet implemented.
**Scope:** replace `bubbles/filepicker` with a vault-rooted modal picker that consumes the same pruned `*tree.Node` the left pane does. By construction, directories without markdown descendants don't appear; files outside `tree.MarkdownExts` don't appear.
**Out of scope:** out-of-vault navigation (the existing picker doesn't really support this anyway — it resets `CurrentDirectory = m.root` on every open). Fuzzy-search input. Directory selection (Enter on a directory only expands/collapses; doesn't "select").

See also: [docs index](../../index.md), [architecture](../../architecture.md).

## Motivation

The current `^p` picker is `bubbles/filepicker`, which:

- Shows directories regardless of contents (so descending into a non-markdown dir shows "empty"); only filters individual *files* via `AllowedTypes`.
- Has its own navigation model (one directory level at a time, push/pop dir stack, `Esc` walks up) that's foreign to the rest of this app.
- Re-reads `os.ReadDir` for each directory navigated into — slow and inconsistent with the watcher-backed `m.rootNode`.

The user wants "files and directories that have markdown" — exactly what `internal/tree.Walk` already produces. Reusing it is honest about scope: this app is a vault browser, and the picker is a vault navigator.

## Architecture

No new packages. All in `internal/tui`. The picker becomes a thin wrapper over the existing `*tree.Node` + `flattenVisible` machinery, rendered as the body of `modalPicker`.

| Today | Proposed |
|---|---|
| `m.picker filepicker.Model` (third-party widget) | `m.picker pickerState` (small struct on Model) |
| Filepicker reads `os.ReadDir` per descent | Picker reads `m.rootNode` (already in memory, pruned, watcher-refreshed) |
| Filepicker has its own keymap (Esc=back, h/l, etc.) | Picker reuses tree-pane keymap: j/k navigate, space/enter on directory toggles, enter on file selects, Esc closes |
| `m.picker.Init()` queues async `readDirMsg` | No async work; picker state is always derivable |
| Top-level `Update` forwards async picker messages | Removed — picker is sync |

`pickerState` is a separate struct because the modal owns its own cursor and expansion state — we don't want collapsing a folder in the picker to also collapse it in the left pane (or vice versa). The picker's tree state is independent from the left pane's:

```go
type pickerState struct {
    cursor   int
    expanded map[string]bool // separate from m.expanded
    flat     []treeRow
}
```

The picker's render reuses the existing `renderTree` shape (chevron + indented name) with the picker's own cursor — this requires either parameterizing `renderTree` or factoring the row-formatting into a helper that takes `(rows, cursor) ([]string)`.

## Components

### Picker state + render

`internal/tui/picker.go` becomes the picker's home (was previously a thin filepicker wrapper). New fields:

```go
type pickerState struct {
    cursor   int
    expanded map[string]bool
    flat     []treeRow
    vp       viewport.Model // scrolls when flat exceeds modal height
}
```

The picker's `flattenVisible(root *tree.Node)` walks the same tree as the left pane but consults `pickerState.expanded` (independent expansion state). On open, `cursor = 0` and `expanded = nil` (everything collapsed by default *except* the root — the picker shows top-level entries on open, the user descends from there).

Actually: defaulting to all-collapsed makes opening the picker show only the root's direct children, which is what filepicker did and matches user expectation. The left pane defaults to all-expanded because the user is browsing; the picker is jumping. Different default makes sense.

### Update routing

`internal/tui/input.go` — the picker block in `handleKey`'s modal-while-open branch is rewritten:

```go
if m.modalOpen == modalPicker {
    switch {
    case key.Matches(msg, m.keys.ClearLink): // Esc closes
        // close picker, restore focus
    case key.Matches(msg, m.keys.Up):
        // picker.cursor--, scroll viewport
    case key.Matches(msg, m.keys.Down):
        // picker.cursor++, scroll viewport
    case key.Matches(msg, m.keys.ToggleFolder), key.Matches(msg, m.keys.Open):
        // on dir: toggle expand; on file: navigateTo + close
    }
    return *m, nil
}
```

No more async forwarding — `model.go`'s `if m.modalOpen == modalPicker` non-key forwarding block can be **removed**. That alone cleans up the architecture significantly.

### Open path

`OpenPicker` toggle: instead of `m.picker.CurrentDirectory = m.root` + `m.picker.Init()`, the open closure resets `m.picker.cursor = 0`, `m.picker.expanded = map[string]bool{}` (everything default-collapsed), and rebuilds `m.picker.flat = m.picker.flattenVisible(m.rootNode)`. No async commands — `toggleModal`'s onOpen returns `nil`.

### View

`view.go`'s modal-body branch already does:
```go
body := m.modalVP.View()
if m.modalOpen == modalPicker {
    body = m.picker.View()
}
```

The new `m.picker.View()` returns the picker's own `vp.View()` (same scroll-on-tall-tree pattern as Issue 1's tree-pane fix).

## Test plan

Tests in `internal/tui/picker_test.go` (replaces the old filepicker integration tests):

1. **Picker shows only directories with markdown.** Fixture: `notes/first.md`, `empty-dir/` (no markdown). Open picker, expand root if needed, assert the picker's flat list contains `notes/` but not `empty-dir/` — guaranteed by reusing `m.rootNode`.
2. **Selecting a file closes modal and opens it.** Open, descend, Enter on a file → assert `m.modalOpen == modalNone` and `m.history.Current() == picked path`.
3. **Esc closes the picker (regardless of depth).** No more "Esc walks up" — single keystroke close. The user explicitly asked about closing earlier; this consolidates the answer.
4. **Picker expansion state is independent from left pane.** Collapse a folder in the picker, close the picker, assert the left pane's `m.expanded` is unchanged. Re-open the picker — its expansion state is reset (default collapsed).

## Migration / risks

- **Removes the `bubbles/filepicker` dependency from `go.mod`.** Worth doing in the same commit so the import graph is honest.
- **Loses out-of-vault file selection.** Documented as out-of-scope; the previous picker reset to vault root on every open anyway, so this isn't a regression in practice.
- **The `pickerState.expanded` map is not persisted across opens** — by design. Each `^p` press starts fresh. If a user wants persistence, that's a follow-up.
- **Picker uses `m.rootNode`, which is updated by the watcher.** When the picker is open and the watcher fires `StructureChanged`, the picker's `flat` should be rebuilt or the cursor restored against the new tree. Decision: rebuild lazily — set `m.picker.dirty = true` on watcher events when the picker is open, and rebuild before the next render. Simpler alternative: always rebuild on Update if `m.modalOpen == modalPicker`. Cheap; tree is small.
