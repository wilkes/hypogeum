# Glamour table wrap — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop Glamour from truncating long markdown-table cells mid-word ("honorif…"); cells should wrap to multiple lines so the user sees the full content.

**Architecture:** Bump `github.com/charmbracelet/glamour` from v0.8.0 → v0.10.0. v0.10.0 flips the table-cell style from `Inline(true)` (truncate) to `Inline(false)` (wrap) and adds two relevant options. We use both: `WithInlineTableLinks(true)` suppresses Glamour's new "links footer" so it doesn't duplicate every table link (and so our URL-suppression + sentinel-instrumentation pipeline keeps working unchanged); pinning the table border glyphs to `│`/`─` in `applyHypogeumOverrides` keeps the existing `isTableBorderByte` / `urlSuppressStrip` alignment invariants intact regardless of which Glamour theme is active (the v0.10.0 ASCII/NoTTY theme started shipping `|`/`-` separators by default).

**Tech Stack:** Go, Charm `glamour`/`lipgloss`, the existing `internal/markdown` package (`render.go`, `style.go`, `links_render.go`).

**Background:** Glamour 0.8.0's `ansi/table.go` hard-codes `lipgloss.NewStyle().Inline(true)` on every table cell. When lipgloss has to shrink a column to fit the table width, an inline-styled cell gets character-truncated instead of wrapping. Glamour 0.10.0 flipped that line to `Inline(false)` and threaded a configurable `Wrap` flag into the underlying `lipgloss.Table`. The v0.10.0 release also ships an opt-in "links footer" that re-emits each in-cell link as `[N]: url` lines after the table — incompatible with hypogeum's URL-hiding + sentinel-driven link-position recovery; we disable it via `WithInlineTableLinks(true)`. Lastly, v0.10.0's `ASCIIStyleConfig` (which `NoTTYStyleConfig` aliases, i.e. the style used in `go test`) started setting `ColumnSeparator: "|"` / `RowSeparator: "-"`; the existing alignment helpers only recognize the U+2502 / U+2500 glyphs, so we pin the separators in `applyHypogeumOverrides` to restore consistent behavior across NoTTY (tests) and TTY (users).

---

### Task 1: Pin the wrap contract with a failing regression test

Lock in the "cells wrap, never truncate" invariant before touching dependencies. The test fails on v0.8.0 (cell content truncated mid-word) and passes after the upgrade — so subsequent Glamour bumps that regress wrapping will fail loudly.

**Files:**
- Create: `internal/markdown/table_wrap_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/markdown/table_wrap_test.go`:

```go
package markdown

import (
	"strings"
	"testing"
)

// TestRender_TableCellWraps pins the contract: a long table cell wraps
// to multiple lines rather than being character-truncated. The Glamour
// 0.8.0 → 0.10.0 upgrade is what made this work; if a future bump
// regresses to truncation, the assertions below fail.
func TestRender_TableCellWraps(t *testing.T) {
	r := rendererForTest(t)
	src := "" +
		"| Field | Description |\n" +
		"| ----- | ----------- |\n" +
		"| name | The full canonical name of the user including honorifics and suffixes |\n"

	out, err := r.Render(src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	visible := stripANSI(out)

	// Truncation drops trailing words; wrapping preserves them. If both
	// the head and tail of the long phrase survive, the cell wrapped.
	if !strings.Contains(visible, "honorifics") {
		t.Errorf("expected 'honorifics' to survive (truncated mid-word?); got:\n%s", visible)
	}
	if !strings.Contains(visible, "suffixes") {
		t.Errorf("expected 'suffixes' to survive (cell truncated?); got:\n%s", visible)
	}

	// Wrapping means the cell content spans at least one additional line
	// versus a single-line truncated cell. Count rows that contain a
	// table column separator — there should be at least 3 (header row +
	// 2 wrapped body lines).
	var rowsWithBorder int
	for _, line := range strings.Split(visible, "\n") {
		if strings.ContainsRune(line, '│') {
			rowsWithBorder++
		}
	}
	if rowsWithBorder < 3 {
		t.Errorf("expected >=3 rows containing │ (header + wrapped body), got %d:\n%s", rowsWithBorder, visible)
	}
}
```

- [ ] **Step 2: Run test and confirm it fails on v0.8.0**

Run: `go test ./internal/markdown/ -run TestRender_TableCellWraps -v`

