# Keyboard Selection (vim visual mode) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a two-phase, vim-style keyboard visual mode for selecting and copying content-pane text, reusing the existing mouse-selection span machinery.

**Architecture:** The existing `selection{anchor, cursor cellPos}` struct and its `applySelectionHighlight`/`extractSelection` helpers are purely position-based, so keyboard selection is a new *input source* feeding the same machinery — not a new selection system. `v` reveals a movable caret (positioning phase); `Space` drops the anchor (extend phase); the dialect's copy key yanks; `Esc` cancels. A `handleVisualKey` branch intercepts all keys while visual mode is active.

**Tech Stack:** Go, Bubble Tea (`tea.KeyMsg`), Bubbles `viewport`, `charmbracelet/bubbles/key`, `charmbracelet/x/ansi` (column-accurate `Cut`/`Strip`), `lipgloss` (reverse-video style).

**Spec:** `docs/superpowers/specs/2026-06-13-keyboard-selection-design.md`

**Conventions for every task:**
- Run `go build ./...` and `go test ./internal/tui/` to verify; CI also runs `go test -race ./...` and `go vet ./...`.
- Tests live in `internal/tui/` next to the code. Helpers `sized(t, root, initial)`, `pressRune(t, m, r)`, `pressKey(t, m, msg)`, `writeFixture(t)`, `isolatedHome(t)` already exist in `helpers_test.go`.
- A `cellPos` is `struct{ line, col int }` (`internal/tui/content.go:22`).

---

### Task 1: Selection state + keybindings + help

Add the two state flags, the two new keybindings (`v`, `Space`) to both dialects, the help-cheatsheet entries, and the overlap whitelist for `Space`.

**Files:**
- Modify: `internal/tui/content.go` (the `selection` struct, ~line 29)
- Modify: `internal/tui/keys.go` (the `keyMap` struct + `pagerKeys()` + `modernKeys()`)
- Modify: `internal/tui/help.go` (`formatHelp` sections)
- Modify: `internal/tui/keys_test.go` (`isAllowedKeyOverlap`)
- Test: `internal/tui/keys_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/keys_test.go`:

```go
func TestVisualModeBindingsPresent(t *testing.T) {
	for _, dialect := range []string{"pager", "modern"} {
		km := keysFor(dialect)
		if !slices.Contains(km.EnterVisual.Keys(), "v") {
			t.Errorf("%s: EnterVisual = %v, want to include \"v\"", dialect, km.EnterVisual.Keys())
		}
		if !slices.Contains(km.BeginSelect.Keys(), " ") {
			t.Errorf("%s: BeginSelect = %v, want to include \" \"", dialect, km.BeginSelect.Keys())
		}
	}
}
```

(`slices` is already imported in `keys_test.go`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestVisualModeBindingsPresent`
Expected: compile error — `km.EnterVisual` / `km.BeginSelect` undefined.

- [ ] **Step 3: Add the keyMap fields**

In `internal/tui/keys.go`, add to the `keyMap` struct (next to `ToggleFolder`):

```go
	EnterVisual key.Binding
	BeginSelect key.Binding
```

- [ ] **Step 4: Bind them in both dialect factories**

In `pagerKeys()` (after the `ToggleFolder` line):

```go
		EnterVisual: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "select mode")),
		BeginSelect: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "start selection")),
```

In `modernKeys()` (after its `ToggleFolder` line):

```go
		EnterVisual: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "select mode")),
		BeginSelect: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "start selection")),
```

- [ ] **Step 5: Run the binding test to verify it passes**

Run: `go test ./internal/tui/ -run TestVisualModeBindingsPresent`
Expected: PASS.

- [ ] **Step 6: Run the overlap test to verify it now fails**

Run: `go test ./internal/tui/ -run TestKeys_NoOverlappingActions`
Expected: FAIL — `key " " bound to both BeginSelect and ToggleFolder` (in both dialects).

- [ ] **Step 7: Whitelist the Space overlap**

In `internal/tui/keys_test.go`, in `isAllowedKeyOverlap`, add before `return false`:

```go
	if pair("BeginSelect", "ToggleFolder") && key == " " {
		return true
	}
