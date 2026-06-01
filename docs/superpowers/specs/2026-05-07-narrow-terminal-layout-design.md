# Narrow-terminal layout — design

**Status:** superseded — the two-pane layout this targeted was replaced by a tree modal on 2026-05-13 (PR #23). The modal clamps to a percentage of terminal size rather than gating on width, so the narrow-terminal threshold no longer exists.
**Scope:** make the two-pane layout degrade gracefully on terminals narrower than 80 columns. Below the threshold, the tree pane auto-hides and the content pane gets the full width.
**Out of scope:** modal floor of 40 cells (cosmetic only at this size); footer help wrapping (pre-existing on any sub-100-col terminal); backlinks pane height handling (already correct via `backlinksMinTotalHeight`).

See also: [docs index](../../index.md), [architecture](../../architecture.md).

## Motivation

`treeWidth()` floors at 20 cells and `contentWidth` floors at 20. At 60 cols today, the tree pane is 20 cells, content gets 38, and prose wraps at ~38 cells with the tree taking more than half the screen. At 40 cols the layout overflows: tree (20) + content floor (20) + borders (2) = 42 cells of demanded width on a 40-cell terminal, so the rightmost cells run off the edge.

The two-pane layout simply doesn't fit on narrow terminals. Forcing the tree on costs more in lost prose width than it returns in navigation visibility — at 60 cols the file names truncate to ~16 cells anyway.

## Architecture

No new packages. All work in `internal/tui`. The change mirrors an existing pattern: `m.backlinksOpen` (user intent) is decoupled from `shouldShowBacklinks()` (effective state, gated on `backlinksMinTotalHeight`). Apply the same shape to the tree pane.

| State name | Meaning |
|---|---|
| `m.treeVisible bool` | *user wants* tree shown (toggled by `^b`) |
| `m.shouldShowTree() bool` | tree is *currently* shown — `treeVisible && m.width >= twoPaneMinWidth` |

Everywhere that reads `m.treeVisible` for layout decisions reads `m.shouldShowTree()` instead. `^b` continues to toggle `m.treeVisible`; nothing else writes it.

`twoPaneMinWidth = 80` constant. Rationale: 16+40+2=58 is the hard floor where a useful tree (16 cells) + comfortable prose (40 cells) + borders fit. 80 leaves a comfortable cushion before the layout feels cramped, and matches the `maxRenderWidth = 80` cap on Glamour wrap width — below 80 cols, single-pane already gets the full window for prose.

`treeWidth()` floor lowered from 20 to 16. Below this, filenames truncate harshly; above the threshold the existing `m.width / 4` formula and `max(40)` ceiling apply unchanged.

## Components

### `internal/tui` (changes)

**`view.go`:**

```go
const twoPaneMinWidth = 80

func (m Model) shouldShowTree() bool {
    return m.treeVisible && m.width >= twoPaneMinWidth
}

func (m Model) treeWidth() int {
    if !m.shouldShowTree() {
        return 0
    }
    w := m.width / 4
    if w < 16 {
        w = 16
    }
    if w > 40 {
        w = 40
    }
    return w
}
```

The `View()` body conditional changes from `if m.treeVisible` to `if m.shouldShowTree()`. No other view-side change.

**`model.go`:** unchanged. The `WindowSizeMsg` handler reads `m.treeWidth()`, which now correctly returns 0 below the threshold without further logic.

**`input.go`:** `^b` continues to flip `m.treeVisible` and synthesize a `WindowSizeMsg`. When the terminal is too narrow the flip has no visible effect, which mirrors how `b` (backlinks) flips `backlinksOpen` under `backlinksMinTotalHeight`. No diagnostic emitted — silent flip preserves user intent for when the terminal grows back.

**`content.go`:** new `m.normalizeFocus()` helper enforces the invariant *focus may not point at a pane that isn't shown*. Called from the `WindowSizeMsg` handler so resize paths repair focus consistently — covers both the user dragging the terminal narrower (auto-hide) and `^b` (which routes through a synthetic resize). Focus on grow is intentionally *not* restored: snapping focus back to the tree as soon as it reappears would yank the cursor away from whatever the user is reading.

**`CLAUDE.md`:** add a gotcha note explaining the `treeVisible` (intent) / `shouldShowTree()` (effective) split and the 80-col threshold; cross-reference the parallel `backlinksOpen` / `shouldShowBacklinks()` pair.

## UX behavior

| Width | `treeVisible` | Result |
|---|---|---|
| ≥ 80 cols | true | Two-pane (current behavior) |
| ≥ 80 cols | false | Single-pane via `^b` (current behavior) |
| < 80 cols | true | Single-pane forced; `^b` toggles intent silently |
| < 80 cols | false | Single-pane (intent matches forced state) |

When the user resizes from 60 → 100 cols, the tree returns automatically with their last-set visibility. When they resize from 100 → 60 cols, the tree disappears regardless of intent. This is the same model as the backlinks pane under height changes.

## Test plan

New tests in `internal/tui/tree_test.go` (or a new `narrow_test.go`, depending on file size when implementing):

1. **Tree force-hidden below threshold.** Construct model, send `WindowSizeMsg{Width: 60, Height: 30}`, assert `m.shouldShowTree() == false`, `m.treeWidth() == 0`, and `View()` doesn't contain a tree row's name.
2. **`^b` flips intent silently when narrow.** Open at 60 cols, send `^b`, assert `m.treeVisible == false` (state flipped), `m.shouldShowTree() == false` (still not shown). Send `^b` again, assert `m.treeVisible == true`, `m.shouldShowTree() == false`.
3. **Tree returns when window grows.** Start at 60 cols (tree force-hidden), send `WindowSizeMsg{Width: 100, Height: 30}`, assert `m.shouldShowTree() == true` because the default `m.treeVisible == true` was preserved.
4. **Threshold boundary regression.** At exactly 80 cols (`twoPaneMinWidth`), assert tree is shown. At 79 cols, assert tree is hidden.

The existing test fixture in `helpers_test.go` opens at `Width: 120, Height: 40` — well above the threshold; existing tests are undisturbed.

## Risks & caveats

- **BubbleZone stale zones across the threshold.** When the tree pane stops rendering, its per-row zones aren't re-Marked on the next `zone.Scan`. The existing hit-test loop in `input.go` iterates `len(m.flatTree)`, so stale zones from previous renders can't match — defensible by current code, worth a one-line comment at the loop site.
- **Threshold flapping at 80 cols.** Resizing right at the boundary flips the tree on/off. No hysteresis proposed: the resize path is already cheap (Glamour rebuilds on every `WindowSizeMsg`), and 1-cell wobbles aren't a real user scenario.
- **Threshold value is a single named constant.** If 80 turns out to be wrong in practice, tuning is a one-line change.
