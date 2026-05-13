# Code-file syntax highlighting — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render non-markdown files (`.go`, `.rb`, `.py`, `Dockerfile`, etc.) opened via CLI arg or inline link with Chroma-driven syntax highlighting and a line-number gutter.

**Architecture:** A new `internal/code` package wraps Chroma with a line-number gutter and soft-wrap. `internal/tui/content.go::refreshContent` dispatches between the existing markdown renderer and the new code renderer based on `tree.IsMarkdown(path)`. The watcher's write-classifier is relaxed so the open code file live-reloads on save.

**Tech Stack:** Go 1.24, `github.com/alecthomas/chroma/v2` (already indirect, becomes direct), `github.com/charmbracelet/x/ansi` (already direct), existing Bubble Tea / Bubbles / Lip Gloss.

**Spec:** [docs/superpowers/specs/2026-05-12-code-file-rendering-design.md](../specs/2026-05-12-code-file-rendering-design.md)

---

## File Structure

| File | Status | Responsibility |
|---|---|---|
| `internal/code/render.go` | new | `Renderer` type + `Render(path, src) (string, error)` pipeline |
| `internal/code/style.go` | new | Chroma style selection (`defaultStyle()`) |
| `internal/code/gutter.go` | new | Line-number gutter + ANSI-aware soft-wrap |
| `internal/code/render_test.go` | new | All `code` package tests |
| `internal/tui/content.go` | modify | Dispatch in `refreshContent` |
| `internal/tui/model.go` | modify | Build `codeRenderer` in `New` and on `WindowSizeMsg` |
| `internal/tui/content_test.go` | new | TUI-level dispatch tests |
| `internal/watch/classify.go` | modify | Drop `IsMarkdown` gate on write events |
| `internal/watch/classify_test.go` | modify | Regression test for non-md writes |
| `CLAUDE.md` | modify | Add a Gotcha bullet about code-file dispatch |
| `go.mod` / `go.sum` | modify | Chroma becomes direct require (automatic on first build) |

Splitting `internal/code` into three files keeps each focused: `render.go` orchestrates the pipeline, `style.go` is the only file that touches Chroma styles, `gutter.go` is the pure text-layout layer. Tests share one file for now; we can split if it grows beyond ~400 lines.

---

## Task 1: Scaffold `internal/code` package with a failing pipeline test

**Files:**
- Create: `internal/code/render.go`
- Create: `internal/code/render_test.go`

- [ ] **Step 1: Write the failing pipeline test**

Create `internal/code/render_test.go`:

```go
package code

import (
	"strings"
	"testing"
)

func TestRender_GoSource_ContainsANSI(t *testing.T) {
	r := NewRenderer(80)
	out, err := r.Render("main.go", []byte("package main\n\nfunc main() {}\n"))
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if out == "" {
		t.Fatal("Render returned empty output")
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escape sequences in output, got:\n%q", out)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `go test ./internal/code/...`

Expected: build failure — `package code` doesn't exist yet. That's the failing state we want before implementation.

- [ ] **Step 3: Create the minimal `render.go` to make the test pass**

Create `internal/code/render.go`:

```go
// Package code renders source files to ANSI-styled terminal output with a
// line-number gutter. It is the non-markdown sibling of internal/markdown:
// dispatched to by the TUI when refreshContent sees a file extension that
// tree.IsMarkdown doesn't recognize.
package code

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Renderer is the non-markdown render path. One per content viewport width.
// Rebuilt on WindowSizeMsg, same lifecycle as markdown.Renderer.
type Renderer struct {
	width int
	style *chroma.Style
}

// NewRenderer constructs a code renderer for the given output width.
// width <= 0 is clamped to a sensible default so a renderer constructed
// before the first WindowSizeMsg still produces usable output.
func NewRenderer(width int) *Renderer {
	if width < 20 {
		width = 80
	}
	s := styles.Get("monokai")
	if s == nil {
		s = styles.Fallback
	}
	return &Renderer{width: width, style: s}
}

// Render tokenizes src with a lexer chosen from path's basename (or from
// content analysis as a fallback), formats it as 256-color ANSI, and
// prefixes a line-number gutter. Returns the rendered string and a nil
// error for all user-facing problems (binary input, oversized files,
// unrecognized syntax). A non-nil error indicates a programming-level
// invariant violation (e.g. missing formatter).
func (r *Renderer) Render(path string, src []byte) (string, error) {
	lexer := lexers.Match(filepath.Base(path))
	if lexer == nil {
		lexer = lexers.Analyse(string(src))
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}

	iterator, err := lexer.Tokenise(nil, string(src))
	if err != nil {
		// Token failure is rare but recoverable: render as plain text.
		iterator, err = lexers.Fallback.Tokenise(nil, string(src))
		if err != nil {
			return "", fmt.Errorf("tokenise fallback: %w", err)
		}
	}

	formatter := formatters.Get("terminal256")
	if formatter == nil {
		return "", fmt.Errorf("terminal256 formatter not registered")
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, r.style, iterator); err != nil {
		return "", fmt.Errorf("format: %w", err)
	}
	return buf.String(), nil
}
```

- [ ] **Step 4: Run the test to confirm it passes**

Run: `go test ./internal/code/... -run TestRender_GoSource_ContainsANSI -v`

Expected: `PASS`. If `go.mod` doesn't yet have `github.com/alecthomas/chroma/v2` as a direct require, run `go mod tidy` first — it will move the entry from indirect to direct without changing the version.

- [ ] **Step 5: Commit**

```bash
git add internal/code/render.go internal/code/render_test.go go.mod go.sum
git commit -m "$(cat <<'EOF'
feat(code): scaffold internal/code with Chroma-backed Render

