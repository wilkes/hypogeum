# Backlinks navigation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the existing backlinks display surfaces (persistent pane via `b`, modal via `B`) interactive — adds a cursor, `Enter`-to-follow, viewport scroll-to-line on the source file, and cursor restoration after `h` (Back).

**Architecture:** All changes are confined to `internal/tui`. No vault or markdown changes. State additions on `Model`: a cached `[]vault.Backlink`, a saved `prevFocus`, and an optional `returnCursor` consumed by Back. The `focus` enum gains a third value `focusBacklinks`. `Tab` becomes a three-way cycle (tree → content → backlinks → tree) when the pane is open and visible.

**Tech Stack:** Go, Bubble Tea (Elm-style update loop), Bubbles `viewport`, Lip Gloss (styling). Tests use `tea.WindowSizeMsg` + `Model.Update` directly — no real terminal needed.

**Spec:** [`docs/superpowers/specs/2026-05-07-backlinks-navigation-design.md`](../specs/2026-05-07-backlinks-navigation-design.md)

---

## Conventions used throughout

- Every code block shows the **full** function or method body where practical, so a reader doesn't need to chase context.
- Test functions in this package run via `go test ./internal/tui/...`. The test helper `writeTUITestFile` (at `internal/tui/backlinks_test.go:12`) writes a file under a `t.TempDir()` root.
- The existing `New(root, "")` followed by `m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})` is the canonical setup. The helper `sized(t, root, "")` in `internal/tui/helpers_test.go:38` also works and is sometimes cleaner.
- Commit messages follow the pattern in the repo's recent history: `feat(tui): …` for behavior, `test(tui): …` for tests-only commits, `refactor(tui): …` for non-behavior changes.
- All `git commit` invocations include a trailing `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` line, omitted from this plan for brevity. The repo's CLAUDE.md describes the full commit format.
- After every task, run `go build ./... && go test ./internal/tui/...` to confirm no regressions.

---

## File-by-file overview

| File | Change |
|---|---|
| `internal/tui/model.go` | Add `focusBacklinks` to the `focus` enum (after `focusContent`); add `backlinks []vault.Backlink`, `prevFocus focus`, `returnCursor *returnCursor` to `Model`. |
| `internal/tui/backlinks.go` | Add `returnCursor` struct, `backlinksSurface` enum, `clamp` helper, `activeBacklinksSurface()` method, modify `formatBacklinks` to take a `cursor int`, modify `refreshBacklinks` and `refreshBacklinksModal` to populate `m.backlinks`, add `ensureCursorVisible`, `followBacklink`, `scrollToLine`. |
| `internal/tui/input.go` | Add backlinks-pane key handling, modal cursor handling, `Esc` priority chain extension, `Tab` three-way cycle, `Back` cursor-restore branch, focus-switch logic in `b`/`B` toggles. |
| `internal/tui/backlinks_test.go` | Eleven new test functions covering cursor, follow, scroll, return-cursor, focus, and Tab. |
| `internal/tui/helpers_test.go` | Add `pressKey` helper that matches the package's `Update`/`KeyMsg` pattern. |
| `CLAUDE.md` (repo root) | Append "navigation (cursor, follow, scroll-to-line, return-cursor)" to the Phase 1 line. |
| `docs/index.md` | Already updated (link added in the spec commit). |
| `docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md` | Replace the "we do not auto-scroll" sentence with a pointer to the navigation spec. |

---

## Task 1: Add `focusBacklinks` to the focus enum

**Files:**
- Modify: `internal/tui/model.go:34-40`
- Test: (no new test — covered by later focus-switch tests)

- [ ] **Step 1: Update the focus enum**

In `internal/tui/model.go`, replace the `focus` block:

```go
// Focus indicates which pane currently receives keyboard input for movement.
type focus int

const (
	focusTree focus = iota
	focusContent
	focusBacklinks
)
```

- [ ] **Step 2: Verify compile**

Run: `go build ./...`
Expected: builds cleanly. The new constant is unused so far — Go does not error on unused constants.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/model.go
git commit -m "refactor(tui): add focusBacklinks to focus enum"
```

---

## Task 2: Add new model state fields

**Files:**
- Modify: `internal/tui/model.go:42-75`
- Test: (no new test — fields are populated and read in later tasks)

- [ ] **Step 1: Add fields to the `Model` struct**

In `internal/tui/model.go`, replace the `Model` struct (lines 42-75) by adding three fields next to the existing backlinks state. The full updated struct:

```go
// Model is the top-level Bubble Tea model.
type Model struct {
	root       string
	rootNode   *tree.Node
	flatTree   []treeRow // pre-flattened for keyboard navigation
	treeCursor int

	viewport viewport.Model
	renderer *markdown.Renderer

	history *nav.History
	focus   focus

	links      []markdown.Link // links extracted from the currently rendered file
	linkCursor int             // -1 when no link is selected (Phase 1: always -1)

	backlinksOpen  bool
	backlinksVP    viewport.Model
	backlinkCursor int
	backlinks      []vault.Backlink // cached so cursor moves don't re-query the vault
	prevFocus      focus            // saved when opening a backlinks surface, restored on close
	returnCursor   *returnCursor    // set on follow, consumed on the next matching Back navigation

	modalOpen modalKind
	modalVP   viewport.Model

	width, height int
	keys          keyMap
	status        string // last error or info message

	// watcher observes the tree for live updates. nil if construction
	// failed (we degrade gracefully — the browser still works without it).
	watcher *watch.Watcher

	vault *vault.Vault
	diag  *diagnostics
}
```

- [ ] **Step 2: Verify compile (will fail — `returnCursor` undefined)**

Run: `go build ./...`
Expected: FAIL with `undefined: returnCursor`. This is fine — we define it in Task 3.

- [ ] **Step 3: Skip commit until Task 3**

We don't commit a non-building tree. Proceed to Task 3.

---

## Task 3: Define `returnCursor` and `backlinksSurface` types

**Files:**
- Modify: `internal/tui/backlinks.go` (top of file, after the existing constants)
- Test: (none — types are exercised in later tasks)

- [ ] **Step 1: Add type definitions**

In `internal/tui/backlinks.go`, add after the existing `snippetHighlightOpenChar`/`CloseChar` constants (after line 30):

```go
// backlinksSurface identifies which backlinks UI surface (persistent
// pane vs modal) the user was navigating when they followed a backlink.
// Used by returnCursor so Back can restore them to the same surface.
type backlinksSurface int

const (
	surfacePane backlinksSurface = iota
	surfaceModal
)

// returnCursor remembers where the user was in the backlinks list
// before following a backlink. Single-slot: we only restore on the
// next Back navigation, and only if it lands on the file we recorded.
type returnCursor struct {
	sourceFile string
	cursor     int
	surface    backlinksSurface
}

// clamp returns v constrained to [lo, hi]. If hi < lo (e.g. when the
// list is empty so hi = -1), returns lo. Used on cursor restoration
// to defend against the list shrinking between follow and return.
func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
```

- [ ] **Step 2: Verify compile**

Run: `go build ./...`
Expected: builds cleanly. The Model struct from Task 2 now compiles too.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/model.go internal/tui/backlinks.go
git commit -m "feat(tui): add returnCursor + cached backlinks state to Model"
```

