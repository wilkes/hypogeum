# Narrow-terminal layout — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-hide the tree pane when the terminal is narrower than 80 columns so the two-pane layout degrades gracefully without overflowing the window.

**Architecture:** Add a `treeShown()` method that gates `treeVisible` on `m.width >= twoPaneMinWidth`. Every layout site that currently checks `treeVisible` reads `treeShown()` instead. `^b` continues to flip user intent (`treeVisible`); the new method computes effective state. Mirrors the existing `backlinksOpen` (intent) / `shouldShowBacklinks()` (effective) split.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, BubbleZone. No new dependencies.

**Spec:** [`docs/superpowers/specs/2026-05-07-narrow-terminal-layout-design.md`](../specs/2026-05-07-narrow-terminal-layout-design.md)

---

## File map

| File | Action | Responsibility |
|---|---|---|
| `internal/tui/view.go` | modify | add `twoPaneMinWidth` const + `treeShown()` accessor; have `treeWidth()` and `View()` consult `treeShown()`; lower the tree-width floor |
| `internal/tui/input.go` | modify | add a one-line comment at the click-hit-test loop noting why stale zones can't match across the threshold |
| `internal/tui/tree_test.go` | modify | add four new tests for narrow-terminal behavior |
| `CLAUDE.md` | modify | add a gotcha note about the `treeVisible` (intent) vs `treeShown()` (effective) split |

No new files. The change is small enough that splitting `view.go` isn't warranted.

---

## Task 1: Add `twoPaneMinWidth` and `treeShown()` (failing test first)

**Files:**
- Test: `internal/tui/tree_test.go`
- Modify: `internal/tui/view.go`

**Background:** The new method has no callers yet; we add it (and a test exercising it) before changing any layout sites. This separates the new state machine from the rewiring.

- [ ] **Step 1: Write the failing test**

Append at the end of `internal/tui/tree_test.go`:

```go
// TestModel_TreeShownAtNarrowWidths checks that treeShown() returns false
// when the terminal is narrower than twoPaneMinWidth even if the user
// has the tree visible — the threshold gates effective state.
func TestModel_TreeShownAtNarrowWidths(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	if !m.treeVisible {
		t.Fatalf("tree should default to visible")
	}

	cases := []struct {
		width int
		want  bool
	}{
		{60, false},
		{79, false},
		{80, true},
		{120, true},
	}
	for _, tc := range cases {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: tc.width, Height: 30})
		mm := updated.(Model)
		if got := mm.treeShown(); got != tc.want {
			t.Errorf("width=%d: treeShown() = %v, want %v", tc.width, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_TreeShownAtNarrowWidths -v`

Expected: build error — `m.treeShown undefined (type Model has no field or method treeShown)`.

- [ ] **Step 3: Add the constant and accessor**

In `internal/tui/view.go`, add the constant block near the top (above the `View` func) and the method:

```go
// twoPaneMinWidth is the minimum terminal width at which the two-pane
// (tree + content) layout is shown. Below this, the tree pane is force-
// hidden regardless of user intent — content gets the full window so
// prose has room to wrap. Mirrors backlinksMinTotalHeight on the height
// axis. Tunable; if 80 turns out to be wrong, change here.
const twoPaneMinWidth = 80

// treeShown returns true when the tree pane is currently rendered.
// Combines user intent (m.treeVisible, toggled by ^b) with terminal
// width: even with treeVisible=true, narrow terminals force-hide the
// pane. Same intent/effective-state pattern as shouldShowBacklinks.
func (m Model) treeShown() bool {
	return m.treeVisible && m.width >= twoPaneMinWidth
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestModel_TreeShownAtNarrowWidths -v`

Expected: PASS.

- [ ] **Step 5: Run full test suite**

Run: `go test ./... && go vet ./...`

Expected: all packages PASS, vet silent.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/view.go internal/tui/tree_test.go
git commit -m "$(cat <<'EOF'
feat(tui): add twoPaneMinWidth and Model.treeShown()

The new accessor combines user intent (treeVisible) with terminal width
to compute effective state. Mirrors shouldShowBacklinks. No layout site
consumes it yet — that swap lands in the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Rewire `treeWidth()` and `View()` to consult `treeShown()`; lower the floor to 16

**Files:**
- Modify: `internal/tui/view.go`
- Test: `internal/tui/tree_test.go`

