# Finder Fuzzy Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add type-to-filter fuzzy matching to the `^p` picker. While the picker is open, every keystroke goes to a query line that fuzzy-filters the recency-ranked list in real time; matched characters are highlighted; `^j`/`^k` move the cursor since `j`/`k` are now typed.

**Architecture:** Layer a `bubbles/textinput.Model` and `[]fuzzy.Match` onto the existing `pickerState`. Capture the full recency list once on open; re-run the matcher on every keystroke. Sort by match score with the source-order (recency) index as a stable tiebreaker. Render two new rows above the existing flat list (prompt + separator) and apply inline bold/cyan highlighting to matched bytes after the per-row truncation.

**Tech Stack:** Go 1.23, Bubble Tea / Bubbles (`textinput`, `viewport`), Lip Gloss, `github.com/sahilm/fuzzy` (new), existing `internal/recent`.

**Spec:** [docs/superpowers/specs/2026-05-12-finder-fuzzy-filter-design.md](../specs/2026-05-12-finder-fuzzy-filter-design.md)

---

## File Structure

- **Modify** `go.mod` / `go.sum` — add `github.com/sahilm/fuzzy` and `github.com/charmbracelet/bubbles/textinput` (already in the bubbles module).
- **Modify** `internal/tui/picker.go` — add `input`, `all`, `matches` fields; add `refilter`, `renderQueryPrompt`, `renderSeparator`, `highlightMatch`; rewrite `View`/`renderRows`/`resizePicker`.
- **Modify** `internal/tui/input.go` — picker key block: dispatch `Esc`/`Enter`/`↑`/`↓`/`^j`/`^k` to picker, everything else to `textinput.Update`, call `refilter` on value change.
- **Modify** `internal/tui/keys.go` — add `PickerCursorUp` (`^k`) and `PickerCursorDown` (`^j`) bindings.
- **Modify** `internal/tui/model.go` — open-picker prep calls `m.modals.picker.openWith(all, root)` so the input is reset and focused.
- **Modify** `internal/tui/picker_test.go` — `TestPickerJKMovesCursor` now expects `j`/`k` to type into the query, not move cursor; rename/replace with `^j`/`^k` test.
- **Create** `internal/tui/picker_fuzzy_test.go` — the new behavior tests listed in the spec.
- **Modify** `README.md`, `CLAUDE.md`, `docs/index.md`, `docs/packages/tui.md`, `docs/superpowers/specs/2026-05-12-unified-finder-recency-design.md` — doc updates.

Single package (`internal/tui`) so test failures stay scoped. Each task below produces a working build with passing existing tests.

---

## Task 1: Add `sahilm/fuzzy` dependency and a smoke test

**Files:**
- Modify: `go.mod`, `go.sum`
- Test: `internal/tui/picker_fuzzy_test.go` (new — minimal sanity test)

- [ ] **Step 1: Add the dependency**

Run from repo root:
```bash
go get github.com/sahilm/fuzzy@latest
```

Expected: `go.mod` gains `github.com/sahilm/fuzzy vX.Y.Z`. `go.sum` updated.

- [ ] **Step 2: Write a smoke test that exercises the dependency**

Create `internal/tui/picker_fuzzy_test.go`:
```go
package tui

import (
	"testing"

	"github.com/sahilm/fuzzy"
)

// TestFuzzyDependencyAvailable is a sanity check that the matcher import
// works and the API shape matches what the picker code expects.
func TestFuzzyDependencyAvailable(t *testing.T) {
	matches := fuzzy.Find("hyp", []string{"hypogeum.md", "other.md"})
	if len(matches) != 1 {
		t.Fatalf("Find: got %d matches, want 1", len(matches))
	}
	if matches[0].Str != "hypogeum.md" {
		t.Errorf("Find: matched %q, want %q", matches[0].Str, "hypogeum.md")
	}
	if len(matches[0].MatchedIndexes) == 0 {
		t.Errorf("MatchedIndexes empty; expected positions in hypogeum.md")
	}
}
```

- [ ] **Step 3: Run the smoke test**

```bash
go test ./internal/tui/ -run TestFuzzyDependencyAvailable -v
```

Expected: PASS.

- [ ] **Step 4: Build everything**

```bash
go build ./...
```

Expected: no output, exit 0.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/tui/picker_fuzzy_test.go
git commit -m "deps: add sahilm/fuzzy for picker fuzzy filtering"
```

---

## Task 2: Add `^j` / `^k` cursor bindings to `keyMap`

**Files:**
- Modify: `internal/tui/keys.go`

Rationale: introduce the bindings before they're used in `input.go`; tests can reference them.

- [ ] **Step 1: Modify `keyMap` and `defaultKeys`**

In `internal/tui/keys.go`, add two fields and their defaults.

Add to the `keyMap` struct, after `OpenPicker`:
```go
	PickerCursorDown key.Binding
	PickerCursorUp   key.Binding