```

- [ ] **Step 8: Run the overlap test to verify it passes**

Run: `go test ./internal/tui/ -run TestKeys_NoOverlappingActions`
Expected: PASS.

- [ ] **Step 9: Add the selection state flags**

In `internal/tui/content.go`, update the `selection` struct:

```go
type selection struct {
	anchored    bool    // a left-press landed in the content pane (mouse)
	moved       bool    // motion seen since press (mouse click-vs-drag)
	copied      bool    // released/yanked with text → highlight persists
	visual      bool    // keyboard visual mode is active (either phase)
	selecting   bool    // anchor dropped (Space) → caret movement extends
	anchor      cellPos // where the selection started
	cursor      cellPos // current end / caret position
	pendingLink int     // link index under a mouse press, or -1
}
```

- [ ] **Step 10: Add a Selection section to the help cheat sheet**

In `internal/tui/help.go`, in `formatHelp`, add a section after the `Links` entry in the `sections` slice:

```go
		{"Selection", []key.Binding{k.EnterVisual, k.BeginSelect}},
```

- [ ] **Step 11: Run the full tui suite**

Run: `go build ./... && go test ./internal/tui/`
Expected: PASS (existing tests unaffected; new fields default to false).

- [ ] **Step 12: Commit**

```bash
git add internal/tui/content.go internal/tui/keys.go internal/tui/help.go internal/tui/keys_test.go
git commit -m "feat(tui): add visual-mode state fields and v/Space keybindings"
```

---

### Task 2: Enter visual mode + render the caret

Add `enterVisual`, wire `v` into the content-key handler, and make `applySelectionHighlight` draw a one-cell caret at a zero-width span so a freshly placed caret is visible.

**Files:**
- Modify: `internal/tui/content.go` (`applySelectionHighlight` ~line 479; add `renderCaretLine`, `enterVisual`)
- Modify: `internal/tui/input.go` (`handleContentKey` switch ~line 460)
- Test: `internal/tui/keyboard_select_test.go` (new file)

- [ ] **Step 1: Write the failing test**

Create `internal/tui/keyboard_select_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// reverseRuns reports whether the rendered viewport contains any
// reverse-video (\x1b[7m) run — i.e. a caret or selection is painted.
func hasReverseVideo(s string) bool {
	return strings.Contains(s, "\x1b[7m")
}

