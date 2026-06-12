# Drag-to-select with auto-copy — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user select text in the rendered content pane with a mouse drag and have it copied to the clipboard automatically on release, with the selection staying highlighted and a "Copied N chars" footer toast.

**Architecture:** hypogeum owns the mouse (`tea.WithMouseCellMotion()`), so selection is app-drawn. A `selection` value on `contentUIState` tracks an anchor/cursor in absolute (line, col) document coordinates. The mouse lifecycle (press → motion* → release) is handled in `handleMouse`; the first motion turns a click into a drag (so a press-then-release still follows a link). Text extraction and the reverse-video highlight overlay are computed over the stored rendered output using `charmbracelet/x/ansi` column-aware `Cut`/`Strip`. Copy goes through an injected `clipboardWriter` (default `termenv.Copy`, OSC 52).

**Tech Stack:** Go, Bubble Tea v1.3.4, Lip Gloss, `github.com/charmbracelet/x/ansi` (already a direct dep), `github.com/muesli/termenv` (already a direct dep).

**Design spec:** [docs/superpowers/specs/2026-06-12-drag-to-select-copy-design.md](../specs/2026-06-12-drag-to-select-copy-design.md)

---

## File Structure

- `internal/tui/clipboard.go` — **new**: `clipboardWriter` type + `defaultClipboardWriter` (termenv.Copy).
- `internal/tui/model.go` — **modify**: add `copyToClipboard clipboardWriter` field to `Model`; default it in `New`.
- `internal/tui/content.go` — **modify**: add `rendered string` + `selection` to `contentUIState`; add `setContent`, `contentLines`, `screenToContent`, `extractSelection`, `applySelectionHighlight`, `clearSelection`, `resetSelectionState`, and the `cellPos`/`selection`/`normalizeSel`/`selColBounds` helpers; route real-content `SetContent` calls through `setContent`; reset selection state at the top of `refreshContent`.
- `internal/tui/links.go` — **modify**: route `applyLinkHighlight`'s `SetContent` through `setContent`.
- `internal/tui/input.go` — **modify**: extend `handleMouse` to dispatch motion/release and start/extend/finalize a selection; clear selection on key input.
- Tests: `internal/tui/selection_test.go` (**new**) for the pure helpers and the mouse-lifecycle integration tests.

Conventions to follow (from existing code): injected-seam pattern mirrors `openExternal` in `external.go`; tests build a model with `sized(t, root, "")` and drive it with `tea.MouseMsg` / `tea.KeyMsg` through `m.Update`; fixtures via `writeFixture(t)`.

---

## Task 1: Clipboard seam

**Files:**
- Create: `internal/tui/clipboard.go`
- Modify: `internal/tui/model.go` (add field + default in `New`)
- Test: `internal/tui/selection_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/tui/selection_test.go`:

```go
package tui

import "testing"

func TestModel_CopyToClipboard_DefaultIsSet(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	if m.copyToClipboard == nil {
		t.Fatal("copyToClipboard should default to a non-nil writer")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_CopyToClipboard_DefaultIsSet -v`
Expected: compile failure — `m.copyToClipboard undefined`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/tui/clipboard.go`:

```go
package tui

import "github.com/muesli/termenv"

// clipboardWriter copies text to the system clipboard. Injected on the
// Model (mirroring openExternal) so tests can record calls instead of
// emitting a real OSC 52 escape sequence to the terminal.
type clipboardWriter func(text string)

// defaultClipboardWriter copies via termenv.Copy, which emits an OSC 52
// escape. OSC 52 copies through the terminal, so it works over SSH and
// inside tmux with no pbcopy/xclip dependency. It returns no error;
// the persistent selection highlight and footer toast are the
// user-visible confirmation that a copy was attempted.
func defaultClipboardWriter(text string) { termenv.Copy(text) }
```

In `internal/tui/model.go`, add the field to the `Model` struct (next to `openExternal`):

```go
	// copyToClipboard writes selected text to the clipboard. Injected so
	// tests record calls instead of emitting a real OSC 52 escape.
	copyToClipboard clipboardWriter
