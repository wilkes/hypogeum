# Source-file embeds and line-range links — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `![[file.go#L10-L20]]` inline embeds and `[t](file.go#L10-L20)` range links in hypogeum, with live-sync when the embedded source changes on disk and source-side highlight when navigating into the range.

**Architecture:** A new pure package `internal/embed` parses tokens, slices files, and renders fence bodies. `internal/markdown` gains a preprocess pass that consumes `![[…]]` tokens before the existing wikilink pass and turns them into fenced code blocks for Glamour to render. `internal/code.Renderer` learns an optional range-highlight. `internal/tui` tracks per-render embed dependencies so a watcher event on any embedded source refreshes the open markdown.

**Tech Stack:** Go, Bubble Tea / Bubbles / Lip Gloss / Glamour, Chroma (via `internal/code`), goldmark (only indirectly, via existing `internal/markdown`), fsnotify (via `internal/watch`).

**Spec:** [docs/superpowers/specs/2026-05-13-source-embeds-design.md](../specs/2026-05-13-source-embeds-design.md)

---

## File Structure

**New files (in `internal/embed/`):**
- `parse.go` — `Embed`, `LineRange`, `ParseEmbedToken(body string) (*Embed, error)`. Pure.
- `parse_test.go`
- `slice.go` — `SliceFile(absPath string, r *LineRange, ctx int) (lines []string, startLine int, err error)`. Reads files; rejects binary / oversize.
- `slice_test.go`
- `fence.go` — `RenderToFence(absPath string, lines []string, startLine int, displayRange string, ctxCount int) string`. Pure string assembly.
- `fence_test.go`
- `lang.go` — `LanguageFromPath(path string) string`. Extension → Chroma/Glamour language tag.
- `lang_test.go`

**Modified files:**
- `internal/markdown/links.go` — `ResolvedLink.Range *LineRange`, `ResolveLink` parses `#L<n>-L<n>` before falling back to anchor.
- `internal/markdown/links_render.go` — new `preprocessEmbeds(src, base) (out string, deps []string)`, called before `preprocessWikilinks` in `RenderWithLinks`. Embed contributes one entry to the returned link slice via a new helper that runs alongside the existing `ExtractLinks` flow.
- `internal/markdown/render.go` — `RenderWithLinks` return signature extends to `(string, []Link, []string /* embedDeps */, error)`.
- `internal/code/render.go` — `Renderer.RenderOpts(path, src, opts RenderOptions) (string, error)`; `Render` becomes a thin wrapper. New `RenderOptions{Highlight *LineRange}`.
- `internal/code/gutter.go` — `addGutter` learns an optional highlight range to reverse-video the gutter for those source lines.
- `internal/watch/watch.go` — `(*Watcher).AddPath(dir string) error`, idempotent wrapper over `fsw.Add`.
- `internal/tui/content.go` — `contentUIState.embedDeps map[string]struct{}`; `refreshContent` consumes the deps from the renderer return, calls `m.watcher.AddPath` for new parent dirs, and `handleFSEvent` checks `embedDeps` in the `FileModified` branch.
- `internal/tui/content.go` — `refreshContent` passes `code.RenderOptions{Highlight: pendingRangeHighlight}` when set.
- `internal/tui/input.go` — `Esc` cascade clears `m.content.rangeHighlight`.
- `internal/tui/model.go` — fields: `pendingRangeHighlight *markdown.LineRange` (set by `followLink` when target is non-markdown and link has a range); `m.content.rangeHighlight *markdown.LineRange` (current).
- `CLAUDE.md` — Gotchas entry for embed grammar, drift semantics, and AddPath behavior.
- `docs/index.md` — link to the source-embeds spec and plan.

**Type ownership note:** `LineRange` is defined once in `internal/markdown` (so `ResolvedLink` and the cross-package contract live in the package most consumers already import). `internal/embed` defines its own `LineRange` *alias* (`type LineRange = markdown.LineRange`) to avoid an import cycle if `embed` ever needs to depend on something else in `markdown`. We pick the alias direction now because `embed` is brand new and zero-cost to wire either way.

---

## Task 1: `internal/embed` package — `LineRange` + token grammar

**Files:**
- Create: `internal/embed/parse.go`
- Create: `internal/embed/parse_test.go`

This task introduces the `Embed` struct and the parser that turns the *body* of `![[…]]` (i.e. the text between the brackets) into a structured value. The package owns embed-related types; `internal/markdown` will alias `LineRange` from here in a later task.