func TestVisual_EnterShowsCaretAtTopLeft(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent("hello world\nsecond line")

	m = pressRune(t, m, 'v')

	if !m.content.selection.visual {
		t.Fatal("v should enter visual mode")
	}
	if m.content.selection.selecting {
		t.Error("entering visual should be in positioning phase (selecting=false)")
	}
	wantCaret := cellPos{line: m.content.viewport.YOffset, col: 0}
	if m.content.selection.cursor != wantCaret {
		t.Errorf("caret = %+v, want %+v", m.content.selection.cursor, wantCaret)
	}
	if m.content.selection.anchor != wantCaret {
		t.Errorf("anchor = %+v, want %+v", m.content.selection.anchor, wantCaret)
	}
	// A caret cell must be painted even though the span is zero-width.
	if !hasReverseVideo(m.content.viewport.View()) {
		t.Errorf("expected a reverse-video caret cell in view:\n%s", ansi.Strip(m.content.viewport.View()))
	}
	// Nothing is selected yet.
	if got := m.extractSelection(); got != "" {
		t.Errorf("extractSelection at entry = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestVisual_EnterShowsCaretAtTopLeft`
Expected: FAIL — `v` does nothing yet, so `selection.visual` is false.

- [ ] **Step 3: Add the caret-cell renderer**

In `internal/tui/content.go`, just below `applySelectionHighlight` (after line ~502), add:

```go
// renderCaretLine returns ln with a single reverse-video caret cell at
// visible column col. If col is at or past the line's content width, a
// reverse-video space is appended (caret on a blank or end-of-line).
func renderCaretLine(ln string, col, width int) string {
	if col >= width {
		return ln + selectionStyle.Render(" ")
	}
	var b strings.Builder
	b.WriteString(ansi.Cut(ln, 0, col))
	b.WriteString(selectionStyle.Render(ansi.Strip(ansi.Cut(ln, col, col+1))))
	b.WriteString(ansi.Cut(ln, col+1, width))
	return b.String()
}
```

- [ ] **Step 4: Draw the caret in the zero-width branch**

In `internal/tui/content.go`, in `applySelectionHighlight`, replace the zero-width branch:

```go
		lo, hi := selColBounds(i, start, end, m.content.lineWidths[i])
		if hi <= lo {
			b.WriteString(ln)
			continue
		}
```

with:

```go
		lo, hi := selColBounds(i, start, end, m.content.lineWidths[i])
		if hi <= lo {
			// Zero-width: in keyboard visual mode draw a one-cell caret on
			// the caret's line so the user can see where they are; mouse
			// selections (visual=false) show nothing for a zero-width span.
			if m.content.selection.visual && i == m.content.selection.cursor.line {
				b.WriteString(renderCaretLine(ln, m.content.selection.cursor.col, m.content.lineWidths[i]))
			} else {
				b.WriteString(ln)
			}
			continue
		}
```

- [ ] **Step 5: Add enterVisual**

In `internal/tui/content.go`, near the other selection helpers (after `clearSelection`, ~line 538), add:

```go
// enterVisual starts keyboard visual mode in the positioning phase: a
// movable caret at the top-left of the visible area, no span yet. The
// caret is selection.cursor; anchor tracks it until Space drops the anchor.
func (m *Model) enterVisual() {
	line := m.content.viewport.YOffset
	if n := len(m.contentLines()); line > n-1 {
		line = n - 1
	}
	if line < 0 {
		line = 0
	}
	at := cellPos{line: line, col: 0}
	m.content.selection = selection{visual: true, anchor: at, cursor: at, pendingLink: -1}
	m.applySelectionHighlight()
}
```

- [ ] **Step 6: Wire `v` into the content-key handler**

In `internal/tui/input.go`, in `handleContentKey`'s `switch` (after the `PrevLink` case, ~line 466), add:

```go
	case key.Matches(msg, m.keys.EnterVisual):
		m.enterVisual()
		return *m, nil
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test ./internal/tui/ -run TestVisual_EnterShowsCaretAtTopLeft`
Expected: PASS.

- [ ] **Step 8: Run the full tui suite (mouse selection must be unaffected)**

Run: `go build ./... && go test ./internal/tui/`
Expected: PASS — mouse paths never set `visual`, so the caret branch never fires for them.

- [ ] **Step 9: Commit**

```bash
git add internal/tui/content.go internal/tui/input.go internal/tui/keyboard_select_test.go
git commit -m "feat(tui): enter visual mode with v and render a caret cell"
```

---

### Task 3: Caret movement (positioning phase) + dispatch routing

Add `placeCaret` + `scrollCaretIntoView`, the `handleVisualKey` dispatcher, and the intercept at the top of `handleKey`. This task covers char/line motion and the `Esc` exit; `Space`/yank/jumps come in Task 4–5.

**Files:**
- Modify: `internal/tui/content.go` (add `placeCaret`, `scrollCaretIntoView`)
- Modify: `internal/tui/input.go` (add `handleVisualKey`; intercept in `handleKey`)
- Test: `internal/tui/keyboard_select_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/keyboard_select_test.go`:

```go
func TestVisual_PositioningMovesCaretNoSpan(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent("hello world\nsecond line")

	m = pressRune(t, m, 'v')          // caret at {0,0}
	m = pressRune(t, m, 'l')          // → {0,1}
	m = pressRune(t, m, 'l')          // → {0,2}
	m = pressRune(t, m, 'j')          // → {1,2}

	if got := m.content.selection.cursor; got != (cellPos{line: 1, col: 2}) {
		t.Fatalf("caret = %+v, want {1,2}", got)
	}
	// Positioning phase: anchor tracks the caret, so there is no span.
	if m.content.selection.anchor != m.content.selection.cursor {
		t.Errorf("anchor %+v should track cursor %+v in positioning phase",
			m.content.selection.anchor, m.content.selection.cursor)
	}
	if got := m.extractSelection(); got != "" {
		t.Errorf("extractSelection in positioning = %q, want empty", got)
	}
}

func TestVisual_EscCancelsFromPositioning(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent("hello world")

	m = pressRune(t, m, 'v')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.content.selection.visual {
		t.Error("Esc should exit visual mode")
	}
	if hasReverseVideo(m.content.viewport.View()) {
		t.Error("Esc should restore the clean (un-highlighted) render")
	}
}

func TestVisual_CaretClampsAtEdges(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent("ab\ncd")

	m = pressRune(t, m, 'v')          // {0,0}
	m = pressRune(t, m, 'h')          // clamp col → {0,0}
	m = pressRune(t, m, 'k')          // clamp line → {0,0}
	if got := m.content.selection.cursor; got != (cellPos{0, 0}) {
		t.Errorf("caret after clamp = %+v, want {0,0}", got)
	}
}
```

(`tea` is already imported by other `_test.go` files in the package; add the import to this file's import block: `tea "github.com/charmbracelet/bubbletea"`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run TestVisual_`
Expected: FAIL — `l`/`j`/`h`/`k` don't move the caret yet (no `handleVisualKey`), and `Esc` falls through to the link-clear path.

- [ ] **Step 3: Add placeCaret + scrollCaretIntoView**

In `internal/tui/content.go`, near `enterVisual`, add:

```go
// placeCaret moves the visual-mode caret to (line, col), clamped to valid
// cells. In the positioning phase (!selecting) the anchor tracks the caret
// so there is no span; in the extend phase only the cursor moves, growing
// the selection. Scrolls the caret into view and repaints.
func (m *Model) placeCaret(line, col int) {
	lines := m.contentLines()
	if len(lines) == 0 {
		return
	}
	if line < 0 {
		line = 0
	}
	if line > len(lines)-1 {
		line = len(lines) - 1
	}
	if col < 0 {
		col = 0
	}
	if w := m.content.lineWidths[line]; col > w {
		col = w
	}
	m.content.selection.cursor = cellPos{line: line, col: col}
	if !m.content.selection.selecting {
		m.content.selection.anchor = m.content.selection.cursor
	}
	m.scrollCaretIntoView()
	m.applySelectionHighlight()
}

// scrollCaretIntoView adjusts the viewport's YOffset so the caret's line is
// within the visible window. applySelectionHighlight (called right after)
// preserves whatever offset this sets.
func (m *Model) scrollCaretIntoView() {
	line := m.content.selection.cursor.line
	top := m.content.viewport.YOffset
	h := m.content.viewport.Height
	if h < 1 {
		return
	}
	if line < top {
		m.content.viewport.SetYOffset(line)
	} else if line >= top+h {
		m.content.viewport.SetYOffset(line - h + 1)
	}
}
```

- [ ] **Step 4: Add handleVisualKey**

In `internal/tui/input.go`, add after `handleContentKey` (before `handleTreeModalKey`):

```go
// handleVisualKey routes every keystroke while keyboard visual mode is
// active. Char/line motions are matched on the raw key (h/j/k/l + arrows)
// rather than the Back/Forward/Up/Down keyMap fields, because the modern
// dialect binds Back/Forward to alt+arrows — plain arrows must still move
// the caret. Jumps (g/G, ^d/^u) reuse the dialect-aware Top/Bottom/HalfPage
// fields. Yank reuses the dialect's copy key; Space drops the anchor; Esc
// cancels. Any other key is inert.
func (m *Model) handleVisualKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cur := m.content.selection.cursor
	half := m.content.viewport.Height / 2
	if half < 1 {
		half = 1
	}
	last := len(m.contentLines()) - 1

	switch {
	case key.Matches(msg, m.keys.ClearLink): // Esc
		m.clearSelection()
		return *m, nil
	case key.Matches(msg, m.keys.CopyPath): // y / ^y → yank
		m.yankVisual()
		return *m, nil
	case key.Matches(msg, m.keys.BeginSelect): // Space → drop anchor
		m.content.selection.selecting = true
		return *m, nil
	case key.Matches(msg, m.keys.Top): // g
		m.placeCaret(0, 0)
		return *m, nil
	case key.Matches(msg, m.keys.Bottom): // G
		m.placeCaret(last, 0)
		return *m, nil
	case key.Matches(msg, m.keys.HalfPageDown): // ^d
		m.placeCaret(cur.line+half, cur.col)
		return *m, nil
	case key.Matches(msg, m.keys.HalfPageUp): // ^u
		m.placeCaret(cur.line-half, cur.col)
		return *m, nil
	}

	switch msg.String() {
	case "h", "left":
		m.placeCaret(cur.line, cur.col-1)
	case "l", "right":
		m.placeCaret(cur.line, cur.col+1)
	case "k", "up":
		m.placeCaret(cur.line-1, cur.col)
	case "j", "down":
		m.placeCaret(cur.line+1, cur.col)
	}
	return *m, nil
}
```

- [ ] **Step 5: Add a temporary yankVisual stub so the package compiles**

`handleVisualKey` references `m.yankVisual`, implemented in Task 4. Add a minimal stub now in `internal/tui/input.go` (just below `handleVisualKey`); Task 4 replaces its body:

```go
// yankVisual copies the current selection to the clipboard and exits visual
// mode. Body filled in Task 4.
func (m *Model) yankVisual() {
	m.clearSelection()
}
```

- [ ] **Step 6: Intercept visual mode at the top of handleKey**

In `internal/tui/input.go`, in `handleKey`, immediately after the `copied` clear block (~line 185), add:

```go
	// Keyboard visual mode intercepts every key while active — before any
	// modal-toggle or global binding. Visual mode is content-pane only and
	// never coexists with an open modal.
	if m.content.selection.visual {
		return m.handleVisualKey(msg)
	}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestVisual_`
Expected: PASS.

- [ ] **Step 8: Run the full tui suite**

Run: `go build ./... && go test ./internal/tui/`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/tui/content.go internal/tui/input.go internal/tui/keyboard_select_test.go
git commit -m "feat(tui): move the visual-mode caret and route keys via handleVisualKey"
```

---

### Task 4: Begin select (Space), extend, and yank

Implement `Space` → extend phase, the real `yankVisual`, and update `finalizeSelection`/`clearSelection` to know about the new flags.

**Files:**
- Modify: `internal/tui/content.go` (`finalizeSelection` ~line 524, `clearSelection` ~line 532)
- Modify: `internal/tui/input.go` (replace the `yankVisual` stub)
- Test: `internal/tui/keyboard_select_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/keyboard_select_test.go`:

```go
func TestVisual_SpaceAnchorsThenExtends(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent("hello world")

	m = pressRune(t, m, 'v')                       // caret {0,0}
	m = pressRune(t, m, 'l')                       // {0,1} (positioning, no span)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace}) // drop anchor at {0,1}
	if !m.content.selection.selecting {
		t.Fatal("Space should enter the extend phase")
	}
	m = pressRune(t, m, 'l') // {0,2}
	m = pressRune(t, m, 'l') // {0,3}
	m = pressRune(t, m, 'l') // {0,4}

	// Anchor frozen at col 1; cursor at col 4 → "ell".
	if got := m.extractSelection(); got != "ell" {
		t.Errorf("extractSelection = %q, want %q", got, "ell")
	}
}

func TestVisual_YankCopiesAndPersists(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	var copied string
	m.copyToClipboard = func(s string) { copied = s }
	m.setContent("hello world")

	m = pressRune(t, m, 'v')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace}) // anchor at {0,0}
	m = pressRune(t, m, 'l')
	m = pressRune(t, m, 'l')
	m = pressRune(t, m, 'l')
	m = pressRune(t, m, 'l')
	m = pressRune(t, m, 'l')                            // cursor {0,5} → "hello"
	m = pressRune(t, m, 'y')                            // yank

	if copied != "hello" {
		t.Errorf("clipboard = %q, want %q", copied, "hello")
	}
	if m.content.selection.visual || m.content.selection.selecting {
		t.Error("yank should exit visual mode")
	}
	if !m.content.selection.copied {
		t.Error("yank should leave the selection in the copied state")
	}
	if !hasReverseVideo(m.content.viewport.View()) {
		t.Error("highlight should persist after yank")
	}
	if !strings.Contains(m.renderFooter(), "Copied 5 chars") {
		t.Errorf("footer should toast the count; got %q", m.renderFooter())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestVisual_SpaceAnchorsThenExtends|TestVisual_YankCopiesAndPersists'`
Expected: FAIL — extend works (Space sets `selecting`) but `yankVisual` is still the stub that clears without copying, so the clipboard/footer/`copied` assertions fail.

- [ ] **Step 3: Update finalizeSelection to clear the visual flags**

In `internal/tui/content.go`, update `finalizeSelection`:

```go
func (m *Model) finalizeSelection() {
	m.content.selection.copied = true
	m.content.selection.anchored = false
	m.content.selection.moved = false
	m.content.selection.visual = false
	m.content.selection.selecting = false
}
```

- [ ] **Step 4: Teach clearSelection about the visual flag**

In `internal/tui/content.go`, in `clearSelection`, update the `had` line:

```go
	had := m.content.selection.moved || m.content.selection.copied || m.content.selection.visual
```

- [ ] **Step 5: Replace the yankVisual stub with the real implementation**

In `internal/tui/input.go`, replace the `yankVisual` stub body:

```go
// yankVisual copies the current selection to the clipboard, toasts the
// count, and finalizes the selection so its highlight persists until the
// user's next action. A zero-width selection (still positioning, or a
// collapsed span) copies nothing and just exits.
func (m *Model) yankVisual() {
	text := m.extractSelection()
	if n := utf8.RuneCountInString(text); n > 0 {
		m.copyToClipboard(text)
		m.diag.Info(fmt.Sprintf("Copied %d chars", n))
		m.finalizeSelection()
		return
	}
	m.clearSelection()
}
```

(`utf8` and `fmt` are already imported in `input.go` — used by `endSelect`.)

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run 'TestVisual_SpaceAnchorsThenExtends|TestVisual_YankCopiesAndPersists'`
Expected: PASS.

- [ ] **Step 7: Run the full tui suite**

Run: `go build ./... && go test ./internal/tui/`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/content.go internal/tui/input.go internal/tui/keyboard_select_test.go
git commit -m "feat(tui): Space anchors and y yanks the keyboard selection"
```

---

### Task 5: Jumps, dialect coverage, modal guard, and regressions

Lock in `g`/`G`/`^d`/`^u` jumps, both-dialect behavior, the modal no-op, and regressions for the keys visual mode shadows.

**Files:**
- Test: `internal/tui/keyboard_select_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/tui/keyboard_select_test.go` (add `"fmt"` to this file's import block — `tallContent` uses `fmt.Fprintf`):

```go
// tallContent builds N numbered lines so jumps/scrolling have room.
func tallContent(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "line %02d\n", i)
	}
	return strings.TrimRight(b.String(), "\n")
}

func TestVisual_GJumpsToBottomAndYanksToEnd(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	var copied string
	m.copyToClipboard = func(s string) { copied = s }
	m.setContent(tallContent(100))

	m = pressRune(t, m, 'v')                            // caret {0,0}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})  // anchor {0,0}
	m = pressRune(t, m, 'G')                            // caret → last line

	last := len(m.contentLines()) - 1
	if got := m.content.selection.cursor.line; got != last {
		t.Fatalf("G caret line = %d, want %d", got, last)
	}
	m = pressRune(t, m, 'y')
	if !strings.HasPrefix(copied, "line 00\n") || !strings.Contains(copied, "line 99") {
		t.Errorf("G-then-yank should copy from top to end; got %q", copied)
	}
}

func TestVisual_HalfPageDownScrolls(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.setContent(tallContent(100))
	startOffset := m.content.viewport.YOffset

	m = pressRune(t, m, 'v')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD}) // ^d

	if m.content.viewport.YOffset <= startOffset {
		t.Errorf("^d should scroll the viewport; offset %d -> %d", startOffset, m.content.viewport.YOffset)
	}
	if m.content.selection.cursor.line == 0 {
		t.Error("^d should advance the caret line")
	}
}

func TestVisual_ModernDialectEntersAndYanks(t *testing.T) {
	root := writeFixture(t)
	isolatedHome(t)
	m, err := New(root, "", Options{Dialect: "modern"})
	if err != nil {
		t.Fatal(err)
	}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)
	var copied string
	m.copyToClipboard = func(s string) { copied = s }
	m.setContent("hello world")

	m = pressRune(t, m, 'v')                              // enter (v, both dialects)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})    // anchor
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})    // → extend with plain arrow
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})    // cursor {0,5} → "hello"
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})    // modern yank = ^y

	if copied != "hello" {
		t.Errorf("modern dialect yank: clipboard = %q, want %q", copied, "hello")
	}
}

func TestVisual_NoOpWhileModalOpen(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Open the tree modal, then press v.
	m = pressRune(t, m, 't')
	if m.modals.kind != modalTree {
		t.Fatalf("precondition: tree modal should be open, got %v", m.modals.kind)
	}
	m = pressRune(t, m, 'v')
	if m.content.selection.visual {
		t.Error("v should be a no-op while a modal is open")
	}
}

func TestVisual_RegressionCopyPathAndHistoryOutsideVisual(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	var copied string
	m.copyToClipboard = func(s string) { copied = s }

	// y outside visual mode still copies the path.
	m = pressRune(t, m, 'y')
	if !strings.HasPrefix(copied, root) {
		t.Errorf("y outside visual should copy the path; got %q", copied)
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./internal/tui/ -run TestVisual_`
Expected: PASS — all behaviors are implemented in Tasks 2–4; these pin them. If `TestVisual_NoOpWhileModalOpen` fails, confirm the `m.content.selection.visual` intercept in `handleKey` sits *after* the modal-toggle dispatch is reachable only when no modal is open (it is: `v` enters via `handleContentKey`, which runs only when no modal is open).

- [ ] **Step 3: Run the full race + vet suite (CI parity)**

Run: `go vet ./... && go test -race ./...`
Expected: PASS across all packages.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/keyboard_select_test.go
git commit -m "test(tui): cover visual-mode jumps, modern dialect, modal guard, regressions"
```

---

### Task 6: Documentation

Update CLAUDE.md and mark the spec/index as shipped.

**Files:**
- Modify: `CLAUDE.md` (the "What this is" summary; add a gotcha if warranted)
- Modify: `docs/index.md` (flip the spec entry from "designed" to "shipped")
- Modify: `docs/superpowers/specs/2026-06-13-keyboard-selection-design.md` (status line)

- [ ] **Step 1: Update the CLAUDE.md summary**

In `CLAUDE.md`, in the "What this is" paragraph, add keyboard selection to the feature list, e.g. after the copy-path clause:

```
…copies the current file's absolute path to the clipboard. Keyboard text selection is a two-phase visual mode — `v` reveals a movable caret, `Space` drops the anchor, motion keys extend, the dialect's copy key yanks, `Esc` cancels — reusing the same `selection{anchor,cursor}` span machinery as mouse drag-to-select.
```

- [ ] **Step 2: Flip the docs/index.md entry to shipped**

In `docs/index.md`, change the keyboard-selection bullet's `— designed —` to `— shipped —`.

- [ ] **Step 3: Update the spec status line**

In `docs/superpowers/specs/2026-06-13-keyboard-selection-design.md`, change:

```
**Status:** designed, not yet implemented.
```

to:

```
**Status:** shipped.
```

- [ ] **Step 4: Verify the build once more**

Run: `go build ./... && go test ./internal/tui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md docs/index.md docs/superpowers/specs/2026-06-13-keyboard-selection-design.md
git commit -m "docs: mark keyboard selection shipped and note it in CLAUDE.md"
```

---

## Self-review notes

- **Spec coverage:** state flags (Task 1), `v`/`Space` bindings + overlap whitelist + help (Task 1), top-left caret + caret rendering (Task 2), motion + scroll + dispatch intercept + Esc (Task 3), Space/extend/yank + finalize/clear updates (Task 4), jumps + dialect + modal guard + regressions (Task 5), docs (Task 6). Every spec section maps to a task.
- **Modern-dialect horizontal motion:** handled by raw-key matching (`h/j/k/l` + arrows) in `handleVisualKey`, per the spec's dispatch note; covered by `TestVisual_ModernDialectEntersAndYanks` using plain `→`.
- **Mouse-path safety:** the caret render branch is gated on `selection.visual`, which mouse selection never sets; `finalizeSelection`/`clearSelection` changes are additive. Existing selection tests in `selection_test.go` are the regression guard.
- **Forward reference:** `handleVisualKey` calls `yankVisual`, introduced as a stub in Task 3 Step 5 and completed in Task 4 Step 5 — the package compiles at every task boundary.
