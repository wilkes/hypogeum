# Broken-link tally — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show ` ⚠ N broken` in the footer's location row when the currently rendered document contains N>0 broken links (unresolved wikilinks + inline links to non-existent local paths).

**Architecture:** A new exported helper `markdown.CountUnresolvedWikilinks` counts unresolved `[[…]]`s in a source against the configured resolver, reusing the same regex + `splitOutsideFences` + `wikilink.Parse` pipeline as `preprocessWikilinks`. The TUI sums that count with a stat-based pass over `m.content.links` (filtering for `LinkLocalFile`) inside `refreshContent`, stashing the total on `contentUIState.brokenCount`. `renderFooter` appends the suffix when count>0 and no transient is active.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, the existing `internal/markdown` + `internal/wikilink` packages.

**Spec:** [docs/superpowers/specs/2026-05-25-broken-link-tally-design.md](../specs/2026-05-25-broken-link-tally-design.md)

---

### Task 1: `markdown.CountUnresolvedWikilinks` helper

**Files:**
- Modify: `internal/markdown/links_render.go` (add exported helper near `preprocessWikilinks`)
- Test: `internal/markdown/links_render_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/markdown/links_render_test.go`:

```go
func TestCountUnresolvedWikilinks(t *testing.T) {
	r := NewRenderer(80, WithResolver(stubResolver{known: map[string]string{
		"Found": "/abs/found.md",
	}}))
	r.SetFromFile("/abs/source.md")

	src := "see [[Found]], [[Missing]] and [[AlsoMissing|alias]]\n" +
		"```\n" +
		"[[InsideFence]]\n" +
		"```\n"

	got := r.CountUnresolvedWikilinks(src)
	if got != 2 {
		t.Fatalf("CountUnresolvedWikilinks = %d, want 2", got)
	}
}
```

If a `stubResolver` doesn't already exist in this package, add it to the test file:

```go
type stubResolver struct{ known map[string]string }

func (s stubResolver) Resolve(_ , name, _, _ string) (string, bool) {
	p, ok := s.known[name]
	return p, ok
}
```

(Check first — `internal/markdown/wikilink_test.go` already defines a fakeResolver/stubResolver. Reuse the existing name if present.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/markdown/ -run TestCountUnresolvedWikilinks -v`
Expected: FAIL with `r.CountUnresolvedWikilinks undefined`.

- [ ] **Step 3: Add the helper**

In `internal/markdown/links_render.go`, add after `preprocessWikilinks`:

```go
// CountUnresolvedWikilinks counts every [[...]] in src that the
// configured resolver does NOT resolve. Fences are skipped (matches
// preprocessWikilinks). When no resolver is configured, every
// well-formed wikilink counts as unresolved.
//
// Used by the TUI footer's broken-link tally; the count complements
// the per-document link list (which intentionally excludes unresolved
// wikilinks, since they can't be followed).
func (r *Renderer) CountUnresolvedWikilinks(src string) int {
	if !strings.Contains(src, "[[") {
		return 0
	}
	count := 0
	check := func(match string) string {
		body := match[2 : len(match)-2]
		w := wikilink.Parse(body)
		if w == nil {
			return match
		}
		if r.resolver == nil {
			count++
			return match
		}
		if _, ok := r.resolver.Resolve(r.fromFile, w.Name, w.Heading, w.Block); !ok {
			count++
		}
		return match
	}
	for _, seg := range splitOutsideFences(src) {
		if seg.isFence {
			continue
		}
		wikilinkRegex.ReplaceAllStringFunc(seg.text, check)
	}
	return count
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/markdown/ -run TestCountUnresolvedWikilinks -v`
Expected: PASS.

- [ ] **Step 5: Run the whole markdown test suite to catch regressions**

Run: `go test ./internal/markdown/...`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add internal/markdown/links_render.go internal/markdown/links_render_test.go
git commit -m "feat(markdown): add CountUnresolvedWikilinks helper