```

And in `New`, set the default in the `Model{...}` literal (next to `openExternal: openExternalURL,`):

```go
		copyToClipboard: defaultClipboardWriter,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestModel_CopyToClipboard_DefaultIsSet -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/clipboard.go internal/tui/model.go internal/tui/selection_test.go
git commit -m "feat(tui): inject clipboardWriter seam (OSC 52 via termenv)"
```

---

## Task 2: Selection state + base-content chokepoint

Introduce the data types and the `setContent` chokepoint that records the rendered output so the selection overlay has a stable base to draw on.

**Files:**
- Modify: `internal/tui/content.go` (struct fields, `setContent`, `contentLines`, route SetContent calls, reset on refresh)
- Modify: `internal/tui/links.go` (`applyLinkHighlight` routes through `setContent`)
- Test: `internal/tui/selection_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/selection_test.go`:

```go
import "strings" // add to the import block

func TestModel_RenderedBaseIsStored(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md")) // open index.md
	if m.content.rendered == "" {
		t.Fatal("content.rendered should hold the rendered output after open")
	}
	if !strings.Contains(stripANSItest(m.content.rendered), "Index") {
		t.Errorf("rendered base should contain the heading text; got %q",
			stripANSItest(m.content.rendered))
	}
}
```

Add the helpers + imports at the top of the test file (keep one import block):

```go
import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func stripANSItest(s string) string { return ansi.Strip(s) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_RenderedBaseIsStored -v`
Expected: compile failure — `m.content.rendered undefined`.

- [ ] **Step 3: Write minimal implementation**

In `internal/tui/content.go`, add the types above `contentUIState` and extend the struct:

```go
// cellPos is a position in the rendered content: an absolute line index
// (independent of viewport scroll) and a visible column (0-based).
type cellPos struct {
	line int
	col  int
}

// selection tracks an in-progress or finalized text selection in the
// content pane, in absolute document coordinates.
type selection struct {
	anchored    bool    // a left-press landed in the content pane
	moved       bool    // motion seen since the press (click-vs-drag)
	copied      bool    // released with text → highlight persists
	anchor      cellPos // where the drag started
	cursor      cellPos // current / final drag point
	pendingLink int     // link index under the press, or -1
}
```

Add two fields to `contentUIState`:

```go
	// rendered is the last full content string handed to the viewport
	// (link-highlight included). The selection overlay is drawn on top
	// of it and recomputed on every motion without re-running Glamour.
	rendered string
	// selection is the current content-pane text selection.
	selection selection
```

Add the chokepoint helper (anywhere in `content.go`):

```go
// setContent stores s as the selection overlay's base and hands it to
// the viewport. Every code path that displays real rendered content
// must go through here so content.rendered stays in sync with what the
// viewport shows.
func (m *Model) setContent(s string) {
	m.content.rendered = s
	m.content.viewport.SetContent(s)
}

// contentLines splits the stored base render into display lines.
func (m *Model) contentLines() []string {
	return strings.Split(m.content.rendered, "\n")
}
```

Replace the **real-content** `m.content.viewport.SetContent(out)` calls with `m.setContent(out)`:
- `internal/tui/content.go:142` (code/dir branch): `m.setContent(out)`
- `internal/tui/content.go:178` (markdown branch): `m.setContent(out)`

In `internal/tui/links.go`, inside `applyLinkHighlight`, replace `m.content.viewport.SetContent(out)` with `m.setContent(out)` (keep the surrounding `offset := ...` / `SetYOffset(offset)`).

At the **top** of `refreshContent` (in `content.go`, right after the function opens), reset selection state — navigation always rebuilds content, so stale line indices must not survive:

```go
	m.content.selection = selection{pendingLink: -1}
```

Confirm `strings` is imported in `content.go` (it is used elsewhere; add it if the build complains).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestModel_RenderedBaseIsStored -v`
Expected: PASS.

- [ ] **Step 5: Run the package to check nothing regressed**

Run: `go build ./... && go test ./internal/tui/ -run 'TestModel_' -count=1`
Expected: PASS (existing mouse/link/content tests still green).

- [ ] **Step 6: Commit**

```bash
git add internal/tui/content.go internal/tui/links.go internal/tui/selection_test.go
git commit -m "feat(tui): store rendered base + selection state via setContent chokepoint"
```

---

## Task 3: Coordinate mapping (screen → content)

**Files:**
- Modify: `internal/tui/content.go` (`screenToContent`)
- Test: `internal/tui/selection_test.go`

The content text starts at screen cell `(x=1, y=1)` because the content pane has a 1-cell rounded border (`paneStyle`, `view.go:157`) and the body sits at the screen origin. So `col = x - 1` and `line = viewport.YOffset + (y - 1)`, both clamped into range.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/selection_test.go`:

```go
func TestModel_ScreenToContent_MapsAndClamps(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	// Top-left content cell (border at 0,0 → text at 1,1) maps to line 0,col 0.
	if got := m.screenToContent(1, 1); got != (cellPos{line: 0, col: 0}) {
		t.Errorf("screenToContent(1,1) = %+v, want {0,0}", got)
	}
	// Negative-ish coords clamp to 0.
	if got := m.screenToContent(0, 0); got != (cellPos{line: 0, col: 0}) {
		t.Errorf("screenToContent(0,0) = %+v, want {0,0} (clamped)", got)
	}
	// A y far past the end clamps to the last line.
	last := len(m.contentLines()) - 1
	if got := m.screenToContent(1, 10_000); got.line != last {
		t.Errorf("screenToContent y=10000 line = %d, want %d (clamped)", got.line, last)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_ScreenToContent_MapsAndClamps -v`
Expected: compile failure — `m.screenToContent undefined`.

- [ ] **Step 3: Write minimal implementation**

In `internal/tui/content.go`:

```go
// screenToContent maps a mouse cell (x, y) to a position in the stored
// base render. The content pane has a 1-cell border, so text begins at
// screen (1, 1); the viewport's YOffset accounts for scroll. Out-of-
// range coordinates clamp to a valid cell so drags that leave the pane
// or run past end-of-line still resolve.
func (m *Model) screenToContent(x, y int) cellPos {
	lines := m.contentLines()
	if len(lines) == 0 {
		return cellPos{}
	}
	line := m.content.viewport.YOffset + (y - 1)
	if line < 0 {
		line = 0
	}
	if line > len(lines)-1 {
		line = len(lines) - 1
	}
	col := x - 1
	if col < 0 {
		col = 0
	}
	if w := ansi.StringWidth(lines[line]); col > w {
		col = w
	}
	return cellPos{line: line, col: col}
}
```

Add `"github.com/charmbracelet/x/ansi"` to `content.go`'s import block.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestModel_ScreenToContent_MapsAndClamps -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/content.go internal/tui/selection_test.go
git commit -m "feat(tui): map mouse cells to content coordinates"
```

---

## Task 4: Text extraction

Pure extraction of the selected span as plain text, newline-joined. Forward, backward, multi-line, and empty selections all covered.

**Files:**
- Modify: `internal/tui/content.go` (`normalizeSel`, `selColBounds`, `extractSelection`)
- Test: `internal/tui/selection_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/selection_test.go`. These tests set `m.content.rendered` directly to a known plain string (no ANSI) so the column math is unambiguous:

```go
func TestModel_ExtractSelection(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	m.content.rendered = "hello world\nsecond line\nthird"

	cases := []struct {
		name           string
		anchor, cursor cellPos
		want           string
	}{
		{"single-line forward", cellPos{0, 0}, cellPos{0, 5}, "hello"},
		{"single-line backward", cellPos{0, 5}, cellPos{0, 0}, "hello"},
		{"mid-line span", cellPos{0, 6}, cellPos{0, 11}, "world"},
		{"multi-line", cellPos{0, 6}, cellPos{1, 6}, "world\nsecond"},
		{"multi-line backward", cellPos{1, 6}, cellPos{0, 6}, "world\nsecond"},
		{"empty (zero width)", cellPos{0, 3}, cellPos{0, 3}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m.content.selection.anchor = tc.anchor
			m.content.selection.cursor = tc.cursor
			if got := m.extractSelection(); got != tc.want {
				t.Errorf("extractSelection() = %q, want %q", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_ExtractSelection -v`
Expected: compile failure — `m.extractSelection undefined`.

- [ ] **Step 3: Write minimal implementation**

In `internal/tui/content.go`:

```go
// normalizeSel returns a, b ordered so start is at or before end in
// reading order, regardless of drag direction.
func normalizeSel(a, b cellPos) (start, end cellPos) {
	if a.line < b.line || (a.line == b.line && a.col <= b.col) {
		return a, b
	}
	return b, a
}

// selColBounds returns the [lo, hi) visible-column range selected on
// line i, given the normalized start/end and the line's visible width.
// First line starts at start.col; last line ends at end.col; middle
// lines span the whole line.
func selColBounds(i int, start, end cellPos, width int) (lo, hi int) {
	lo, hi = 0, width
	if i == start.line {
		lo = start.col
	}
	if i == end.line {
		hi = end.col
	}
	return lo, hi
}

// extractSelection returns the selected text as plain (ANSI-stripped)
// content, newline-joined across lines. Empty if the span is zero-width.
func (m *Model) extractSelection() string {
	start, end := normalizeSel(m.content.selection.anchor, m.content.selection.cursor)
	lines := m.contentLines()
	if start.line >= len(lines) {
		return ""
	}
	if end.line >= len(lines) {
		end.line = len(lines) - 1
	}
	var parts []string
	for i := start.line; i <= end.line; i++ {
		lo, hi := selColBounds(i, start, end, ansi.StringWidth(lines[i]))
		if hi < lo {
			hi = lo
		}
		parts = append(parts, ansi.Strip(ansi.Cut(lines[i], lo, hi)))
	}
	return strings.Join(parts, "\n")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestModel_ExtractSelection -v`
Expected: PASS (all six sub-cases).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/content.go internal/tui/selection_test.go
git commit -m "feat(tui): extract selected text via ANSI-aware column cut"
```

---

## Task 5: Highlight overlay + clear

Draw the reverse-video selection over the base render, and restore the base when cleared.

**Files:**
- Modify: `internal/tui/content.go` (`applySelectionHighlight`, `clearSelection`, `selectionStyle`)
- Test: `internal/tui/selection_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/selection_test.go`:

```go
func TestModel_SelectionHighlightAppliesAndClears(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	m.content.rendered = "hello world"
	m.content.viewport.SetContent(m.content.rendered)
	m.content.selection.anchor = cellPos{0, 0}
	m.content.selection.cursor = cellPos{0, 5}

	m.applySelectionHighlight()
	view := m.content.viewport.View()
	if !strings.Contains(view, "\x1b[7m") {
		t.Errorf("expected reverse-video escape in highlighted view; got %q", view)
	}

	m.clearSelection()
	view = m.content.viewport.View()
	if strings.Contains(view, "\x1b[7m") {
		t.Errorf("expected no reverse-video after clearSelection; got %q", view)
	}
}
```

(Note: `lipgloss` emits `\x1b[7m` for `Reverse(true)` only when a color profile that supports styling is active; `sized` runs under the test profile which renders escapes. If lipgloss strips styles in the test environment, assert instead on `ansi.Strip(view)` round-tripping the text and on `m.content.selection.copied`/overlay difference — see fallback below.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_SelectionHighlightAppliesAndClears -v`
Expected: compile failure — `m.applySelectionHighlight undefined`.

- [ ] **Step 3: Write minimal implementation**

In `internal/tui/content.go`:

```go
// selectionStyle paints the selected span. Reverse-video swaps fg/bg so
// the selection reads like a GUI highlight regardless of theme.
var selectionStyle = lipgloss.NewStyle().Reverse(true)

// applySelectionHighlight redraws the base render with the selected span
// replaced by a uniform reverse-video block, then pushes it to the
// viewport (preserving scroll). The span is stripped before styling, so
// there are no inner escapes to cancel the reverse-video mid-span.
func (m *Model) applySelectionHighlight() {
	start, end := normalizeSel(m.content.selection.anchor, m.content.selection.cursor)
	lines := m.contentLines()
	out := make([]string, len(lines))
	copy(out, lines)
	for i := start.line; i <= end.line && i < len(lines); i++ {
		w := ansi.StringWidth(lines[i])
		lo, hi := selColBounds(i, start, end, w)
		if hi <= lo {
			continue
		}
		before := ansi.Cut(lines[i], 0, lo)
		mid := ansi.Strip(ansi.Cut(lines[i], lo, hi))
		after := ansi.Cut(lines[i], hi, w)
		out[i] = before + selectionStyle.Render(mid) + after
	}
	offset := m.content.viewport.YOffset
	m.content.viewport.SetContent(strings.Join(out, "\n"))
	m.content.viewport.SetYOffset(offset)
}

// resetSelectionState zeroes the selection without touching the
// viewport. Used where content is about to be re-set anyway.
func (m *Model) resetSelectionState() {
	m.content.selection = selection{pendingLink: -1}
}

// clearSelection drops the selection and restores the un-highlighted
// base render (preserving scroll) if a highlight was showing.
func (m *Model) clearSelection() {
	had := m.content.selection.moved || m.content.selection.copied
	m.resetSelectionState()
	if had {
		offset := m.content.viewport.YOffset
		m.content.viewport.SetContent(m.content.rendered)
		m.content.viewport.SetYOffset(offset)
	}
}
```

Ensure `"github.com/charmbracelet/lipgloss"` is imported in `content.go` (add if missing).

Replace the inline `selection{pendingLink: -1}` reset added to `refreshContent` in Task 2 with a call to the new helper for DRY:

```go
	m.resetSelectionState()
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestModel_SelectionHighlightAppliesAndClears -v`
Expected: PASS. If the reverse-video escape is absent under the test color profile, switch the assertions to the fallback noted in Step 1 (compare `m.content.viewport.View()` before vs after — they must differ when highlighted and match the base when cleared) and re-run.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/content.go internal/tui/selection_test.go
git commit -m "feat(tui): reverse-video selection overlay with restore-on-clear"
```

---

## Task 6: Mouse lifecycle wiring

Wire press/motion/release into `handleMouse`: start a selection on a content-pane press (remembering a link under the press), extend it on motion, and finalize on release — copying, toasting, and keeping the highlight. A press-then-release with no motion still follows the link.

**Files:**
- Modify: `internal/tui/input.go` (`handleMouse`, plus `dragSelect`/`endSelect` helpers)
- Test: `internal/tui/selection_test.go`

- [ ] **Step 1: Write the failing tests**

Add a drag helper near the top of `internal/tui/selection_test.go` (alongside the existing imports — add `tea "github.com/charmbracelet/bubbletea"`):

```go
func mouseAt(action tea.MouseAction, x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: action, Button: tea.MouseButtonLeft}
}
```

Then the integration tests:

```go
func TestModel_DragSelectsAndCopies(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))

	var copied string
	m.copyToClipboard = func(s string) { copied = s }

	// Force a known base so column math is predictable.
	m.content.rendered = "hello world"
	m.content.viewport.SetContent(m.content.rendered)

	// Press at content (1,1) → doc (0,0); drag to (6,1) → doc (0,5); release.
	updated, _ := m.Update(mouseAt(tea.MouseActionPress, 1, 1))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionMotion, 6, 1))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionRelease, 6, 1))
	m = updated.(Model)

	if copied != "hello" {
		t.Errorf("clipboard got %q, want %q", copied, "hello")
	}
	if !m.content.selection.copied {
		t.Error("selection.copied should be true after release with text")
	}
}

