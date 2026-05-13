# Source embeds — follow-up fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land four narrow follow-up fixes from post-merge review of #30 — fence-aware embed preprocessing, no-scroll on embed links, Esc-preserves-scroll on range-highlight clear, and `followBacklink` symmetry with Back/Forward.

**Architecture:** One commit per fix on branch `source-embeds-followups` (already created from `origin/main`). Each fix is small (< 30 lines including tests) and independent — no cross-fix dependencies. The spec commit is already on the branch.

**Tech Stack:** Go 1.x, Bubble Tea, Glamour, Chroma. Tests use `go test ./...` and `stretchr/testify`-free direct assertions per the repo's existing pattern.

---

## Pre-flight

- [ ] **Step 0: Confirm you're on the right branch**

Run: `git rev-parse --abbrev-ref HEAD && git log --oneline -3`

Expected: branch `source-embeds-followups`, top commit is the spec (`docs(source-embeds): spec for four post-merge follow-up fixes`), two parents of that commit are from `origin/main`.

If wrong branch: `git checkout source-embeds-followups`. If branch missing: `git fetch origin main && git checkout -b source-embeds-followups origin/main` then commit the spec (already done — if missing, recreate from `docs/superpowers/specs/2026-05-13-source-embeds-followups-design.md`).

- [ ] **Step 1: Establish a green baseline**

Run: `go build ./... && go test ./...`

Expected: all tests pass. Note: if anything fails, *stop* and investigate before adding tasks on top of a broken baseline.

---

### Task 1: Fence-aware `preprocessEmbeds`

**Files:**
- Modify: `internal/markdown/links_render.go` (`preprocessEmbeds` + the regex comment around lines 334–423)
- Test: `internal/markdown/embed_render_test.go`

The fix splits `src` into alternating non-fence and fence segments via a line-based scanner. The regex replacer runs only on non-fence segments. Fence detection accepts ``` ``` ``` and ``` ~~~ ``` openings with up to 3 leading spaces; closes on the same marker char at equal-or-greater length, also with up to 3 leading spaces, optional trailing whitespace, no other content.

- [ ] **Step 1: Write the failing tests**

Append to `internal/markdown/embed_render_test.go`:

```go
func TestPreprocessEmbeds_LeavesFencedBlockUntouched(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "doc.md")
	target := filepath.Join(dir, "target.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	md := "before\n\n```\n![[target.go]]\n```\n\nafter\n"
	r := NewRenderer(80)
	out, _, _, err := r.RenderWithLinks(md, src)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// The literal token must survive inside the fence.
	if !strings.Contains(out, "![[target.go]]") {
		t.Fatalf("expected literal embed token inside fence; got:\n%s", out)
	}
	// And no warning blockquote should have fired for a fenced demo.
	if strings.Contains(out, "⚠") {
		t.Fatalf("expected no warning for fenced embed; got:\n%s", out)
	}
}

func TestPreprocessEmbeds_LeavesTildeFenceUntouched(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "doc.md")
	md := "~~~\n![[missing.go]]\n~~~\n"
	r := NewRenderer(80)
	out, _, _, err := r.RenderWithLinks(md, src)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "![[missing.go]]") {
		t.Fatalf("expected literal embed in tilde fence; got:\n%s", out)
	}
	if strings.Contains(out, "file not found") {
		t.Fatalf("expected no resolution attempt inside tilde fence; got:\n%s", out)
	}
}

func TestPreprocessEmbeds_StillProcessesOutsideFence(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "doc.md")
	target := filepath.Join(dir, "target.go")
	if err := os.WriteFile(target, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	md := "lead\n\n![[target.go]]\n\n```\n![[target.go]]\n```\n\n![[target.go]]\ntail\n"
	r := NewRenderer(80)
	out, deps, _, err := r.RenderWithLinks(md, src)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// Two real embeds outside fences should both resolve; we expect at
	// least two occurrences of the gutter's first line marker "  1".
	if strings.Count(out, "alpha") < 2 {
		t.Fatalf("expected target body to appear twice (one per outside-fence embed); got:\n%s", out)
	}
	// Dep list is deduped, so a single absPath even with two outside-fence embeds.
	if len(deps) != 1 {
		t.Fatalf("expected 1 deduped dep, got %d: %v", len(deps), deps)
	}
}