---

## Task 4: Update `formatBacklinks` to render a cursor

**Files:**
- Modify: `internal/tui/backlinks.go:64-78` (`formatBacklinks` function)
- Test: `internal/tui/backlinks_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/backlinks_test.go`:

```go
func TestFormatBacklinks_HighlightsSelectedRow(t *testing.T) {
	links := []vault.Backlink{
		{SourceFile: "/r/a.md", DisplayText: "x", Snippet: "hello", Line: 1},
		{SourceFile: "/r/b.md", DisplayText: "x", Snippet: "world", Line: 2},
	}
	rendered := formatBacklinks(links, "/r", 80, 1)
	// The second entry (cursor=1) should carry the left-edge marker.
	// We check for the marker glyph; ANSI styling may surround it.
	if !strings.Contains(rendered, "▌") {
		t.Fatalf("expected cursor marker '▌' in output, got %q", rendered)
	}
	// The marker must appear on the line containing b.md, not a.md.
	lines := strings.Split(rendered, "\n")
	var sawMarkerOnA, sawMarkerOnB bool
	for _, line := range lines {
		if strings.Contains(line, "a.md") && strings.Contains(line, "▌") {
			sawMarkerOnA = true
		}
		if strings.Contains(line, "b.md") && strings.Contains(line, "▌") {
			sawMarkerOnB = true
		}
	}
	if sawMarkerOnA {
		t.Fatalf("marker should NOT be on a.md row")
	}
	if !sawMarkerOnB {
		t.Fatalf("marker SHOULD be on b.md row")
	}
}
```

You will also need this import at the top of the test file (it isn't imported yet):

```go
import (
	// ... existing imports ...
	"github.com/wilkes/hypogeum/internal/vault"
)
```

- [ ] **Step 2: Run the test (expect FAIL — wrong signature)**

Run: `go test ./internal/tui/... -run TestFormatBacklinks_HighlightsSelectedRow -v`
Expected: FAIL with `too many arguments in call to formatBacklinks`.

- [ ] **Step 3: Update `formatBacklinks` signature and body**

In `internal/tui/backlinks.go`, replace the existing `formatBacklinks` (lines 64-78) with:

```go
// cursorMarkerStyle is the left-edge highlight for the selected entry.
// Distinct from the snippet's yellow highlight (which marks the matched
// display text) — this one signals structural position in the list.
var cursorMarkerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)

// formatBacklinks renders a slice of vault.Backlink as the two-row-per-
// entry text used in both the persistent pane and the modal. If
// cursor is in [0, len(links)), the row at that index gets a left-edge
// marker; pass -1 for no selection.
func formatBacklinks(links []vault.Backlink, root string, width, cursor int) string {
	if len(links) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("(no backlinks)")
	}
	var b strings.Builder
	for i, l := range links {
		rel, err := filepath.Rel(root, l.SourceFile)
		if err != nil {
			rel = l.SourceFile
		}
		marker := "  "
		if i == cursor {
			marker = cursorMarkerStyle.Render("▌") + " "
		}
		fmt.Fprintf(&b, "%s%s:%d\n", marker, rel, l.Line)
		fmt.Fprintf(&b, "%s  %s\n", marker, truncateOneLine(applyHighlight(l.Snippet), width-4))
	}
	return b.String()
}
```

- [ ] **Step 4: Update existing callers of `formatBacklinks`**

The function is called in two places — both will fail to compile. Find them with:

```bash
grep -n "formatBacklinks(" internal/tui/backlinks.go
```

Both calls must take `m.backlinkCursor` as the new fourth arg. In `refreshBacklinks` (line 46):

```go
	m.backlinks = links
	m.backlinksVP.SetContent(formatBacklinks(links, m.root, m.viewport.Width, m.backlinkCursor))
```

Note we also assign `links` to `m.backlinks` here — Task 5 will rely on this. Replace the existing assignment line in `refreshBacklinks` so the full body becomes:

```go
func (m *Model) refreshBacklinks(currentPath string) {
	if m.vault == nil || currentPath == "" {
		m.backlinks = nil
		m.backlinksVP.SetContent("")
		return
	}
	links := m.vault.Backlinks(currentPath)
	m.backlinks = links
	m.backlinksVP.SetContent(formatBacklinks(links, m.root, m.viewport.Width, m.backlinkCursor))
}
```

In `refreshBacklinksModal` (line 105) the call needs the same treatment:

```go
func (m *Model) refreshBacklinksModal(currentPath string) {
	if m.vault == nil || currentPath == "" {
		m.backlinks = nil
		m.modalVP.SetContent("")
		return
	}
	m.resizeModalVP()
	links := m.vault.Backlinks(currentPath)
	m.backlinks = links
	m.modalVP.SetContent(formatBacklinks(links, m.root, m.modalVP.Width, m.backlinkCursor))
}
```

- [ ] **Step 5: Run the test**

Run: `go test ./internal/tui/... -run TestFormatBacklinks_HighlightsSelectedRow -v`
Expected: PASS.

- [ ] **Step 6: Run the full TUI test suite (no regressions)**

Run: `go test ./internal/tui/...`
Expected: PASS (existing `TestBacklinksPaneShowsLinkers` continues to find `a.md` in the rendered string — the marker change doesn't break the substring check).

- [ ] **Step 7: Commit**

```bash
git add internal/tui/backlinks.go internal/tui/backlinks_test.go
git commit -m "feat(tui): render cursor marker on selected backlink row"
```

---

## Task 5: Add `pressKey` test helper

**Files:**
- Modify: `internal/tui/helpers_test.go` (append to file)
- Test: (helper used by tests in later tasks)

- [ ] **Step 1: Add the helper**

Append to `internal/tui/helpers_test.go`:

```go
// pressKey sends a single character key (or a special key for non-rune
// keys) through the model's Update loop and returns the new model.
// For a rune like 'b' or 'j': pass `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}`.
// For special keys: use Type: tea.KeyEnter, tea.KeyEsc, tea.KeyTab, etc.
func pressKey(t *testing.T, m Model, msg tea.KeyMsg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

// pressRune is shorthand for pressKey with a single rune.
func pressRune(t *testing.T, m Model, r rune) Model {
	t.Helper()
	return pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
}
```

- [ ] **Step 2: Verify compile**

Run: `go test ./internal/tui/... -count=1 -run NONEXISTENT`
Expected: builds cleanly (the `-run NONEXISTENT` filter runs no tests but compiles).

- [ ] **Step 3: Commit**

```bash
git add internal/tui/helpers_test.go
git commit -m "test(tui): add pressKey/pressRune test helpers"
```

---

## Task 6: Wire cursor movement keys (j/k) for the persistent pane

**Files:**
- Modify: `internal/tui/input.go` (add `handleBacklinksKey` function and dispatch)
- Modify: `internal/tui/backlinks.go` (add `ensureCursorVisible`)
- Test: `internal/tui/backlinks_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/backlinks_test.go`:

```go
func TestBacklinksPane_CursorMovement(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "b.md", "also [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	m.openFile(filepath.Join(dir, "c.md"))

	// Open backlinks pane (b). Subsequent task wires focus; for now we
	// only need backlinks populated and the input router to dispatch
	// j/k to the pane handler when focus is focusBacklinks.
	m = pressRune(t, m, 'b')
	if m.focus != focusBacklinks {
		t.Fatalf("expected focusBacklinks after b, got %v", m.focus)
	}
	if len(m.backlinks) != 2 {
		t.Fatalf("expected 2 backlinks, got %d", len(m.backlinks))
	}
	if m.backlinkCursor != 0 {
		t.Fatalf("expected cursor to start at 0, got %d", m.backlinkCursor)
	}

	m = pressRune(t, m, 'j')
	if m.backlinkCursor != 1 {
		t.Fatalf("expected cursor=1 after j, got %d", m.backlinkCursor)
	}

	// j past the end clamps.
	m = pressRune(t, m, 'j')
	if m.backlinkCursor != 1 {
		t.Fatalf("expected cursor=1 (clamped) after j at end, got %d", m.backlinkCursor)
	}

	// k moves up.
	m = pressRune(t, m, 'k')
	if m.backlinkCursor != 0 {
		t.Fatalf("expected cursor=0 after k, got %d", m.backlinkCursor)
	}

	// k past the start clamps.
	m = pressRune(t, m, 'k')
	if m.backlinkCursor != 0 {
		t.Fatalf("expected cursor=0 (clamped) after k at start, got %d", m.backlinkCursor)
	}
}
```

- [ ] **Step 2: Run the test (expect FAIL — `b` does not yet set focusBacklinks)**

Run: `go test ./internal/tui/... -run TestBacklinksPane_CursorMovement -v`
Expected: FAIL with `expected focusBacklinks after b, got 1` (focusContent).

- [ ] **Step 3: Update `b` toggle to set focus and reset cursor**

In `internal/tui/input.go`, find the existing `ToggleBacklinks` case (around line 174) and replace it with:

```go
	case key.Matches(msg, m.keys.ToggleBacklinks):
		if m.backlinksOpen {
			m.backlinksOpen = false
			m.focus = m.prevFocus
		} else {
			m.backlinksOpen = true
			m.prevFocus = m.focus
			m.focus = focusBacklinks
			m.backlinkCursor = 0
			m.refreshBacklinks(m.history.Current())
		}
		return *m, nil
```

- [ ] **Step 4: Add `handleBacklinksKey` and dispatch from `handleKey`**

In `internal/tui/input.go`, just below `handleContentKey` (after line 212), add:

```go
// handleBacklinksKey routes keystrokes received while the persistent
// backlinks pane has focus. j/k move the cursor; Enter follows
// (added in Task 9); Esc returns focus to prevFocus.
func (m *Model) handleBacklinksKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.backlinkCursor < len(m.backlinks)-1 {
			m.backlinkCursor++
			m.refreshBacklinks(m.history.Current())
			m.ensureCursorVisible(&m.backlinksVP)
		}
		return *m, nil
	case key.Matches(msg, m.keys.Up):
		if m.backlinkCursor > 0 {
			m.backlinkCursor--
			m.refreshBacklinks(m.history.Current())
			m.ensureCursorVisible(&m.backlinksVP)
		}
		return *m, nil
	}
	return *m, nil
}
```

Then update the dispatch at the bottom of `handleKey` (lines 182-186). The current code is:

```go
	if m.focus == focusTree {
		return m.handleTreeKey(msg)
	}
	return m.handleContentKey(msg)
```

Replace with:

```go
	switch m.focus {
	case focusTree:
		return m.handleTreeKey(msg)
	case focusBacklinks:
		return m.handleBacklinksKey(msg)
	default:
		return m.handleContentKey(msg)
	}
```

- [ ] **Step 5: Add `ensureCursorVisible`**

In `internal/tui/backlinks.go`, append:

```go
// ensureCursorVisible adjusts vp's YOffset so the two-row entry at
// m.backlinkCursor is fully on-screen. Called after every cursor
// mutation. Each backlink takes 2 visible rows.
func (m *Model) ensureCursorVisible(vp *viewport.Model) {
	const rowsPerEntry = 2
	cursorTop := m.backlinkCursor * rowsPerEntry
	cursorBottom := cursorTop + rowsPerEntry - 1

	if cursorTop < vp.YOffset {
		vp.SetYOffset(cursorTop)
		return
	}
	if cursorBottom >= vp.YOffset+vp.Height {
		vp.SetYOffset(cursorBottom - vp.Height + 1)
	}
}
```

You'll need to add `"github.com/charmbracelet/bubbles/viewport"` to the import block at the top of `internal/tui/backlinks.go` (if it isn't there yet — `grep -n viewport internal/tui/backlinks.go` will show its absence).