Expected: FAIL — "expected >=3 content lines" (or the truncation message). On v0.8.0, Glamour renders the long cell on a single line with content character-truncated to fit the column width.

- [ ] **Step 3: Commit the failing test**

```bash
git add internal/markdown/table_wrap_test.go
git commit -m "$(cat <<'EOF'
test(markdown): pin table-cell wrap invariant

Add a regression test that fails on Glamour v0.8.0 (mid-word truncation)
and will pass once the renderer wraps cells across multiple lines.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Upgrade Glamour to v0.10.0 and adjust style config

Bump the dependency and apply the two follow-ups needed to keep the existing alignment + link-extraction tests green.

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `internal/markdown/render.go:44-57`
- Modify: `internal/markdown/style.go:152-154`

- [ ] **Step 1: Upgrade the dependency**

Run: `go get github.com/charmbracelet/glamour@v0.10.0 && go mod tidy`

Expected: `go.mod` shows `github.com/charmbracelet/glamour v0.10.0`. Transitive `lipgloss`, `goldmark`, `goldmark-emoji`, `golang.org/x/net`, `golang.org/x/term` get bumped too — that's expected.

- [ ] **Step 2: Confirm baseline regressions land where predicted**

Run: `go test ./internal/markdown/ -v 2>&1 | tail -40`

Expected:
- `TestRender_TableCellWraps` now passes (target invariant works).
- `TestRender_TableWithLinkInLastColumn`, `TestRender_TableWithLinkInFirstColumn`, `TestRender_TableWithLinksInEveryColumn`, `TestRender_TableExternalAndLocalLinksAlign`, `TestRender_TableWithLinks_PlainEqualsInstrumented`, `TestRender_TableWithWikilinkAligns` fail on width-mismatch — column widths differ between header and data rows because the test path (NoTTY/ASCII style) now uses `|`/`-` separators that `isTableBorderByte` doesn't recognize, so `urlSuppressStrip` takes the clean-strip branch and shortens cells.
- `TestRenderWithLinks_TableAligns` fails with "got 4 links, want 2" — v0.10.0's new table-links footer re-emits each cell link.

This step is for grounding the diagnosis. Do not modify code yet.

- [ ] **Step 3: Add `WithInlineTableLinks(true)` to both renderers**

Edit `internal/markdown/render.go`. Find:

```go
	g, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithStyles(hypogeumStyle(width)),
	)
```

Replace with:

```go
	g, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithStyles(hypogeumStyle(width)),
		// v0.10.0 added a "links at the bottom of each table" footer
		// that's incompatible with our URL-hiding + sentinel-driven
		// link-position recovery. Opt out: render links inline as we
		// always have.
		glamour.WithInlineTableLinks(true),
	)
```

And in the same file find:

```go
	instrumented, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithStyles(linkInstrumentationStyles(width)),
	)
```

Replace with:

```go
	instrumented, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithStyles(linkInstrumentationStyles(width)),
		glamour.WithInlineTableLinks(true),
	)
