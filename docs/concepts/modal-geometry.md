# modal-geometry

The single-modal invariant and layout rules that govern the backlinks pane (`b`), backlinks modal (`B`), and log viewer modal (`?`).

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/tui`](../packages/tui.md), the [wikilinks-and-backlinks design](../superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md), and the [backlinks-navigation design](../superpowers/specs/2026-05-07-backlinks-navigation-design.md); press `b` for the full backlinks list.

## Why it exists

Three surfaces compete for screen space and input focus: the persistent backlinks bottom split (`b`), the backlinks modal overlay (`B`), and the log viewer modal (`?`). Without a coordination rule they'd stack, conflict over `Esc`, and require per-surface geometry calculations. The single-modal invariant and shared modal viewport collapse this to one decision per keypress: open / swap / close.

## How it works

**Persistent pane (`b`):** `m.backlinksOpen` toggles. When open *and* `m.height >= 20`, the content viewport's height shrinks by `backlinksHeight` (8 rows including border). When `m.height < 20`, the pane is suppressed in `View()` but `m.backlinksOpen` stays true — when the terminal grows again, the pane reappears.

**Modals (`B`, `?`):** `m.modalOpen` is a single enum (`modalNone`/`modalBacklinks`/`modalLogs`). Pressing `B` while `modalLogs` is up swaps to `modalBacklinks` (and vice versa). The two modals share one viewport (`m.modalVP`) and one set of geometry — content is the only thing that changes. While any modal is open, geometry is recomputed as if `backlinksOpen` were false; the content viewport reclaims the bottom split's space and the modal renders centered on top.

**Modal size:** fixed at 60% width × 60% height, clamped to min 40 cols × 12 rows, max 120 cols × 40 rows.

**`Esc` priority** (extending the existing chain):
1. If a modal is open → close it.
2. Else if `m.focus == focusBacklinks` → restore `prevFocus` (pane stays open).
3. Else if `m.linkCursor >= 0` → clear it.
4. Else → no-op.

## Invariants / gotchas

- **`B` and `?` are mutually aware.** They can never render simultaneously. Pressing one while the other is open swaps content, doesn't stack. Tests assert this.
- **The persistent pane and a modal can coexist as state.** When a modal opens with the pane open, the pane is hidden in `View()` for that frame; closing the modal brings the pane back. State and rendering are decoupled.
- **`prevFocus` is saved on modal open.** Opening from the backlinks pane saves `focusBacklinks` so `Esc` returns there, not to `focusContent`. There's a subtle bug in this area (recently fixed in commit `3df72c0`) — opening a modal from the backlinks pane must not stomp `prevFocus` if it was already set during the pane open.
- **Below height 20, the pane is suppressed but not closed.** The user's intent (`backlinksOpen = true`) is honored; only the rendering is conditional. This is the same graceful-degradation rule as the watcher and vault.
- **Modal viewport is shared.** Don't add per-modal scroll state — both modals scroll the same `modalVP`, with content swapped on open.