**Background:** Now that the accessor exists and is tested, swap the consumers. `treeWidth()` returns 0 below the threshold; `View()` skips rendering the tree column. Drop the floor from 20 to 16 in the same change since the layout math counts on it.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/tree_test.go`:

```go
// TestModel_TreeForceHiddenAt60Cols checks that below twoPaneMinWidth
// the tree pane is rendered as 0 cells wide and its row text doesn't
// appear in the View output, regardless of treeVisible.
func TestModel_TreeForceHiddenAt60Cols(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = updated.(Model)

	if m.treeShown() {
		t.Errorf("treeShown() should be false at 60 cols")
	}
	if w := m.treeWidth(); w != 0 {
		t.Errorf("treeWidth() = %d at 60 cols, want 0", w)
	}
	if strings.Contains(m.View(), "notes/") {
		t.Errorf("View() should not contain tree row 'notes/' at 60 cols")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_TreeForceHiddenAt60Cols -v`

Expected: FAIL — `treeWidth()` still returns 20 because the function reads `m.treeVisible` (true by default), not `m.treeShown()`.

- [ ] **Step 3: Update `treeWidth()` to consult `treeShown()` and lower the floor**

In `internal/tui/view.go`, replace the existing `treeWidth` function:

```go
func (m Model) treeWidth() int {
	if !m.treeShown() {
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

- [ ] **Step 4: Update `View()` to consult `treeShown()`**

In `internal/tui/view.go`, change the `if m.treeVisible` line in `View()` to use `treeShown()`. The full body of the conditional block (lines 33–42 in current view.go):

```go
	var body string
	if m.treeShown() {
		treeStyled := zone.Mark(zoneTreePane, paneStyle(m.focus == focusTree).
			Width(m.treeWidth()).
			Height(m.height-4).
			Render(m.renderTree()))
		body = lipgloss.JoinHorizontal(lipgloss.Top, treeStyled, contentColumn)
	} else {
		body = contentColumn
	}
```

Only one character changes (`treeVisible` → `treeShown()`); everything else is unchanged.

- [ ] **Step 5: Run the new test**

Run: `go test ./internal/tui/ -run TestModel_TreeForceHiddenAt60Cols -v`

Expected: PASS.

- [ ] **Step 6: Run the full test suite**

Run: `go test ./... && go vet ./...`

Expected: all packages PASS. Existing tests use `Width: 100` or `Width: 120` (above threshold), so they're undisturbed.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/view.go internal/tui/tree_test.go
git commit -m "$(cat <<'EOF'
feat(tui): force-hide tree pane below 80-column threshold

treeWidth() and View() now consult m.treeShown() instead of the raw
m.treeVisible bool. Below twoPaneMinWidth (80) the tree pane has zero
width and View skips rendering it; content gets the full window.

Floor lowered from 20 to 16 in the same change — at the new threshold
the tree gets at least 16 useful cells, with filename truncation
handled by lipgloss.Width-clamped rendering.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Verify `^b` flips intent silently when narrow

**Files:**
- Test: `internal/tui/tree_test.go`

**Background:** With Tasks 1–2 in place, pressing `^b` on a narrow terminal already does the right thing: `treeVisible` flips, but `treeShown()` stays false because the width gate fails. We add a regression test to lock the behavior in.

- [ ] **Step 1: Write the test**

Append to `internal/tui/tree_test.go`:

```go
// TestModel_ToggleTreeNarrowFlipsIntentOnly checks that ^b at a narrow
// terminal width flips treeVisible (so the user's preference survives
// resize) but doesn't change effective state — treeShown stays false
// because the width gate fails.
func TestModel_ToggleTreeNarrowFlipsIntentOnly(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = updated.(Model)
	if !m.treeVisible {
		t.Fatalf("precondition: treeVisible should still be true after a narrow resize")
	}
	if m.treeShown() {
		t.Fatalf("precondition: treeShown should be false at 60 cols")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)
	if m.treeVisible {
		t.Errorf("treeVisible should be false after ^b")
	}
	if m.treeShown() {
		t.Errorf("treeShown should still be false at 60 cols")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)
	if !m.treeVisible {
		t.Errorf("treeVisible should flip back to true on second ^b")
	}
	if m.treeShown() {
		t.Errorf("treeShown should still be false at 60 cols")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/tui/ -run TestModel_ToggleTreeNarrowFlipsIntentOnly -v`

Expected: PASS — Tasks 1–2 already implemented the behavior; this test just locks it in.

- [ ] **Step 3: Run the full suite**

Run: `go test ./... && go vet ./...`

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/tree_test.go
git commit -m "$(cat <<'EOF'
test(tui): ^b at narrow widths flips intent silently

Lock in the behavior that ^b on a too-narrow terminal toggles
m.treeVisible (so the preference is preserved across resize) without
changing m.treeShown(). The user can prep their preference for when
the terminal grows back without seeing any visible flicker.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Verify the tree returns when the window grows

**Files:**
- Test: `internal/tui/tree_test.go`

**Background:** Ensures the round-trip works: a narrow → wide resize restores the tree to its `treeVisible`-respecting state without the user pressing anything. This is the payoff for keeping intent and effective state separate.

- [ ] **Step 1: Write the test**

Append to `internal/tui/tree_test.go`:

```go
// TestModel_TreeReturnsOnGrow checks that after a narrow resize hides
// the tree, growing the terminal back above the threshold restores it
// without any user interaction — m.treeVisible is preserved.
func TestModel_TreeReturnsOnGrow(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = updated.(Model)
	if m.treeShown() {
		t.Fatalf("precondition: treeShown should be false at 60 cols")
	}

	updated, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	if !m.treeShown() {
		t.Errorf("treeShown should be true after growing to 100 cols")
	}
	if w := m.treeWidth(); w == 0 {
		t.Errorf("treeWidth should be nonzero after growing to 100 cols")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/tui/ -run TestModel_TreeReturnsOnGrow -v`

Expected: PASS.

- [ ] **Step 3: Run the full suite**

Run: `go test ./... && go vet ./...`

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/tree_test.go
git commit -m "$(cat <<'EOF'
test(tui): tree returns on resize back above 80 cols

Round-trip test: narrow → wide resize restores the tree without user
interaction because m.treeVisible was preserved through the narrow
window.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Defensive comment on the BubbleZone hit-test loop

**Files:**
- Modify: `internal/tui/input.go`

**Background:** When the tree disappears across the threshold, BubbleZone's per-row zones from a previous render are never re-Marked. The existing click loop already iterates `len(m.flatTree)` so stale zones can't match — but the spec calls for a comment at the loop site so a future reader doesn't accidentally remove that bound.

- [ ] **Step 1: Update the comment at `internal/tui/input.go:65-72`**

Replace the existing block comment immediately above the tree-row click loop:

```go
	// Tree row hit. Iterate visible rows; the first that contains the
	// click wins. Stops at len(m.flatTree) so out-of-range zones from a
	// previous longer document don't match — also defends against stale
	// zones left over when the tree pane is hidden (^b or narrow-width
	// auto-hide), since len(m.flatTree) is bounded by what's currently
	// rendered.
	for i := range m.flatTree {
```

- [ ] **Step 2: Run the full suite**

Run: `go test ./... && go vet ./...`

Expected: all PASS — the comment is the only change.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/input.go
git commit -m "$(cat <<'EOF'
docs(tui): note narrow-width hide in tree-row hit-test comment

The existing len(m.flatTree) bound on the click loop already defends
against stale BubbleZone entries when the tree disappears across the
80-col threshold. Make the protection explicit in the comment so a
future reader doesn't refactor away the bound.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Document the gotcha in CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

**Background:** CLAUDE.md is loaded into every Claude Code session for this repo. The intent/effective split is non-obvious; documenting it next to the existing tree-pane gotchas means future work doesn't re-introduce the bug we just fixed.

- [ ] **Step 1: Update `CLAUDE.md`**

Find the line beginning `- **Tree visibility toggle synthesizes a resize.**` (currently in the Gotchas section). Append a new bullet immediately after it:

```markdown
- **Tree visibility has two states: intent vs effective.** `m.treeVisible` is what the user wants (toggled by `^b`); `m.treeShown()` is what's actually rendered. The latter additionally requires `m.width >= twoPaneMinWidth` (80 cols). Below the threshold, `^b` flips the intent silently — preserved for when the terminal grows back. This mirrors `m.backlinksOpen` (intent) / `shouldShowBacklinks()` (effective, gated on height). Layout code reads `treeShown()`; only `^b` writes `treeVisible`.
```

- [ ] **Step 2: Run the full suite (no functional change, but verify)**

Run: `go test ./... && go vet ./...`

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "$(cat <<'EOF'
docs: note tree intent/effective-state split in CLAUDE.md

m.treeVisible (user intent) and m.treeShown() (effective state, gated
on terminal width >= 80) are separate booleans. Future work should
read treeShown() at layout sites and only write treeVisible from the
^b handler.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Manual verification

**Files:** none — this is a manual smoke test before opening the PR.

**Background:** The TUI requires a real terminal; automated tests can't observe the actual render. Resize the terminal to validate end-to-end that nothing visually broken made it through.

- [ ] **Step 1: Build and run**

```bash
go build ./... && go run ./cmd/hypogeum docs/
```

- [ ] **Step 2: At full width, confirm two-pane**

Resize the terminal to ~120 cols. Verify the tree pane is visible on the left, content on the right.

- [ ] **Step 3: Resize narrow**

Slowly drag the terminal narrower. At 80 cols the tree should still be visible (16-cell minimum); at 79 cols and below it should disappear and content should expand to the full window.

- [ ] **Step 4: Verify `^b` is preserved across resize**

At 60 cols, press `^b`. Nothing visible should change. Resize back to 120 cols — the tree should remain hidden because the user's intent flipped.

Press `^b` again. The tree should reappear.

- [ ] **Step 5: Verify modals at narrow width**

At 60 cols, press `^p` (picker), `B` (backlinks modal), and `?` (logs). All three modals clamp to terminal width and should render without overflow. (Out of scope to fix; we're just confirming we didn't regress.)

---

## Self-review notes

- **Spec coverage:** all five spec test scenarios are covered (Tasks 1–4 implement the four spec test cases; Task 5 adds the defensive comment the spec calls out; Task 6 adds the CLAUDE.md note the spec calls out).
- **Type/method consistency:** `treeShown()`, `twoPaneMinWidth`, `treeWidth()`, `treeVisible` used consistently across all tasks. `m.width` is the field name (not `m.Width`); confirmed against `model.go`.
- **No placeholders:** every step has executable code or explicit commit text.
- **Order check:** Task 1 introduces the symbol; Task 2 swaps consumers; Tasks 3–4 are regression tests for behavior already implemented in Task 2; Tasks 5–6 are documentation polish; Task 7 is manual smoke. Each commits independently and passes the full suite at every step.