```

- [ ] **Step 4: Pin the table separator glyphs in `applyHypogeumOverrides`**

Edit `internal/markdown/style.go`. Find the trailing lines of `applyHypogeumOverrides`:

```go
	cfg.Link.BlockPrefix = string(urlSuppressStart) + cfg.Link.BlockPrefix
	cfg.Link.BlockSuffix = cfg.Link.BlockSuffix + string(urlSuppressEnd)
}
```

Replace with:

```go
	cfg.Link.BlockPrefix = string(urlSuppressStart) + cfg.Link.BlockPrefix
	cfg.Link.BlockSuffix = cfg.Link.BlockSuffix + string(urlSuppressEnd)

	// Table separators: pin to the U+2502 / U+2500 box-drawing glyphs
	// across all themes. Glamour 0.10.0's ASCII/NoTTY config started
	// shipping `|` / `-`, which `isTableBorderByte` (in links_render.go)
	// doesn't recognize — and that helper is what tells
	// `urlSuppressStrip` to preserve column width in padding contexts.
	// Forcing the glyphs keeps width-alignment consistent in both TTY
	// (dark theme) and NoTTY (tests) paths.
	tableCol := "│"
	tableRow := "─"
	cfg.Table.CenterSeparator = &tableCol
	cfg.Table.ColumnSeparator = &tableCol
	cfg.Table.RowSeparator = &tableRow
}
```

- [ ] **Step 5: Run the full package test suite**

Run: `go test ./internal/markdown/ -v 2>&1 | tail -25`

Expected: PASS for every test in the package, including all `TestRender_Table*` cases and the new `TestRender_TableCellWraps`.

- [ ] **Step 6: Run the full repo test suite with race detector**

Run: `go test -race ./...`

Expected: every package reports `ok`. The race detector is required because CI runs `go test -race ./...` (see `.github/workflows/ci.yml`).

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/markdown/render.go internal/markdown/style.go
git commit -m "$(cat <<'EOF'
feat(markdown): upgrade Glamour to v0.10.0 so table cells wrap

Glamour 0.8.0 hard-codes Inline(true) on table cells, which makes
lipgloss character-truncate long cell content instead of wrapping it.
v0.10.0 flipped that to Inline(false) plus a configurable wrap flag.

Two follow-ups keep the existing pipeline intact:

* WithInlineTableLinks(true) suppresses v0.10.0's new "links footer"
  that would otherwise duplicate every in-cell link as [N]: url lines —
  incompatible with our URL-hiding + sentinel-driven link-position
  recovery.

* Pinning the table separators to │ / ─ in applyHypogeumOverrides keeps
  isTableBorderByte + urlSuppressStrip recognizing the cell boundary
  across all themes; v0.10.0's ASCII/NoTTY config started shipping
  | / -, which broke column-width alignment in tests.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Update CLAUDE.md gotcha for the table render path

Document the new contract so future maintainers don't trip over the two non-obvious knobs (`WithInlineTableLinks`, pinned separator glyphs).

**Files:**
- Modify: `CLAUDE.md` (Gotchas section)

- [ ] **Step 1: Locate the URL-suppress gotcha**

Open `CLAUDE.md` and find the bullet that starts with `**URL-suppress preserves column width in tables.**` (currently the last bullet before the `/` search bullet).

- [ ] **Step 2: Append a sibling gotcha after it**

Insert the following bullet immediately after the URL-suppress one:

```markdown
- **Table-cell wrap relies on Glamour ≥0.10.0 plus two opt-ins.** v0.8.0 hard-coded `Inline(true)` on every table cell, which made lipgloss character-truncate long content. v0.10.0 flipped that. `internal/markdown/render.go` passes `glamour.WithInlineTableLinks(true)` to **both** the plain and instrumented renderers so v0.10.0's new "links at the bottom of each table" footer doesn't fire — that footer would re-emit each cell link as `[N]: url` and break our URL-hiding + sentinel-driven link-position recovery. `internal/markdown/style.go` also pins `cfg.Table.CenterSeparator` / `ColumnSeparator` / `RowSeparator` to `│` / `─` because v0.10.0's ASCII/NoTTY style started shipping `|` / `-` by default, which the `isTableBorderByte` helper in `links_render.go` doesn't recognize. Adding a third Glamour renderer? Pass the same option. Switching themes? Don't drop the separator pins.
```

- [ ] **Step 3: Verify the file still builds as markdown**

Run: `go run ./cmd/hypogeum CLAUDE.md` from a real terminal (not strictly required by tests, but a sanity check that we didn't malform the file). Skip if no TTY available.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "$(cat <<'EOF'
docs: explain Glamour table-wrap upgrade contract in CLAUDE.md

Future maintainers need to know that both `WithInlineTableLinks(true)`
and the pinned `│` / `─` separator glyphs are load-bearing for
hypogeum's URL-suppression + alignment invariants.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Final verification

Belt-and-braces re-run after the docs commits to make sure nothing in the working tree drifted.

- [ ] **Step 1: Build everything**

Run: `go build ./...`

Expected: exits cleanly with no output.

- [ ] **Step 2: Test everything with race detector**

Run: `go test -race ./...`

Expected: every package reports `ok`.

- [ ] **Step 3: Vet**

Run: `go vet ./...`

Expected: exits cleanly with no output.

- [ ] **Step 4: Confirm branch is clean and ready for PR**

Run: `git status` and `git log --oneline origin/main..HEAD`

Expected: 3 implementation commits ahead of the plan-creation commit on this branch (one per Task 1–3), working tree clean.

The branch is now ready for `gh pr create` (the repo policy from CLAUDE.md is `gh pr merge --merge`, not squash).