$(printf '%s\n' 'Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>')"
```

---

### Task 2: `brokenCount` field on `contentUIState`

**Files:**
- Modify: `internal/tui/content.go:22-38` (the `contentUIState` struct)

- [ ] **Step 1: Add the field**

In `internal/tui/content.go`, extend the struct:

```go
type contentUIState struct {
	viewport     viewport.Model
	renderer     *markdown.Renderer
	codeRenderer *code.Renderer
	links        []markdown.Link
	linkCursor   int
	// brokenCount is the sum of unresolved wikilinks plus inline local
	// links whose target file is missing in the currently rendered
	// document. Recomputed by refreshContent; surfaced by renderFooter.
	brokenCount int
	embedDeps   map[string]struct{}
	rangeHighlight *markdown.LineRange
}
```

- [ ] **Step 2: Verify it still compiles**

Run: `go build ./...`
Expected: builds cleanly.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/content.go
git commit -m "feat(tui): add brokenCount field to contentUIState

$(printf '%s\n' 'Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>')"
```

---

### Task 3: Compute `brokenCount` in `refreshContent`

**Files:**
- Modify: `internal/tui/content.go` — `refreshContent`, around the markdown render block (lines 90–211)
- Test: `internal/tui/content_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/content_test.go`. Build a tmpdir with one resolved wikilink target, one markdown file containing two unresolved wikilinks and a broken inline link, then assert `brokenCount == 3`.

Look at how existing tests in `content_test.go` build a `Model` (likely via a helper like `newTestModel(t, root)` or `tui.New`). Mirror that. If no such helper exists, use `tui.New(tmpdir, "")` and call `m.refreshContent(path)` directly.

```go
func TestRefreshContent_BrokenCount(t *testing.T) {
	dir := t.TempDir()
	// vault has Found.md so [[Found]] resolves; [[Missing]] and
	// [[AlsoMissing]] do not. Inline link points at gone.md (no file).
	mustWrite(t, filepath.Join(dir, "Found.md"), "# Found\n")
	notePath := filepath.Join(dir, "note.md")
	mustWrite(t, notePath,
		"[[Found]] [[Missing]] [[AlsoMissing]]\n\n[gone]("+filepath.Join(dir, "gone.md")+")\n",
	)

	m := newModelForTest(t, dir)
	m.refreshContent(notePath)

	if got := m.content.brokenCount; got != 3 {
		t.Fatalf("brokenCount = %d, want 3", got)
	}
}
```

If `mustWrite` / `newModelForTest` helpers don't exist, define them at the bottom of `content_test.go`:

```go
func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newModelForTest(t *testing.T, root string) *tui.Model {
	t.Helper()
	m, err := tui.New(root, "")
	if err != nil {
		t.Fatalf("tui.New: %v", err)
	}
	return m
}
```