The grammar (from the spec):
```
<path>                            — whole file
<path>#L<n>                       — single line
<path>#L<a>-L<b>                  — line range, inclusive
<path>#L<a>-L<b>+<c>              — range with c context lines on each side
```
Trailing whitespace inside the brackets is trimmed. An alias suffix (`|name`) is *not* part of this grammar (wikilinks accept it; embeds don't). The path may contain `/` and `.` but no `#`, `|`, or `^`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/embed/parse_test.go
package embed

import (
	"reflect"
	"testing"
)

func TestParseEmbedToken_WholeFile(t *testing.T) {
	got, err := ParseEmbedToken("main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := &Embed{Path: "main.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseEmbedToken_SingleLine(t *testing.T) {
	got, err := ParseEmbedToken("main.go#L5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := &Embed{Path: "main.go", Range: &LineRange{Start: 5, End: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseEmbedToken_Range(t *testing.T) {
	got, err := ParseEmbedToken("a/b/main.go#L10-L20")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := &Embed{Path: "a/b/main.go", Range: &LineRange{Start: 10, End: 20}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseEmbedToken_RangeWithContext(t *testing.T) {
	got, err := ParseEmbedToken("main.go#L10-L20+3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := &Embed{Path: "main.go", Range: &LineRange{Start: 10, End: 20}, ContextLines: 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseEmbedToken_TrimmedWhitespace(t *testing.T) {
	got, err := ParseEmbedToken("  main.go#L5  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Path != "main.go" || got.Range == nil || got.Range.Start != 5 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseEmbedToken_Errors(t *testing.T) {
	cases := []string{
		"",                    // empty
		"  ",                  // whitespace only
		"main.go#L",           // empty line spec
		"main.go#L0",          // zero is invalid (1-indexed)
		"main.go#Labc",        // non-numeric
		"main.go#L10-L5",      // inverted range
		"main.go#L10-L20+",    // missing context number
		"main.go#L10-L20+abc", // non-numeric context
		"main.go#L10-",        // partial range
		"main.go#L-L20",       // partial range
		"main.go#XYZ",         // not a line spec at all (would route to wikilink/anchor elsewhere)
	}
	for _, body := range cases {
		if got, err := ParseEmbedToken(body); err == nil {
			t.Errorf("body %q: expected error, got %+v", body, got)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/embed/...`
Expected: FAIL with "no Go files" or "undefined: ParseEmbedToken".

- [ ] **Step 3: Write the implementation**

```go
// internal/embed/parse.go
// Package embed turns ![[file.go#L10-L20+3]] source-embed tokens into
// renderable code-fence strings. It is pure: it does not know about
// Glamour, Bubble Tea, or the TUI. The markdown package's render
// pipeline calls into here to preprocess embeds before Glamour sees
// the source.
package embed

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// LineRange is an inclusive [Start, End] pair of 1-indexed source-file
// line numbers. End == Start represents a single line.
type LineRange struct {
	Start, End int
}

// Embed is the parsed body of a ![[…]] token.
type Embed struct {
	Path         string     // relative or absolute path as written
	Range        *LineRange // nil = whole file
	ContextLines int        // 0 unless +<c> suffix present
}

// lineSpec matches L<n> or L<n>-L<n> (the inner fragment after '#').
var lineSpec = regexp.MustCompile(`^L(\d+)(?:-L(\d+))?(?:\+(\d+))?$`)

// ParseEmbedToken parses the contents *between* the brackets of an
// embed token. Returns an error for any malformed body; the caller
// (markdown.preprocessEmbeds) renders these as a warning blockquote.
func ParseEmbedToken(body string) (*Embed, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("empty embed token")
	}
	hash := strings.IndexByte(body, '#')
	if hash < 0 {
		return &Embed{Path: body}, nil
	}
	path := strings.TrimSpace(body[:hash])
	frag := strings.TrimSpace(body[hash+1:])
	if path == "" {
		return nil, errors.New("empty path")
	}
	m := lineSpec.FindStringSubmatch(frag)
	if m == nil {
		return nil, errors.New("invalid line spec: " + frag)
	}
	start, _ := strconv.Atoi(m[1])
	if start < 1 {
		return nil, errors.New("line numbers are 1-indexed")
	}
	end := start
	if m[2] != "" {
		end, _ = strconv.Atoi(m[2])
		if end < start {
			return nil, errors.New("inverted range")
		}
	}
	ctx := 0
	if m[3] != "" {
		ctx, _ = strconv.Atoi(m[3])
	}
	return &Embed{Path: path, Range: &LineRange{Start: start, End: end}, ContextLines: ctx}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/embed/...`
Expected: PASS (all 6 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/embed/parse.go internal/embed/parse_test.go
git commit -m "feat(embed): parse ![[file#L10-L20+3]] embed tokens"
```

---

## Task 2: `internal/embed` — file slicing

**Files:**
- Create: `internal/embed/slice.go`
- Create: `internal/embed/slice_test.go`

`SliceFile` reads the source file from disk, validates size/binary, applies the range and context-lines padding, and returns the slice plus the absolute starting line number. Errors come back as typed values so the caller can render distinct warnings.

- [ ] **Step 1: Write the failing tests**

```go
// internal/embed/slice_test.go
package embed

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeTmp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestSliceFile_WholeFile(t *testing.T) {
	p := writeTmp(t, "x.go", "a\nb\nc\n")
	lines, start, err := SliceFile(p, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != 1 {
		t.Fatalf("start = %d, want 1", start)
	}
	if len(lines) != 3 || lines[0] != "a" || lines[1] != "b" || lines[2] != "c" {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestSliceFile_Range(t *testing.T) {
	p := writeTmp(t, "x.go", "1\n2\n3\n4\n5\n")
	lines, start, err := SliceFile(p, &LineRange{Start: 2, End: 4}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != 2 {
		t.Fatalf("start = %d, want 2", start)
	}
	if len(lines) != 3 || lines[0] != "2" || lines[2] != "4" {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestSliceFile_Context(t *testing.T) {
	p := writeTmp(t, "x.go", "1\n2\n3\n4\n5\n6\n7\n8\n")
	lines, start, err := SliceFile(p, &LineRange{Start: 4, End: 5}, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != 2 {
		t.Fatalf("start = %d, want 2 (4 - 2 context)", start)
	}
	if len(lines) != 6 { // 2..7 inclusive
		t.Fatalf("len = %d, want 6", len(lines))
	}
}

func TestSliceFile_ContextClampedAtStart(t *testing.T) {
	p := writeTmp(t, "x.go", "1\n2\n3\n4\n5\n")
	lines, start, err := SliceFile(p, &LineRange{Start: 2, End: 2}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != 1 {
		t.Fatalf("start = %d, want 1 (clamped)", start)
	}
	if len(lines) != 5 {
		t.Fatalf("len = %d, want 5 (whole file)", len(lines))
	}
}

func TestSliceFile_RangeEndPastEOF(t *testing.T) {
	p := writeTmp(t, "x.go", "1\n2\n3\n")
	lines, start, err := SliceFile(p, &LineRange{Start: 2, End: 100}, 0)
	if !errors.Is(err, ErrRangePastEOF) {
		t.Fatalf("err = %v, want ErrRangePastEOF", err)
	}
	if start != 2 || len(lines) != 2 || lines[0] != "2" || lines[1] != "3" {
		t.Fatalf("lines = %#v, start = %d", lines, start)
	}
}

func TestSliceFile_StartPastEOF(t *testing.T) {
	p := writeTmp(t, "x.go", "1\n2\n3\n")
	_, _, err := SliceFile(p, &LineRange{Start: 100, End: 200}, 0)
	if !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("err = %v, want ErrInvalidRange", err)
	}
}

func TestSliceFile_Binary(t *testing.T) {
	p := writeTmp(t, "x.bin", "abc\x00def")
	_, _, err := SliceFile(p, nil, 0)
	if !errors.Is(err, ErrBinary) {
		t.Fatalf("err = %v, want ErrBinary", err)
	}
}

func TestSliceFile_OversizeWholeFile(t *testing.T) {
	big := make([]byte, 5*1024*1024+1)
	for i := range big {
		big[i] = 'x'
	}
	p := writeTmp(t, "x.txt", string(big))
	_, _, err := SliceFile(p, nil, 0)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestSliceFile_Missing(t *testing.T) {
	_, _, err := SliceFile("/no/such/path", nil, 0)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/embed/...`
Expected: FAIL (undefined: SliceFile and the sentinel errors).

- [ ] **Step 3: Write the implementation**

```go
// internal/embed/slice.go
package embed

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"strings"
)

// Sentinel errors for the four embed warnings the renderer surfaces.
// ErrRangePastEOF is *non-fatal*: SliceFile clamps and still returns
// a usable slice so the renderer can show what it has alongside the
// "file ends at line N" header note.
var (
	ErrNotFound     = errors.New("source file not found")
	ErrBinary       = errors.New("source file is binary")
	ErrTooLarge     = errors.New("source file too large to embed")
	ErrInvalidRange = errors.New("invalid range")
	ErrRangePastEOF = errors.New("range past EOF (clamped)")
)

// maxSize mirrors internal/code.maxSize so we don't embed anything we
// wouldn't render as a standalone code file.
const maxSize = 5 * 1024 * 1024

// SliceFile reads absPath, validates it, and returns the requested
// line slice expanded by ctx lines on each side. r == nil returns the
// whole file. Lines have no trailing newline. startLine is 1-indexed
// and reflects any context-expansion (so for a #L10-L20+3 embed on a
// long-enough file, startLine == 7).
func SliceFile(absPath string, r *LineRange, ctx int) ([]string, int, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	if len(data) > maxSize {
		return nil, 0, ErrTooLarge
	}
	if looksBinary(data) {
		return nil, 0, ErrBinary
	}

	// Split keeping line semantics intact. strings.Split on "\n" produces
	// a trailing empty element when the file ends with a newline; trim it
	// so line counts match what users see in their editor.
	all := strings.Split(string(data), "\n")
	if n := len(all); n > 0 && all[n-1] == "" {
		all = all[:n-1]
	}
	total := len(all)

	if r == nil {
		return all, 1, nil
	}
	if r.Start < 1 || r.Start > total {
		return nil, 0, ErrInvalidRange
	}
	start := r.Start - ctx
	if start < 1 {
		start = 1
	}
	end := r.End + ctx
	clamped := false
	if end > total {
		end = total
		clamped = true
	}
	lines := all[start-1 : end]
	if clamped {
		return lines, start, ErrRangePastEOF
	}
	return lines, start, nil
}

// looksBinary uses the same NUL-in-first-8KB rule as internal/code.
func looksBinary(src []byte) bool {
	n := len(src)
	if n > 8192 {
		n = 8192
	}
	return bytes.IndexByte(src[:n], 0) >= 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/embed/...`
Expected: PASS (all 9 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/embed/slice.go internal/embed/slice_test.go
git commit -m "feat(embed): slice source files with context-line padding"
```

---

## Task 3: `internal/embed` — language detection

**Files:**
- Create: `internal/embed/lang.go`
- Create: `internal/embed/lang_test.go`

A small mapping from file extension to a language tag we'll write into the markdown code fence (e.g. ```` ```go ````). Glamour passes the tag through to Chroma for syntax highlighting. We do this here rather than via Chroma's `lexers.Match` because the fence-language vocabulary is markdown-side and Chroma's lexer names occasionally diverge from common fence aliases.

- [ ] **Step 1: Write the failing tests**

```go
// internal/embed/lang_test.go
package embed

import "testing"

func TestLanguageFromPath(t *testing.T) {
	cases := map[string]string{
		"main.go":         "go",
		"app.py":          "python",
		"index.ts":        "typescript",
		"index.tsx":       "tsx",
		"foo.js":          "javascript",
		"a.rs":            "rust",
		"a.rb":            "ruby",
		"a.sh":            "bash",
		"Makefile":        "makefile",
		"Dockerfile":      "dockerfile",
		"config.yaml":     "yaml",
		"config.yml":      "yaml",
		"data.json":       "json",
		"styles.css":      "css",
		"page.html":       "html",
		"q.sql":           "sql",
		"notes.md":        "markdown",
		"unknown.xyzqux":  "",
		"":                "",
	}
	for path, want := range cases {
		if got := LanguageFromPath(path); got != want {
			t.Errorf("LanguageFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/embed/...`
Expected: FAIL (undefined: LanguageFromPath).

- [ ] **Step 3: Write the implementation**

```go
// internal/embed/lang.go
package embed

import (
	"path/filepath"
	"strings"
)

var extLang = map[string]string{
	".go":   "go",
	".py":   "python",
	".ts":   "typescript",
	".tsx":  "tsx",
	".js":   "javascript",
	".jsx":  "jsx",
	".rs":   "rust",
	".rb":   "ruby",
	".sh":   "bash",
	".bash": "bash",
	".zsh":  "bash",
	".yaml": "yaml",
	".yml":  "yaml",
	".json": "json",
	".toml": "toml",
	".css":  "css",
	".scss": "scss",
	".html": "html",
	".htm":  "html",
	".sql":  "sql",
	".md":   "markdown",
	".c":    "c",
	".h":    "c",
	".cpp":  "cpp",
	".cc":   "cpp",
	".hpp":  "cpp",
	".java": "java",
	".kt":   "kotlin",
	".swift":"swift",
	".clj":  "clojure",
	".cljs": "clojure",
	".ex":   "elixir",
	".exs":  "elixir",
	".lua":  "lua",
	".php":  "php",
}

var nameLang = map[string]string{
	"Makefile":   "makefile",
	"makefile":   "makefile",
	"Dockerfile": "dockerfile",
	"dockerfile": "dockerfile",
}

// LanguageFromPath returns the markdown fence-language tag for path's
// basename. Returns "" for unrecognized extensions; callers should
// render an untagged fence in that case.
func LanguageFromPath(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	if lang, ok := nameLang[base]; ok {
		return lang
	}
	ext := strings.ToLower(filepath.Ext(base))
	return extLang[ext]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/embed/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/embed/lang.go internal/embed/lang_test.go
git commit -m "feat(embed): map file extensions to fence language tags"
```

---

## Task 4: `internal/embed` — fence assembly

**Files:**
- Create: `internal/embed/fence.go`
- Create: `internal/embed/fence_test.go`

This produces the markdown text we splice into the source before Glamour renders it: provenance header + fenced code block with literal-text gutter inside the fence body. Context lines (the ones added by the `+<c>` form) get a `~` in place of their line number so the reader can tell them apart from the requested range.

- [ ] **Step 1: Write the failing tests**

```go
// internal/embed/fence_test.go
package embed

import (
	"strings"
	"testing"
)

func TestRenderToFence_RangeWithGutter(t *testing.T) {
	lines := []string{"func parse(s string) Tree {", "    // build AST", "}"}
	got := RenderToFence("main.go", lines, 42, "42–44", 0, 0, "")

	if !strings.HasPrefix(got, "> `main.go:42–44`\n") {
		t.Fatalf("missing provenance header:\n%s", got)
	}
	if !strings.Contains(got, "```go\n") {
		t.Fatalf("missing language fence:\n%s", got)
	}
	if !strings.Contains(got, " 42 │ func parse(s string) Tree {\n") {
		t.Fatalf("missing first gutter line:\n%s", got)
	}
	if !strings.Contains(got, " 44 │ }\n") {
		t.Fatalf("missing last gutter line:\n%s", got)
	}
	if !strings.HasSuffix(got, "```\n") {
		t.Fatalf("missing closing fence:\n%s", got)
	}
}

func TestRenderToFence_ContextLinesMarkedFaint(t *testing.T) {
	lines := []string{"before", "primary1", "primary2", "after"}
	// Range is [primary1, primary2] = absolute lines 11-12;
	// startLine 10 means line 10 is "before" (context), line 13 is "after" (context).
	got := RenderToFence("main.go", lines, 10, "11–12", 1, 1, "")

	if !strings.Contains(got, "  ~ │ before\n") {
		t.Fatalf("context line should use ~ gutter, got:\n%s", got)
	}
	if !strings.Contains(got, " 11 │ primary1\n") {
		t.Fatalf("primary line 11 missing or wrong gutter:\n%s", got)
	}
	if !strings.Contains(got, " 12 │ primary2\n") {
		t.Fatalf("primary line 12 missing or wrong gutter:\n%s", got)
	}
	if !strings.Contains(got, "  ~ │ after\n") {
		t.Fatalf("trailing context line should use ~ gutter, got:\n%s", got)
	}
}

func TestRenderToFence_UnknownExtensionUntagged(t *testing.T) {
	got := RenderToFence("notes.zzz", []string{"hello"}, 1, "1", 0, 0, "")
	if !strings.Contains(got, "```\n  1 │ hello\n") {
		t.Fatalf("unknown extension should produce untagged fence:\n%s", got)
	}
}

func TestRenderToFence_WithSoftWarning(t *testing.T) {
	got := RenderToFence("main.go", []string{"x"}, 10, "10–20", 0, 0, "file ends at line 10")
	if !strings.Contains(got, "> `main.go:10–20 (file ends at line 10)`") {
		t.Fatalf("soft warning not in header:\n%s", got)
	}
}

func TestRenderToFence_GutterWidthFromMaxLineNumber(t *testing.T) {
	// 9 lines means single-digit gutter, but if startLine = 95, max = 103 → 3-wide.
	lines := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}
	got := RenderToFence("x.go", lines, 95, "95–103", 0, 0, "")
	if !strings.Contains(got, " 95 │ a\n") || !strings.Contains(got, "103 │ i\n") {
		t.Fatalf("gutter width should accommodate widest number:\n%s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/embed/...`
Expected: FAIL (undefined: RenderToFence).

- [ ] **Step 3: Write the implementation**

```go
// internal/embed/fence.go
package embed

import (
	"strconv"
	"strings"
)

// RenderToFence builds the markdown source we'll splice into the document.
//
// absPath        — the source file's path (only its basename is shown in the header).
// lines          — the actual source-line content (no trailing newlines).
// startLine      — 1-indexed line number of lines[0] in the original file.
// displayRange   — the range the user *asked for*, formatted for the header
//                  (e.g. "42–58" or "42"). Distinct from len(lines)/startLine
//                  because context-line padding expands the body but not the
//                  header.
// leadContext    — count of lines at the head of `lines` that are context
//                  (rendered with the ~ gutter).
// tailContext    — count of lines at the tail that are context.
// softWarning    — optional header annotation, e.g. "file ends at line 50".
func RenderToFence(absPath string, lines []string, startLine int, displayRange string, leadContext, tailContext int, softWarning string) string {
	lang := LanguageFromPath(absPath)

	// Gutter width: largest line number that will appear.
	maxLine := startLine + len(lines) - 1
	gw := len(strconv.Itoa(maxLine))
	if gw < 2 {
		gw = 2
	}

	var b strings.Builder
	// Provenance header. Use the basename so an absolute path like
	// /Users/foo/bar/main.go renders as "main.go:42–58".
	header := basename(absPath) + ":" + displayRange
	if softWarning != "" {
		header += " (" + softWarning + ")"
	}
	b.WriteString("> `")
	b.WriteString(header)
	b.WriteString("`\n")

	// Open fence.
	b.WriteString("```")
	b.WriteString(lang) // empty for unknown — yields ``` alone, which is valid.
	b.WriteByte('\n')

	// Body with gutter.
	primaryEnd := len(lines) - tailContext
	for i, line := range lines {
		isContext := i < leadContext || i >= primaryEnd
		if isContext {
			b.WriteString(strings.Repeat(" ", gw-1))
			b.WriteByte('~')
		} else {
			n := strconv.Itoa(startLine + i)
			b.WriteString(strings.Repeat(" ", gw-len(n)))
			b.WriteString(n)
		}
		b.WriteString(" │ ")
		b.WriteString(line)
		b.WriteByte('\n')
	}

	// Close fence.
	b.WriteString("```\n")
	return b.String()
}

func basename(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/embed/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/embed/fence.go internal/embed/fence_test.go
git commit -m "feat(embed): render embeds as fenced code with gutter and provenance header"
```

---

## Task 5: `markdown.LineRange` + `ResolvedLink` extension

**Files:**
- Modify: `internal/markdown/links.go`
- Modify: `internal/markdown/links_test.go`

The markdown package gets the canonical `LineRange` type (`internal/embed` will alias it later — Task 7). `ResolveLink` learns to parse the `L<n>-L<n>` form in the fragment *before* falling back to anchor handling.

- [ ] **Step 1: Read the existing `links_test.go` to find the right insertion point**

Run: `wc -l internal/markdown/links_test.go` (just to see the file size; no edit yet).

- [ ] **Step 2: Write the failing tests (append to `internal/markdown/links_test.go`)**

```go
func TestResolveLink_LineRange(t *testing.T) {
	got := ResolveLink("/base/notes.md", "code/main.go#L10-L20")
	if got.Kind != LinkLocalFile {
		t.Fatalf("kind = %v, want LinkLocalFile", got.Kind)
	}
	if got.Range == nil {
		t.Fatalf("range is nil")
	}
	if got.Range.Start != 10 || got.Range.End != 20 {
		t.Fatalf("range = %+v", got.Range)
	}
	if got.Anchor != "" {
		t.Fatalf("anchor = %q, want empty (line range claims the fragment)", got.Anchor)
	}
}

func TestResolveLink_SingleLine(t *testing.T) {
	got := ResolveLink("/base/notes.md", "main.go#L5")
	if got.Range == nil || got.Range.Start != 5 || got.Range.End != 5 {
		t.Fatalf("range = %+v", got.Range)
	}
}

func TestResolveLink_NonLineFragmentIsStillAnchor(t *testing.T) {
	got := ResolveLink("/base/notes.md", "page.md#some-heading")
	if got.Range != nil {
		t.Fatalf("range = %+v, want nil", got.Range)
	}
	if got.Anchor != "some-heading" {
		t.Fatalf("anchor = %q", got.Anchor)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/markdown/...`
Expected: FAIL ("unknown field Range in struct literal of type ResolvedLink" / "ResolvedLink has no field Range").

- [ ] **Step 4: Modify `internal/markdown/links.go`**

Add the `LineRange` type near the top of the file and a `Range` field on `ResolvedLink`:

```go
// (near the top of links.go, after the LinkKind constants)

// LineRange is an inclusive [Start, End] pair of 1-indexed source-file line
// numbers. Carried by ResolvedLink when the link's fragment is a #L<n>-L<n>
// or #L<n> form, and produced by internal/embed.ParseEmbedToken. The type
// lives in markdown so cross-package consumers can pass it through without
// importing embed (which would create a cycle the other direction).
type LineRange struct {
	Start, End int
}
```

In `ResolvedLink`, add the `Range` field:

```go
type ResolvedLink struct {
	Kind   LinkKind
	Target string
	Anchor string
	Range  *LineRange // non-nil when href fragment was a #L<n>-L<n> form
}
```

In `ResolveLink`, after computing `target` and `anchor` for a local file, parse the line-range form:

```go
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(base), target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return ResolvedLink{Kind: LinkInvalid}
	}

	out := ResolvedLink{Kind: LinkLocalFile, Target: abs, Anchor: anchor}
	if r := parseLineFragment(anchor); r != nil {
		out.Range = r
		out.Anchor = "" // line-range claims the fragment; not an anchor
	}
	return out
```

Add the helper at the bottom of the file:

```go
// lineFragmentRegex matches "L<n>" or "L<n>-L<n>" with no surrounding
// characters. Kept separate from internal/embed.lineSpec so the markdown
// package doesn't import embed.
var lineFragmentRegex = regexp.MustCompile(`^L(\d+)(?:-L(\d+))?$`)

// parseLineFragment returns a *LineRange when fragment is exactly a
// GitHub-style L<n> or L<n>-L<n> spec, or nil otherwise.
func parseLineFragment(fragment string) *LineRange {
	if fragment == "" {
		return nil
	}
	m := lineFragmentRegex.FindStringSubmatch(fragment)
	if m == nil {
		return nil
	}
	start, _ := strconv.Atoi(m[1])
	if start < 1 {
		return nil
	}
	end := start
	if m[2] != "" {
		end, _ = strconv.Atoi(m[2])
		if end < start {
			return nil
		}
	}
	return &LineRange{Start: start, End: end}
}
```

Add the new imports at the top:

```go
import (
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/markdown/...`
Expected: PASS (existing tests still pass; three new tests pass).

- [ ] **Step 6: Commit**

```bash
git add internal/markdown/links.go internal/markdown/links_test.go
git commit -m "feat(markdown): parse #L10-L20 line-range fragments on local-file links"
```

---

## Task 6: Markdown preprocess pass for `![[…]]` embeds

**Files:**
- Modify: `internal/markdown/links_render.go`
- Modify: `internal/markdown/render.go`
- Create: `internal/markdown/embed_render_test.go`

This is the central wiring. `RenderWithLinks` calls a new `preprocessEmbeds` before `preprocessWikilinks`. The pass scans for `![[…]]` outside code fences, parses each via `internal/embed.ParseEmbedToken`, slices the file, formats a fence, and substitutes back. It also returns the list of absolute paths embedded, which propagates through `RenderWithLinks`'s return signature.

**Important:** embeds also contribute to the link slice — each embed becomes a `Link` whose `Resolved.Range` is the embed's range, `Resolved.Target` is the absolute path, and `Row` is the rendered row of the provenance header. We synthesize these links *separately* from `ExtractLinks` (the AST walker) because Glamour will see the *substituted* source, where the original `![[…]]` is gone. We compute embed link rows by tracking the line offset between source and rendered output for each substitution.

Since computing exact rendered rows for synthetic embed links would require deep coupling to Glamour, we take a simpler approach: synthesize embed links with `Row = -1`. The TUI's link cycler treats `Row = -1` as "no scroll-to-link on focus" but still allows `Enter` to follow. Visual cursor position is still managed by the existing highlight marker (which we'll arrange so embeds aren't reverse-video highlighted — they're already visually distinct).

- [ ] **Step 1: Write the failing test**

```go
// internal/markdown/embed_render_test.go
package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderWithLinks_EmbedFromSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("line1\nline2\nline3\nline4\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	mdPath := filepath.Join(dir, "notes.md")
	mdSrc := "Before.\n\n![[main.go#L2-L3]]\n\nAfter.\n"

	r := NewRenderer(80)
	out, links, deps, err := r.RenderWithLinks(mdSrc, mdPath, nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if !strings.Contains(out, "line2") || !strings.Contains(out, "line3") {
		t.Fatalf("rendered output missing embed body:\n%s", out)
	}
	if strings.Contains(out, "line1") || strings.Contains(out, "line4") {
		t.Fatalf("embed leaked lines outside range:\n%s", out)
	}
	if len(deps) != 1 || deps[0] != src {
		t.Fatalf("deps = %v, want [%q]", deps, src)
	}
	if len(links) == 0 {
		t.Fatalf("expected at least one embed-derived Link")
	}
	found := false
	for _, l := range links {
		if l.Resolved.Target == src && l.Resolved.Range != nil &&
			l.Resolved.Range.Start == 2 && l.Resolved.Range.End == 3 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no embed link with range 2-3 in links: %+v", links)
	}
}

func TestRenderWithLinks_EmbedMissingFile(t *testing.T) {
	r := NewRenderer(80)
	mdSrc := "![[no-such-file.go#L1-L2]]\n"
	out, _, _, err := r.RenderWithLinks(mdSrc, "/tmp/notes.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if !strings.Contains(out, "file not found") {
		t.Fatalf("expected warning text in output:\n%s", out)
	}
}

func TestRenderWithLinks_NoEmbedsReturnsEmptyDeps(t *testing.T) {
	r := NewRenderer(80)
	out, _, deps, err := r.RenderWithLinks("just plain prose\n", "/tmp/notes.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("deps = %v, want empty", deps)
	}
	if out == "" {
		t.Fatalf("output should not be empty")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/markdown/ -run TestRenderWithLinks_Embed -v`
Expected: FAIL — `RenderWithLinks` returns three values, not four.

- [ ] **Step 3: Update `RenderWithLinks` signature in `internal/markdown/render.go` and `links_render.go`**

Open `internal/markdown/links_render.go`. Change the signature:

```go
func (r *Renderer) RenderWithLinks(src, base string, marker LinkMarker) (string, []Link, []string, error) {
	src, embedDeps, embedLinks := r.preprocessEmbeds(src, base)
	src = r.preprocessWikilinks(src)
	raw, err := r.instrumented.Render(src)
	if err != nil {
		return "", nil, nil, fmt.Errorf("render markdown: %w", err)
	}

	asts := ExtractLinks(src)
	cleaned, spans := stripSentinels(raw, marker)
	links := make([]Link, 0, len(spans)+len(embedLinks))
	for i, s := range spans {
		l := Link{Row: s.row}
		if i < len(asts) {
			l.Text = asts[i].Text
			l.Href = asts[i].Href
			l.Resolved = ResolveLink(base, asts[i].Href)
		} else {
			l.Text = s.text
		}
		links = append(links, l)
	}
	links = append(links, embedLinks...)
	return cleaned, links, embedDeps, nil
}
```

- [ ] **Step 4: Add `preprocessEmbeds` to `internal/markdown/links_render.go`**

```go
// embedTokenRegex matches ![[...]] outside of inline code spans.
// We deliberately scan the raw source pre-render; goldmark would have
// reparsed embed bodies as wikilinks. Order with preprocessWikilinks
// matters: this pass runs first so the ![[...]] form is consumed before
// the [[...]] regex sees it.
var embedTokenRegex = regexp.MustCompile(`!\[\[([^\]\n]+)\]\]`)

// preprocessEmbeds replaces every ![[...]] in src with a markdown fenced
// code block sliced from the referenced source file. Returns the rewritten
// src, the absolute paths of every successfully embedded source file
// (one entry per *distinct* path, deduped), and the synthetic Link entries
// that represent the embeds in the navigable link list.
//
// Failures (missing/binary/oversize/invalid range) render as a one-line
// blockquote warning in place of the embed.
func (r *Renderer) preprocessEmbeds(src, base string) (string, []string, []Link) {
	if !strings.Contains(src, "![[") {
		return src, nil, nil
	}

	var (
		deps     []string
		seen     = map[string]struct{}{}
		links    []Link
	)
	out := embedTokenRegex.ReplaceAllStringFunc(src, func(match string) string {
		body := match[3 : len(match)-2] // strip ![[ and ]]
		em, perr := embed.ParseEmbedToken(body)
		if perr != nil {
			return warningBlock(body, perr.Error())
		}

		absPath := em.Path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(filepath.Dir(base), absPath)
		}
		absPath, _ = filepath.Abs(absPath)

		lines, startLine, serr := embed.SliceFile(absPath, em.Range, em.ContextLines)
		soft := ""
		switch {
		case errors.Is(serr, embed.ErrNotFound):
			return warningBlock(em.Path, "file not found")
		case errors.Is(serr, embed.ErrBinary):
			return warningBlock(em.Path, "binary file, not embedded")
		case errors.Is(serr, embed.ErrTooLarge):
			return warningBlock(em.Path, "file too large to embed")
		case errors.Is(serr, embed.ErrInvalidRange):
			return warningBlock(em.Path, "invalid range")
		case errors.Is(serr, embed.ErrRangePastEOF):
			soft = "file ends at line " + strconv.Itoa(startLine+len(lines)-1)
			// keep going; lines and startLine are populated and valid
		case serr != nil:
			return warningBlock(em.Path, serr.Error())
		}

		displayRange := embedDisplayRange(em)
		leadCtx, tailCtx := 0, 0
		if em.Range != nil && em.ContextLines > 0 {
			leadCtx = em.Range.Start - startLine
			if leadCtx < 0 {
				leadCtx = 0
			}
			tailCtx = (startLine + len(lines) - 1) - em.Range.End
			if tailCtx < 0 {
				tailCtx = 0
			}
		}

		if _, ok := seen[absPath]; !ok {
			seen[absPath] = struct{}{}
			deps = append(deps, absPath)
		}
		l := Link{
			Text: em.Path,
			Href: body,
			Row:  -1,
			Resolved: ResolvedLink{
				Kind:   LinkLocalFile,
				Target: absPath,
			},
		}
		if em.Range != nil {
			l.Resolved.Range = &LineRange{Start: em.Range.Start, End: em.Range.End}
		}
		links = append(links, l)

		return embed.RenderToFence(absPath, lines, startLine, displayRange, leadCtx, tailCtx, soft)
	})
	return out, deps, links
}

// warningBlock formats an embed failure as a one-line blockquote that
// Glamour will style faintly, preserving the surrounding document flow.
func warningBlock(path, reason string) string {
	return "> ⚠ `" + path + "`: " + reason + "\n"
}

// embedDisplayRange formats em.Range for the provenance header in the
// fence; matches what the user typed inside the brackets.
func embedDisplayRange(em *embed.Embed) string {
	if em.Range == nil {
		return "whole file"
	}
	if em.Range.Start == em.Range.End {
		return strconv.Itoa(em.Range.Start)
	}
	return strconv.Itoa(em.Range.Start) + "–" + strconv.Itoa(em.Range.End)
}
```

Update imports in `internal/markdown/links_render.go`:

```go
import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/wilkes/hypogeum/internal/embed"
	"github.com/wilkes/hypogeum/internal/wikilink"
)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/markdown/...`
Expected: PASS.

Note: any other call sites of `RenderWithLinks` (e.g. in `internal/tui/content.go`) will break compilation. Fix them in this same step:

Run: `go build ./...`
Expected: FAIL — `internal/tui/content.go:112` "assignment mismatch".

Apply this temporary edit in `internal/tui/content.go` (the proper handling lands in Task 9; for now we must keep the build green):

```go
	out, links, _, err := m.content.renderer.RenderWithLinks(string(src), path, linkZoneMarker)
```

Run: `go build ./...`
Expected: PASS.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/markdown/links_render.go internal/markdown/render.go internal/markdown/embed_render_test.go internal/tui/content.go
git commit -m "feat(markdown): preprocess ![[file#L10-L20]] embeds into fenced code blocks"
```

---

## Task 7: `internal/embed` aliases `markdown.LineRange`

**Files:**
- Modify: `internal/embed/parse.go`

Now that `markdown.LineRange` exists, fold `embed.LineRange` into an alias so downstream consumers see one type. We did the brand-new package's tests against its own type in Task 1, so this is a behavior-free refactor; the tests will pass unchanged because Go type aliases are structurally identical.

We considered the reverse direction (`markdown.LineRange = embed.LineRange`). That would create an import cycle the moment `embed` depended on anything in `markdown`. The alias goes from embed → markdown because markdown is the older, more central package.

- [ ] **Step 1: Modify `internal/embed/parse.go`**

Replace the existing `LineRange` declaration with an alias:

```go
import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/wilkes/hypogeum/internal/markdown"
)

// LineRange is re-exported from internal/markdown so consumers (TUI,
// markdown's own ResolvedLink, etc.) can share one type without an
// import cycle.
type LineRange = markdown.LineRange
```

- [ ] **Step 2: Run tests to verify everything still passes**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/embed/parse.go
git commit -m "refactor(embed): alias markdown.LineRange to unify the type"
```

---

## Task 8: `code.Renderer` range highlight

**Files:**
- Modify: `internal/code/render.go`
- Modify: `internal/code/gutter.go`
- Modify: `internal/code/render_test.go`

Add `RenderOptions{Highlight *markdown.LineRange}`. The gutter renders reverse-video for source lines whose line number falls in the highlight range. Existing `Render(path, src)` becomes a thin wrapper passing zero options.

- [ ] **Step 1: Write the failing test**

Append to `internal/code/render_test.go`:

```go
import (
	// add to existing imports:
	"github.com/wilkes/hypogeum/internal/markdown"
)

func TestRender_HighlightReverseVideosGutter(t *testing.T) {
	src := []byte("line1\nline2\nline3\nline4\n")
	r := NewRenderer(80)
	out, err := r.RenderOpts("plain.txt", src, RenderOptions{
		Highlight: &markdown.LineRange{Start: 2, End: 3},
	})
	if err != nil {
		t.Fatalf("RenderOpts: %v", err)
	}
	// Line 1's gutter should not contain reverse-video SGR (\x1b[7m).
	// Lines 2 and 3 should.
	// Line 4 should not.
	lines := strings.Split(out, "\n")
	if len(lines) < 4 {
		t.Fatalf("got %d lines:\n%s", len(lines), out)
	}
	contains := func(s, sub string) bool { return strings.Contains(s, sub) }
	if contains(lines[0], "\x1b[7m") {
		t.Errorf("line 1 should not have reverse-video gutter: %q", lines[0])
	}
	if !contains(lines[1], "\x1b[7m") {
		t.Errorf("line 2 should have reverse-video gutter: %q", lines[1])
	}
	if !contains(lines[2], "\x1b[7m") {
		t.Errorf("line 3 should have reverse-video gutter: %q", lines[2])
	}
	if contains(lines[3], "\x1b[7m") {
		t.Errorf("line 4 should not have reverse-video gutter: %q", lines[3])
	}
}

func TestRender_NoHighlightIsUnchanged(t *testing.T) {
	src := []byte("a\nb\n")
	r := NewRenderer(80)
	got, err := r.RenderOpts("plain.txt", src, RenderOptions{})
	if err != nil {
		t.Fatalf("RenderOpts: %v", err)
	}
	want, err := r.Render("plain.txt", src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != want {
		t.Fatalf("RenderOpts({}) differs from Render():\n got: %q\nwant: %q", got, want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/code/ -run TestRender_Highlight -v`
Expected: FAIL — undefined `RenderOpts` and `RenderOptions`.

- [ ] **Step 3: Modify `internal/code/render.go`**

```go
// RenderOptions tunes Render's output. The zero value matches Render's
// pre-options behavior.
type RenderOptions struct {
	// Highlight, when non-nil, marks the line-number gutter for source
	// lines in [Start, End] in reverse-video so the eye can find the
	// referenced range. Outside the range, gutter rendering is unchanged.
	Highlight *markdown.LineRange
}

// Render keeps the old single-arg signature for callers that don't need
// any options. It is equivalent to RenderOpts(path, src, RenderOptions{}).
func (r *Renderer) Render(path string, src []byte) (string, error) {
	return r.RenderOpts(path, src, RenderOptions{})
}

// RenderOpts is Render with explicit options.
func (r *Renderer) RenderOpts(path string, src []byte, opts RenderOptions) (string, error) {
	const maxSize = 5 * 1024 * 1024
	if len(src) > maxSize {
		return "file too large to display", nil
	}
	if looksBinary(src) {
		return "binary file, not displayed", nil
	}

	lexer := lexers.Match(filepath.Base(path))
	if lexer == nil {
		lexer = lexers.Analyse(string(src))
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}

	iterator, err := lexer.Tokenise(nil, string(src))
	if err != nil {
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
	return addGutter(buf.String(), r.width, opts.Highlight), nil
}
```

Add the import:

```go
import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/wilkes/hypogeum/internal/markdown"
)
```

- [ ] **Step 4: Modify `internal/code/gutter.go`**

Change `addGutter`'s signature and behavior:

```go
func addGutter(formatted string, contentWidth int, highlight *markdown.LineRange) string {
	// ... existing body until formatLineNumber is called ...
}
```

In the body, replace the `formatLineNumber` call with a version that respects the highlight:

```go
		for i, sub := range rows {
			if i == 0 {
				b.WriteString(formatLineNumberFor(lineNum, gutterWidth, highlight))
			} else {
				b.WriteString(blankGutterFor(gutterWidth, lineNum, highlight))
			}
			// ... rest unchanged ...
```

Replace `formatLineNumber` / `blankGutter` at the bottom of the file with:

```go
func formatLineNumber(n, w int) string {
	return formatLineNumberFor(n, w, nil)
}

func blankGutter(w int) string {
	return blankGutterFor(w, 0, nil)
}

func formatLineNumberFor(n, w int, hi *markdown.LineRange) string {
	s := strconv.Itoa(n)
	pad := w - len(s)
	if pad < 0 {
		pad = 0
	}
	if inRange(n, hi) {
		// Reverse-video the whole gutter cell (padding + number + separator
		// space) so the band reads as a continuous bar. Reset at the end
		// so the source body that follows is not reverse-video.
		return "\x1b[7m" + strings.Repeat(" ", pad) + s + " \x1b[27m"
	}
	return "\x1b[2m" + strings.Repeat(" ", pad) + s + "\x1b[0m "
}

func blankGutterFor(w, sourceLine int, hi *markdown.LineRange) string {
	if inRange(sourceLine, hi) {
		return "\x1b[7m" + strings.Repeat(" ", w+1) + "\x1b[27m"
	}
	return strings.Repeat(" ", w+1)
}

func inRange(n int, hi *markdown.LineRange) bool {
	if hi == nil {
		return false
	}
	return n >= hi.Start && n <= hi.End
}
```

Add the import:

```go
import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/wilkes/hypogeum/internal/markdown"
)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/code/...`
Expected: PASS.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/code/render.go internal/code/gutter.go internal/code/render_test.go
git commit -m "feat(code): RenderOpts with line-range highlight in the gutter"
```

---

## Task 9: TUI — embed-dependency tracking and live-sync

**Files:**
- Modify: `internal/tui/content.go`
- Modify: `internal/tui/model_test.go`

The TUI consumes the new `embedDeps` slice from `RenderWithLinks`, persists it on `contentUIState`, and consults it in `handleFSEvent`'s `FileModified` branch. We also add the temporary `_` from Task 6 into the real assignment.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/model_test.go`:

```go
func TestModel_EmbedDepsPopulatedOnOpen(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	mdPath := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(mdPath, []byte("# notes\n\n![[main.go#L1-L2]]\n"), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}

	m, err := New(dir, mdPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := m.content.embedDeps[src]; !ok {
		t.Fatalf("embedDeps missing %q; got %v", src, m.content.embedDeps)
	}
}

func TestModel_FileModifiedOnEmbedDepRefreshesOpenMarkdown(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	mdPath := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(mdPath, []byte("![[main.go#L1-L2]]\n"), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}
	m, err := New(dir, mdPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Modify the embedded source on disk, then fire a synthetic FS event.
	if err := os.WriteFile(src, []byte("X\nY\nZ\n"), 0o644); err != nil {
		t.Fatalf("rewrite src: %v", err)
	}
	m.handleFSEvent(watch.Event{Kind: watch.FileModified, Paths: []string{src}})

	if !strings.Contains(m.content.viewport.View(), "X") {
		t.Fatalf("expected re-rendered content to contain new source line; viewport:\n%s",
			m.content.viewport.View())
	}
}
```

Required imports added/extended at top of file:

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wilkes/hypogeum/internal/watch"
)
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_EmbedDeps -v`
Expected: FAIL — `m.content.embedDeps` undefined.

- [ ] **Step 3: Modify `internal/tui/content.go`**

Add `embedDeps` to `contentUIState`:

```go
type contentUIState struct {
	viewport     viewport.Model
	renderer     *markdown.Renderer
	codeRenderer *code.Renderer
	links        []markdown.Link
	linkCursor   int
	// embedDeps holds the absolute paths of every source file embedded
	// in the currently displayed markdown. The TUI's handleFSEvent
	// FileModified branch re-renders the open file when a watcher event
	// arrives for any of these paths.
	embedDeps map[string]struct{}
}
```

Replace the markdown-render block in `refreshContent`:

```go
	m.content.renderer.SetFromFile(path)
	out, links, deps, err := m.content.renderer.RenderWithLinks(string(src), path, linkZoneMarker)
	if err != nil {
		m.status = err.Error()
		m.content.viewport.SetContent(fmt.Sprintf("Error: %v", err))
		m.content.links = nil
		m.content.linkCursor = -1
		m.content.embedDeps = nil
		return
	}
	m.status = path
	m.content.viewport.SetContent(out)
	m.content.viewport.GotoTop()
	m.content.links = links

	// Rebuild the embed-dependency set and ensure the watcher covers any
	// out-of-tree parent directories. AddPath is idempotent.
	m.content.embedDeps = make(map[string]struct{}, len(deps))
	for _, p := range deps {
		m.content.embedDeps[p] = struct{}{}
		if m.watcher != nil {
			_ = m.watcher.AddPath(filepath.Dir(p))
		}
	}
```

And in the *code-render* branch (non-markdown), clear `embedDeps`:

```go
	if !tree.IsMarkdown(path) {
		out, rerr := m.content.codeRenderer.Render(path, src)
		if rerr != nil {
			// ... existing error handling ...
		} else {
			// ... existing success handling ...
		}
		m.content.links = nil
		m.content.linkCursor = -1
		m.content.embedDeps = nil
		_ = target
		return
	}
```

Modify `handleFSEvent` in the same file. Replace the `FileModified` branch:

```go
	case watch.FileModified:
		if m.vault != nil {
			for _, p := range ev.Paths {
				if err := m.vault.RefreshFile(p); err != nil {
					m.diag.Warn("vault refresh failed: " + err.Error())
				}
			}
		}
		cur := m.history.Current()
		if cur == "" {
			return
		}
		for _, p := range ev.Paths {
			matched := p == cur
			if !matched {
				if _, ok := m.content.embedDeps[p]; ok {
					matched = true
				}
			}
			if matched {
				offset := m.content.viewport.YOffset
				m.refreshContent(cur)
				m.content.viewport.SetYOffset(offset)
				return
			}
		}
	}
```

Add the `filepath` import to the existing import block at the top of `content.go`:

```go
import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/viewport"
	zone "github.com/lrstanley/bubblezone"

	"github.com/wilkes/hypogeum/internal/code"
	"github.com/wilkes/hypogeum/internal/markdown"
	"github.com/wilkes/hypogeum/internal/tree"
	"github.com/wilkes/hypogeum/internal/watch"
)
```

- [ ] **Step 4: Add `Watcher.AddPath` in `internal/watch/watch.go`**

```go
// AddPath adds dir to the underlying fsnotify watcher so writes inside
// dir surface as Events. Idempotent and nil-safe — callers can invoke
// it freely from per-render code paths. Used by the TUI to extend the
// watch set to source files referenced by ![[...]] embeds living
// outside the markdown root.
func (w *Watcher) AddPath(dir string) error {
	if w == nil || w.fsw == nil {
		return nil
	}
	return w.fsw.Add(dir)
}
```

Add a smoke test in `internal/watch/watch_test.go`:

```go
func TestWatcher_AddPathIdempotent(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	extra := t.TempDir()
	if err := w.AddPath(extra); err != nil {
		t.Fatalf("AddPath: %v", err)
	}
	if err := w.AddPath(extra); err != nil { // second call is a no-op
		t.Fatalf("AddPath (second call): %v", err)
	}
}

func TestWatcher_AddPathNilSafe(t *testing.T) {
	var w *Watcher
	if err := w.AddPath("/tmp"); err != nil {
		t.Fatalf("nil receiver AddPath: %v", err)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/content.go internal/tui/model_test.go internal/watch/watch.go internal/watch/watch_test.go
git commit -m "feat(tui,watch): track embed deps and live-sync on source change"
```

---

## Task 10: TUI — navigate to source on Enter, with range highlight

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/content.go`
- Modify: `internal/tui/input.go`
- Modify: `internal/tui/model_test.go`

When the user presses `Enter` on a link whose `Resolved.Range != nil`, hypogeum opens the target source file and renders it with `code.RenderOptions{Highlight: range}`. After render, the viewport scrolls so the range is ~25% from the top (existing `scrollToLine` helper).

The highlight persists across scrolling and clears on `Esc`, on opening any other file, and on following a different range link.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/model_test.go`:

```go
func TestModel_EnterOnRangeLinkOpensSourceWithHighlight(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("a\nb\nc\nd\ne\nf\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	mdPath := filepath.Join(dir, "notes.md")
	mdBody := "[the parser](main.go#L2-L3)\n"
	if err := os.WriteFile(mdPath, []byte(mdBody), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}

	m, err := New(dir, mdPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Move link cursor onto the range link (the only link in the document).
	m.content.linkCursor = 0
	// Synthesize Enter.
	m.followCurrentLink()

	if m.history.Current() != src {
		t.Fatalf("expected current file = %q, got %q", src, m.history.Current())
	}
	if m.content.rangeHighlight == nil ||
		m.content.rangeHighlight.Start != 2 || m.content.rangeHighlight.End != 3 {
		t.Fatalf("rangeHighlight = %+v", m.content.rangeHighlight)
	}
	// Viewport should be rendered with the highlight SGR present somewhere
	// (the source file is short enough to fit in one screen).
	if !strings.Contains(m.content.viewport.View(), "\x1b[7m") {
		t.Fatalf("expected reverse-video SGR in viewport:\n%s",
			m.content.viewport.View())
	}
}

func TestModel_EscClearsRangeHighlight(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	m, err := New(dir, src)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.content.rangeHighlight = &markdown.LineRange{Start: 1, End: 2}
	m.refreshContent(src)
	if !strings.Contains(m.content.viewport.View(), "\x1b[7m") {
		t.Fatalf("setup: expected reverse-video in viewport")
	}

	// Press Esc.
	m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.content.rangeHighlight != nil {
		t.Fatalf("Esc should clear rangeHighlight; got %+v", m.content.rangeHighlight)
	}
	if strings.Contains(m.content.viewport.View(), "\x1b[7m") {
		t.Fatalf("Esc should have re-rendered without highlight; viewport still has SGR:\n%s",
			m.content.viewport.View())
	}
}
```

Add to imports if missing:

```go
	tea "github.com/charmbracelet/bubbletea"
	"github.com/wilkes/hypogeum/internal/markdown"
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_EnterOnRangeLink -v`
Expected: FAIL — `followCurrentLink`, `m.content.rangeHighlight` undefined.

- [ ] **Step 3: Modify `internal/tui/content.go` — store and apply the highlight**

Add to `contentUIState`:

```go
type contentUIState struct {
	viewport       viewport.Model
	renderer       *markdown.Renderer
	codeRenderer   *code.Renderer
	links          []markdown.Link
	linkCursor     int
	embedDeps      map[string]struct{}
	// rangeHighlight is non-nil when the open file is a non-markdown
	// source viewed via a range-link or embed navigation. It is cleared
	// by Esc, by opening any other file, and by following a different
	// range link.
	rangeHighlight *markdown.LineRange
}
```

In the code-render branch of `refreshContent`, pass the highlight:

```go
	if !tree.IsMarkdown(path) {
		out, rerr := m.content.codeRenderer.RenderOpts(path, src, code.RenderOptions{
			Highlight: m.content.rangeHighlight,
		})
		if rerr != nil {
			m.status = rerr.Error()
			m.content.viewport.SetContent(fmt.Sprintf("Error: %v", rerr))
		} else {
			m.status = path
			m.content.viewport.SetContent(out)
			m.content.viewport.GotoTop()
			if m.content.rangeHighlight != nil {
				m.scrollToLine(m.content.rangeHighlight.Start)
			}
		}
		m.content.links = nil
		m.content.linkCursor = -1
		m.content.embedDeps = nil
		_ = target
		return
	}
```

In the markdown-render branch, clear the highlight on entry (we are not on a source file):

```go
	m.content.rangeHighlight = nil
	m.content.renderer.SetFromFile(path)
	// ... existing code ...
```

- [ ] **Step 4: Modify `internal/tui/input.go` — `followCurrentLink` and Esc**

Add a `followCurrentLink` helper near the existing link-handling code in `internal/tui/input.go`. (The codebase already has follow logic inside `handleContentKey`; this extracts/extends it. Locate the existing Enter handler; the change is: when `link.Resolved.Range != nil` and `link.Resolved.Kind == markdown.LinkLocalFile`, set `m.content.rangeHighlight = link.Resolved.Range` *before* calling `m.navigateTo(link.Resolved.Target)`.)

```go
// followCurrentLink follows whatever is at m.content.linkCursor. Called
// from handleContentKey on Enter and from tests. Handles plain local
// files, range-link local files (sets rangeHighlight before nav), and
// the existing external-URL confirm flow.
func (m *Model) followCurrentLink() {
	if m.content.linkCursor < 0 || m.content.linkCursor >= len(m.content.links) {
		return
	}
	link := m.content.links[m.content.linkCursor]
	switch link.Resolved.Kind {
	case markdown.LinkLocalFile:
		if link.Resolved.Range != nil {
			m.content.rangeHighlight = link.Resolved.Range
		} else {
			m.content.rangeHighlight = nil
		}
		m.navigateTo(link.Resolved.Target)
	case markdown.LinkExternal:
		// existing external-URL handoff flow — unchanged
		m.armExternal(link.Resolved.Target)
	case markdown.LinkAnchor:
		// existing same-doc anchor scroll — unchanged
		m.scrollToAnchor(link.Resolved.Anchor)
	}
}
```

Find the existing Enter-on-link handler (most likely `case key.Matches(msg, m.keys.Follow):` in `handleContentKey`) and replace its body with `m.followCurrentLink()`.

For Esc: locate the existing Esc cascade (clears link cursor selection, closes modals, etc.) and add a step at the top of the cascade that, if the open file is non-markdown and `rangeHighlight != nil`, clears the highlight and re-renders. Sketch:

```go
// Inside the Esc handler, before existing cascade:
if !tree.IsMarkdown(m.history.Current()) && m.content.rangeHighlight != nil {
	m.content.rangeHighlight = nil
	m.refreshContent(m.history.Current())
	return
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/content.go internal/tui/input.go internal/tui/model_test.go
git commit -m "feat(tui): follow #L10-L20 links into source with range highlight"
```

---

## Task 11: Documentation — CLAUDE.md Gotchas + docs index

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/index.md`

- [ ] **Step 1: Add a Gotchas entry to `CLAUDE.md`**

Locate the "Gotchas" section in `CLAUDE.md` (right after "Non-markdown files render via `internal/code`..."). Append:

```markdown
- **Source embeds (`![[file.go#L10-L20]]`) preprocess to fenced code blocks.** `markdown.Renderer.preprocessEmbeds` runs *before* `preprocessWikilinks` in `RenderWithLinks`. It slices the source file with `internal/embed.SliceFile`, formats a fenced code block with a literal-text gutter inside the fence body (no separate gutter pipeline — that's the deliberate Approach-A simplification), and synthesizes one `Link` per embed so `n`/`p`/`Enter` work on them. Line numbers are *literal*: editing the source shifts embeds to whatever the line numbers now point at. Named anchors are out of scope.
- **Embed live-sync uses `m.content.embedDeps`.** `RenderWithLinks` returns the list of absolute source paths sliced into the output; `refreshContent` persists them and calls `m.watcher.AddPath` for each parent directory. `handleFSEvent`'s `FileModified` branch checks `embedDeps` alongside the open path. A markdown file that *removes* an embed still leaves the prior source dir watched until the watcher is destroyed — cheap, churn-free.
- **Range-link Enter sets `m.content.rangeHighlight`** before `navigateTo`. The code renderer reads it via `RenderOptions.Highlight` and reverse-videos the gutter for those lines. Esc clears the highlight (handled at the *top* of the Esc cascade so it fires before link-cursor clear).
```

- [ ] **Step 2: Add a link to the spec and plan from `docs/index.md`**

Add to the docs index near the link-following entry:

```markdown
- [Source embeds and line-range links](superpowers/specs/2026-05-13-source-embeds-design.md) — `![[file.go#L10-L20]]` transclusion + `[t](file.go#L10-L20)` navigation. [Plan](superpowers/plans/2026-05-13-source-embeds.md).
```

- [ ] **Step 3: Run final verification**

```bash
go build ./...
go test ./...
go vet ./...
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md docs/index.md
git commit -m "docs(source-embeds): CLAUDE.md gotchas + docs index entry"
```

---

## Task 12: Open the PR

**Files:** none

- [ ] **Step 1: Push the branch and open a PR**

```bash
git push -u origin source-embeds
gh pr create --title "Source-file embeds and line-range links" --body "$(cat <<'EOF'
## Summary

- `![[main.go#L42-L58]]` renders an inline syntax-highlighted snippet of the source file's lines 42-58, with a faint provenance header and a line-number gutter.
- `[parser](main.go#L42-L58)` is a navigable range link: Enter opens main.go, scrolls to line 42, and highlights lines 42-58 in the source-view gutter.
- Embeds re-render automatically when the embedded source changes on disk (live-sync via per-render embed-dependency set; watcher extended to out-of-tree directories via the new `Watcher.AddPath`).
- New pure package `internal/embed` handles parsing, file slicing, and fence assembly.
- Line numbers are literal (GitHub semantics). Named anchors are out of scope.

Spec: [docs/superpowers/specs/2026-05-13-source-embeds-design.md](docs/superpowers/specs/2026-05-13-source-embeds-design.md)
Plan: [docs/superpowers/plans/2026-05-13-source-embeds.md](docs/superpowers/plans/2026-05-13-source-embeds.md)

## Test plan

- [ ] `go test ./...` passes
- [ ] Open a directory with a markdown file that embeds a `.go` file — embed renders with gutter and provenance header
- [ ] Edit the embedded `.go` file in another editor — embed re-renders in place, viewport scroll preserved
- [ ] Press `Enter` on a `[t](file.go#L10-L20)` range link — opens the source, scrolls to line 10, highlights lines 10-20
- [ ] Press `Esc` on a source file with a highlight — highlight clears, file re-renders without it
- [ ] Embed a missing file (`![[nope.go]]`) — renders as a faint warning blockquote, surrounding doc still renders

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 2: Verify the PR URL is reported**

The `gh pr create` command prints the PR URL to stdout. Save it for the user.

---

## Self-Review

**Spec coverage check — every spec requirement maps to at least one task:**
- Embed grammar (whole file, single line, range, +context): Task 1 (parse) + Task 4 (fence).
- Range-link grammar `[t](path#L10-L20)`: Task 5 (`ResolveLink`).
- Drift semantics (literal line numbers): naturally falls out of Task 2's `SliceFile` always re-reading from disk.
- File slicing with binary/oversize/missing/invalid-range warnings: Task 2 (errors) + Task 6 (warning blocks).
- Fence-body shape with literal gutter: Task 4.
- En-dash separator: Task 4 (`embedDisplayRange` emits `–`).
- Preprocess runs before wikilink pass: Task 6.
- `RenderWithLinks` returns embed deps: Task 6.
- Live-sync via `embedDeps`: Task 9.
- Watcher `AddPath` for out-of-tree sources: Task 9.
- Cycles impossible (embeds are raw, not re-parsed): preserved by design — `preprocessEmbeds` returns markdown source as text, not as a re-rendered fragment.
- Embeds join link cycler: Task 6 synthesizes Links; Task 10's `followCurrentLink` handles their `Range`.
- `Range` on `ResolvedLink`: Task 5.
- `code.RenderOptions.Highlight` reverse-videos gutter: Task 8.
- Esc clears highlight: Task 10.
- Tests live next to code: every task does.

**Placeholder scan:** No "TBD" / "implement later" / "add error handling". All code blocks are complete; sentinel errors are named; regex patterns are written out.

**Type consistency:**
- `LineRange` defined in Task 5 (`internal/markdown`), aliased in Task 7 (`internal/embed`). Both packages use the same underlying type.
- `RenderOpts(path string, src []byte, opts RenderOptions)` consistent between Task 8 definition and Task 10 callsite.
- `embedDeps map[string]struct{}` consistent between Task 9 definition and Task 10 reference.
- `rangeHighlight *markdown.LineRange` consistent between Task 10 definition and the test references.
- `Watcher.AddPath` consistent between Task 9 definition and Task 9 callsite in `refreshContent`.

One real fix found during review: Task 6 mentions calling `_ = m.watcher.AddPath` for out-of-tree dirs but at that point `AddPath` doesn't exist yet (added in Task 9). Solution: move the `AddPath` method declaration up — defined in Task 9 alongside its use. The temporary fix in Task 6 only adjusts the `refreshContent` return-value count; it does *not* call `AddPath`. Task 9 is the one that adds both the method and the call site. Plan now reads consistently.