func TestModel_ClickWithoutDragFollowsLink(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	var copied string
	m.copyToClipboard = func(s string) { copied = s }

	// index.md links to notes/first.md. Find its link zone and click it
	// (press + release, no motion).
	if len(m.content.links) == 0 {
		t.Fatal("expected at least one link in index.md")
	}
	renderAndScan(t, m, zoneContentPane)
	lz := zone.Get(linkZoneID(0))
	if lz.IsZero() {
		t.Fatal("link zone 0 not registered")
	}
	updated, _ := m.Update(mouseAt(tea.MouseActionPress, lz.StartX, lz.StartY))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionRelease, lz.StartX, lz.StartY))
	m = updated.(Model)

	if copied != "" {
		t.Errorf("a click should not copy; got %q", copied)
	}
	// Following the first link navigates away from index.md.
	if got := m.history.Current(); filepath.Base(got) != "first.md" {
		t.Errorf("click should follow link to first.md; current=%q", got)
	}
}

func TestModel_EmptyDragDoesNotCopy(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	var calls int
	m.copyToClipboard = func(string) { calls++ }
	m.content.rendered = "hello world"
	m.content.viewport.SetContent(m.content.rendered)

	updated, _ := m.Update(mouseAt(tea.MouseActionPress, 3, 1))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionMotion, 3, 1)) // same cell
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionRelease, 3, 1))
	m = updated.(Model)

	if calls != 0 {
		t.Errorf("zero-width drag should not copy; got %d calls", calls)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestModel_DragSelectsAndCopies|TestModel_ClickWithoutDragFollowsLink|TestModel_EmptyDragDoesNotCopy' -v`
Expected: FAIL — drags currently don't copy; the link still follows on press (so the click test may pass for the wrong reason, but the drag/empty tests fail).

- [ ] **Step 3: Write the implementation**

Rewrite `handleMouse` in `internal/tui/input.go`. Replace the existing function body (the wheel guard stays first; the press logic is restructured and motion/release are added):

```go
func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if tea.MouseEvent(msg).IsWheel() {
		var cmd tea.Cmd
		m.content.viewport, cmd = m.content.viewport.Update(msg)
		return *m, cmd
	}

	// Motion / release only matter while a content-pane selection is
	// being tracked. Gating on `anchored` (not the button, which some
	// terminals report as None on release) keeps this robust.
	switch msg.Action {
	case tea.MouseActionMotion:
		if m.content.selection.anchored {
			return m.dragSelect(msg)
		}
		return *m, nil
	case tea.MouseActionRelease:
		if m.content.selection.anchored {
			return m.endSelect()
		}
		return *m, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return *m, nil
	}

	debugMouse(msg, m)

	// Tree row hit (tree modal only). Unchanged.
	if m.modals.kind == modalTree {
		for i := range m.tree.flat {
			if zone.Get(treeRowZoneID(i)).InBounds(msg) {
				return m.clickTree(i)
			}
		}
	}

	// Content-pane press: start a potential selection. Only when no modal
	// is open (modals keep their own click behavior). Remember a link
	// under the press so a no-motion release still follows it; do NOT
	// follow it here — the first motion event turns this into a drag.
	if m.modals.kind == modalNone && zone.Get(zoneContentPane).InBounds(msg) {
		m.clearSelection() // drop any prior finalized highlight
		m.focus = focusContent
		pos := m.screenToContent(msg.X, msg.Y)
		link := -1
		for i := range m.content.links {
			if zone.Get(linkZoneID(i)).InBounds(msg) {
				link = i
				break
			}
		}
		m.content.selection = selection{
			anchored:    true,
			anchor:      pos,
			cursor:      pos,
			pendingLink: link,
		}
		return *m, nil
	}

	return *m, nil
}

// dragSelect extends the in-progress selection to the motion point and
// repaints the highlight.
func (m *Model) dragSelect(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.content.selection.cursor = m.screenToContent(msg.X, msg.Y)
	m.content.selection.moved = true
	m.content.selection.copied = false
	m.applySelectionHighlight()
	return *m, nil
}

// endSelect finalizes the selection on button release. With motion, it
// copies the selected text (keeping the highlight) and toasts. Without
// motion, it was a click: follow the remembered link if any.
func (m *Model) endSelect() (tea.Model, tea.Cmd) {
	sel := m.content.selection
	if sel.moved {
		text := m.extractSelection()
		if n := utf8.RuneCountInString(text); n > 0 {
			m.copyToClipboard(text)
			m.diag.Info(fmt.Sprintf("Copied %d chars", n))
			m.content.selection.copied = true
			m.content.selection.anchored = false
			m.content.selection.moved = false
			return *m, clearTransientAfter(time.Second)
		}
		// Zero-width drag → treat as a click with no link.
		m.clearSelection()
		return *m, nil
	}

	link := sel.pendingLink
	m.clearSelection()
	if link >= 0 && link < len(m.content.links) {
		m.focus = focusContent
		m.content.linkCursor = link
		m.followLink(m.content.links[link])
	}
	return *m, nil
}
```

Add imports to `input.go`: `"time"`, `"unicode/utf8"` (and confirm `"fmt"` is present — it is). `clearTransientAfter` already exists (used by the footer-transient expiry); it schedules the toast to clear after 1s.

Delete the now-obsolete content-link and content-fall-through blocks that the old `handleMouse` ran on press (the loop over `m.content.links` that called `followLink` immediately, and the `zoneContentPane` fall-through that forwarded the press to the viewport) — their behavior is now subsumed by the selection press/release path.

- [ ] **Step 4: Run the new tests**

Run: `go test ./internal/tui/ -run 'TestModel_DragSelectsAndCopies|TestModel_ClickWithoutDragFollowsLink|TestModel_EmptyDragDoesNotCopy' -v`
Expected: PASS.

- [ ] **Step 5: Run the existing mouse suite for regressions**

Run: `go test ./internal/tui/ -run TestModel_Mouse -v -count=1`
Expected: PASS. If `TestModel_MouseClick_OnLink...`-style tests assumed link-follow on press, they still pass because those tests issue a single press; update any that now require a matching release to follow a link (add a release `mouseAt` after the press). Make that edit if a test fails for this reason and note it in the commit.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/input.go internal/tui/selection_test.go
git commit -m "feat(tui): drag-to-select copies on release, click still follows links"
```

---

## Task 7: Clear selection on key input + footer toast assertion

A finalized selection's highlight must clear on the next keystroke, and the footer must show "Copied N chars".

**Files:**
- Modify: `internal/tui/input.go` (`handleKey` — clear a finalized selection first)
- Test: `internal/tui/selection_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/selection_test.go`:

```go
func TestModel_KeystrokeClearsFinalizedSelection(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	m.copyToClipboard = func(string) {}
	m.content.rendered = "hello world"
	m.content.viewport.SetContent(m.content.rendered)

	updated, _ := m.Update(mouseAt(tea.MouseActionPress, 1, 1))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionMotion, 6, 1))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionRelease, 6, 1))
	m = updated.(Model)
	if !m.content.selection.copied {
		t.Fatal("precondition: selection should be copied")
	}

	// Any key clears it (use 'j', a harmless scroll key).
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.content.selection.copied {
		t.Error("keystroke should clear the finalized selection")
	}
}

