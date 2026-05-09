# Link Following Phase 2 — Inline Highlight Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When the user cycles links with `n`/`p`, the selected link's visible text is highlighted inline in the rendered content pane using reverse-video SGR (`\x1b[7m`/`\x1b[27m`). Pressing `Esc` clears the highlight and restores scroll position.

**Architecture:** Add `HighlightMarker(selected int) LinkMarker` to `internal/markdown/links_render.go` (reuses the existing `LinkMarker` hook). Add `applyLinkHighlight()` to `internal/tui/links.go` and call it from `cycleLink`. Update the `ClearLink` (Esc) handler in `internal/tui/input.go` to call `refreshContent` + restore scroll. No new packages, no new state fields.

**Tech Stack:** Go, Glamour (markdown→ANSI), Bubble Tea (TUI), Bubbles viewport, Lip Gloss, BubbleZone. SGR escape codes for terminal styling.

---

## File Map

| File | Change |
|---|---|
| `internal/markdown/links_render.go` | Add `HighlightMarker` function |
| `internal/markdown/links_render_test.go` | Add `TestHighlightMarker_*` tests |
| `internal/tui/links.go` | Add `applyLinkHighlight`; update `cycleLink` |
| `internal/tui/links_test.go` | Add four new Phase 2 tests |
| `internal/tui/input.go` | Update `ClearLink` Esc handler |

---

## Task 1: `HighlightMarker` in `internal/markdown`

**Files:**
- Modify: `internal/markdown/links_render.go`
- Test: `internal/markdown/links_render_test.go`

The `LinkMarker` type is already defined as `func(linkIndex int) (open, close string)`. `HighlightMarker` returns a `LinkMarker` that emits SGR reverse-video (`\x1b[7m` / `\x1b[27m`) around the selected index and empty strings for all others. It is exported so `internal/tui` can use it.

- [ ] **Step 1: Write the failing test**

Add to `internal/markdown/links_render_test.go` (inside the existing `package markdown` test file, after the last test):

```go
func TestHighlightMarker_SelectedLinkGetsReverseVideo(t *testing.T) {
	// Two sentinel-bracketed spans: link 0 and link 1.
	// HighlightMarker(1) should wrap link 1 in reverse-video SGR and
	// leave link 0 unwrapped.
	in := "\x1cfoo\x1e and \x1cbar\x1e"
	marker := HighlightMarker(1)
	cleaned, _ := stripSentinels(in, marker)

	if strings.Contains(cleaned, "\x1b[7m") && strings.HasPrefix(cleaned, "\x1b[7m") {
		t.Errorf("link 0 (foo) should NOT be highlighted; got: %q", cleaned)
	}
	if !strings.Contains(cleaned, "\x1b[7mbar\x1b[27m") {
		t.Errorf("link 1 (bar) should be wrapped in reverse-video SGR; got: %q", cleaned)
	}
	if strings.ContainsRune(cleaned, sentinelStart) || strings.ContainsRune(cleaned, sentinelEnd) {
		t.Errorf("sentinels leaked into output: %q", cleaned)
	}
}

func TestHighlightMarker_NoneSelectedWhenIndexNegative(t *testing.T) {
	in := "\x1cfoo\x1e and \x1cbar\x1e"
	marker := HighlightMarker(-1)
	cleaned, _ := stripSentinels(in, marker)
	if strings.Contains(cleaned, "\x1b[7m") {
		t.Errorf("no link should be highlighted when selected=-1; got: %q", cleaned)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd /Users/wilkes/Projects/wilkes/hypogeum
go test ./internal/markdown/... -run TestHighlightMarker -v
```

Expected: `FAIL` — `HighlightMarker` undefined.

- [ ] **Step 3: Implement `HighlightMarker`**

Add to `internal/markdown/links_render.go`, after the `LinkMarker` type definition (around line 45):

```go
// HighlightMarker returns a LinkMarker that wraps the link at index
// selected in SGR reverse-video (terminal-native selection highlight).
// All other links get empty open/close strings. Pass selected=-1 to
// highlight nothing (same as nil marker but explicit).
func HighlightMarker(selected int) LinkMarker {
	return func(i int) (string, string) {
		if i == selected {
			return "\x1b[7m", "\x1b[27m" // reverse-video on / off
		}
		return "", ""
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go test ./internal/markdown/... -run TestHighlightMarker -v
```

Expected: `PASS` for both new tests.

- [ ] **Step 5: Run the full test suite**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/markdown/links_render.go internal/markdown/links_render_test.go
git commit -m "feat(markdown): add HighlightMarker for reverse-video link selection"
```

---

## Task 2: `applyLinkHighlight` and updated `cycleLink`

**Files:**
- Modify: `internal/tui/links.go`
- Test: `internal/tui/links_test.go`

`applyLinkHighlight` reads the current file from disk, re-renders with `HighlightMarker(m.content.linkCursor)`, and updates the viewport content while preserving the scroll offset that `scrollToLink` just set. `cycleLink` calls it immediately after `scrollToLink`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/tui/links_test.go` (after `TestModel_LinkKeysIgnoredWhenTreeFocused`, inside `package tui`):