```

Add to `defaultKeys` return, after the `OpenPicker` line:
```go
		PickerCursorDown: key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("^j", "picker: next")),
		PickerCursorUp:   key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("^k", "picker: prev")),
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: no output, exit 0.

- [ ] **Step 3: Run existing tests**

```bash
go test ./...
```

Expected: PASS. No new bindings are bound to handlers yet, so behavior is unchanged.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/keys.go
git commit -m "feat(tui): keyMap bindings for picker ^j/^k cursor"
```

---

## Task 3: Add textinput + `all` field to `pickerState`; reset on open

**Files:**
- Modify: `internal/tui/picker.go`
- Modify: `internal/tui/model.go` (initialization site)
- Test: `internal/tui/picker_fuzzy_test.go` (extend)

This task adds the storage for the query, but no key dispatch yet — the picker still works exactly as before from the user's perspective.

- [ ] **Step 1: Write a failing test for the new fields**

Append to `internal/tui/picker_fuzzy_test.go`:
```go
import (
	"path/filepath"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPickerOpenInitializesQuery(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "b.md"), "# B")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	if got := m.modals.picker.input.Value(); got != "" {
		t.Errorf("input.Value on open: got %q want empty", got)
	}
	if !m.modals.picker.input.Focused() {
		t.Errorf("input should be focused on picker open")
	}
	if len(m.modals.picker.all) != 2 {
		t.Errorf("all: got %d entries, want 2", len(m.modals.picker.all))
	}
	if len(m.modals.picker.all) != len(m.modals.picker.ranked) {
		t.Errorf("ranked should equal all on open: %d vs %d",
			len(m.modals.picker.ranked), len(m.modals.picker.all))
	}
}
```

Combine the existing `"testing"` and `"github.com/sahilm/fuzzy"` imports with the new ones into a single import block — Go's `goimports` will sort them.

- [ ] **Step 2: Run the test — expect failure**

```bash
go test ./internal/tui/ -run TestPickerOpenInitializesQuery -v
```

Expected: FAIL (compile error: `m.modals.picker.input` and `.all` do not exist).

- [ ] **Step 3: Add the fields to `pickerState`**

In `internal/tui/picker.go`, change the struct definition:
```go
type pickerState struct {
	all     []recent.Ranked  // full ranked list captured at open time
	ranked  []recent.Ranked  // currently visible (filtered or all)
	matches []fuzzy.Match    // parallel to ranked when query non-empty
	cursor  int
	vp      viewport.Model
	root    string
	input   textinput.Model
}
```

Add imports (combine with existing):
```go
import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/sahilm/fuzzy"
)
```

- [ ] **Step 4: Update `newPicker` to construct the textinput**

Replace the current `newPicker`:
```go
func newPicker() pickerState {
	ti := textinput.New()
	ti.Prompt = ""           // we render our own "> " prefix
	ti.Placeholder = ""
	ti.CharLimit = 256
	return pickerState{
		vp:    viewport.New(0, 0),
		input: ti,
	}
}
```

- [ ] **Step 5: Update `reset` to also set `all`, clear the query, focus the input**

Replace the current `reset`:
```go
// reset populates the picker with a fresh ranked list, resets the cursor
// and query, and focuses the textinput. Called on every picker open.
func (p *pickerState) reset(ranked []recent.Ranked, root string) {
	p.all = ranked
	p.ranked = ranked
	p.matches = nil
	p.cursor = 0
	p.root = root
	p.input.SetValue("")
	p.input.Focus()
	p.refreshVP()
}
```

The picker-open site in `internal/tui/input.go` already calls `m.modals.picker.reset(ranked, m.root)` — no change needed there yet.

- [ ] **Step 6: Run the new test — expect pass**

```bash
go test ./internal/tui/ -run TestPickerOpenInitializesQuery -v
```

Expected: PASS.

- [ ] **Step 7: Run the full suite — expect pass**

```bash
go test ./...
```

Expected: PASS. The picker still works as before because no key dispatch routes to `input`.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/picker.go internal/tui/picker_fuzzy_test.go
git commit -m "feat(tui): textinput + 'all' on pickerState; reset focuses input"
```

---

## Task 4: Implement `refilter` and use it from a unit test (no key dispatch yet)

**Files:**
- Modify: `internal/tui/picker.go`
- Test: `internal/tui/picker_fuzzy_test.go` (extend)

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/picker_fuzzy_test.go`:
```go
func TestRefilterEmptyQueryRestoresAll(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "hyp.md"), "# H")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	m.modals.picker.input.SetValue("hyp")
	m.modals.picker.refilter()
	if len(m.modals.picker.ranked) != 1 {
		t.Fatalf("after 'hyp': %d matches, want 1", len(m.modals.picker.ranked))
	}

	m.modals.picker.input.SetValue("")
	m.modals.picker.refilter()
	if len(m.modals.picker.ranked) != 2 {
		t.Errorf("after clearing query: %d entries, want 2", len(m.modals.picker.ranked))
	}
	if m.modals.picker.matches != nil {
		t.Errorf("matches should be nil after clearing query")
	}
}