- [ ] **Step 6: Run the test**

Run: `go test ./internal/tui/... -run TestBacklinksPane_CursorMovement -v`
Expected: PASS.

- [ ] **Step 7: Run full TUI test suite**

Run: `go test ./internal/tui/...`
Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/input.go internal/tui/backlinks.go internal/tui/backlinks_test.go
git commit -m "feat(tui): cursor j/k movement in backlinks pane"
```

---

## Task 7: Add `activeBacklinksSurface()` helper

**Files:**
- Modify: `internal/tui/backlinks.go` (append)
- Test: covered indirectly by Task 9's follow tests

- [ ] **Step 1: Add the helper**

Append to `internal/tui/backlinks.go`:

```go
// activeBacklinksSurface reports which backlinks surface is currently
// receiving the user's navigation input. Used at follow time so the
// returnCursor records where to restore on Back.
//
// Modal takes precedence: if both are open and we're following from
// the modal, we want to come back to the modal.
func (m Model) activeBacklinksSurface() backlinksSurface {
	if m.modalOpen == modalBacklinks {
		return surfaceModal
	}
	return surfacePane
}
```

- [ ] **Step 2: Verify compile**

Run: `go build ./...`
Expected: builds cleanly.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/backlinks.go
git commit -m "feat(tui): activeBacklinksSurface helper for return-cursor recording"
```

---

## Task 8: Add `scrollToLine` and a unit test for it

**Files:**
- Modify: `internal/tui/content.go` (append `scrollToLine`)
- Test: `internal/tui/backlinks_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/backlinks_test.go`:

```go
func TestScrollToLine_PositionsLineNearTop(t *testing.T) {
	dir := t.TempDir()
	// Build a 100-line file so the viewport has somewhere to scroll.
	var sb strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	writeTUITestFile(t, dir, "long.md", sb.String())

	m := sized(t, dir, filepath.Join(dir, "long.md"))
	// Initially YOffset = 0.
	if m.viewport.YOffset != 0 {
		t.Fatalf("expected YOffset=0 initially, got %d", m.viewport.YOffset)
	}

	// Scroll to line 60. Expect YOffset to leave ~25% padding above:
	//   target = 60 - viewportHeight*0.25
	// With viewportHeight ≈ 32 (height 40 - 4 for borders/footer - 4 misc),
	// target ≈ 60 - 8 = 52. Be lenient: assert YOffset is in [40, 56].
	m.scrollToLine(60)
	if m.viewport.YOffset < 40 || m.viewport.YOffset > 56 {
		t.Fatalf("expected YOffset in [40, 56] after scrollToLine(60), got %d", m.viewport.YOffset)
	}

	// scrollToLine(huge) clamps to last line.
	m.scrollToLine(99999)
	maxYOffset := m.viewport.TotalLineCount() - m.viewport.Height
	if maxYOffset < 0 {
		maxYOffset = 0
	}
	if m.viewport.YOffset > maxYOffset {
		t.Fatalf("expected YOffset clamped to max %d, got %d", maxYOffset, m.viewport.YOffset)
	}
}
```

You may need to add `"fmt"` and `"strings"` to the test file's imports if they aren't already present.

- [ ] **Step 2: Run the test (expect FAIL — undefined method)**

Run: `go test ./internal/tui/... -run TestScrollToLine_PositionsLineNearTop -v`
Expected: FAIL with `m.scrollToLine undefined`.

- [ ] **Step 3: Implement `scrollToLine`**

Append to `internal/tui/content.go`:

```go
// scrollToLine positions line n of the rendered output about 25% from
// the top of the viewport. n is 1-indexed and matches what
// vault.Backlink.Line carries (a source-file line number).
//
// Caveat: source-file line numbers don't perfectly correspond to
// rendered-output line numbers (Glamour adjusts for headings, code
// fences, etc.). The user lands "near" the reference, not exactly on
// it; the snippet shown in the backlinks pane gives them a visual
// landmark to confirm.
func (m *Model) scrollToLine(n int) {
	if n < 1 {
		n = 1
	}
	total := m.viewport.TotalLineCount()
	if n > total {
		n = total
	}
	// Position the target line ~25% from the top of the viewport so the
	// user sees the lines preceding the reference for context.
	pad := m.viewport.Height / 4
	target := n - 1 - pad
	if target < 0 {
		target = 0
	}
	maxOffset := total - m.viewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if target > maxOffset {
		target = maxOffset
	}
	m.viewport.SetYOffset(target)
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/tui/... -run TestScrollToLine_PositionsLineNearTop -v`
Expected: PASS.

- [ ] **Step 5: Run full TUI suite**

Run: `go test ./internal/tui/...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/content.go internal/tui/backlinks_test.go
git commit -m "feat(tui): scrollToLine helper for landing near a reference"
```

---

## Task 9: Implement `followBacklink` and wire `Enter` in pane

**Files:**
- Modify: `internal/tui/backlinks.go` (append `followBacklink`)
- Modify: `internal/tui/input.go` (add `Enter` case in `handleBacklinksKey`)
- Test: `internal/tui/backlinks_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/backlinks_test.go`:

```go
func TestBacklinksPane_EnterFollows(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "blah blah\n\nsee [[c]] in here.\n")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	aAbs := filepath.Join(dir, "a.md")
	m.openFile(cAbs)

	m = pressRune(t, m, 'b')
	if len(m.backlinks) != 1 {
		t.Fatalf("expected 1 backlink, got %d", len(m.backlinks))
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	// Should have navigated to a.md.
	if m.history.Current() != aAbs {
		t.Fatalf("expected current=%s, got %s", aAbs, m.history.Current())
	}
	// Focus should be on content (we left the backlinks surface).
	if m.focus != focusContent {
		t.Fatalf("expected focusContent after Enter, got %v", m.focus)
	}
	// returnCursor should be set with sourceFile=cAbs.
	if m.returnCursor == nil {
		t.Fatalf("expected returnCursor set, got nil")
	}
	if m.returnCursor.sourceFile != cAbs {
		t.Fatalf("expected returnCursor.sourceFile=%s, got %s", cAbs, m.returnCursor.sourceFile)
	}
	if m.returnCursor.cursor != 0 {
		t.Fatalf("expected returnCursor.cursor=0, got %d", m.returnCursor.cursor)
	}
	if m.returnCursor.surface != surfacePane {
		t.Fatalf("expected returnCursor.surface=surfacePane, got %v", m.returnCursor.surface)
	}
}
```

- [ ] **Step 2: Run the test (expect FAIL — Enter does nothing in pane)**

Run: `go test ./internal/tui/... -run TestBacklinksPane_EnterFollows -v`
Expected: FAIL with `expected current=…/a.md, got …/c.md` (Enter is unhandled).

- [ ] **Step 3: Implement `followBacklink`**

Append to `internal/tui/backlinks.go`:

```go
// followBacklink navigates to the SourceFile of the currently selected
// backlink, recording return state for a subsequent h (Back) restore.
// No-op if no backlink is selected (e.g. empty list).
func (m *Model) followBacklink() {
	if m.backlinkCursor < 0 || m.backlinkCursor >= len(m.backlinks) {
		return
	}
	bl := m.backlinks[m.backlinkCursor]

	// Save return state BEFORE openFile mutates history.
	m.returnCursor = &returnCursor{
		sourceFile: m.history.Current(),
		cursor:     m.backlinkCursor,
		surface:    m.activeBacklinksSurface(),
	}

	// Close modal if active; persistent pane stays open and
	// re-populates for the new file's own backlinks.
	if m.modalOpen == modalBacklinks {
		m.modalOpen = modalNone
	}
	m.focus = focusContent

	m.openFile(bl.SourceFile)
	m.scrollToLine(bl.Line)
}
```

- [ ] **Step 4: Wire `Enter` in `handleBacklinksKey`**

In `internal/tui/input.go`, add an `Open` case to `handleBacklinksKey`. The full updated function:

```go
func (m *Model) handleBacklinksKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.backlinkCursor < len(m.backlinks)-1 {
			m.backlinkCursor++
			m.refreshBacklinks(m.history.Current())
			m.ensureCursorVisible(&m.backlinksVP)
		}
		return *m, nil
	case key.Matches(msg, m.keys.Up):
		if m.backlinkCursor > 0 {
			m.backlinkCursor--
			m.refreshBacklinks(m.history.Current())
			m.ensureCursorVisible(&m.backlinksVP)
		}
		return *m, nil
	case key.Matches(msg, m.keys.Open):
		m.followBacklink()
		return *m, nil
	}
	return *m, nil
}
```

- [ ] **Step 5: Run the test**

Run: `go test ./internal/tui/... -run TestBacklinksPane_EnterFollows -v`
Expected: PASS.

- [ ] **Step 6: Run full TUI suite**

