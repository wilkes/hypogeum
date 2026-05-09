# modal-geometry

The single-modal invariant and layout rules that govern the backlinks pane (`b`), backlinks modal (`B`), log viewer modal (`^l`), help modal (`?`), and file picker modal (`^p`).

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/tui`](../packages/tui.md), the [wikilinks-and-backlinks design](../superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md), and the [backlinks-navigation design](../superpowers/specs/2026-05-07-backlinks-navigation-design.md); press `b` for the full backlinks list.

## Why it exists

Several surfaces compete for screen space and input focus: the persistent backlinks bottom split (`b`), the backlinks modal overlay (`B`), the log viewer modal (`^l`), the help modal (`?`), and the file picker modal (`^p`). Without a coordination rule they'd stack, conflict over `Esc`, and require per-surface geometry calculations. The single-modal invariant and shared modal viewport collapse this to one decision per keypress: open / swap / close.

## How it works

**Persistent pane (`b`):** `m.backlinks.open` toggles. When open *and* `m.height >= 20`, the content viewport's height shrinks by `backlinksHeight` (8 rows including border). When `m.height < 20`, the pane is suppressed in `View()` but `m.backlinks.open` stays true — when the terminal grows again, the pane reappears.

**Modals:** `m.modals.kind` is a single enum (`modalNone`/`modalBacklinks`/`modalLogs`/`modalPicker`/`modalHelp`). `B` and `^l` swap with each other under the single-modal-swap rule (pressing one while the other is open replaces it). `?` (help) is *anchored*: pressing it while a different modal is open is a no-op so the cheat sheet can't steal focus from a mid-task modal. The picker (`^p`) opens via the same toggle path but has its own viewport (`m.modals.picker.vp`); the other three (backlinks, logs, help) share `m.modals.vp` with content swapped on open. While any modal is open, geometry is recomputed as if `m.backlinks.open` were false; the content viewport reclaims the bottom split's space and the modal renders centered on top.

**Modal size:** fixed at 60% width × 60% height, clamped to min 40 cols × 12 rows, max 120 cols × 40 rows.

**`Esc` priority** (extending the existing chain):
1. If a modal is open → close it.
2. Else if `m.focus == focusBacklinks` → restore `m.modals.prevFocus` (pane stays open).
3. Else if `m.content.linkCursor >= 0` → clear it.
4. Else → no-op.

## Invariants / gotchas

- **`B` and `^l` are mutually aware.** They can never render simultaneously. Pressing one while the other is open swaps content, doesn't stack. Tests assert this.
- **`?` is anchored, not a swap participant.** Help opens only from `modalNone` or while help is already open (toggles closed). Pressing `?` while backlinks/logs/picker is open is a no-op. Test: `TestHelpModalDoesNotSwap`.
- **The persistent pane and a modal can coexist as state.** When a modal opens with the pane open, the pane is hidden in `View()` for that frame; closing the modal brings the pane back. State and rendering are decoupled.
- **`m.modals.prevFocus` is saved on modal open.** Opening from the backlinks pane saves `focusBacklinks` so `Esc` returns there, not to `focusContent`. There's a subtle bug in this area (recently fixed in commit `3df72c0`) — opening a modal from the backlinks pane must not stomp `prevFocus` if it was already set during the pane open.
- **Below height 20, the pane is suppressed but not closed.** The user's intent (`m.backlinks.open = true`) is honored; only the rendering is conditional. This is the same graceful-degradation rule as the watcher and vault.
- **Modal viewport is shared.** Don't add per-modal scroll state — backlinks/logs/help all scroll the same `m.modals.vp`, with content swapped on open. The picker has its own (`m.modals.picker.vp`).