func TestPreprocessEmbeds_LongerFenceContainsShorterRun(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "doc.md")
	md := "````\n```\n![[target.go]]\n```\n````\n"
	r := NewRenderer(80)
	out, _, _, err := r.RenderWithLinks(md, src)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "![[target.go]]") {
		t.Fatalf("expected literal embed inside outer 4-backtick fence; got:\n%s", out)
	}
}
```

If `os` or `filepath` imports are missing from the test file, add them. (Existing tests in this file already import them — check the import block.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/markdown/ -run TestPreprocessEmbeds -v`

Expected: at least the `LeavesFencedBlockUntouched` and `LeavesTildeFenceUntouched` tests FAIL (the regex currently fires inside fences and produces a warning blockquote instead of preserving the literal token).

- [ ] **Step 3: Add the fence scanner and route the regex through it**

In `internal/markdown/links_render.go`, replace the comment block above `embedTokenRegex` and the body of `preprocessEmbeds` so the regex runs only on non-fence segments.

First, update the comment on lines 334–338:

```go
// embedTokenRegex matches ![[...]] outside of fenced code blocks.
// Fence detection is handled by splitOutsideFences below — inline
// `code` spans are NOT detected, so an embed inside a single-backtick
// span will still be processed. Order with preprocessWikilinks
// matters: this pass runs first so the ![[...]] form is consumed
// before the [[...]] regex sees it.
```

Then change `preprocessEmbeds` so the `ReplaceAllStringFunc` runs per-segment. Replace lines 359 (the `out := embedTokenRegex.ReplaceAllStringFunc(src, ...)` call) so the call is wrapped:

Find this block (currently around line 359):
```go
	out := embedTokenRegex.ReplaceAllStringFunc(src, func(match string) string {
```

The closure body itself does not change. Wrap the call so it iterates over non-fence segments. Replace:

```go
	out := embedTokenRegex.ReplaceAllStringFunc(src, func(match string) string {
```

with:

```go
	replace := func(match string) string {
```

and change the closing `})` of that `ReplaceAllStringFunc` (currently the `})` followed by `return out, deps, links` near line 422) to a plain `}`. Then immediately before `return out, deps, links`, add the segment scan:

```go
	var b strings.Builder
	b.Grow(len(src))
	for seg, isFence := range splitOutsideFences(src) {
		if isFence {
			b.WriteString(seg)
			continue
		}
		b.WriteString(embedTokenRegex.ReplaceAllStringFunc(seg, replace))
	}
	out := b.String()
```

Note: Go does not have a native `range func` over a function value (without iter.Seq). Implement `splitOutsideFences` to return a slice of `(string, bool)` pairs directly. Use this signature instead:

```go
	var b strings.Builder
	b.Grow(len(src))
	for _, seg := range splitOutsideFences(src) {
		if seg.isFence {
			b.WriteString(seg.text)
			continue
		}
		b.WriteString(embedTokenRegex.ReplaceAllStringFunc(seg.text, replace))
	}
	out := b.String()
```

Then add the helper at the bottom of `links_render.go` (after `warningBlock` so related embed-preprocessing code stays clustered):