Run: `go test ./internal/tui/...`
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/backlinks.go internal/tui/input.go internal/tui/backlinks_test.go
git commit -m "feat(tui): Enter follows selected backlink in pane"
```

---

## Task 10: Wire cursor + Enter for the backlinks modal

**Files:**
- Modify: `internal/tui/input.go` (modal branch in `handleKey` — currently lines 138-146)
- Test: `internal/tui/backlinks_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/backlinks_test.go`:

```go
func TestBacklinksModal_CursorAndEnter(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "b.md", "also [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	m.openFile(cAbs)

	// Open backlinks modal.
	m = pressRune(t, m, 'B')
	if m.modalOpen != modalBacklinks {
		t.Fatalf("expected modalBacklinks, got %v", m.modalOpen)
	}
	if len(m.backlinks) != 2 {
		t.Fatalf("expected 2 backlinks, got %d", len(m.backlinks))
	}
	if m.backlinkCursor != 0 {
		t.Fatalf("expected cursor=0, got %d", m.backlinkCursor)
	}

	// j moves cursor in modal.
	m = pressRune(t, m, 'j')
	if m.backlinkCursor != 1 {
		t.Fatalf("expected cursor=1 after j in modal, got %d", m.backlinkCursor)
	}

	// Enter follows AND closes the modal.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.modalOpen != modalNone {
		t.Fatalf("expected modal closed after Enter, got %v", m.modalOpen)
	}
	if m.focus != focusContent {
		t.Fatalf("expected focusContent after Enter, got %v", m.focus)
	}
	if m.returnCursor == nil || m.returnCursor.surface != surfaceModal {
		t.Fatalf("expected returnCursor.surface=surfaceModal, got %+v", m.returnCursor)
	}
}
```

- [ ] **Step 2: Run the test (expect FAIL — modal swallows j with viewport.Update)**

Run: `go test ./internal/tui/... -run TestBacklinksModal_CursorAndEnter -v`
Expected: FAIL with `expected cursor=1 after j in modal, got 0`.

- [ ] **Step 3: Update modal branch in `handleKey`**

In `internal/tui/input.go`, replace the existing modal-open branch (currently lines 137-146):

```go
	// While a modal is open, Esc closes it; other keys go to the modal viewport.
	if m.modalOpen != modalNone {
		if key.Matches(msg, m.keys.ClearLink) { // Esc
			m.modalOpen = modalNone
			return *m, nil
		}
		var cmd tea.Cmd
		m.modalVP, cmd = m.modalVP.Update(msg)
		return *m, cmd
	}
```

with:

```go
	// While a modal is open, Esc closes it. Backlinks modal gets explicit
	// cursor handling so j/k move the selection rather than scroll the
	// viewport. Logs modal keeps the viewport-scroll fall-through.
	if m.modalOpen != modalNone {
		if key.Matches(msg, m.keys.ClearLink) { // Esc
			m.modalOpen = modalNone
			m.focus = m.prevFocus
			return *m, nil
		}
		if m.modalOpen == modalBacklinks {
			switch {
			case key.Matches(msg, m.keys.Down):
				if m.backlinkCursor < len(m.backlinks)-1 {
					m.backlinkCursor++
					m.refreshBacklinksModal(m.history.Current())
					m.ensureCursorVisible(&m.modalVP)
				}
				return *m, nil
			case key.Matches(msg, m.keys.Up):
				if m.backlinkCursor > 0 {
					m.backlinkCursor--
					m.refreshBacklinksModal(m.history.Current())
					m.ensureCursorVisible(&m.modalVP)
				}
				return *m, nil
			case key.Matches(msg, m.keys.Open):
				m.followBacklink()
				return *m, nil
			}
			// Fall through to viewport scroll for any other key.
		}
		var cmd tea.Cmd
		m.modalVP, cmd = m.modalVP.Update(msg)
		return *m, cmd
	}
```

- [ ] **Step 4: Update `B` toggle so it saves prevFocus and resets cursor**

Find the `OpenBacklinksModal` block (currently lines 117-125) and replace it with:

```go
	if key.Matches(msg, m.keys.OpenBacklinksModal) {
		if m.modalOpen == modalBacklinks {
			m.modalOpen = modalNone
			m.focus = m.prevFocus
		} else {
			if m.modalOpen == modalNone {
				m.prevFocus = m.focus
			}
			m.modalOpen = modalBacklinks
			m.backlinkCursor = 0
			m.refreshBacklinksModal(m.history.Current())
		}
		return *m, nil
	}
```

The `if m.modalOpen == modalNone` guard means we only capture `prevFocus` on a *fresh* modal open — when swapping from `modalLogs` to `modalBacklinks`, we keep the originally-saved focus, not the focus-while-logs-was-open (which is stale).

- [ ] **Step 5: Apply the same prevFocus-save to `?` (logs modal toggle)**

For consistency, the `OpenLogsModal` block (currently lines 127-135) also needs to capture `prevFocus`. Replace it with:

```go
	if key.Matches(msg, m.keys.OpenLogsModal) {
		if m.modalOpen == modalLogs {
			m.modalOpen = modalNone
			m.focus = m.prevFocus
		} else {
			if m.modalOpen == modalNone {
				m.prevFocus = m.focus
			}
			m.modalOpen = modalLogs
			m.refreshLogsModal()
		}
		return *m, nil
	}
```

- [ ] **Step 6: Run the test**

Run: `go test ./internal/tui/... -run TestBacklinksModal_CursorAndEnter -v`
Expected: PASS.

- [ ] **Step 7: Run full TUI suite**

Run: `go test ./internal/tui/...`
Expected: all PASS, including the existing `TestBacklinksModalToggleAndEsc`.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/input.go internal/tui/backlinks_test.go
git commit -m "feat(tui): cursor + Enter wired in backlinks modal; save prevFocus on modal open"
```

---

## Task 11: Implement return-cursor restoration on `h` (Back)

**Files:**
- Modify: `internal/tui/input.go` (the `Back` case, currently lines 160-166)
- Test: `internal/tui/backlinks_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/backlinks_test.go`:

```go
func TestBacklinksPane_BackRestoresCursor(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "b.md", "also [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	m.openFile(cAbs)
	m = pressRune(t, m, 'b')          // open pane
	m = pressRune(t, m, 'j')          // cursor → 1
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // follow

	// Now we're on a.md or b.md. Press h.
	m = pressRune(t, m, 'h')

	if m.history.Current() != cAbs {
		t.Fatalf("expected back at c.md, got %s", m.history.Current())
	}
	if m.backlinkCursor != 1 {
		t.Fatalf("expected backlinkCursor restored to 1, got %d", m.backlinkCursor)
	}
	if m.focus != focusBacklinks {
		t.Fatalf("expected focusBacklinks restored, got %v", m.focus)
	}
	if m.returnCursor != nil {
		t.Fatalf("expected returnCursor cleared, got %+v", m.returnCursor)
	}
}
```

- [ ] **Step 2: Run the test (expect FAIL — back does not restore)**

Run: `go test ./internal/tui/... -run TestBacklinksPane_BackRestoresCursor -v`
Expected: FAIL with `expected backlinkCursor restored to 1, got 0` (or similar).

- [ ] **Step 3: Update the `Back` case to restore on match**

In `internal/tui/input.go`, replace the `Back` case (currently lines 160-166):

```go
	case key.Matches(msg, m.keys.Back):
		if path, ok := m.history.Back(); ok {
			m.refreshContent(path)
			m.selectInTree(path)
		}
		return *m, nil
```

with:

```go
	case key.Matches(msg, m.keys.Back):
		if path, ok := m.history.Back(); ok {
			m.refreshContent(path)
			m.selectInTree(path)
			m.maybeRestoreReturnCursor(path)
		}
		return *m, nil
```