func TestRefilterScoresWithRecencyTiebreaker(t *testing.T) {
	// Two paths with identical fuzzy scores — the one earlier in `all`
	// (more recent) must win the stable tiebreak.
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a-hyp.md")
	p2 := filepath.Join(dir, "b-hyp.md")
	writePickerFile(t, p1, "# A")
	writePickerFile(t, p2, "# B")

	m := sized(t, dir, "")
	// Open p1 first; this bumps it ahead of p2 in `all`.
	m.openFile(p1)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	m.modals.picker.input.SetValue("hyp")
	m.modals.picker.refilter()
	if len(m.modals.picker.ranked) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m.modals.picker.ranked))
	}
	if got := m.modals.picker.ranked[0].Path; got != p1 {
		t.Errorf("after recency tiebreak: top=%q, want %q", got, p1)
	}
}

func TestRefilterCursorResetsToZero(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "alpha.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "beta.md"), "# B")
	writePickerFile(t, filepath.Join(dir, "gamma.md"), "# G")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	m.modals.picker.cursor = 2
	m.modals.picker.input.SetValue("a")
	m.modals.picker.refilter()
	if got := m.modals.picker.cursor; got != 0 {
		t.Errorf("cursor after refilter: %d want 0", got)
	}
}
```

- [ ] **Step 2: Run the tests — expect failure**

```bash
go test ./internal/tui/ -run TestRefilter -v
```

Expected: FAIL (compile error: `refilter` undefined).

- [ ] **Step 3: Implement `refilter`**

Add to `internal/tui/picker.go` after `reset`:
```go
// refilter recomputes p.ranked and p.matches from p.all and the current
// query. Empty query → ranked == all, matches == nil. Otherwise: run
// sahilm/fuzzy over a lowercased copy of the paths, then stable-sort by
// score descending with the source-order index (i.e. recency rank) as
// the tiebreaker. Cursor resets to 0 on every call.
func (p *pickerState) refilter() {
	q := strings.ToLower(p.input.Value())
	if q == "" {
		p.ranked = p.all
		p.matches = nil
		p.cursor = 0
		p.refreshVP()
		return
	}
	src := make([]string, len(p.all))
	for i, r := range p.all {
		src[i] = strings.ToLower(relativeTo(p.root, r.Path))
	}
	raw := fuzzy.Find(q, src)
	sort.SliceStable(raw, func(i, j int) bool {
		if raw[i].Score != raw[j].Score {
			return raw[i].Score > raw[j].Score
		}
		return raw[i].Index < raw[j].Index
	})
	p.ranked = make([]recent.Ranked, len(raw))
	p.matches = make([]fuzzy.Match, len(raw))
	for i, m := range raw {
		p.ranked[i] = p.all[m.Index]
		p.matches[i] = m
	}
	p.cursor = 0
	p.refreshVP()
}
```

Add the `"sort"` import to the existing import block.

- [ ] **Step 4: Run the new tests — expect pass**

```bash
go test ./internal/tui/ -run TestRefilter -v
```

Expected: PASS for all three.

- [ ] **Step 5: Run the full suite — expect pass**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/picker.go internal/tui/picker_fuzzy_test.go
git commit -m "feat(tui): refilter() runs fuzzy.Find with recency tiebreaker"
```

---

## Task 5: Render the prompt and separator rows; resize math

**Files:**
- Modify: `internal/tui/picker.go`
- Test: existing visual tests in `picker_test.go` (no new tests; verified via integration test in Task 8)

- [ ] **Step 1: Add the renderQueryPrompt and renderSeparator helpers**

Append to `internal/tui/picker.go`:
```go
// renderQueryPrompt returns the "> <input>" row at the top of the picker.
func (p *pickerState) renderQueryPrompt() string {
	return "> " + p.input.View()
}

// renderSeparator returns a horizontal rule the width of the viewport.
func (p *pickerState) renderSeparator() string {
	w := p.vp.Width
	if w < 1 {
		w = 1
	}
	return strings.Repeat("─", w)
}
```

- [ ] **Step 2: Update `View` to prepend prompt + separator**

Replace the existing `View` method:
```go
// View returns the picker's renderable string: prompt, separator, list.
func (p *pickerState) View() string {
	if len(p.all) == 0 {
		return p.renderQueryPrompt() + "\n" + p.renderSeparator() + "\n" +
			lipgloss.NewStyle().Faint(true).Render("(no markdown files in vault)")
	}
	return p.renderQueryPrompt() + "\n" + p.renderSeparator() + "\n" + p.vp.View()
}
```

- [ ] **Step 3: Update `resizePicker` to shrink viewport height by 2**

Replace `resizePicker` in `internal/tui/picker.go`:
```go
// resizePicker fits the picker viewport into the modal interior, leaving
// two rows at the top for the query prompt and separator.
func (m *Model) resizePicker() {
	_, _, w, h := modalGeometry(m.width, m.height)
	pw := w - 2
	ph := h - 2 - 2 // border (2) + prompt+separator (2)
	if pw < 1 {
		pw = 1
	}
	if ph < 1 {
		ph = 1
	}
	m.modals.picker.vp.Width = pw
	m.modals.picker.vp.Height = ph
	m.modals.picker.input.Width = pw - 2 // leave room for "> " prefix
	m.modals.picker.refreshVP()
}
```