```go
// fenceSegment is a chunk of source paired with whether it lies inside
// a fenced code block. Used by preprocessEmbeds to skip embed scanning
// inside fences.
type fenceSegment struct {
	text    string
	isFence bool
}

// splitOutsideFences walks src line-by-line and returns alternating
// segments: false = embed-eligible prose, true = fenced code block
// (including the fence delimiters themselves). Trailing newlines are
// preserved so concatenating segments reproduces src exactly.
//
// Fence semantics (a subset of CommonMark sufficient for our docs):
//   - Opening fence: ≤3 leading spaces, then 3+ backticks OR 3+ tildes.
//   - Closing fence: same marker char, ≥ opening length, ≤3 leading
//     spaces, only optional whitespace after the marker run.
//   - Mismatched marker char or shorter run does NOT close the fence.
func splitOutsideFences(src string) []fenceSegment {
	if !strings.ContainsAny(src, "`~") {
		return []fenceSegment{{text: src, isFence: false}}
	}
	var segs []fenceSegment
	var cur strings.Builder
	inFence := false
	var fenceChar byte
	var fenceLen int

	lines := strings.SplitAfter(src, "\n")
	for _, line := range lines {
		if !inFence {
			if ch, n, ok := openingFence(line); ok {
				if cur.Len() > 0 {
					segs = append(segs, fenceSegment{text: cur.String(), isFence: false})
					cur.Reset()
				}
				inFence = true
				fenceChar = ch
				fenceLen = n
				cur.WriteString(line)
				continue
			}
			cur.WriteString(line)
			continue
		}
		// inside a fence
		cur.WriteString(line)
		if closingFence(line, fenceChar, fenceLen) {
			segs = append(segs, fenceSegment{text: cur.String(), isFence: true})
			cur.Reset()
			inFence = false
		}
	}
	if cur.Len() > 0 {
		segs = append(segs, fenceSegment{text: cur.String(), isFence: inFence})
	}
	return segs
}

// openingFence returns the marker char, run length, and ok=true if line
// is an opening code fence under our subset of CommonMark. Accepts up
// to 3 leading spaces; the run begins with the first non-space char.
func openingFence(line string) (byte, int, bool) {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) {
		return 0, 0, false
	}
	ch := line[i]
	if ch != '`' && ch != '~' {
		return 0, 0, false
	}
	start := i
	for i < len(line) && line[i] == ch {
		i++
	}
	n := i - start
	if n < 3 {
		return 0, 0, false
	}
	return ch, n, true
}