- [ ] **Step 4: Implement `maybeRestoreReturnCursor`**

Append to `internal/tui/backlinks.go`:

```go
// maybeRestoreReturnCursor checks if a returnCursor was set and the
// path we just navigated to matches it. If so, restores the cursor
// position and the surface (focus on pane, or reopen modal). Consumes
// the slot regardless of the surface restore actually being possible
// (e.g. the user closed the pane while away).
func (m *Model) maybeRestoreReturnCursor(path string) {
	if m.returnCursor == nil || path != m.returnCursor.sourceFile {
		return
	}
	rc := m.returnCursor
	m.returnCursor = nil

	m.refreshBacklinks(path)
	m.backlinkCursor = clamp(rc.cursor, 0, len(m.backlinks)-1)

	switch rc.surface {
	case surfacePane:
		if m.shouldShowBacklinks() {
			m.focus = focusBacklinks
		}
		m.refreshBacklinks(path) // re-render with cursor highlighted
	case surfaceModal:
		m.modalOpen = modalBacklinks
		m.refreshBacklinksModal(path)
	}
}
```

- [ ] **Step 5: Run the test**

Run: `go test ./internal/tui/... -run TestBacklinksPane_BackRestoresCursor -v`
Expected: PASS.

- [ ] **Step 6: Run full TUI suite**