- [ ] **Step 4: Build and run all tests**

```bash
go build ./... && go test ./...
```

Expected: PASS. Visual regression in existing picker tests is fine because they assert behavior, not rendered geometry.

- [ ] **Step 5: Manual sanity check**

```bash
go run ./cmd/hypogeum docs
```

Press `^p`. Expected: modal opens with a `> ` prompt line, a horizontal separator below it, and the recency-ranked list below that. Press `q` to quit.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/picker.go
git commit -m "feat(tui): render query prompt + separator above picker list"
```

---

## Task 6: Wire key dispatch — textinput receives keystrokes, refilter on change

**Files:**
- Modify: `internal/tui/input.go`
- Modify: `internal/tui/picker_test.go` (existing j/k test must be updated)
- Test: `internal/tui/picker_fuzzy_test.go` (extend)

This is the user-facing turning point: typing into the picker filters.

- [ ] **Step 1: Update the existing `TestPickerJKMovesCursor` test**

In `internal/tui/picker_test.go`, replace `TestPickerJKMovesCursor` (entirely):
```go
// `j` / `k` are now typed into the query — they no longer move the cursor.
// The new bindings are `^j` / `^k`.
func TestPickerCtrlJKMovesCursor(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "b.md"), "# B")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	if got := m.modals.picker.cursor; got != 0 {
		t.Fatalf("initial cursor: %d, want 0", got)
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlJ})
	if got := m.modals.picker.cursor; got != 1 {
		t.Errorf("after ^j: cursor=%d, want 1", got)
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlK})
	if got := m.modals.picker.cursor; got != 0 {
		t.Errorf("after ^k: cursor=%d, want 0", got)
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlK})
	if got := m.modals.picker.cursor; got != 0 {
		t.Errorf("^k at top: cursor=%d, want 0", got)
	}
}
```

- [ ] **Step 2: Write failing tests for the dispatch behavior**

Append to `internal/tui/picker_fuzzy_test.go`:
```go
func TestPickerTypingFiltersList(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "alpha.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "beta.md"), "# B")
	writePickerFile(t, filepath.Join(dir, "alphabet.md"), "# AB")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	m = pressRune(t, m, 'a')
	// All three contain 'a'. Confirm typing flows into the query.
	if got := m.modals.picker.input.Value(); got != "a" {
		t.Errorf("input.Value after 'a': %q want %q", got, "a")
	}
	// All three matched.
	if got := len(m.modals.picker.ranked); got != 3 {
		t.Errorf("ranked after 'a': %d want 3", got)
	}

	m = pressRune(t, m, 'l')
	// "alpha", "alphabet" both contain 'a' then 'l'. "beta" does not.
	if got := len(m.modals.picker.ranked); got != 2 {
		t.Errorf("ranked after 'al': %d want 2", got)
	}
}

func TestPickerEscClearsQueryBeforeClosing(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "b.md"), "# B")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m = pressRune(t, m, 'a')

	if m.modals.picker.input.Value() == "" {
		t.Fatal("setup: query should be non-empty")
	}

	// First Esc → clear query, modal still open.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.modals.kind != modalPicker {
		t.Errorf("after first Esc: modal kind=%d, want modalPicker", m.modals.kind)
	}
	if m.modals.picker.input.Value() != "" {
		t.Errorf("after first Esc: query=%q, want empty", m.modals.picker.input.Value())
	}

	// Second Esc → close.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.modals.kind != modalNone {
		t.Errorf("after second Esc: modal kind=%d, want modalNone", m.modals.kind)
	}
}

