# modal-geometry

The single-modal invariant and layout rules that govern every overlay: tree (`^b`), backlinks (`b`), file finder (`^p`), log viewer (`^l`), and help (`?`).

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/tui`](../packages/tui.md), the [wikilinks-and-backlinks design](../superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md), and the [backlinks-navigation design](../superpowers/specs/2026-05-07-backlinks-navigation-design.md); press `b` for the full backlinks list.

## Why it exists

Five surfaces compete for screen space and input focus. Without a coordination rule they'd stack, conflict over `Esc`, and require per-surface geometry calculations. The single-modal invariant and shared modal viewport collapse this to one decision per keypress: open / swap / close.

## How it works

**`m.modals.kind` is a single enum** (`modalNone` / `modalBacklinks` / `modalLogs` / `modalPicker` / `modalHelp` / `modalTree`). At most one is open at a time. Pressing a toggle key for a *different* kind while one is open swaps content under the single-modal-swap rule. Pressing the same toggle key again closes the modal.

**`?` is anchored, not a swap participant.** Help opens only from `modalNone` or toggles itself closed. Pressing `?` while another modal is open is a no-op and surfaces a footer transient explaining why.

**Modal viewport sharing.** Backlinks, logs, and help all render through the shared `m.modals.vp`. The picker has its own (`m.modals.picker.vp`) because it owns a text input. The tree uses `m.tree.vp` so its cursor and expansion state survive modal-close.

**Modal size:** fixed at 60% width × 60% height of the terminal, clamped to min 40×12 and max 120×40. Computed once per `WindowSizeMsg` in `modalGeometry`.

**`Esc` priority chain:**

1. If a modal is open → close it.
2. Else if `m.content.linkCursor >= 0` → clear it.
3. Else → no-op.

## Invariants / gotchas

- **`?` is anchored, not a swap participant.** Test: `TestHelpModalDoesNotSwap`. Anything else just toggles or swaps.
- **`m.modals.prevFocus` is saved on modal open.** Since `focus` is currently a one-value enum (`focusContent` only), prevFocus is always `focusContent` and the save/restore is effectively a no-op. Kept as a hook for if/when another focus value is reintroduced.
- **`closeModal` is symmetric with `toggleModal`.** Both live in `modal.go`. Callers should never inline `m.modals.kind = modalNone; m.focus = m.modals.prevFocus` — always use the helper.
- **Picker grabs printable runes before global modal-toggles.** `handleKey` routes `tea.KeyRunes` to the picker's text input first when `modals.kind == modalPicker`. Without this, plain-letter modal toggles like `b` would swap the picker out the moment the user typed those letters into the fuzzy-filter query. Non-rune keys (`Esc`, `Enter`, `^P`, `^j`, `^k`, arrows) still flow through the normal modal block.
- **Tree-row zones are scanned after overlay.** `View()` calls `zone.Scan` on the *composed* output (base + modal splice) so BubbleZone records final screen coordinates. The tree modal renders interactive rows; the other modals do not.