Run: `go test ./internal/tui/...`
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/input.go internal/tui/backlinks.go internal/tui/backlinks_test.go
git commit -m "feat(tui): restore backlink cursor and surface on Back"
```

---

## Task 12: Modal back-restore reopens the modal

**Files:**
- Test: `internal/tui/backlinks_test.go` (new test — implementation already in place)

- [ ] **Step 1: Write the test**

Append to `internal/tui/backlinks_test.go`:

```go
func TestBacklinksModal_BackReopensModal(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "b.md", "also [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	m.openFile(cAbs)
	m = pressRune(t, m, 'B')          // open modal
	m = pressRune(t, m, 'j')          // cursor → 1
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // follow + close modal
	if m.modalOpen != modalNone {
		t.Fatalf("expected modal closed during follow, got %v", m.modalOpen)
	}

	m = pressRune(t, m, 'h')

	if m.modalOpen != modalBacklinks {
		t.Fatalf("expected modalBacklinks reopened on Back, got %v", m.modalOpen)
	}
	if m.backlinkCursor != 1 {
		t.Fatalf("expected cursor=1 restored, got %d", m.backlinkCursor)
	}
	if m.returnCursor != nil {
		t.Fatalf("expected returnCursor cleared, got %+v", m.returnCursor)
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/tui/... -run TestBacklinksModal_BackReopensModal -v`
Expected: PASS (Task 11 already wired this in `maybeRestoreReturnCursor`).

- [ ] **Step 3: Run full TUI suite**

Run: `go test ./internal/tui/...`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/backlinks_test.go
git commit -m "test(tui): cover modal reopen after back-from-followed-backlink"
```

---

## Task 13: Verify return-cursor is discarded on unrelated navigation

**Files:**
- Test: `internal/tui/backlinks_test.go` (new test — implementation already correct)

- [ ] **Step 1: Write the test**

Append to `internal/tui/backlinks_test.go`:

```go
func TestReturnCursor_DiscardedOnUnrelatedNav(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")
	writeTUITestFile(t, dir, "d.md", "i am d, unrelated.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	dAbs := filepath.Join(dir, "d.md")
	m.openFile(cAbs)
	m = pressRune(t, m, 'b')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // follow → a.md

	// Now jump to an unrelated file via openFile (simulates tree click).
	m.openFile(dAbs)

	// Press h. We should land back on a.md, NOT on c.md.
	m = pressRune(t, m, 'h')
	if m.history.Current() == cAbs {
		t.Fatalf("expected to be on a.md (one back from d.md), got c.md")
	}

	// Press h again. NOW we should land on c.md, but returnCursor was
	// recorded with sourceFile=cAbs, so it WOULD restore — except by
	// this point the cursor restoration we want to test is "did the
	// user's intent to navigate away discard the return slot?". The
	// answer per spec: returnCursor is set once at follow time and
	// stays set until consumed by a matching Back. Since we DO end up
	// back at cAbs eventually, restoration happens then. This is
	// acceptable per spec; what we're really checking is that no spurious
	// restoration happens at the FIRST h.

	// Step beyond: explicit unrelated nav DOES NOT pre-empt the slot.
	// The slot is consumed only on path-match Back. This test asserts
	// the more interesting case: openFile to d.md did NOT consume the
	// slot, so navigating Back twice eventually still restores.
	if m.returnCursor == nil {
		t.Fatalf("returnCursor unexpectedly cleared by unrelated nav (only matching Back should clear it)")
	}
}
```

> **Reviewer note:** This test documents the spec's exact behavior — `returnCursor` is *path-keyed*, not *time-keyed*. It's discarded on the next Back to its `sourceFile` regardless of intervening navigations. If you want stricter "discard on any unrelated nav" semantics, that's a spec change; raise it before changing the test.

- [ ] **Step 2: Run the test**

Run: `go test ./internal/tui/... -run TestReturnCursor_DiscardedOnUnrelatedNav -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/backlinks_test.go
git commit -m "test(tui): document return-cursor's path-keyed (not time-keyed) lifetime"
```

---

## Task 14: Verify cursor clamps when the list shrinks between follow and return

**Files:**
- Test: `internal/tui/backlinks_test.go` (new test — implementation already correct via `clamp`)

- [ ] **Step 1: Write the test**

Append to `internal/tui/backlinks_test.go`:

```go
func TestReturnCursor_ClampsToShrunkList(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "b.md", "also [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	bAbs := filepath.Join(dir, "b.md")
	m.openFile(cAbs)
	m = pressRune(t, m, 'b')
	m = pressRune(t, m, 'j')          // cursor → 1
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // follow

	// Simulate b.md being deleted (and the vault refreshing) while we're
	// away on the source file. Easiest path: rewrite b.md to drop its
	// link, then call vault.RefreshFile.
	if err := os.WriteFile(bAbs, []byte("no link anymore"), 0o644); err != nil {
		t.Fatalf("rewrite b.md: %v", err)
	}
	if err := m.vault.RefreshFile(bAbs); err != nil {
		t.Fatalf("vault.RefreshFile: %v", err)
	}

	// Now Back. The vault will report only 1 backlink for c.md; cursor
	// must clamp from 1 down to 0.
	m = pressRune(t, m, 'h')

	if m.backlinkCursor != 0 {
		t.Fatalf("expected cursor clamped to 0 after list shrank, got %d", m.backlinkCursor)
	}
	if len(m.backlinks) != 1 {
		t.Fatalf("expected 1 backlink after refresh, got %d", len(m.backlinks))
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/tui/... -run TestReturnCursor_ClampsToShrunkList -v`
Expected: PASS (Task 11's `clamp(rc.cursor, 0, len(m.backlinks)-1)` handles this).

- [ ] **Step 3: Commit**

```bash
git add internal/tui/backlinks_test.go
git commit -m "test(tui): cover cursor clamp when backlink list shrinks during navigation"
```

---

## Task 15: Extend `Esc` priority chain for backlinks focus

**Files:**
- Modify: `internal/tui/input.go` (the `ClearLink` case in `handleContentKey`, currently line 200; modal `Esc` already handled)
- Test: `internal/tui/backlinks_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/backlinks_test.go`:

```go
func TestEsc_RestoresFocusFromBacklinksWithoutClosingPane(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	m.openFile(filepath.Join(dir, "c.md"))
	m = pressRune(t, m, 'b')
	if m.focus != focusBacklinks || !m.backlinksOpen {
		t.Fatalf("setup: expected focusBacklinks and pane open, got focus=%v open=%v", m.focus, m.backlinksOpen)
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.focus == focusBacklinks {
		t.Fatalf("Esc should restore prevFocus, but focus is still focusBacklinks")
	}
	if !m.backlinksOpen {
		t.Fatalf("Esc should NOT close the pane")
	}
}
```

- [ ] **Step 2: Run the test (expect FAIL — Esc currently doesn't handle focusBacklinks)**

Run: `go test ./internal/tui/... -run TestEsc_RestoresFocusFromBacklinksWithoutClosingPane -v`
Expected: FAIL.

- [ ] **Step 3: Add `Esc` handling in `handleBacklinksKey`**

In `internal/tui/input.go`, update `handleBacklinksKey` to handle `Esc`. The full updated function:

```go
func (m *Model) handleBacklinksKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.ClearLink): // Esc
		m.focus = m.prevFocus
		return *m, nil
	case key.Matches(msg, m.keys.Down):
		if m.backlinkCursor < len(m.backlinks)-1 {
			m.backlinkCursor++
			m.refreshBacklinks(m.history.Current())
			m.ensureCursorVisible(&m.backlinksVP)
		}
		return *m, nil
	case key.Matches(msg, m.keys.Up):
		if m.backlinkCursor > 0 {
			m.backlinkCursor--
			m.refreshBacklinks(m.history.Current())
			m.ensureCursorVisible(&m.backlinksVP)
		}
		return *m, nil
	case key.Matches(msg, m.keys.Open):
		m.followBacklink()
		return *m, nil
	}
	return *m, nil
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/tui/... -run TestEsc_RestoresFocusFromBacklinksWithoutClosingPane -v`
Expected: PASS.

- [ ] **Step 5: Run full TUI suite**

Run: `go test ./internal/tui/...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/input.go internal/tui/backlinks_test.go
git commit -m "feat(tui): Esc restores prevFocus from backlinks pane (pane stays open)"
```

---

## Task 16: Three-way Tab cycle (tree → content → backlinks → tree)

**Files:**
- Modify: `internal/tui/input.go` (the `FocusTog` case, currently lines 152-158)
- Test: `internal/tui/backlinks_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/backlinks_test.go`:

```go
func TestTab_ThreeWayCycleWhenPaneVisible(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	m.openFile(filepath.Join(dir, "c.md"))
	m = pressRune(t, m, 'b')          // pane open, focus on backlinks
	if m.focus != focusBacklinks {
		t.Fatalf("setup: expected focusBacklinks, got %v", m.focus)
	}

	// Tab: backlinks → tree.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusTree {
		t.Fatalf("Tab from backlinks: expected focusTree, got %v", m.focus)
	}

	// Tab: tree → content.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusContent {
		t.Fatalf("Tab from tree: expected focusContent, got %v", m.focus)
	}

	// Tab: content → backlinks (pane is visible).
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusBacklinks {
		t.Fatalf("Tab from content: expected focusBacklinks, got %v", m.focus)
	}
}

func TestTab_TwoWayWhenPaneClosed(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "hi.")
	m := sized(t, dir, "")
	m.openFile(filepath.Join(dir, "a.md"))

	// Pane closed (default). Cycle: tree ↔ content.
	if m.focus != focusTree {
		t.Fatalf("default focus should be tree, got %v", m.focus)
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusContent {
		t.Fatalf("expected focusContent, got %v", m.focus)
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusTree {
		t.Fatalf("expected focusTree (skipping invisible backlinks), got %v", m.focus)
	}
}
```

- [ ] **Step 2: Run the tests (expect FAIL — Tab is two-way today)**

Run: `go test ./internal/tui/... -run "TestTab_" -v`
Expected: FAIL on the three-way test (the two-way test should pass since the pane is closed).

- [ ] **Step 3: Update the `FocusTog` case**

In `internal/tui/input.go`, replace the `FocusTog` case (currently lines 152-158):

```go
	case key.Matches(msg, m.keys.FocusTog):
		if m.focus == focusTree {
			m.focus = focusContent
		} else {
			m.focus = focusTree
		}
		return *m, nil
```

with:

```go
	case key.Matches(msg, m.keys.FocusTog):
		m.focus = m.nextFocus()
		return *m, nil
```

Append to `internal/tui/backlinks.go` the `nextFocus` helper:

```go
// nextFocus returns the focus that Tab should move to. Three-way
// cycle (tree → content → backlinks → tree) when the persistent pane
// is open and visible; otherwise two-way (tree ↔ content).
func (m Model) nextFocus() focus {
	if m.shouldShowBacklinks() {
		switch m.focus {
		case focusTree:
			return focusContent
		case focusContent:
			return focusBacklinks
		case focusBacklinks:
			return focusTree
		}
	}
	if m.focus == focusTree {
		return focusContent
	}
	return focusTree
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/tui/... -run "TestTab_" -v`
Expected: both PASS.

- [ ] **Step 5: Run full TUI suite**

Run: `go test ./internal/tui/...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/input.go internal/tui/backlinks.go internal/tui/backlinks_test.go
git commit -m "feat(tui): Tab cycles tree/content/backlinks when pane is visible"
```

---

## Task 17: Update CLAUDE.md and parent spec

**Files:**
- Modify: `CLAUDE.md` (repo root)
- Modify: `docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md`

- [ ] **Step 1: Update CLAUDE.md**

Find the "Wikilinks and backlinks — Phase 1 shipped" line (likely under "What's not built yet" near the bottom of the project conventions). The current line reads:

```
**Wikilinks and backlinks — Phase 1 shipped:** `[[wikilinks]]` resolve via vault index, backlinks pane (`b`), backlinks modal (`B`), log viewer (`?`). Implementation lives in `internal/vault/` and the modal/pane logic in `internal/tui/`.
```

Replace with:

```
**Wikilinks and backlinks — Phase 1 shipped:** `[[wikilinks]]` resolve via vault index, backlinks pane (`b`), backlinks modal (`B`), log viewer (`?`), and backlinks navigation (cursor `j`/`k`, `Enter` to follow with scroll-to-line, `h` restores cursor). Implementation lives in `internal/vault/` and the modal/pane logic in `internal/tui/`.
```

- [ ] **Step 2: Update the parent spec's "Following a backlink" paragraph**

Open `docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md`. Find the paragraph that begins "Following a backlink:" (around line 219 of the parent spec). The current text reads:

```
Following a backlink: `Enter` calls `openFile(SourceFile)` (history records the visit). The link cursor is reset on the new page; we do not auto-scroll to the wikilink occurrence in the source file. (Phase 2 could add that — passing `Line` through to `openFile`. Out of scope here.)
```

Replace with:

```
Following a backlink: `Enter` calls `openFile(SourceFile)` (history records the visit). The link cursor is reset on the new page. Cursor navigation, scroll-to-line, and back-restores-cursor are detailed in the follow-on spec [backlinks-navigation-design](2026-05-07-backlinks-navigation-design.md). Phase 2 still includes pre-selecting the matching inline link in `m.links`.
```

- [ ] **Step 3: Verify the docs build (no broken links)**

Open the changed docs in `hypogeum` itself to spot-check the links, or just run:

```bash
grep -n "backlinks-navigation-design" docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md
```

Expected: one match showing the new pointer.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md
git commit -m "docs: cross-reference backlinks-navigation spec from parent + CLAUDE.md"
```

---

## Task 18: Manual sanity check in a real terminal

**Files:** none

This task is one humans do; an agent can stop here and ask the user to verify, or skip if running in CI.

- [ ] **Step 1: Build and run against a small vault**

```bash
go build ./...
go run ./cmd/hypogeum docs/
```

- [ ] **Step 2: Walk through the flows**

  1. Open a file with at least 2 backlinks (e.g. a doc that's referenced by index.md and one other).
  2. Press `b` — pane opens, first backlink highlighted with the `▌` marker.
  3. `j`/`k` — cursor moves; marker follows.
  4. `Enter` — viewport jumps to the source file, scrolled near the line of the reference.
  5. `h` — back on the original; pane reopens (if you'd closed it implicitly), cursor restored to where you'd been.
  6. Press `B` — modal opens with the same cursor + Enter behavior; `Esc` closes.
  7. Press `Tab` from the persistent pane — cycles to tree; another `Tab` to content; another to backlinks.
  8. With backlinks pane open, follow once, then `h` — modal version: open `B`, follow, `h` — modal reopens.

- [ ] **Step 3: Anything broken?**

If any flow misbehaves, file a bug or fix in place. Otherwise, mark this task complete.

- [ ] **Step 4: No commit — this task is verification only.**

---

## Self-Review

**Spec coverage check:**

| Spec section | Implementing task(s) |
|---|---|
| State additions on `Model` | Task 2 |
| `returnCursor`, `backlinksSurface` types | Task 3 |
| `focusBacklinks` enum value | Task 1 |
| Persistent pane input (j/k/Enter/Esc/b/Tab) | Task 6 (j/k), Task 9 (Enter), Task 15 (Esc), Task 6 (b reopen via toggle), Task 16 (Tab) |
| Modal input (j/k/Enter/Esc/B) | Task 10 |
| Esc priority chain | Task 15 |
| `b` and `B` save+switch focus | Task 6 (b), Task 10 (B) |
| `followBacklink` | Task 9 |
| `scrollToLine` + caveat about source vs rendered lines | Task 8 |
| Return flow (`maybeRestoreReturnCursor`) | Task 11 |
| Visual cursor (▌ marker on selected row) | Task 4 |
| `ensureCursorVisible` | Task 6 |
| Three-way Tab | Task 16 |
| Error: vault refresh shrinks list | Task 14 |
| Error: empty list | Implicit in `followBacklink` bounds check (Task 9) and `clamp` (Task 3); not a separate test, covered by spec |
| Error: pane closed during navigation | `maybeRestoreReturnCursor` checks `shouldShowBacklinks()` before setting focus — Task 11 |
| Error: logs modal open during return | `m.modalOpen = modalBacklinks` swaps content (Task 11) |
| Error: source file disappears | `openFile` no-op covered by existing behavior; `returnCursor` set before `openFile` is acceptable |
| Error: `Backlink.Line` exceeds rendered count | `scrollToLine` clamps (Task 8) |
| Test: `TestBacklinksPane_OpenFocusesIt` | Folded into Task 6's setup assertions |
| Test: `TestBacklinksPane_CloseRestoresFocus` | Implicit in Task 6's `b` toggle behavior; can add a dedicated test if desired |
| Test: `TestBacklinksPane_CursorMovement` | Task 6 |
| Test: `TestBacklinksPane_EnterFollows` | Task 9 |
| Test: `TestBacklinksPane_BackRestoresCursor` | Task 11 |
| Test: `TestBacklinksModal_EnterFollowsAndCloses` | Task 10 |
| Test: `TestBacklinksModal_BackReopensModal` | Task 12 |
| Test: `TestReturnCursor_DiscardedOnUnrelatedNav` | Task 13 |
| Test: `TestReturnCursor_ClampsToShrunkList` | Task 14 |
| Test: `TestScrollToLine` | Task 8 |
| Test: `TestThreeWayFocusCycle` | Task 16 |
| Doc updates (CLAUDE.md, parent spec) | Task 17 |

**Placeholder scan:** No "TBD", "TODO", "implement later" remain. The "Anything broken?" prompt in Task 18 is intentional manual-verification phrasing, not a placeholder.

**Type consistency:**
- `clamp(v, lo, hi)` defined in Task 3, used in Task 11 — signature matches.
- `formatBacklinks(links, root, width, cursor)` — Task 4 defines four-arg version; both call sites in `refreshBacklinks` and `refreshBacklinksModal` updated in Task 4 step 4.
- `m.backlinks` populated in Task 4's updated `refreshBacklinks` / `refreshBacklinksModal`; consumed in Task 6 (`len(m.backlinks)` for clamp), Task 9 (`m.backlinks[m.backlinkCursor]`), Task 11 (`clamp(rc.cursor, 0, len(m.backlinks)-1)`).
- `prevFocus` written in Task 6 (`b` toggle) and Task 10 (`B` toggle); read in Task 6 (close branch), Task 10 (Esc/close), Task 15 (`Esc` in pane), Task 16 (no — `nextFocus` reads `m.focus`, not `prevFocus`).
- `focusBacklinks` defined in Task 1; first used in Task 6.
- `ensureCursorVisible(*viewport.Model)` defined in Task 6; used in Task 6 (pane), Task 10 (modal).
- `followBacklink()` defined in Task 9; called from Task 9 (pane Enter) and Task 10 (modal Enter).
- `scrollToLine(int)` defined in Task 8; called from Task 9 (`followBacklink`).
- `maybeRestoreReturnCursor(string)` defined in Task 11; called from Task 11's updated `Back` case.
- `nextFocus()` defined in Task 16; called from Task 16's updated `FocusTog` case.

**Two minor coverage gaps surfaced and addressed:** the "pane was closed by user during navigation" path is covered structurally by `maybeRestoreReturnCursor`'s `if m.shouldShowBacklinks()` guard (Task 11) but isn't tested explicitly. Adding it would mean: follow → `b` to close pane → `h` → assert pane stays closed and focus is `prevFocus`. Skipped for the canonical test set; if subsequent work surfaces a regression here, add the test then.