// closingFence reports whether line closes an open fence whose marker
// char is ch and whose opening run length is n. Closing requires ≥n
// markers of the same char, ≤3 leading spaces, and only optional
// whitespace afterward.
func closingFence(line string, ch byte, n int) bool {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) || line[i] != ch {
		return false
	}
	start := i
	for i < len(line) && line[i] == ch {
		i++
	}
	if i-start < n {
		return false
	}
	for i < len(line) {
		c := line[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
		i++
	}
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/markdown/ -run TestPreprocessEmbeds -v`

Expected: all four new tests PASS. Then `go test ./...` to confirm no regressions in any package.

- [ ] **Step 5: Commit**

```bash
git add internal/markdown/links_render.go internal/markdown/embed_render_test.go
git commit -m "$(cat <<'EOF'
fix(markdown): skip fenced code blocks in preprocessEmbeds

The embedTokenRegex previously matched ![[...]] anywhere in the source
string, including inside triple-backtick or tilde fences. Docs that
demonstrate the syntax inside a fence (the design spec for #30, for
one) had their demo replaced by a warning blockquote. Add a line-based
fence scanner and run the regex only over non-fence segments. Inline
`code` spans are still not detected; documented in the regex comment.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Embed link `Row=-1` no longer scroll-jumps

**Files:**
- Modify: `internal/tui/links.go:94` (`scrollToLink`)
- Modify: `internal/markdown/links_render.go:410` (add explanatory comment)
- Test: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/model_test.go`:

```go
func TestModel_CyclingOntoEmbedDoesNotScroll(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.go")
	if err := os.WriteFile(target, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	// A markdown file long enough that we can scroll, with an embed at
	// the end. The pad lines before the embed force the viewport to
	// have a non-zero YOffset when we land on the embed link.
	var pad strings.Builder
	for i := 0; i < 50; i++ {
		pad.WriteString("filler line\n\n")
	}
	doc := pad.String() + "![[target.go]]\n"
	md := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(md, []byte(doc), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	m := sized(t, dir, md)
	// Scroll well past the top.
	m.content.viewport.SetYOffset(40)
	want := m.content.viewport.YOffset

	// One cycleLink(+1) call: empty cursor -> first link (the embed).
	m.cycleLink(+1)

	if m.content.viewport.YOffset != want {
		t.Fatalf("cycling onto embed link must not scroll: YOffset %d -> %d",
			want, m.content.viewport.YOffset)
	}
	if m.content.linkCursor != 0 {
		t.Fatalf("expected linkCursor=0, got %d", m.content.linkCursor)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_CyclingOntoEmbedDoesNotScroll -v`

Expected: FAIL. The viewport will be jumped to a low offset (likely 0 or near it) because `scrollToLink` sees `Row=-1 < top=40` and calls `SetYOffset(max(0, -2)) = 0`.

- [ ] **Step 3: Add the sentinel guard**

Edit `internal/tui/links.go`. Before the existing line 95 (`top := m.content.viewport.YOffset`), add:

```go
	// Row < 0 is the embed-link sentinel: such links have no single
	// representative row in the rendered output (they're whole fenced
	// blocks). Move the cursor without disturbing scroll.
	if l.Row < 0 {
		return
	}
```

- [ ] **Step 4: Update the comment at the synthesis site**

Edit `internal/markdown/links_render.go` around the `Row: -1` literal (currently line 410). Change:

```go
		l := Link{
			Text: em.Path,
			Href: body,
			Row:  -1,
```

to:

```go
		l := Link{
			Text: em.Path,
			Href: body,
			// Row=-1 is the no-scroll sentinel honored by
			// (*Model).scrollToLink — embeds have no representative
			// single line, so cursor moves but viewport stays put.
			Row: -1,
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestModel_CyclingOntoEmbedDoesNotScroll -v && go test ./...`

Expected: new test passes, no regressions.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/links.go internal/markdown/links_render.go internal/tui/model_test.go
git commit -m "$(cat <<'EOF'
fix(tui): treat Link.Row<0 as no-scroll sentinel for embed links

preprocessEmbeds synthesizes embed Links with Row=-1 since a fenced
block has no single representative line. scrollToLink saw Row<top
and jumped the viewport to the document top whenever n/p landed on
an embed. Honor the sentinel and let cycleLink update the cursor
without disturbing scroll.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Esc on code file preserves scroll when clearing `rangeHighlight`

**Files:**
- Modify: `internal/tui/input.go:344–349`
- Test: `internal/tui/model_test.go` (extend `TestModel_EscClearsRangeHighlight`)

- [ ] **Step 1: Write the failing test**

Add a new test in `internal/tui/model_test.go` *after* the existing `TestModel_EscClearsRangeHighlight`:

```go
func TestModel_EscClearingRangeHighlightPreservesScroll(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "big.go")
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("line content\n")
	}
	if err := os.WriteFile(src, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	m := sized(t, dir, src)
	m.content.rangeHighlight = &markdown.LineRange{Start: 1, End: 2}
	m.refreshContent(src)
	m.content.viewport.SetYOffset(60)
	want := m.content.viewport.YOffset

	m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})

	if m.content.rangeHighlight != nil {
		t.Fatalf("Esc should clear rangeHighlight; got %+v", m.content.rangeHighlight)
	}
	if m.content.viewport.YOffset != want {
		t.Fatalf("Esc should preserve scroll: YOffset %d -> %d",
			want, m.content.viewport.YOffset)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_EscClearingRangeHighlightPreservesScroll -v`

Expected: FAIL — YOffset returns to 0 because `refreshContent` calls `GotoTop()` and the current Esc branch does not save/restore.

- [ ] **Step 3: Wrap the branch with save/restore**

Edit `internal/tui/input.go` around lines 344–349. Change:

```go
		cur := m.history.Current()
		if m.content.rangeHighlight != nil && !tree.IsMarkdown(cur) {
			m.content.rangeHighlight = nil
			m.refreshContent(cur)
			return *m, nil
		}
```

to:

```go
		cur := m.history.Current()
		if m.content.rangeHighlight != nil && !tree.IsMarkdown(cur) {
			offset := m.content.viewport.YOffset
			m.content.rangeHighlight = nil
			m.refreshContent(cur)
			m.content.viewport.SetYOffset(offset)
			return *m, nil
		}
```

This mirrors the existing pattern on lines 350–353. The link-cursor branch below restores unconditionally (no `err == nil` guard), and we match that exactly — symmetric behavior with the surrounding code.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestModel_Esc -v && go test ./...`

Expected: both `TestModel_EscClearsRangeHighlight` and `TestModel_EscClearingRangeHighlightPreservesScroll` pass; no regressions.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/input.go internal/tui/model_test.go
git commit -m "$(cat <<'EOF'
fix(tui): preserve viewport scroll when Esc clears rangeHighlight

The Esc cascade branch that clears rangeHighlight on non-markdown
files called refreshContent without saving/restoring YOffset. Since
refreshContent calls GotoTop, the viewport snapped to line 1 each
time the user dismissed a gutter highlight on a code file. Mirror
the link-cursor branch's save/restore pattern.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: `followBacklink` captures `pendingPreselectRange`

**Files:**
- Modify: `internal/tui/backlinks.go:158`
- Modify: `CLAUDE.md` (one-sentence addendum to the range-highlight gotcha)
- Test: `internal/tui/backlinks_test.go` (file exists; check first — if not, create it next to backlinks.go)

- [ ] **Step 1: Locate the test file**

Run: `ls internal/tui/backlinks_test.go 2>/dev/null || echo "missing"`

If missing, you'll create it in Step 3. If present, you'll append to it.

- [ ] **Step 2: Write the failing test**

If `internal/tui/backlinks_test.go` exists, append to it. Otherwise create it with the snippet below. The real struct names (verified against current source) are `contentUIState` and `backlinksUIState`; `nav.New()` takes no args and `History.Visit(path)` seeds the cursor.

```go
package tui

import (
	"testing"

	"github.com/wilkes/hypogeum/internal/markdown"
	"github.com/wilkes/hypogeum/internal/nav"
	"github.com/wilkes/hypogeum/internal/vault"
)

func TestFollowBacklink_CapturesPendingPreselectRange(t *testing.T) {
	// Build a minimal model directly. We bypass tui.New because the
	// scenario (a code file with rangeHighlight AND a backlink pointing
	// to it) is unreachable through vault.Build today — vault only
	// indexes markdown files. We're locking the invariant against a
	// future regression in followBacklink, not exercising the path.
	h := nav.New()
	h.Visit("/tmp/code.go")
	want := &markdown.LineRange{Start: 10, End: 20}
	m := &Model{
		history: h,
		content: contentUIState{
			rangeHighlight: want,
		},
		backlinks: backlinksUIState{
			items:  []vault.Backlink{{SourceFile: "/tmp/other.md", Line: 3}},
			cursor: 0,
		},
	}

	// followBacklink calls openFile -> refreshContent on the fake
	// destination path. refreshContent returns an error gracefully
	// (status set, no panic) on a missing file, which is fine: the
	// capture of pendingPreselectRange must happen *before* openFile,
	// so by the time openFile returns the field is already set.
	m.followBacklink()

	if m.pendingPreselectRange != want {
		t.Fatalf("followBacklink should set pendingPreselectRange = rangeHighlight; got %v want %v",
			m.pendingPreselectRange, want)
	}
}
```

If `contentUIState` or `backlinksUIState` has unexported fields that this test can't reach, fall back to constructing a `Model` via `New()` against a temp dir and then writing the two relevant fields directly — the assertion is the same.

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestFollowBacklink_CapturesPendingPreselectRange -v`

Expected: FAIL — `pendingPreselectRange` will be `nil` because the current `followBacklink` doesn't set it.

- [ ] **Step 4: Add the capture line**

Edit `internal/tui/backlinks.go` around line 158. Change:

```go
	// Pre-select the inline link in the source file that points back to
	// the file we're leaving. Consumed by refreshContent during openFile.
	m.pendingPreselectTarget = m.history.Current()
```

to:

```go
	// Pre-select the inline link in the source file that points back to
	// the file we're leaving, plus any active range highlight, so the
	// destination's refreshContent can reapply it. Mirrors the capture
	// in Back/Forward (input.go) — every navigation-out path must
	// capture both fields so the invariant holds uniformly.
	m.pendingPreselectTarget = m.history.Current()
	m.pendingPreselectRange = m.content.rangeHighlight
```

- [ ] **Step 5: Update CLAUDE.md**

Edit `CLAUDE.md`. Find the existing gotcha bullet that starts with **"Range-link Enter sets `m.content.rangeHighlight`"**. Append to that bullet:

```
 Every navigation-out path (Back, Forward, followBacklink) captures `rangeHighlight` into `pendingPreselectRange` before navigating so the destination can reapply it; if you add a fifth navigation path, capture both `pendingPreselectTarget` and `pendingPreselectRange` together.
```

(Match the existing prose style of that bullet — single paragraph, no bullet sub-list.)

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestFollowBacklink -v && go test ./...`

Expected: new test passes, no regressions.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/backlinks.go internal/tui/backlinks_test.go CLAUDE.md
git commit -m "$(cat <<'EOF'
fix(tui): followBacklink captures pendingPreselectRange

Back and Forward in input.go capture m.content.rangeHighlight into
m.pendingPreselectRange before navigating; followBacklink set only
pendingPreselectTarget. The vault-is-markdown-only constraint makes
the code-file-with-backlink path unreachable today, so this is a
latent gap — but locking the invariant keeps the next contributor
who adds a fifth navigation path from forgetting the capture. Add a
direct unit test and an addendum to the relevant CLAUDE.md gotcha.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Final verification

- [ ] **Step 1: Full build + test**

Run: `go build ./... && go test ./...`

Expected: all green. If any test fails, investigate and fix on the same branch with an additional commit before opening the PR — do not amend the per-fix commits.

- [ ] **Step 2: Skim the diff**

Run: `git log --oneline origin/main..HEAD`

Expected: 5 commits in order — the spec commit (`docs(source-embeds): spec for four post-merge follow-up fixes`) followed by 4 fix commits in Task 1–4 order.

- [ ] **Step 3: Hand off**

Plan execution is complete. The next step is opening the PR. Don't auto-create the PR — confirm with the user first, since they may want to bundle in additional review feedback.

---

## Self-review notes

- **Spec coverage:** All four fixes from the spec map 1:1 to Tasks 1–4. The CLAUDE.md addendum lives in Task 4.
- **Placeholder scan:** No TBDs. The only "if differs from actual struct" caveat is in Task 4 Step 2's test scaffold, where the field names *might* differ from this plan's guess; that's a legitimate "verify against current state" instruction rather than a placeholder.
- **Type consistency:** `Link.Row`, `m.content.rangeHighlight`, `m.pendingPreselectRange`, `m.content.viewport.YOffset` all match what's actually in the code (verified by reading the source before writing this plan).
- **Spec mapping:**
  - Spec §"Fix 1" → Task 1
  - Spec §"Fix 2" → Task 2
  - Spec §"Fix 3" → Task 3
  - Spec §"Fix 4" → Task 4 (including the CLAUDE.md addendum)
- **Risk note on Task 4 test scaffold:** The test constructs `Model` and `contentState` directly. If the package's struct field names or types differ from what I wrote, adapt them — the goal is the assertion, not the literal scaffold.