func TestModel_FooterShowsCopiedCount(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	m.copyToClipboard = func(string) {}
	m.content.rendered = "hello world"
	m.content.viewport.SetContent(m.content.rendered)

	updated, _ := m.Update(mouseAt(tea.MouseActionPress, 1, 1))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionMotion, 6, 1))
	m = updated.(Model)
	updated, _ = m.Update(mouseAt(tea.MouseActionRelease, 6, 1))
	m = updated.(Model)

	if !strings.Contains(m.renderFooter(), "Copied 5 chars") {
		t.Errorf("footer should show copied count; got %q", m.renderFooter())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestModel_KeystrokeClearsFinalizedSelection|TestModel_FooterShowsCopiedCount' -v`
Expected: the keystroke test FAILS (selection stays copied); the footer test should PASS already (Task 6 emits the toast). If the footer test fails, verify `m.diag.Info` feeds `transientStatus` and that `renderFooter` reads it.

- [ ] **Step 3: Write the implementation**

In `internal/tui/input.go`, at the **top** of `handleKey` (before any dispatch), clear a finalized selection so its highlight doesn't linger:

```go
	if m.content.selection.copied {
		m.clearSelection()
	}
```

This runs before the picker-rune routing and the global switch, so the key still performs its normal action — it just also drops the stale highlight. (A mid-drag selection — `anchored` true, `copied` false — is not cleared here; only finalized highlights are.)

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/tui/ -run 'TestModel_KeystrokeClearsFinalizedSelection|TestModel_FooterShowsCopiedCount' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/input.go internal/tui/selection_test.go
git commit -m "feat(tui): clear finalized selection on next keystroke"
```

---

## Task 8: Full verification, docs, and CLAUDE.md gotcha

**Files:**
- Modify: `CLAUDE.md` (Gotchas section), `docs/index.md` (mark spec status), `docs/superpowers/specs/2026-06-12-drag-to-select-copy-design.md` (status line)
- No code change beyond what's already in place.

- [ ] **Step 1: Run the whole suite race-clean (CI parity)**

Run: `go build ./... && go vet ./... && go test -race ./...`
Expected: all PASS. Fix any failure before continuing. (CI runs exactly this per CLAUDE.md.)

- [ ] **Step 2: Manual smoke (optional but recommended)**

Run: `go run ./cmd/hypogeum .` in a real terminal, drag across some rendered text, confirm the highlight appears, the footer shows "Copied N chars", and paste lands the text elsewhere. (OSC 52 must be enabled in your terminal.)

- [ ] **Step 3: Add a CLAUDE.md gotcha**

Add to the Gotchas section of `CLAUDE.md`:

```markdown
- **Drag-to-select copies via OSC 52 and is content-pane only.** A left-press in the content pane starts a selection (`internal/tui/input.go` `handleMouse`); the *first* motion event turns it into a drag, so a press-then-release with no motion still follows a link under the press. On release with a non-empty span, the selected text is extracted from `m.content.rendered` with `ansi.Cut`/`ansi.Strip` (column-accurate over the ANSI-styled render), copied via the injected `m.copyToClipboard` (default `termenv.Copy`, OSC 52 — works over SSH, no pbcopy/xclip), and the footer toasts "Copied N chars". The reverse-video highlight persists until the next press, keystroke, or navigation. Every real-content display path must go through `m.setContent` so `content.rendered` (the selection overlay's base) stays in sync; `refreshContent` resets selection state via `resetSelectionState`. Selection is deliberately disabled while a modal is open.
```

- [ ] **Step 4: Mark the spec shipped**

In `docs/superpowers/specs/2026-06-12-drag-to-select-copy-design.md`, change the status line to:

```markdown
Status: shipped.
```

In `docs/index.md`, update the drag-to-select entry's lead-in from "design approved" to "shipped".

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md docs/index.md docs/superpowers/specs/2026-06-12-drag-to-select-copy-design.md
git commit -m "docs: drag-to-select gotcha + mark spec shipped"
```

- [ ] **Step 6: Open the PR**

```bash
git push -u origin feature/drag-to-select-copy
gh pr create --title "Drag-to-select with auto-copy" --body "$(cat <<'EOF'
Adds app-drawn character-level mouse selection in the content pane.
Drag to select; on release the text is copied to the clipboard via
OSC 52 (termenv.Copy), the selection stays highlighted, and the footer
shows "Copied N chars". A press-then-release with no motion still
follows a link. Content-pane only; modals keep their click behavior.

Spec: docs/superpowers/specs/2026-06-12-drag-to-select-copy-design.md
Plan: docs/superpowers/plans/2026-06-12-drag-to-select-copy.md

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

(Per CLAUDE.md, the PR merges with `gh pr merge --merge`, not squash.)

---

## Self-Review notes

- **Spec coverage:** selection model (Tasks 2–6), character granularity via `ansi.Cut` (Tasks 3–5), content-pane-only scope (`modalNone` guard, Task 6), persistent highlight + footer toast (Tasks 6–7), OSC 52 clipboard seam (Task 1), click-vs-drag link preservation (Task 6), clear-on-keystroke/navigation (Tasks 2 & 7), coordinate/scroll correctness via absolute lines (Task 3), edge cases empty/out-of-range (Tasks 3–4, 6) — all mapped.
- **Type consistency:** `cellPos{line,col}`, `selection{anchored,moved,copied,anchor,cursor,pendingLink}`, `setContent`/`contentLines`/`screenToContent`/`extractSelection`/`applySelectionHighlight`/`clearSelection`/`resetSelectionState`/`normalizeSel`/`selColBounds`/`dragSelect`/`endSelect`/`selectionStyle` are used with identical signatures across tasks.
- **No placeholders:** every code step shows complete code; commands have expected output.
- **Known soft spots flagged inline:** the reverse-video assertion may need the fallback form depending on the test color profile (Task 5, Step 4); an existing press-only link test may need a release added (Task 6, Step 5).