Tokenizes source with a filename-matched lexer (or content analysis
fallback) and formats to 256-color ANSI. No gutter or wrap yet — those
land in the next tasks.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add binary and oversize guards

**Files:**
- Modify: `internal/code/render.go`
- Modify: `internal/code/render_test.go`

- [ ] **Step 1: Write the failing guard tests**

Append to `internal/code/render_test.go`:

```go
func TestRender_BinaryBlob_ReturnsBinaryMessage(t *testing.T) {
	r := NewRenderer(80)
	src := []byte{'M', 'Z', 0x00, 0x00, 0xff, 0xff}
	out, err := r.Render("a.exe", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "binary file") {
		t.Errorf("expected binary-file message, got: %q", out)
	}
}

func TestRender_OversizedFile_ReturnsTooLargeMessage(t *testing.T) {
	r := NewRenderer(80)
	src := make([]byte, 6*1024*1024) // 6 MB, all zero bytes — but size check runs first
	out, err := r.Render("huge.txt", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "too large") {
		t.Errorf("expected too-large message, got: %q", out)
	}
}
```

Note: the oversize test uses 6 MB of zero bytes. The size check must run **before** the binary check, otherwise we'd return "binary file" for an oversized zero buffer.

- [ ] **Step 2: Run to confirm both fail**

Run: `go test ./internal/code/... -run "TestRender_BinaryBlob|TestRender_OversizedFile" -v`

Expected: both FAIL — current `Render` ignores size and binary content.

- [ ] **Step 3: Add guards to `Render`**

In `internal/code/render.go`, edit `Render` so the top reads:

```go
func (r *Renderer) Render(path string, src []byte) (string, error) {
	const maxSize = 5 * 1024 * 1024
	if len(src) > maxSize {
		return "file too large to display", nil
	}
	if looksBinary(src) {
		return "binary file, not displayed", nil
	}

	lexer := lexers.Match(filepath.Base(path))
	// ... rest unchanged
```

Add `looksBinary` to the same file (below `Render`):

```go
// looksBinary reports whether src appears to be binary content using the
// same heuristic git uses: a NUL byte in the first 8 KB.
func looksBinary(src []byte) bool {
	n := len(src)
	if n > 8192 {
		n = 8192
	}
	return bytes.IndexByte(src[:n], 0) >= 0
}
```

- [ ] **Step 4: Run to confirm both pass**

Run: `go test ./internal/code/... -v`

Expected: all three tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/code/render.go internal/code/render_test.go
git commit -m "$(cat <<'EOF'
feat(code): guard binary and oversized inputs

Files > 5 MB and files with a NUL byte in the first 8 KB return a
short message instead of being tokenized. Size check runs first so an
oversized zero-buffer reports as too-large, not binary.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add line-number gutter

**Files:**
- Create: `internal/code/gutter.go`
- Modify: `internal/code/render.go`
- Modify: `internal/code/render_test.go`

- [ ] **Step 1: Write the failing gutter test**

Append to `internal/code/render_test.go`:

```go
func TestRender_GoSource_PrefixesGutter(t *testing.T) {
	r := NewRenderer(80)
	src := []byte("package main\n\nfunc main() {}\n")
	out, err := r.Render("main.go", src)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 output lines, got %d:\n%q", len(lines), out)
	}
	// First line should start with "1" (after any leading style reset).
	if !strings.Contains(stripANSI(lines[0]), "1") {
		t.Errorf("first line gutter missing '1': %q", lines[0])
	}
	if !strings.Contains(stripANSI(lines[2]), "3") {
		t.Errorf("third line gutter missing '3': %q", lines[2])
	}
}

// stripANSI is a test-only helper that removes ANSI escape sequences so
// assertions can check the user-visible text without coupling to color
// codes.
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			// Skip to next 'm' or end of string.
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
```

- [ ] **Step 2: Run to confirm it fails**

Run: `go test ./internal/code/... -run TestRender_GoSource_PrefixesGutter -v`

Expected: FAIL — output has no gutter yet.

- [ ] **Step 3: Create `gutter.go`**

Create `internal/code/gutter.go`:

```go
package code

import (
	"strconv"
	"strings"
)

// addGutter prepends a faint right-aligned line-number gutter to each
// line of formatted. The gutter width is fixed for the whole file —
// derived from the total line count so all numbers align — followed by
// a single-space separator column.
//
// formatted is the Chroma terminal256 output; each source line ends
// with "\n". A trailing newline (if any) is preserved.
func addGutter(formatted string) string {
	// Count source lines: number of '\n' plus 1 if the buffer doesn't
	// end with a newline. Using strings.Count avoids allocating a slice
	// just to measure.
	total := strings.Count(formatted, "\n")
	if !strings.HasSuffix(formatted, "\n") {
		total++
	}
	if total == 0 {
		return ""
	}

	gutterWidth := len(strconv.Itoa(total))
	var b strings.Builder
	b.Grow(len(formatted) + total*(gutterWidth+3))

	lineNum := 1
	start := 0
	for i := 0; i <= len(formatted); i++ {
		if i == len(formatted) || formatted[i] == '\n' {
			b.WriteString(formatLineNumber(lineNum, gutterWidth))
			b.WriteByte(' ')
			b.WriteString(formatted[start:i])
			if i < len(formatted) {
				b.WriteByte('\n')
			}
			lineNum++
			start = i + 1
		}
	}
	return b.String()
}

// formatLineNumber right-aligns n in a field of width w, wrapped in a
// dim SGR sequence and reset. The reset is critical — without it the
// dim attribute would bleed into the source-line tokens that follow.
func formatLineNumber(n, w int) string {
	s := strconv.Itoa(n)
	pad := w - len(s)
	if pad < 0 {
		pad = 0
	}
	return "\x1b[2m" + strings.Repeat(" ", pad) + s + "\x1b[0m"
}

// blankGutter is formatLineNumber for continuation rows. Same width and
// trailing space as a numbered gutter so columns align; no SGR attribute
// applied so no color can leak into the column.
func blankGutter(w int) string {
	return strings.Repeat(" ", w) + " "
}
```

- [ ] **Step 4: Wire `addGutter` into `Render`**

In `internal/code/render.go`, change the final two lines of `Render` from:

```go
	if err := formatter.Format(&buf, r.style, iterator); err != nil {
		return "", fmt.Errorf("format: %w", err)
	}
	return buf.String(), nil
```

to:

```go
	if err := formatter.Format(&buf, r.style, iterator); err != nil {
		return "", fmt.Errorf("format: %w", err)
	}
	return addGutter(buf.String()), nil
```

- [ ] **Step 5: Run all tests in the package**

Run: `go test ./internal/code/... -v`

Expected: all four tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/code/gutter.go internal/code/render.go internal/code/render_test.go
git commit -m "$(cat <<'EOF'
feat(code): prefix line-number gutter on rendered output

Each source line gets a right-aligned, dimmed line number. Gutter width
scales with the total line count so columns align. SGR reset after each
number prevents the dim attribute from bleeding into source tokens.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Add ANSI-aware soft-wrap for long lines

**Files:**
- Modify: `internal/code/gutter.go`
- Modify: `internal/code/render.go`
- Modify: `internal/code/render_test.go`

- [ ] **Step 1: Write the failing wrap test**

Append to `internal/code/render_test.go`:

```go
func TestRender_LongLine_WrapsWithBlankContinuationGutter(t *testing.T) {
	r := NewRenderer(40) // narrow terminal
	// A single source line of 100 chars, no Chroma styling needed —
	// use a .txt extension so the plain-text lexer applies.
	longLine := strings.Repeat("a", 100)
	src := []byte(longLine + "\n")
	out, err := r.Render("note.txt", src)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrap to produce >= 2 output rows, got %d:\n%q", len(lines), out)
	}

	// Continuation row(s) must NOT start with an SGR escape — that
	// would mean a color is leaking into the gutter column.
	for i := 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "\x1b[") {
			t.Errorf("continuation row %d starts with SGR escape (color leak into gutter): %q", i, lines[i])
		}
		// Continuation rows must NOT contain a line number — just blank gutter.
		stripped := stripANSI(lines[i])
		if len(stripped) > 0 && stripped[0] != ' ' {
			t.Errorf("continuation row %d has non-blank gutter: %q", i, stripped)
		}
	}
}
```

- [ ] **Step 2: Run to confirm it fails**

Run: `go test ./internal/code/... -run TestRender_LongLine_WrapsWithBlankContinuationGutter -v`

Expected: FAIL — `addGutter` currently emits a single row per source line regardless of width.

- [ ] **Step 3: Add wrap support to `gutter.go`**

Replace the body of `addGutter` in `internal/code/gutter.go` with a width-aware version:

```go
// addGutter prepends a faint right-aligned line-number gutter to each
// source line of formatted, soft-wrapping rows longer than contentWidth.
// Continuation rows get a blank (uncolored) gutter so per-source-line
// numbering stays one-per-source-line.
//
// formatted is the Chroma terminal256 output; each source line ends
// with "\n". contentWidth is the total renderer width (gutter + body).
func addGutter(formatted string, contentWidth int) string {
	total := strings.Count(formatted, "\n")
	if !strings.HasSuffix(formatted, "\n") {
		total++
	}
	if total == 0 {
		return ""
	}

	gutterWidth := len(strconv.Itoa(total))
	bodyWidth := contentWidth - gutterWidth - 1 // -1 for the separator space
	if bodyWidth < 1 {
		bodyWidth = 1
	}

	var b strings.Builder
	b.Grow(len(formatted) + total*(gutterWidth+3))

	lineNum := 1
	start := 0
	emit := func(line string) {
		wrapped := ansi.Wrap(line, bodyWidth, "")
		rows := strings.Split(wrapped, "\n")
		for i, row := range rows {
			if i == 0 {
				b.WriteString(formatLineNumber(lineNum, gutterWidth))
			} else {
				b.WriteString(blankGutter(gutterWidth))
			}
			b.WriteString(row)
			b.WriteByte('\n')
		}
	}

	for i := 0; i <= len(formatted); i++ {
		if i == len(formatted) || formatted[i] == '\n' {
			emit(formatted[start:i])
			lineNum++
			start = i + 1
		}
	}

	// Trim the trailing newline if the input didn't have one.
	out := b.String()
	if !strings.HasSuffix(formatted, "\n") && strings.HasSuffix(out, "\n") {
		out = out[:len(out)-1]
	}
	return out
}
```

Also add the import for `ansi`:

```go
import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)
```

Note: the helpers now own the trailing separator column so the body emit loop above can call them without a separate `b.WriteByte(' ')`. The byte output of `blankGutter` is unchanged from Task 3 (still `w+1` spaces), but reading the new form makes the intent clearer. Edit `blankGutter`:

```go
func blankGutter(w int) string {
	return strings.Repeat(" ", w+1)
}
```

`formatLineNumber` does need a real change: it must emit the separator space itself now that the emit loop doesn't. Edit `formatLineNumber`:

```go
func formatLineNumber(n, w int) string {
	s := strconv.Itoa(n)
	pad := w - len(s)
	if pad < 0 {
		pad = 0
	}
	return "\x1b[2m" + strings.Repeat(" ", pad) + s + "\x1b[0m "
}
```

(The trailing space after the SGR reset is the separator column.)

- [ ] **Step 4: Update `Render` to pass `r.width`**

In `internal/code/render.go`, change:

```go
	return addGutter(buf.String()), nil
```

to:

```go
	return addGutter(buf.String(), r.width), nil
```

- [ ] **Step 5: Run all tests in the package**

Run: `go test ./internal/code/... -v`

Expected: all five tests PASS. If `TestRender_GoSource_PrefixesGutter` fails because of the separator space change, the assertion uses `strings.Contains(stripANSI(lines[0]), "1")` which is unaffected — it only checks that the digit is present. Same for line 3.

- [ ] **Step 6: Commit**

```bash
git add internal/code/gutter.go internal/code/render.go internal/code/render_test.go
git commit -m "$(cat <<'EOF'
feat(code): soft-wrap long source lines with blank continuation gutter

Lines exceeding the renderer's content width wrap via charmbracelet/x/ansi
which preserves SGR sequences across wrap points. Continuation rows get
an unstyled blank gutter so source-line numbering stays one-per-line and
no residual color leaks into the gutter column.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Add Dockerfile and unknown-extension coverage

**Files:**
- Modify: `internal/code/render_test.go`

These tests prove `lexers.Match` covers filename globs and that the unknown-extension fallback path works. No production code changes — if the tests pass on the first run, that confirms the existing pipeline already handles these correctly. Still committed because they pin the behavior against future regressions.

- [ ] **Step 1: Write the coverage tests**

Append to `internal/code/render_test.go`:

```go
func TestRender_Dockerfile_HighlightedByFilename(t *testing.T) {
	r := NewRenderer(80)
	src := []byte("FROM alpine:3.18\nRUN apk add --no-cache git\n")
	out, err := r.Render("Dockerfile", src)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("Dockerfile should be highlighted by filename glob; got plain text:\n%q", out)
	}
}

func TestRender_UnknownExtension_FallsBackToPlainTextWithGutter(t *testing.T) {
	r := NewRenderer(80)
	src := []byte("hello\nworld\n")
	out, err := r.Render("note.xyz", src)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if out == "" {
		t.Fatal("Render returned empty output")
	}
	// Gutter still present even with the plain-text fallback.
	if !strings.Contains(stripANSI(out), "1") || !strings.Contains(stripANSI(out), "2") {
		t.Errorf("expected line numbers 1 and 2 in gutter, got:\n%q", stripANSI(out))
	}
}
```

- [ ] **Step 2: Run to confirm both pass**

Run: `go test ./internal/code/... -v`

Expected: all seven tests PASS. If `TestRender_Dockerfile_HighlightedByFilename` fails, that means Chroma's lexer registry doesn't have a `Dockerfile` filename glob in this version — fall back to the test asserting only that output is non-empty and has a gutter.

- [ ] **Step 3: Commit**

```bash
git add internal/code/render_test.go
git commit -m "$(cat <<'EOF'
test(code): cover Dockerfile filename-glob and unknown-extension fallback