(Adjust the import path / return type to match what `tui.New` actually returns. If `Model` is unexported, the test file is already in package `tui`; use `*Model` directly.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestRefreshContent_BrokenCount -v`
Expected: FAIL — `brokenCount` is always 0 because nothing assigns it yet.

- [ ] **Step 3: Compute the count in `refreshContent`**

In `internal/tui/content.go`, find the markdown render branch (after the `if err != nil` block that returns on render failure, around line 169 where `m.status = path` is set). Insert the count computation **after** `m.content.links = links` and **before** `m.content.embedDeps = make(...)`:

```go
m.content.links = links
m.content.brokenCount = m.content.renderer.CountUnresolvedWikilinks(string(src))
for _, l := range links {
	if l.Resolved.Kind != markdown.LinkLocalFile {
		continue
	}
	if _, err := os.Stat(l.Resolved.Target); err != nil {
		m.content.brokenCount++
	}
}
```

Also reset `brokenCount` on the early-return paths so a previous document's count doesn't leak when the new render fails:

- After the directory-listing failure branch (around line 111): add `m.content.brokenCount = 0`.
- After the file-read failure branch (around line 122): add `m.content.brokenCount = 0`.
- After the code-renderer branch (around line 144, before `return`): add `m.content.brokenCount = 0`.
- After the markdown render failure branch (around line 167): add `m.content.brokenCount = 0`.

Directory listings are rendered via `RenderWithLinks` like markdown, so the markdown success path handles them — no extra branch needed.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestRefreshContent_BrokenCount -v`
Expected: PASS.

- [ ] **Step 5: Run the whole tui test suite**

Run: `go test ./internal/tui/...`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/content.go internal/tui/content_test.go
git commit -m "feat(tui): compute brokenCount in refreshContent

$(printf '%s\n' 'Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>')"
```

---

### Task 4: Render ` ⚠ N broken` suffix in the footer

**Files:**
- Modify: `internal/tui/view.go` — `renderFooter` (lines 95–127)
- Test: `internal/tui/view_test.go` (create if absent — look first for an existing footer/view test file in `internal/tui/`)

- [ ] **Step 1: Locate the footer test home**

Run: `grep -ln "renderFooter\|footer" internal/tui/*_test.go`. If a test file exists that already exercises footer rendering, append there. Otherwise create `internal/tui/view_test.go`.

- [ ] **Step 2: Write the failing tests**

```go
func TestRenderFooter_AppendsBrokenSuffixWhenNonZero(t *testing.T) {
	m := minimalModel(t)
	m.content.brokenCount = 3
	out := m.renderFooter()
	if !strings.Contains(out, "3 broken") {
		t.Fatalf("expected '3 broken' in footer, got %q", out)
	}
	if !strings.Contains(out, "⚠") {
		t.Fatalf("expected warning glyph in footer, got %q", out)
	}
}

func TestRenderFooter_NoSuffixWhenZero(t *testing.T) {
	m := minimalModel(t)
	m.content.brokenCount = 0
	out := m.renderFooter()
	if strings.Contains(out, "broken") {
		t.Fatalf("did not expect 'broken' in footer, got %q", out)
	}
}

func TestRenderFooter_SuffixSuppressedDuringTransient(t *testing.T) {
	m := minimalModel(t)
	m.content.brokenCount = 3
	m.diag.Warn("something happened") // produces a transient
	out := m.renderFooter()
	if strings.Contains(out, "3 broken") {
		t.Fatalf("transient should hide broken suffix, got %q", out)
	}
}
```

`minimalModel(t)` should construct a `Model` over a tmpdir using `tui.New` (the same pattern as Task 3's `newModelForTest`). If `m.diag` is nil for a freshly constructed Model, skip the transient test by setting up diagnostics the way existing tests do — search `internal/tui/diagnostics_test.go` for the pattern and copy it.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run TestRenderFooter -v`
Expected: FAIL — `m.renderFooter()` returns no `broken` suffix.

- [ ] **Step 4: Add the suffix to `renderFooter`**

In `internal/tui/view.go`, modify `renderFooter`. Track whether a transient is active and append the broken suffix after the link-cursor block, when a transient is NOT showing and `brokenCount > 0`:

```go
func (m Model) renderFooter() string {
	help := "?: help  q: quit"

	loc := m.status
	if loc != "" {
		if rel, err := filepath.Rel(m.root, loc); err == nil {
			loc = rel
		}
	}

	transientActive := false
	transientStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	if m.diag != nil {
		if e, ok := m.diag.transientStatus(); ok {
			loc = transientStyle.Render(e.Severity.String() + ": " + e.Message)
			transientActive = true
		}
	}

	hasLink := false
	if sel := m.selectedLink(); sel != nil {
		loc = fmt.Sprintf("%s%s [%d/%d] %s", linkFooterMarker, loc, m.content.linkCursor+1, len(m.content.links), linkLabel(*sel, m.root))
		hasLink = true
	}

	if !transientActive && m.content.brokenCount > 0 {
		brokenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Faint(true)
		loc += brokenStyle.Render(fmt.Sprintf(" ⚠ %d broken", m.content.brokenCount))
	}

	helpStyle := lipgloss.NewStyle().Faint(true)
	locStyle := helpStyle
	if hasLink {
		locStyle = lipgloss.NewStyle()
	}
	return fmt.Sprintf("%s\n%s", locStyle.Render(loc), helpStyle.Render(help))
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestRenderFooter -v`
Expected: PASS.

- [ ] **Step 6: Run the full test suite with -race**

Run: `go test -race ./...`
Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/view.go internal/tui/view_test.go
git commit -m "feat(tui): show broken-link tally in footer

$(printf '%s\n' 'Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>')"
```

---

### Task 5: Documentation updates

**Files:**
- Modify: `docs/index.md` — wikilinks Phase 2 line
- Modify: `CLAUDE.md` — "What's not built yet" wikilinks paragraph

- [ ] **Step 1: Update `docs/index.md`**

Find the line beginning `- [Wikilinks and backlinks](superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md)` (~line 30) and add a sibling bullet under the active feature work list:

```markdown
- [Broken-link tally](superpowers/specs/2026-05-25-broken-link-tally-design.md) — shipped — footer shows ` ⚠ N broken` when the current document has unresolved wikilinks or inline links to missing local paths.
```

Also amend the Wikilinks Phase 2 line so the remaining-items list reads "block references and configurable vault root" (drop "broken-links tally").

- [ ] **Step 2: Update `CLAUDE.md`**

Find the "Wikilinks and backlinks — Phase 2 in progress" paragraph in the "What's not built yet" section. Edit it so the remaining items list reads "block references (`[[note#^blockid]]`) and configurable vault root". Drop the "broken-link tally in the status bar" mention.

- [ ] **Step 3: Verify the docs render in hypogeum**

Run: `go run ./cmd/hypogeum docs/` in a real terminal. Navigate to `docs/index.md`, confirm the new bullet is present and the wikilinks Phase 2 line reads correctly. Quit with `q`.

This is a manual TUI check — record the outcome in the next commit message if anything looked off. Skip the manual check if working in a non-TTY harness, but still commit; the docs themselves are tested by inspection.

- [ ] **Step 4: Commit**

```bash
git add docs/index.md CLAUDE.md
git commit -m "docs: note broken-link tally shipped; trim phase-2 remaining list

$(printf '%s\n' 'Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>')"
```

---

### Task 6: Final verification + PR

**Files:** none (verification only)

- [ ] **Step 1: Full build + race-test**

```bash
go build ./...
go vet ./...
go test -race ./...
```

All three must pass.

- [ ] **Step 2: Manual smoke (if a TTY is available)**

Run: `go run ./cmd/hypogeum docs/`. Open a known-good page (footer shows no `broken`). Open `docs/index.md` (should show `broken` count if any wikilinks happen to be unresolved — `[[vault-index]]` etc. should all resolve, so likely 0). Construct a quick fixture: a markdown file with `[[NoSuchThing]]` and `[broken](./does-not-exist.md)` and confirm the footer reads ` ⚠ 2 broken`.

- [ ] **Step 3: Push branch and open PR**

```bash
git push -u origin broken-link-tally
gh pr create --title "Broken-link tally in footer" --body "$(cat <<'EOF'
## Summary
- footer shows ` ⚠ N broken` when the current document contains unresolved wikilinks or inline links to missing local paths
- closes one of the remaining wikilinks Phase 2 items (block refs and configurable vault root still open)

Spec: docs/superpowers/specs/2026-05-25-broken-link-tally-design.md

## Test plan
- [ ] CI: go build / vet / test -race pass
- [ ] Manual: open a file with intact links — no suffix
- [ ] Manual: open a file with broken wikilink + broken inline link — suffix reads ` ⚠ 2 broken`
- [ ] Manual: trigger a diagnostic transient — suffix hidden during transient

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-review notes

- **Spec coverage:** every section of the spec maps to a task — counter helper (Task 1), state field (Task 2), per-document compute (Task 3), footer render (Task 4), docs (Task 5).
- **Stat-during-render concern:** `os.Stat` per inline local link is acceptable per the spec; `refreshContent` already does file I/O and embed slicing.
- **Wikilink count vs. links slice:** the markdown package's link slice intentionally excludes unresolved wikilinks (so they're not in the cycler). We count them separately via the new helper — no behavior change in the cycler or in the rendered `?` placeholder.
- **Transient suppression:** Task 4 reads `m.diag.transientStatus()` only once to avoid double-render flicker; the `transientActive` bool gates the suffix.
- **No new goroutines:** stays race-clean.