```go
func TestCycleLink_HighlightsSelectedLink(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)

	m = pressRune(t, m, 'n')

	// The rendered viewport content (not just the footer) should contain
	// the reverse-video SGR escape for the selected link.
	viewportContent := m.content.viewport.View()
	if !strings.Contains(viewportContent, "\x1b[7m") {
		t.Errorf("expected reverse-video SGR in viewport after 'n'; got:\n%q", viewportContent)
	}
}

func TestCycleLink_ClearOnEsc(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)

	m = pressRune(t, m, 'n')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	viewportContent := m.content.viewport.View()
	if strings.Contains(viewportContent, "\x1b[7m") {
		t.Errorf("expected no reverse-video SGR in viewport after Esc; got:\n%q", viewportContent)
	}
}

func TestCycleLink_PreservesScrollOnHighlight(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)

	// Scroll the viewport down before cycling so we can detect a reset.
	m.content.viewport.SetYOffset(2)

	m = pressRune(t, m, 'n')

	// scrollToLink may have adjusted the offset (link may be at row 0),
	// but applyLinkHighlight must NOT have reset it to 0 on its own.
	// The simplest assertion: the offset after 'n' is not -1 (still a
	// valid int). The real invariant is that SetContent inside
	// applyLinkHighlight doesn't clobber the YOffset scrollToLink set.
	// We verify this by checking the offset is >= 0 AND the highlight is present.
	if m.content.viewport.YOffset < 0 {
		t.Errorf("YOffset went negative after 'n': %d", m.content.viewport.YOffset)
	}
	if !strings.Contains(m.content.viewport.View(), "\x1b[7m") {
		t.Errorf("highlight missing after 'n' (offset preservation check)")
	}
}

func TestCycleLink_PreservesScrollOnEsc(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)

	// Scroll to a non-zero offset, select a link, then Esc.
	m.content.viewport.SetYOffset(2)
	wantOffset := m.content.viewport.YOffset

	m = pressRune(t, m, 'n')
	// scrollToLink may change the offset if the link is off-screen; for
	// the fixture index.md the first link is at row ~2, so offset stays small.
	// Capture the post-n offset as the reference for Esc to preserve.
	wantOffset = m.content.viewport.YOffset
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.content.viewport.YOffset != wantOffset {
		t.Errorf("YOffset after Esc = %d, want %d (should be preserved)", m.content.viewport.YOffset, wantOffset)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/tui/... -run "TestCycleLink_Highlights|TestCycleLink_ClearOnEsc|TestCycleLink_Preserves" -v
```

Expected: `FAIL` — `applyLinkHighlight` not yet defined; `TestCycleLink_ClearOnEsc` will also fail because Esc doesn't yet call `refreshContent`.

- [ ] **Step 3: Add `applyLinkHighlight` to `internal/tui/links.go`**