func TestPickerEnterOpensFilteredSelection(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "alpha.md")
	p2 := filepath.Join(dir, "beta.md")
	writePickerFile(t, p1, "# A")
	writePickerFile(t, p2, "# B")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	m = pressRune(t, m, 'b')
	if got := len(m.modals.picker.ranked); got != 1 {
		t.Fatalf("after 'b': %d matches, want 1", got)
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.history.Current(); got != p2 {
		t.Errorf("Enter after filter: opened %q want %q", got, p2)
	}
}
```

- [ ] **Step 3: Run new tests — expect failure**

```bash
go test ./internal/tui/ -run "TestPickerTyping|TestPickerEscClears|TestPickerEnterOpensFiltered|TestPickerCtrlJK" -v
```

Expected: FAIL (typing keys do not reach textinput; ^j/^k not bound).

- [ ] **Step 4: Rewrite the picker key block in `input.go`**

In `internal/tui/input.go`, replace the existing `if m.modals.kind == modalPicker { … }` block (currently the `switch` on lines 167–190) with:
```go
		if m.modals.kind == modalPicker {
			switch {
			case key.Matches(msg, m.keys.ClearLink): // Esc
				if m.modals.picker.input.Value() != "" {
					m.modals.picker.input.SetValue("")
					m.modals.picker.refilter()
					return *m, nil
				}
				m.modals.kind = modalNone
				m.focus = m.modals.prevFocus
				return *m, nil
			case key.Matches(msg, m.keys.Open):
				if path, ok := m.modals.picker.selectedPath(); ok {
					m.modals.kind = modalNone
					m.focus = m.modals.prevFocus
					m.navigateTo(path)
				}
				return *m, nil
			case key.Matches(msg, m.keys.Up),
				key.Matches(msg, m.keys.PickerCursorUp):
				if m.modals.picker.cursor > 0 {
					m.modals.picker.cursor--
					m.modals.picker.refreshVP()
				}
				return *m, nil
			case key.Matches(msg, m.keys.Down),
				key.Matches(msg, m.keys.PickerCursorDown):
				if m.modals.picker.cursor < len(m.modals.picker.ranked)-1 {
					m.modals.picker.cursor++
					m.modals.picker.refreshVP()
				}
				return *m, nil
			}
			// Forward anything else to the textinput; refilter on change.
			before := m.modals.picker.input.Value()
			var cmd tea.Cmd
			m.modals.picker.input, cmd = m.modals.picker.input.Update(msg)
			if m.modals.picker.input.Value() != before {
				m.modals.picker.refilter()
			}
			return *m, cmd
		}
```

Notes for the implementer:

- `m.keys.ClearLink` is the existing Esc binding (kept across the codebase as the "clear" key).
- The `Up`/`Down` cases match both the arrow keys (already in `m.keys.Up`/`Down`) and the new `^j`/`^k` bindings.
- Mouse events still go through `handleMouse`; picker has no mouse target today and that stays.

- [ ] **Step 5: Run all picker tests — expect pass**

```bash
go test ./internal/tui/ -run "TestPicker|TestRefilter|TestFuzzy" -v
```

Expected: PASS for all.

- [ ] **Step 6: Run the full suite — expect pass**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Manual sanity check**

```bash
go run ./cmd/hypogeum docs
```

Press `^p`, type a few letters, watch the list narrow. Press `^j`/`^k` to move the cursor. Press Esc once to clear the query; press Esc again to close. Quit with `q`.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/input.go internal/tui/picker_test.go internal/tui/picker_fuzzy_test.go
git commit -m "feat(tui): wire textinput keystrokes through the picker; ^j/^k move cursor"
```

---

## Task 7: No-match state, overflow cap, and "X more" footer

**Files:**
- Modify: `internal/tui/picker.go`
- Test: `internal/tui/picker_fuzzy_test.go` (extend)

- [ ] **Step 1: Write failing tests**

Add `"strconv"` and `"strings"` to the existing import block in `internal/tui/picker_fuzzy_test.go` (consolidate with imports added in earlier tasks).

Append to `internal/tui/picker_fuzzy_test.go`:
```go
func TestPickerNoMatchState(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "alpha.md"), "# A")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	for _, r := range "xyzqq" {
		m = pressRune(t, m, r)
	}
	if got := len(m.modals.picker.ranked); got != 0 {
		t.Fatalf("ranked: got %d, want 0", got)
	}
	view := m.modals.picker.View()
	if !strings.Contains(view, `no match for "xyzqq"`) {
		t.Errorf("View should report no match; got:\n%s", view)
	}

	// Enter on empty list is a no-op (modal stays open).
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.modals.kind != modalPicker {
		t.Errorf("Enter on no-match should not close picker; kind=%d", m.modals.kind)
	}
}

func TestPickerOverflowCap(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 250; i++ {
		name := "x" + strconv.Itoa(i) + ".md"
		writePickerFile(t, filepath.Join(dir, name), "# x")
	}

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m = pressRune(t, m, 'x')

	if got := len(m.modals.picker.ranked); got < 200 {
		t.Fatalf("setup: %d matches; need >=200", got)
	}
	overflow := len(m.modals.picker.ranked) - 200
	view := m.modals.picker.View()
	want := "… " + strconv.Itoa(overflow) + " more"
	if !strings.Contains(view, want) {
		t.Errorf("expected footer %q in View; got:\n%s", want, view)
	}
}
```

- [ ] **Step 2: Run new tests — expect failure**

```bash
go test ./internal/tui/ -run "TestPickerNoMatch|TestPickerOverflowCapClean" -v
```