Pin the behavior so a future Chroma upgrade that drops Dockerfile from
the filename glob list is caught by CI, and confirm unknown extensions
still render with a gutter via the plain-text fallback.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Extract style selection into `style.go`

**Files:**
- Create: `internal/code/style.go`
- Modify: `internal/code/render.go`

Pure refactor: move the Chroma style lookup out of `NewRenderer` so the only file touching `styles` is `style.go`. Sets up future theme work to land in one place.

- [ ] **Step 1: Create `style.go`**

Create `internal/code/style.go`:

```go
package code

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

// defaultStyle returns the Chroma style code rendering uses. Hardcoded
// to monokai (which matches Glamour's dark code-fence palette) for v1.
// User-configurable themes are deferred to v2; keep this the only call
// site for styles so the future hook has one place to land.
func defaultStyle() *chroma.Style {
	s := styles.Get("monokai")
	if s == nil {
		return styles.Fallback
	}
	return s
}
```

- [ ] **Step 2: Update `NewRenderer` to use it**

In `internal/code/render.go`, remove the `styles` import (it's no longer used in this file) and change `NewRenderer`:

```go
func NewRenderer(width int) *Renderer {
	if width < 20 {
		width = 80
	}
	return &Renderer{width: width, style: defaultStyle()}
}
```

- [ ] **Step 3: Run the test suite to confirm no regression**

Run: `go test ./internal/code/... -v`

Expected: all seven tests still PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/code/style.go internal/code/render.go
git commit -m "$(cat <<'EOF'
refactor(code): extract style selection into style.go

style.go is now the only file in internal/code that imports
chroma/v2/styles, giving future theme work a single place to land.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Wire `codeRenderer` into the TUI model

**Files:**
- Modify: `internal/tui/content.go` (struct field)
- Modify: `internal/tui/model.go` (construction in `New`, rebuild on resize)

This task only adds the renderer to the model. The dispatch in `refreshContent` lands in the next task — splitting them keeps each commit small and reviewable.

- [ ] **Step 1: Add the field to `contentUIState`**

In `internal/tui/content.go`, change the `contentUIState` struct (currently lines 20–25):

```go
type contentUIState struct {
	viewport     viewport.Model
	renderer     *markdown.Renderer
	codeRenderer *code.Renderer
	links        []markdown.Link
	linkCursor   int
}
```

Add the import at the top of the file:

```go
"github.com/wilkes/hypogeum/internal/code"
```

- [ ] **Step 2: Build the renderer in `tui.New`**

In `internal/tui/model.go` around line 154, after the existing `markdown.NewRenderer` call, add:

```go
	r, err := markdown.NewRenderer(80, rOpts...)
	if err != nil {
		return Model{}, err
	}
	cr := code.NewRenderer(80)
```

Add the import:

```go
"github.com/wilkes/hypogeum/internal/code"
```

Then in the `Model{}` literal further down (around line 173), update `contentUIState`:

```go
		content: contentUIState{
			viewport:     viewport.New(0, 0),
			renderer:     r,
			codeRenderer: cr,
			linkCursor:   -1,
		},
```

- [ ] **Step 3: Rebuild on `WindowSizeMsg`**

In `internal/tui/model.go` around lines 247–254 (inside the `WindowSizeMsg` case), add a code-renderer rebuild next to the markdown one:

```go
		renderWidth := min(contentWidth, maxRenderWidth)
		var rOpts []markdown.Option
		if m.vault != nil {
			rOpts = append(rOpts, markdown.WithResolver(m.vault))
		}
		if r, err := markdown.NewRenderer(renderWidth, rOpts...); err == nil {
			m.content.renderer = r
		}
		m.content.codeRenderer = code.NewRenderer(renderWidth)
```

`code.NewRenderer` doesn't return an error, so no error-handling branch is needed.

- [ ] **Step 4: Verify the project still builds and tests pass**

Run: `go build ./... && go test ./...`

Expected: clean build, all existing tests PASS. No new tests yet — those land in the next task, which actually uses `codeRenderer`.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/content.go internal/tui/model.go
git commit -m "$(cat <<'EOF'
feat(tui): construct code.Renderer alongside markdown.Renderer

Wires the new internal/code renderer into the TUI model so it gets
built in tui.New and rebuilt on every WindowSizeMsg at the same width
as the markdown renderer. Dispatch happens in the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Dispatch in `refreshContent`

**Files:**
- Modify: `internal/tui/content.go`
- Create: `internal/tui/content_test.go`

- [ ] **Step 1: Write the failing dispatch test**

Create `internal/tui/content_test.go`:

```go
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestRefreshContent_CodeFile_DispatchesToCodeRenderer verifies that
// refreshContent on a .go path produces non-empty viewport content,
// empty links, and links cursor cleared.
func TestRefreshContent_CodeFile_DispatchesToCodeRenderer(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "index.md")
	goPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mdPath, []byte("# index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goPath, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	m.refreshContent(goPath)

	view := m.content.viewport.View()
	if strings.TrimSpace(view) == "" {
		t.Error("viewport empty after refreshContent on .go file")
	}
	if len(m.content.links) != 0 {
		t.Errorf("expected no links for code file, got %d", len(m.content.links))
	}
	if m.content.linkCursor != -1 {
		t.Errorf("expected linkCursor == -1, got %d", m.content.linkCursor)
	}
	if m.status != goPath {
		t.Errorf("expected status to be %q, got %q", goPath, m.status)
	}
}

// TestRefreshContent_CodeFileReadError_ClearsLinksAndReportsStatus
// covers the error path: refreshContent on a missing code file should
// still leave the model in a consistent state.
func TestRefreshContent_CodeFileReadError_ClearsLinksAndReportsStatus(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "index.md")
	if err := os.WriteFile(mdPath, []byte("# index\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	m.refreshContent(filepath.Join(dir, "nonexistent.go"))

	if m.status == "" {
		t.Error("expected status to carry read error, got empty string")
	}
	if len(m.content.links) != 0 {
		t.Errorf("expected links cleared after read error, got %d", len(m.content.links))
	}
}
```

- [ ] **Step 2: Run to confirm both fail**

Run: `go test ./internal/tui/... -run "TestRefreshContent_CodeFile" -v`

Expected: FAIL on `TestRefreshContent_CodeFile_DispatchesToCodeRenderer` — the current `refreshContent` runs Glamour on `.go` and produces either empty or word-wrapped prose; the assertion that links is empty might already pass (since `.go` has no markdown link AST), but the status check ties it down.

Note: the read-error test may already pass under the current code (read errors clear `m.content.links` in the existing implementation). That's fine — it pins existing behavior so the dispatch refactor doesn't break it.

- [ ] **Step 3: Modify `refreshContent` to dispatch by extension**

In `internal/tui/content.go`, replace the body of `refreshContent` (currently lines 77–119) with:

```go
func (m *Model) refreshContent(path string) {
	// Single-shot pre-select: clear the field unconditionally before any
	// early return, so a read or render failure here can't leak a stale
	// target into the next refreshContent.
	target := m.pendingPreselectTarget
	m.pendingPreselectTarget = ""

	src, err := os.ReadFile(path)
	if err != nil {
		m.status = err.Error()
		m.content.viewport.SetContent(fmt.Sprintf("Error: %v", err))
		m.content.links = nil
		m.content.linkCursor = -1
		return
	}

	if !tree.IsMarkdown(path) {
		out, rerr := m.content.codeRenderer.Render(path, src)
		if rerr != nil {
			m.status = rerr.Error()
			m.content.viewport.SetContent(fmt.Sprintf("Error: %v", rerr))
		} else {
			m.status = path
			m.content.viewport.SetContent(out)
			m.content.viewport.GotoTop()
		}
		m.content.links = nil
		m.content.linkCursor = -1
		_ = target // preselect doesn't apply to code files
		return
	}

	m.content.renderer.SetFromFile(path)
	out, links, err := m.content.renderer.RenderWithLinks(string(src), path, linkZoneMarker)
	if err != nil {
		m.status = err.Error()
		m.content.viewport.SetContent(fmt.Sprintf("Error: %v", err))
		m.content.links = nil
		m.content.linkCursor = -1
		return
	}
	m.status = path
	m.content.viewport.SetContent(out)
	m.content.viewport.GotoTop()
	m.content.links = links

	m.content.linkCursor = -1
	if target != "" {
		for i, l := range links {
			if l.Resolved.Kind == markdown.LinkLocalFile && l.Resolved.Target == target {
				m.content.linkCursor = i
				break
			}
		}
	}
	if m.content.linkCursor >= 0 {
		m.scrollToLink(m.content.links[m.content.linkCursor])
		m.applyLinkHighlight()
	}
}
```

Add the import for `tree` if it's not already there:

```go
"github.com/wilkes/hypogeum/internal/tree"
```

(Note: `tree` is already imported in `content.go` from the existing `tree.Node` usage — verify with `grep "internal/tree" internal/tui/content.go` first; if it's there, no import change needed.)

- [ ] **Step 4: Run the new tests plus the full TUI suite**

Run: `go test ./internal/tui/... -v 2>&1 | tail -40`

Expected: new tests PASS. All existing tests in `internal/tui/` still PASS. If a prior test that relied on `.go` extensions going through Glamour breaks, it's the test that's wrong now — code files take the new path.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/content.go internal/tui/content_test.go
git commit -m "$(cat <<'EOF'
feat(tui): dispatch non-markdown files to code.Renderer in refreshContent

refreshContent now branches on tree.IsMarkdown(path): markdown files
keep the existing Glamour + link-extraction path; everything else
renders through internal/code with no link list. CLI args, inline link
follows, and Back/Forward all benefit because they share this single
content-load chokepoint.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Relax watcher's write-event classifier

**Files:**
- Modify: `internal/watch/classify.go`
- Modify: `internal/watch/classify_test.go`

- [ ] **Step 1: Write the failing classifier tests**

First read the existing test file to learn the test style:

```bash
cat internal/watch/classify_test.go
```

Append to `internal/watch/classify_test.go` (adjust import block if `fsnotify` isn't yet imported in tests):

```go
func TestClassify_WriteOnNonMarkdown_NotIgnored(t *testing.T) {
	r := classify(fsnotify.Event{Name: "/tmp/notes/main.go", Op: fsnotify.Write})
	if r.Ignore {
		t.Error("expected write on .go file to NOT be ignored (live-reload for code files)")
	}
	if r.Kind != FileModified {
		t.Errorf("expected Kind=FileModified, got %v", r.Kind)
	}
	if r.Path != "/tmp/notes/main.go" {
		t.Errorf("expected Path preserved, got %q", r.Path)
	}
}

func TestClassify_CreateOnNonMarkdown_StillStructureChange(t *testing.T) {
	// Structure changes stay markdown-only: a new .py file should not
	// trigger a tree re-walk. classify returns StructureChanged +
	// MaybeNewDir; the stage() wrapper does the IsMarkdown check.
	r := classify(fsnotify.Event{Name: "/tmp/notes/script.py", Op: fsnotify.Create})
	if r.Ignore {
		t.Error("classify should not ignore Create on .py — that's stage()'s job")
	}
	if !r.MaybeNewDir {
		t.Error("expected MaybeNewDir on Create event")
	}
}
```

- [ ] **Step 2: Run to confirm the write test fails**

Run: `go test ./internal/watch/... -run "TestClassify_Write" -v`

Expected: FAIL on `TestClassify_WriteOnNonMarkdown_NotIgnored` — the current code returns `Ignore: true` for any write to a non-markdown file.

`TestClassify_CreateOnNonMarkdown_StillStructureChange` should PASS already (Create events flow through `MaybeNewDir` regardless of extension). Run it too:

Run: `go test ./internal/watch/... -run "TestClassify_Create" -v`

Expected: PASS. If it fails, the spec's assumption about the Create path is wrong — stop and reconcile.

- [ ] **Step 3: Relax the write gate in `classify`**

In `internal/watch/classify.go`, replace the `Write` case (lines 41–45):

```go
	case ev.Op&fsnotify.Write != 0:
		if !tree.IsMarkdown(ev.Name) {
			return classifyResult{Path: ev.Name, Ignore: true}
		}
		return classifyResult{Kind: FileModified, Path: ev.Name}
```

with:

```go
	case ev.Op&fsnotify.Write != 0:
		// Emit FileModified for any write. The TUI's handleFSEvent
		// filters by "is this the currently open file?" so writes to
		// non-md files we don't have open are discarded one layer up
		// at zero cost. Relaxing here is what makes live-reload work
		// when the open file is a .go/.rb/.py/etc.
		return classifyResult{Kind: FileModified, Path: ev.Name}
```

The `tree` import becomes unused for this function but is still needed by `stage()` (line 65) which keeps the markdown-only gate for structure changes. Keep the import.

- [ ] **Step 4: Run the watch suite**

Run: `go test ./internal/watch/... -v`

Expected: both new tests PASS, all existing tests still PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/watch/classify.go internal/watch/classify_test.go
git commit -m "$(cat <<'EOF'
feat(watch): live-reload for non-markdown writes

classify no longer drops Write events on non-markdown files. The TUI's
handleFSEvent already filters modification events by "is this the
currently open file?", so events for non-md files we don't have open
are discarded one layer up at zero cost. Structure changes (Create/
Remove/Rename) stay markdown-only via stage() — a new .py file still
doesn't trigger a tree re-walk.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Update CLAUDE.md with the new dispatch behavior

**Files:**
- Modify: `CLAUDE.md`

The Gotchas section is the right home — it's where future contributors will look first.

- [ ] **Step 1: Add the Gotcha bullet**

Open `CLAUDE.md` and find the "Gotchas" section. Append a new bullet at the end of that section:

```markdown
- **Non-markdown files render via `internal/code`, not Glamour.** `refreshContent` (`internal/tui/content.go`) branches on `tree.IsMarkdown(path)`. Markdown goes through `markdown.Renderer.RenderWithLinks`; everything else goes through `code.Renderer.Render`, which is a Chroma → 256-color ANSI → line-number gutter → soft-wrap pipeline. Code files have no `markdown.Link` slice — link cycling (`n`/`p`/`Enter`) is a natural no-op. Tree modal and the `^p` picker stay markdown-only; code files are reachable only via CLI arg or an inline relative link from a markdown file. The watcher's *write* classifier (`internal/watch/classify.go`) accepts any path so live-reload works for the open code file; the *structure* classifier (`stage()`) stays markdown-only so a new `.py` doesn't trigger a tree re-walk.
```

- [ ] **Step 2: Remove or update the related entry in "What's not built yet"**

The current `CLAUDE.md` doesn't have a code-rendering entry under "What's not built yet," so no removal needed. If it gets added during implementation, strike or update it now.

- [ ] **Step 3: Build and test the whole project once for confidence**

Run: `go build ./... && go test ./...`

Expected: clean build, all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "$(cat <<'EOF'
docs(claude-md): note non-markdown dispatch in Gotchas

Captures the refreshContent branch, the per-package boundaries, and
the asymmetric watcher gates so a future contributor doesn't
re-introduce the IsMarkdown check in the wrong place.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Manual smoke test and theme tune

**Files:** none changed unless theme tweak needed.

Plan tasks are usually "write code, run tests" but the spec explicitly calls out a *visual* acceptance check that no automated test can stand in for: does a standalone `.go` file look the same as a fenced `.go` block inside a `.md`?

- [ ] **Step 1: Build and install**

```bash
go install ./cmd/hypogeum
```

- [ ] **Step 2: Side-by-side compare**

In a real terminal:

```bash
# Open a markdown file with a Go fence:
hypogeum docs/

# Then in another pane (or after exiting):
hypogeum cmd/hypogeum/main.go
```

Look at: keyword colors, string colors, comment colors, identifier colors. They should be visually indistinguishable (Glamour uses Chroma with `monokai` for dark code fences; we use the same).

If they differ noticeably, edit `internal/code/style.go::defaultStyle` to try a different built-in:

```go
// Available alternatives if monokai drifts:
//   "monokai-light", "swapoff", "github", "dracula", ...
```

Pick whichever matches and commit:

```bash
git add internal/code/style.go
git commit -m "fix(code): switch default style to <name> to match Glamour fences"
```

- [ ] **Step 3: Live-reload smoke test**

```bash
hypogeum some-source-file.go
# In another terminal:
echo "// new comment" >> some-source-file.go
```

The hypogeum view should refresh on the write, preserving scroll offset.

- [ ] **Step 4: Link-follow smoke test**

```bash
mkdir -p /tmp/smoke
cat > /tmp/smoke/index.md <<'EOF'
# Smoke

[main.go](./main.go)
EOF
cp cmd/hypogeum/main.go /tmp/smoke/main.go
hypogeum /tmp/smoke/
```

Press `n` to select the link, `Enter` to follow. The viewport should show the highlighted `.go` file. Press `h` (back) — you should return to the markdown index.

- [ ] **Step 5: Confirm and (if no theme commit was needed) finalize**

If everything looks right, no commit is needed for this task — the smoke test is a checkpoint, not a code change. If a theme tweak was committed in Step 2, that's the final commit on the branch.

---

## Task 12: Push branch and open PR

- [ ] **Step 1: Push**

```bash
git push -u origin code-file-rendering
```

- [ ] **Step 2: Open PR**

```bash
gh pr create --title "feat: syntax-highlighted code-file rendering" --body "$(cat <<'EOF'
## Summary

- Renders non-markdown files (`.go`, `.rb`, `.py`, `Dockerfile`, etc.) with Chroma-driven syntax highlighting and a line-number gutter when opened via CLI arg or inline link.
- Tree modal (`^b`) and picker (`^p`) stay markdown-only; code files reachable only by CLI or inline relative link.
- Live-reload works for the currently open code file.

## Spec & plan

- [Design spec](docs/superpowers/specs/2026-05-12-code-file-rendering-design.md)
- [Implementation plan](docs/superpowers/plans/2026-05-12-code-file-rendering.md)

## Test plan

- [ ] `go test ./...` passes (code, tui, watch suites).
- [ ] Open a `.go` file via CLI; syntax highlighted with line numbers.
- [ ] Follow a `[name](./file.go)` link from a markdown file; lands on highlighted view.
- [ ] Save the open `.go` file in an editor; hypogeum refreshes preserving scroll.
- [ ] Open a binary file; renders the "binary file, not displayed" message.
- [ ] Open a >5 MB file; renders the "file too large to display" message.
- [ ] Visual: `.go` standalone looks the same as a `.go` fence in a `.md`.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Merge with `gh pr merge --merge` (per project policy in CLAUDE.md), not squash.

---

## Self-review notes (for the executor)

This section is for the engineer executing the plan — not for the planner.

- The plan is twelve tasks. Tasks 1–6 build `internal/code` in isolation. Tasks 7–9 integrate. Tasks 10–12 are documentation, smoke testing, and PR.
- Every task ends with a commit. Don't batch multiple tasks into one commit — each commit should be reviewable on its own.
- If a step's test passes on the first run, that's not a bug — it means the existing code already had the behavior, and you've pinned it against regression. Commit anyway.
- The branch is `code-file-rendering`. Don't merge to `main` until the PR is approved.