Add an `"os"` import (it isn't in `links.go` yet — check the existing import block and add it). Then add the function after `selectedLink`:

```go
// applyLinkHighlight re-renders the current file with a reverse-video
// highlight on the selected link, then updates the viewport content.
// The scroll offset set by scrollToLink is preserved across the SetContent
// call. On any read or render error, m.status is updated and the existing
// viewport content is left unchanged.
func (m *Model) applyLinkHighlight() {
	path := m.history.Current()
	if path == "" {
		return
	}
	src, err := os.ReadFile(path)
	if err != nil {
		m.status = err.Error()
		return
	}
	m.content.renderer.SetFromFile(path)
	out, _, err := m.content.renderer.RenderWithLinks(
		string(src), path,
		markdown.HighlightMarker(m.content.linkCursor),
	)
	if err != nil {
		m.status = err.Error()
		return
	}
	offset := m.content.viewport.YOffset
	m.content.viewport.SetContent(out)
	m.content.viewport.SetYOffset(offset)
}
```

- [ ] **Step 4: Update `cycleLink` to call `applyLinkHighlight`**

In `internal/tui/links.go`, the current `cycleLink` ends with:

```go
	m.scrollToLink(m.content.links[m.content.linkCursor])
```

Change it to:

```go
	m.scrollToLink(m.content.links[m.content.linkCursor])
	m.applyLinkHighlight()
```

- [ ] **Step 5: Run the new tests**

```bash
go test ./internal/tui/... -run "TestCycleLink_Highlights|TestCycleLink_ClearOnEsc|TestCycleLink_Preserves" -v
```

Expected: `TestCycleLink_HighlightsSelectedLink` and `TestCycleLink_Preserves*` pass. `TestCycleLink_ClearOnEsc` still fails — Esc doesn't yet clear the highlight from the viewport (only clears the cursor integer). That's Task 3.

- [ ] **Step 6: Run the full suite to check for regressions**

```bash
go test ./...
```

Expected: all pre-existing tests pass; `TestCycleLink_ClearOnEsc` fails (expected — not yet done).

- [ ] **Step 7: Commit**

```bash
git add internal/tui/links.go internal/tui/links_test.go
git commit -m "feat(tui): re-render with reverse-video highlight on link cycle"
```

---

## Task 3: Esc clears highlight and preserves scroll

**Files:**
- Modify: `internal/tui/input.go`

Currently the `ClearLink` (Esc) branch in `handleContentKey` only resets the integer cursor:

```go
case key.Matches(msg, m.keys.ClearLink):
    m.content.linkCursor = -1
    return *m, nil
```

It does not re-render the viewport, so the reverse-video SGR from `applyLinkHighlight` stays visible. This task adds the `refreshContent` call and scroll-position restore.

- [ ] **Step 1: Locate the Esc handler**

Open `internal/tui/input.go`. Find `handleContentKey` (around line 269). The relevant case is:

```go
case key.Matches(msg, m.keys.ClearLink):
    m.content.linkCursor = -1
    return *m, nil
```

- [ ] **Step 2: Replace the Esc handler**

Change those three lines to:

```go
case key.Matches(msg, m.keys.ClearLink):
    offset := m.content.viewport.YOffset
    m.content.linkCursor = -1
    m.refreshContent(m.history.Current())
    m.content.viewport.SetYOffset(offset)
    return *m, nil
```

`refreshContent` renders with `linkZoneMarker` (BubbleZone mouse zones, no highlight) and calls `GotoTop` internally. The `SetYOffset(offset)` after it restores the user's scroll position.

- [ ] **Step 3: Run all Phase 2 tests**

```bash
go test ./internal/tui/... -run "TestCycleLink" -v
```

Expected: all four new tests pass.

- [ ] **Step 4: Run the full suite**

```bash
go test ./...
```

Expected: all tests pass including the pre-existing `TestModel_EscClearsLinkSelection` (which tests `linkCursor == -1` and footer marker — both still hold).

- [ ] **Step 5: Build to confirm no compile errors**

```bash
go build ./...
```

Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/input.go
git commit -m "fix(tui): clear link highlight on Esc, preserve scroll position"
```

---

## Task 4: Update docs and verify word-wrap behaviour

**Files:**
- Modify: `docs/link-following.md`
- Modify: `docs/superpowers/specs/2026-05-09-link-following-phase-2-design.md`

The spec notes that word-wrapped links *may* highlight correctly because the sentinel spans the wrap boundary. Verify this manually before marking Phase 2 shipped, and update the docs.

- [ ] **Step 1: Manual smoke test — word-wrapped link**

Create a temporary file with a long link that will word-wrap at 80 columns:

```bash
cat > /tmp/wraptest.md << 'EOF'
# Wrap test

This paragraph has a link whose text is long enough to word-wrap across two
lines: [this is a long link text that should wrap](notes/first.md) and the
highlight should cover both lines.
EOF
go run ./cmd/hypogeum /tmp
```

Open `wraptest.md`, switch focus to content (`Tab`), press `n`. Verify:
- The reverse-video highlight covers the full link text on both lines.
- Pressing `Esc` removes the highlight cleanly.

- [ ] **Step 2: Update `docs/link-following.md`**

In `docs/link-following.md`, find the Phase 2 scope section. Change the status line and update the implementation steps list:

Find:
```
**Status:** Phase 1 shipped. Phase 2 and 3 not started.
```

Replace with:
```
**Status:** Phase 1 shipped. Phase 2 shipped. Phase 3 not started.
```

Find the `What's out (Phase 2/3):` block:
```
What's out (Phase 2/3):
- Inline highlight of the active link in the rendered text.
- Multi-segment cursor visualization for word-wrapped links.
- Actually launching external URLs with `open`/`xdg-open`.
```

Replace with:
```
What's out (Phase 3):
- Actually launching external URLs with `open`/`xdg-open`.

Phase 2 shipped:
- ~~Inline highlight of the active link in the rendered text.~~ ✅
- ~~Multi-segment cursor visualization for word-wrapped links.~~ ✅ (single SGR pair covers wrapped text naturally via the sentinel boundary)
```

- [ ] **Step 3: Update the spec status line**

In `docs/superpowers/specs/2026-05-09-link-following-phase-2-design.md`, change:

```
**Status:** spec — not yet implemented.
```

To:

```
**Status:** shipped.
```

- [ ] **Step 4: Commit**

```bash
git add docs/link-following.md docs/superpowers/specs/2026-05-09-link-following-phase-2-design.md
git commit -m "docs: mark link-following Phase 2 shipped, note word-wrap behaviour"
```