Expected: FAIL (renderer doesn't yet emit no-match text or footer).

- [ ] **Step 3: Add the cap constant and update `renderRows`**

In `internal/tui/picker.go`, add at the top of the file (after the imports):
```go
// pickerMaxVisible caps how many rows render at once. The cursor is
// clamped to this range; refining the query is the way to reach hidden
// rows.
const pickerMaxVisible = 200
```

Replace `renderRows` with:
```go
// renderRows builds the picker's display string. Honors pickerMaxVisible
// (appending "… N more" when filtered length exceeds it) and the no-match
// state. No score is shown — it's a sorting signal, not a UX signal.
func (p *pickerState) renderRows() string {
	width := p.vp.Width
	if width < 20 {
		width = 20
	}
	if p.input.Value() != "" && len(p.ranked) == 0 {
		return lipgloss.NewStyle().Faint(true).
			Render(`(no match for "` + p.input.Value() + `")`)
	}

	now := time.Now()
	var b strings.Builder
	visible := len(p.ranked)
	if visible > pickerMaxVisible {
		visible = pickerMaxVisible
	}
	for i := 0; i < visible; i++ {
		r := p.ranked[i]
		rel := relativeTo(p.root, r.Path)
		recencyLabel, edited := pickRecencyLabel(now, r.MTime, r.Visit)
		suffix := recencyLabel
		if edited {
			suffix += " · edited"
		}
		line := formatPickerRow(rel, suffix, width)
		if i == p.cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if overflow := len(p.ranked) - pickerMaxVisible; overflow > 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).
			Render("… " + strconv.Itoa(overflow) + " more"))
		b.WriteByte('\n')
	}
	return b.String()
}
```

Add `"strconv"` to the imports.

- [ ] **Step 4: Clamp cursor inside `refilter` and on cursor moves**

The cursor must not point past the visible cap. Update the cursor-step block in `internal/tui/input.go` for `Up`/`Down` (the picker block written in Task 6) to clamp at the visible cap. Replace the relevant cases:
```go
			case key.Matches(msg, m.keys.Down),
				key.Matches(msg, m.keys.PickerCursorDown):
				lim := len(m.modals.picker.ranked)
				if lim > pickerMaxVisible {
					lim = pickerMaxVisible
				}
				if m.modals.picker.cursor < lim-1 {
					m.modals.picker.cursor++
					m.modals.picker.refreshVP()
				}
				return *m, nil
```

(`Up` already clamps to 0 — leave it as-is.)

- [ ] **Step 5: Run the new tests — expect pass**

```bash
go test ./internal/tui/ -run "TestPickerNoMatch|TestPickerOverflowCapClean" -v
```

Expected: PASS.

- [ ] **Step 6: Run the full suite — expect pass**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/picker.go internal/tui/input.go internal/tui/picker_fuzzy_test.go
git commit -m "feat(tui): picker no-match state + 200-row visible cap with overflow footer"
```

---

## Task 8: Highlight matched characters

**Files:**
- Modify: `internal/tui/picker.go`
- Test: `internal/tui/picker_fuzzy_test.go` (extend)

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/picker_fuzzy_test.go`:
```go
func TestPickerHighlightsMatchedChars(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "hypogeum.md"), "# H")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m = pressRune(t, m, 'h')
	m = pressRune(t, m, 'y')
	m = pressRune(t, m, 'p')

	view := m.modals.picker.View()
	// The bold SGR is the cheapest fingerprint of the highlight style.
	// CSI 1 m is "bold on". Without highlighting the view would not
	// contain it for a plain-path row.
	if !strings.Contains(view, "\x1b[") {
		t.Errorf("expected ANSI escape in view; got:\n%q", view)
	}
	if !strings.Contains(view, "hypogeum.md") {
		t.Errorf("expected basename in view; got:\n%q", view)
	}
}

func TestPickerHighlightMultibyte(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "日本語.md"), "# JA")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	// Type the first character of the filename.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'日'}})

	if got := len(m.modals.picker.ranked); got != 1 {
		t.Fatalf("expected 1 match for '日', got %d", got)
	}
	view := m.modals.picker.View()
	if !strings.Contains(view, "日本語.md") {
		t.Errorf("expected multibyte basename in view; got:\n%q", view)
	}
}
```

- [ ] **Step 2: Run the new tests — expect failure**

```bash
go test ./internal/tui/ -run "TestPickerHighlight" -v
```

Expected: `TestPickerHighlightsMatchedChars` FAILS (no ANSI emitted for matched chars).
`TestPickerHighlightMultibyte` may PASS — it tests the filter, not highlighting. Either is fine; the highlight assertion is the meaningful one.

- [ ] **Step 3: Add `highlightMatch` and wire it into `renderRows`**

Add to `internal/tui/picker.go`:
```go
// highlightStyle is the lipgloss style for matched characters in a row.
var highlightStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))

// highlightMatch wraps the bytes of display that match query (via a
// fresh fuzzy.Find pass) in highlightStyle. Returns display unchanged
// if query is empty or doesn't match. Called per visible row after
// truncation, so indices map to the truncated string.
func highlightMatch(display, query string) string {
	if query == "" {
		return display
	}
	src := strings.ToLower(display)
	matches := fuzzy.Find(strings.ToLower(query), []string{src})
	if len(matches) == 0 {
		return display
	}
	idx := matches[0].MatchedIndexes
	if len(idx) == 0 {
		return display
	}
	var b strings.Builder
	in := false
	for i := 0; i < len(display); i++ {
		if contains(idx, i) {
			if !in {
				b.WriteString(highlightStyle.Render(string(display[i])))
				in = true
				continue
			}
			b.WriteString(highlightStyle.Render(string(display[i])))
			continue
		}
		in = false
		b.WriteByte(display[i])
	}
	return b.String()
}

// contains reports whether n is in the (small) sorted slice s.
func contains(s []int, n int) bool {
	for _, v := range s {
		if v == n {
			return true
		}
		if v > n {
			return false
		}
	}
	return false
}
```

Update `renderRows` to highlight the (already-truncated) path *before* the row is assembled. That side-steps having to slice `formatPickerRow`'s output. Replace the row body in `renderRows`:
```go
		rel := relativeTo(p.root, r.Path)
		recencyLabel, edited := pickRecencyLabel(now, r.MTime, r.Visit)
		suffix := recencyLabel
		if edited {
			suffix += " · edited"
		}
		// Highlight the path *before* formatting. We pre-truncate the
		// path to the column budget so the highlight indices map to the
		// visible string. formatPickerRow then pads/joins the highlighted
		// path with the suffix; the ANSI escapes survive the joining.
		pathDisplay := preTruncatePath(rel, suffix, width)
		if p.input.Value() != "" {
			pathDisplay = highlightMatch(pathDisplay, p.input.Value())
		}
		line := formatPickerRow(pathDisplay, suffix, width)
		if i == p.cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
```

Add a small helper to keep the truncation logic with the row code:
```go
// preTruncatePath returns the path trimmed to whatever column budget
// formatPickerRow would have available for the left side. Centralizes
// the width math so highlightMatch operates on the actually-visible
// characters.
func preTruncatePath(path, suffix string, width int) string {
	const gap = 2
	rightW := ansi.StringWidth(suffix)
	leftBudget := width - rightW - gap
	if leftBudget < 5 {
		return path // formatPickerRow will substitute padding+suffix anyway
	}
	return truncateLeadingEllipsis(path, leftBudget)
}
```

Why this works: `formatPickerRow` already calls `truncateLeadingEllipsis` itself, but it's idempotent — calling it again on the pre-truncated string is a cheap no-op. `formatPickerRow` then pads with spaces and appends the (un-highlighted) suffix. The ANSI escapes in the path survive the surrounding `strings.Repeat`/concatenation because they're just bytes.

- [ ] **Step 4: Run the new tests — expect pass**

```bash
go test ./internal/tui/ -run "TestPickerHighlight" -v
```

Expected: both PASS.

- [ ] **Step 5: Run the full suite — expect pass**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Manual sanity check**

```bash
go run ./cmd/hypogeum docs
```

Press `^p`, type a few characters. The matched characters in each visible row should appear bold cyan. Selected row stays reverse-video over the top.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/picker.go internal/tui/picker_fuzzy_test.go
git commit -m "feat(tui): highlight matched chars in picker rows"
```

---

## Task 9: Documentation updates

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `docs/index.md`
- Modify: `docs/packages/tui.md`
- Modify: `docs/superpowers/specs/2026-05-12-unified-finder-recency-design.md`

- [ ] **Step 1: Update `CLAUDE.md`**

In `/Users/wilkes/Projects/wilkes/hypogeum/CLAUDE.md`, find the `^p` gotcha (currently starts with "`^p` opens a flat recency-ranked finder as a fourth `modalKind`"). Replace the entire bullet (one bullet only) with:
```markdown
- **`^p` opens a flat recency-ranked finder with type-to-filter.** Opens as a fourth `modalKind`. The textinput is focused from the moment the picker opens; printable keystrokes go to the query, and the result list re-filters via `sahilm/fuzzy` (subsequence, case-insensitive). Sort is match score first with the source-order index (i.e. recency rank in `p.all`) as a stable tiebreaker. Empty query falls back to the pure recency list. `^j` / `^k` move the cursor since `j` / `k` now type into the query; arrow keys also move the cursor. `Esc` clears a non-empty query before closing on the second press. Visible rows are capped at `pickerMaxVisible` (200); overflow shows a faint `… N more` footer. The hybrid recency score (filesystem mtime, 7-day half-life + persisted visits, 2-day half-life × 1.5) lives in `internal/recent`. See [finder-fuzzy-filter](docs/superpowers/specs/2026-05-12-finder-fuzzy-filter-design.md) and [unified-finder-recency](docs/superpowers/specs/2026-05-12-unified-finder-recency-design.md).
```

- [ ] **Step 2: Update `README.md`**

Find the row in the keys table where `^p` is described (currently along the lines of "file picker" / "file finder"). Replace with:
```markdown
| `^p`          | Open file finder (type to fuzzy-filter; ^j/^k cursor)                |
```

If the table has a "details" or "notes" subsection mentioning the finder, append:
```markdown
The `^p` finder ranks by visit/mtime recency by default. Type to fuzzy-filter; matched characters are highlighted. `Esc` clears the query before closing.
```

- [ ] **Step 3: Update `docs/index.md`**

Under "Active feature work" (or the equivalent section), add:
```markdown
- [finder-fuzzy-filter](superpowers/specs/2026-05-12-finder-fuzzy-filter-design.md) — type-to-filter on top of the recency-ranked picker. Shipped 2026-05-12.
```

- [ ] **Step 4: Update `docs/packages/tui.md`**

Find the "finder" or "picker" section. Replace its body with:
```markdown
The `^p` finder is a modal (`modalKind == modalPicker`) over every markdown file in the vault. On open, the model calls `recent.Rank(allVaultMarkdownPaths())` to get a `[]recent.Ranked` ordered by the hybrid recency score; that slice is stored on `pickerState.all` and not refreshed mid-modal.

The textinput is focused immediately. Each keystroke flows through `textinput.Update`; on value change, `refilter` runs `sahilm/fuzzy.Find` over a lowercased copy of the paths and re-sorts by match score (descending) with the source-order index as a stable tiebreaker — the latter preserves the recency order within a score tier. Rendered rows highlight matched bytes in bold cyan; selected row is reverse-video.

`^j`/`^k` (and `↑`/`↓`) move the cursor; `j`/`k` are typed characters now. `Esc` clears a non-empty query first, closes on the second press. `Enter` opens the selected row through `m.openFile`, which records a visit through `recent.Record`.

Specs: [unified-finder-recency](../superpowers/specs/2026-05-12-unified-finder-recency-design.md), [finder-fuzzy-filter](../superpowers/specs/2026-05-12-finder-fuzzy-filter-design.md).
```

- [ ] **Step 5: Update the prior spec's "Phasing" section**

In `docs/superpowers/specs/2026-05-12-unified-finder-recency-design.md`, find the "Phasing" section's Phase 2 bullet for name-filter typing. Append:
```markdown
  - Extended by: [finder-fuzzy-filter](2026-05-12-finder-fuzzy-filter-design.md).
```

- [ ] **Step 6: Verify docs render-ish (no broken intra-repo links)**

```bash
git -C /Users/wilkes/Projects/wilkes/hypogeum grep -F "2026-05-12-finder-fuzzy-filter"
```

Expected: links appear in `CLAUDE.md`, `docs/index.md`, `docs/packages/tui.md`, both spec files.

- [ ] **Step 7: Commit**

```bash
git add CLAUDE.md README.md docs/index.md docs/packages/tui.md docs/superpowers/specs/2026-05-12-unified-finder-recency-design.md
git commit -m "docs: update finder docs for fuzzy filter"
```

---

## Task 10: Final verification and push

**Files:** none (verification only)

- [ ] **Step 1: Build, vet, test the whole module**

```bash
go build ./... && go vet ./... && go test ./...
```

Expected: all pass.

- [ ] **Step 2: Run the binary against a real directory**

```bash
go run ./cmd/hypogeum docs
```

Walk through the user flow once more:
1. `^p` — picker opens with `> ` prompt at top, separator, recency list below.
2. Type `f` — list filters; matched `f` characters are bold cyan.
3. Type `fu` — list filters further.
4. `^j` / `^k` — cursor moves; selected row reverse-video.
5. `Esc` — query clears; full recency list returns; picker still open.
6. `Esc` — picker closes.
7. `^p`, navigate with arrows, `Enter` — file opens, history updates.
8. `q` — quit.

If anything looks off, debug; don't commit until clean.

- [ ] **Step 3: Push and confirm CI**

```bash
git push
```

Expected: branch updated; PR #18 (`finder-fuzzy-filter`) shows the new commits. Spec PR is now also implementation PR. Update the PR title and body to reflect that — open the existing PR description and replace the "spec only" lines with an implementation summary.

```bash
gh pr edit 18 --title "Finder: fuzzy filter on the recency picker" --body "$(cat <<'EOF'
## Summary

- Adds type-to-filter fuzzy matching to the `^p` picker.
- Always-on `textinput` is focused on open; printable keystrokes filter via `sahilm/fuzzy` (subsequence, case-insensitive).
- Sort: match score with the source-order (recency) index as a stable tiebreaker; empty query falls back to the pure recency list.
- `^j` / `^k` move the cursor since `j` / `k` now type into the query.
- `Esc` clears a non-empty query before closing on the second press.
- Matched characters are highlighted bold cyan; visible rows capped at 200 with a faint `… N more` overflow footer.

Spec: [docs/superpowers/specs/2026-05-12-finder-fuzzy-filter-design.md](docs/superpowers/specs/2026-05-12-finder-fuzzy-filter-design.md)
Plan: [docs/superpowers/plans/2026-05-12-finder-fuzzy-filter.md](docs/superpowers/plans/2026-05-12-finder-fuzzy-filter.md)

## Test plan

- [ ] `go test ./...` passes
- [ ] `^p` then type — list filters, matched chars highlighted
- [ ] `^j` / `^k` move cursor; arrow keys also work
- [ ] First `Esc` clears query; second closes
- [ ] `Enter` on a filtered row opens that file

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Plan complete.
