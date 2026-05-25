# Block references and heading anchors — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve `#Heading` and `^block-id` anchors in wikilinks and standard inline links to a destination line, scrolling the destination on follow. Anchors that don't resolve render broken and count in the footer tally.

**Architecture:** `internal/vault` gains per-file heading and block-id indexes built during the existing goldmark AST walk. The `Resolver` interface grows a `ResolveAnchor` method. `internal/markdown` strips trailing `^id` markers before rendering and routes anchored wikilinks/standard links through a broken-or-anchored render path. `internal/tui`'s follow paths set `pendingPreselectRange` from `ResolveAnchor`, reusing the existing scroll-to-line plumbing.

**Tech Stack:** Go, goldmark AST, Bubble Tea, existing `pendingPreselectRange` machinery.

**Spec:** [`docs/superpowers/specs/2026-05-25-block-references-design.md`](../specs/2026-05-25-block-references-design.md)

---

## File map

- **Create:** `internal/vault/anchors.go`, `internal/vault/anchors_test.go`
- **Modify:** `internal/vault/vault.go` (fileEntry fields, indexFile call), `internal/vault/extract.go` (return anchors alongside refs, OR call a sibling extractor), `internal/vault/resolver.go` (add ResolveAnchor), `internal/markdown/resolver.go` (interface), `internal/markdown/resolver_test.go` if it exists, `internal/markdown/links_render.go` (preprocess + render anchor handling + CountUnresolvedWikilinks), `internal/markdown/links_render_test.go` (extend), `internal/tui/links.go` (followLink anchor path), `internal/tui/input.go` (wikilink follow), `internal/tui/links_test.go` / `model_test.go` (extend)

---

### Task 1: Add anchor extraction for headings to vault

**Files:**
- Create: `internal/vault/anchors.go`
- Create: `internal/vault/anchors_test.go`
- Modify: `internal/vault/vault.go` (add fields to `fileEntry`)

- [ ] **Step 1: Write the failing test**

Create `internal/vault/anchors_test.go`:

```go
package vault

import (
	"testing"
)

func TestExtractAnchors_Headings(t *testing.T) {
	src := "# Top\n\nIntro paragraph.\n\n## Sub Section\n\nbody\n\n### Deep One\n"
	got := extractAnchors(src)

	want := map[string]int{
		"top":         1,
		"sub-section": 5,
		"deep-one":    9,
	}
	if len(got.headings) != len(want) {
		t.Fatalf("headings len = %d, want %d (%v)", len(got.headings), len(want), got.headings)
	}
	for slug, line := range want {
		if got.headings[slug] != line {
			t.Errorf("headings[%q] = %d, want %d", slug, got.headings[slug], line)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/vault/ -run TestExtractAnchors_Headings -v`
Expected: FAIL with `undefined: extractAnchors`.

- [ ] **Step 3: Implement heading extraction**

Create `internal/vault/anchors.go`:

```go
package vault

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// anchors holds the per-file lookup tables for [[Note#Heading]] and
// [[Note#^block-id]] anchor resolution.
type anchors struct {
	headings map[string]int // slug → 1-based line of the '#' marker
	blocks   map[string]int // id   → 1-based line of the first line of the enclosing block
}

func newAnchors() anchors {
	return anchors{
		headings: map[string]int{},
		blocks:   map[string]int{},
	}
}

// extractAnchors parses src and returns the per-file anchor index.
// Heading slugs follow the same rule used by internal/markdown.slugify
// (kept in sync; see slugifyAnchor below).
func extractAnchors(src string) anchors {
	source := []byte(src)
	md := goldmark.New(goldmark.WithExtensions(WikilinkExtension))
	doc := md.Parser().Parse(text.NewReader(source))
	out := newAnchors()

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line := lineForNode(h, source)
			slug := slugifyAnchor(headingText(h, source))
			if slug != "" {
				if _, dup := out.headings[slug]; !dup {
					out.headings[slug] = line
				}
			}
		}
		return ast.WalkContinue, nil
	})
	return out
}

// headingText returns the visible text under a heading node.
func headingText(n ast.Node, source []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(source))
			continue
		}
		b.WriteString(headingText(c, source))
	}
	return b.String()
}

// slugifyAnchor must stay in sync with internal/markdown.slugify.
// Copied (not imported) to avoid an internal/vault → internal/markdown
// import cycle.
func slugifyAnchor(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/vault/ -run TestExtractAnchors_Headings -v`
Expected: PASS.

- [ ] **Step 5: Run the whole vault package**

Run: `go test ./internal/vault/...`
Expected: PASS (no regressions).

- [ ] **Step 6: Commit**

```bash
git add internal/vault/anchors.go internal/vault/anchors_test.go
git commit -m "feat(vault): extract heading anchors per file"
```

---

### Task 2: Add block-id extraction

**Files:**
- Modify: `internal/vault/anchors.go`
- Modify: `internal/vault/anchors_test.go`

- [ ] **Step 1: Add failing tests for block extraction**

Append to `internal/vault/anchors_test.go`:

```go
func TestExtractAnchors_Blocks(t *testing.T) {
	src := "First paragraph. ^p1\n\n- list item with id ^li\n- second item\n\n> quoted block ^q\n\n```\ncode ^notcounted\n```\n\nLast para. ^last\n"
	got := extractAnchors(src)

	cases := map[string]int{
		"p1":   1,
		"li":   3,
		"q":    6,
		"last": 12,
	}
	for id, line := range cases {
		if got.blocks[id] != line {
			t.Errorf("blocks[%q] = %d, want %d (got=%v)", id, got.blocks[id], line, got.blocks)
		}
	}
	if _, present := got.blocks["notcounted"]; present {
		t.Errorf("block marker inside fenced code should be ignored; got %v", got.blocks)
	}
}

func TestExtractAnchors_DuplicateBlockIDs_FirstWins(t *testing.T) {
	src := "First. ^dup\n\nSecond. ^dup\n"
	got := extractAnchors(src)
	if got.blocks["dup"] != 1 {
		t.Errorf("blocks[dup] = %d, want 1 (first wins)", got.blocks["dup"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/vault/ -run TestExtractAnchors_Blocks -v`
Expected: FAIL — `blocks` is empty.

- [ ] **Step 3: Implement block extraction**

Edit `internal/vault/anchors.go`. Add an import for `regexp`. Inside `extractAnchors`, extend the walk to also visit block-level nodes (paragraph, list item, blockquote — *not* fenced code blocks). Add at the top of the file:

```go
import (
	"regexp"
	// ... existing imports
)

// blockMarkerRegex matches a trailing block-id marker ` ^id` at end of
// text. The id is alphanumerics + hyphens, matching Obsidian's syntax.
var blockMarkerRegex = regexp.MustCompile(`(?:^| )\^([a-zA-Z0-9-]+)\s*$`)
```

Modify the walk function in `extractAnchors`:

```go
_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
    if !entering {
        return ast.WalkContinue, nil
    }
    switch nn := n.(type) {
    case *ast.Heading:
        line := lineForNode(nn, source)
        slug := slugifyAnchor(headingText(nn, source))
        if slug != "" {
            if _, dup := out.headings[slug]; !dup {
                out.headings[slug] = line
            }
        }
    case *ast.Paragraph, *ast.ListItem, *ast.Blockquote:
        if id, ok := trailingBlockID(nn, source); ok {
            line := lineForNode(nn, source)
            if _, dup := out.blocks[id]; !dup {
                out.blocks[id] = line
            }
        }
    case *ast.FencedCodeBlock, *ast.CodeBlock:
        return ast.WalkSkipChildren, nil
    }
    return ast.WalkContinue, nil
})
```

Add the helper at the bottom of `anchors.go`:

```go
// trailingBlockID returns the block-id from a trailing ` ^id` marker on
// the last text segment of block n. Returns ("", false) if no marker.
func trailingBlockID(n ast.Node, source []byte) (string, bool) {
	text := blockText(n, source)
	text = strings.TrimRight(text, " \t\n")
	m := blockMarkerRegex.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// blockText returns the concatenated text content of block n, joined
// across child paragraphs/inlines. Skips children that are themselves
// container blocks (paragraphs inside a blockquote yield the paragraph
// text once, not twice).
func blockText(n ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(source))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/vault/ -run TestExtractAnchors -v`
Expected: PASS.

- [ ] **Step 5: Run the whole package**

Run: `go test ./internal/vault/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/vault/anchors.go internal/vault/anchors_test.go
git commit -m "feat(vault): extract ^block-id markers per file"
```

---

### Task 3: Wire anchors into fileEntry and rebuild lifecycle

**Files:**
- Modify: `internal/vault/vault.go`
- Modify: `internal/vault/extract.go` (call `extractAnchors`)
- Modify: `internal/vault/anchors_test.go` (integration test via Build)

- [ ] **Step 1: Write a failing integration test**

Append to `internal/vault/anchors_test.go`:

```go
import (
	"os"
	"path/filepath"
	// ... existing imports
)

func TestVault_BuildPopulatesAnchors(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "note.md")
	src := "# Top Heading\n\nA paragraph. ^para1\n"
	if err := os.WriteFile(notePath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatal(err)
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	entry := v.files[notePath]
	if entry == nil {
		t.Fatalf("file entry missing for %s", notePath)
	}
	if entry.anchors.headings["top-heading"] != 1 {
		t.Errorf("headings[top-heading] = %d, want 1", entry.anchors.headings["top-heading"])
	}
	if entry.anchors.blocks["para1"] != 3 {
		t.Errorf("blocks[para1] = %d, want 3", entry.anchors.blocks["para1"])
	}
}
```

- [ ] **Step 2: Run, expect failure**

Run: `go test ./internal/vault/ -run TestVault_BuildPopulatesAnchors -v`
Expected: FAIL — `entry.anchors` undefined.

- [ ] **Step 3: Add anchors field to fileEntry**

Modify `internal/vault/vault.go`. Update the `fileEntry` struct:

```go
type fileEntry struct {
	path    string
	refs    []reference
	anchors anchors
}
```

- [ ] **Step 4: Populate anchors during indexing**

Find the call to `extractReferences` in `internal/vault/extract.go` or wherever `indexFile` constructs a `fileEntry`. The vault calls `indexFile` from `walkAndIndex` and `RefreshFile`. Locate it:

```bash
grep -n "extractReferences\|indexFile" internal/vault/*.go
```

In the file that owns `indexFile` (likely `vault.go` or a sibling), where the `fileEntry` is built, add:

```go
entry := &fileEntry{
    path:    abs,
    refs:    extractReferences(string(src), abs),
    anchors: extractAnchors(string(src)),
}
```

(Adapt to existing structure — preserve the existing fields and assignment style.)

- [ ] **Step 5: Run integration test**

Run: `go test ./internal/vault/ -run TestVault_BuildPopulatesAnchors -v`
Expected: PASS.

- [ ] **Step 6: Run full vault suite**

Run: `go test ./internal/vault/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/vault/vault.go internal/vault/extract.go internal/vault/anchors_test.go
git commit -m "feat(vault): index anchors during Build and RefreshFile"
```

---

### Task 4: Add `ResolveAnchor` to the vault and the Resolver interface

**Files:**
- Modify: `internal/markdown/resolver.go`
- Modify: `internal/vault/resolver.go`
- Modify: `internal/vault/resolver_test.go` (or create — check if it exists)

- [ ] **Step 1: Failing test for `ResolveAnchor`**

Add to `internal/vault/anchors_test.go`:

```go
func TestVault_ResolveAnchor(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "note.md")
	src := "# A Heading\n\nBody paragraph. ^bx\n"
	if err := os.WriteFile(notePath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatal(err)
	}

	// Heading lookup.
	if line, ok := v.ResolveAnchor(notePath, "A Heading", ""); !ok || line != 1 {
		t.Errorf("ResolveAnchor heading: got (%d, %v), want (1, true)", line, ok)
	}
	// Block lookup.
	if line, ok := v.ResolveAnchor(notePath, "", "bx"); !ok || line != 3 {
		t.Errorf("ResolveAnchor block: got (%d, %v), want (3, true)", line, ok)
	}
	// Block wins when both supplied.
	if line, ok := v.ResolveAnchor(notePath, "A Heading", "bx"); !ok || line != 3 {
		t.Errorf("ResolveAnchor both: got (%d, %v), want (3, true)", line, ok)
	}
	// Missing block.
	if _, ok := v.ResolveAnchor(notePath, "", "nope"); ok {
		t.Error("ResolveAnchor missing block: ok=true, want false")
	}
	// Missing file.
	if _, ok := v.ResolveAnchor("/nonexistent.md", "A Heading", ""); ok {
		t.Error("ResolveAnchor missing file: ok=true, want false")
	}
}
```

- [ ] **Step 2: Run, expect failure**

Run: `go test ./internal/vault/ -run TestVault_ResolveAnchor -v`
Expected: FAIL — `v.ResolveAnchor` undefined.

- [ ] **Step 3: Add the method to vault**

Append to `internal/vault/resolver.go`:

```go
// ResolveAnchor looks up the destination line for a heading or block
// anchor inside the file at path. Both args empty returns (0, false).
// When both heading and block are non-empty, block wins (it's more
// specific — Obsidian's `#Heading^block` syntax).
func (v *Vault) ResolveAnchor(path, heading, block string) (int, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	entry := v.files[path]
	if entry == nil {
		return 0, false
	}
	if block != "" {
		line, ok := entry.anchors.blocks[block]
		return line, ok
	}
	if heading != "" {
		line, ok := entry.anchors.headings[slugifyAnchor(heading)]
		return line, ok
	}
	return 0, false
}
```

- [ ] **Step 4: Update the Resolver interface**

Edit `internal/markdown/resolver.go`:

```go
type Resolver interface {
	Resolve(fromFile, name, heading, block string) (path string, ok bool)
	ResolveAnchor(path, heading, block string) (line int, ok bool)
}
```

Update `nopResolver`:

```go
func (nopResolver) Resolve(string, string, string, string) (string, bool) {
	return "", false
}

func (nopResolver) ResolveAnchor(string, string, string) (int, bool) {
	return 0, false
}
```

- [ ] **Step 5: Update test fakes**

Find every fake implementing Resolver:

```bash
grep -rn "Resolve(fromFile" internal/markdown/ internal/tui/ | grep -v "_test.go.*resolver.go"
```

For each test fake, add a stub `ResolveAnchor` returning `(0, false)` unless the test specifically needs anchor lookups (none should yet — added in later tasks).

Example:

```go
type fakeResolver struct{ /* existing */ }

func (fakeResolver) ResolveAnchor(string, string, string) (int, bool) {
	return 0, false
}
```

- [ ] **Step 6: Build and test**

Run: `go build ./...`
Expected: PASS — every Resolver implementation now satisfies the new method.

Run: `go test ./internal/vault/ -run TestVault_ResolveAnchor -v`
Expected: PASS.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/markdown/resolver.go internal/vault/resolver.go internal/vault/anchors_test.go internal/markdown/*_test.go internal/tui/*_test.go
git commit -m "feat(vault): add ResolveAnchor for heading and block lookup"
```

---

### Task 5: Strip block-id markers before rendering

**Files:**
- Modify: `internal/markdown/links_render.go` (add `preprocessBlockMarkers`)
- Modify: `internal/markdown/renderer.go` or wherever `RenderWithLinks` runs preprocesses
- Modify: `internal/markdown/links_render_test.go`

- [ ] **Step 1: Find where preprocess passes are invoked**

Run: `grep -n "preprocessEmbeds\|preprocessWikilinks" internal/markdown/*.go`

Expected: a `RenderWithLinks` method invokes them in order. Identify the file and line.

- [ ] **Step 2: Write failing test**

Add to `internal/markdown/links_render_test.go`:

```go
func TestPreprocessBlockMarkers_StripsOutsideFences(t *testing.T) {
	src := "First paragraph. ^p1\n\n```\nin code ^kept\n```\n\nSecond. ^p2\n"
	got := preprocessBlockMarkers(src)
	want := "First paragraph.\n\n```\nin code ^kept\n```\n\nSecond.\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}
```

- [ ] **Step 3: Run, expect failure**

Run: `go test ./internal/markdown/ -run TestPreprocessBlockMarkers -v`
Expected: FAIL — undefined.

- [ ] **Step 4: Implement**

Add to `internal/markdown/links_render.go`:

```go
// blockMarkerLineRegex matches a trailing ` ^id` marker at end of line.
// Block-id grammar matches the vault's extractor.
var blockMarkerLineRegex = regexp.MustCompile(` \^[a-zA-Z0-9-]+\s*$`)

// preprocessBlockMarkers strips trailing ` ^id` block-id markers from
// every non-fence line in src. Markers inside fenced code blocks are
// left intact (consistent with preprocessEmbeds / preprocessWikilinks).
func preprocessBlockMarkers(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for _, seg := range splitOutsideFences(src) {
		if seg.isFence {
			b.WriteString(seg.text)
			continue
		}
		lines := strings.Split(seg.text, "\n")
		for i, line := range lines {
			lines[i] = blockMarkerLineRegex.ReplaceAllString(line, "")
		}
		b.WriteString(strings.Join(lines, "\n"))
	}
	return b.String()
}
```

- [ ] **Step 5: Wire into RenderWithLinks**

In the file identified in Step 1, insert `preprocessBlockMarkers` *before* `preprocessEmbeds`:

```go
src = r.preprocessBlockMarkers(src)
src = r.preprocessEmbeds(src, fromFile)
src = r.preprocessWikilinks(src)
```

(If `preprocessBlockMarkers` doesn't need `r`, drop the receiver — keep it package-level.)

- [ ] **Step 6: Run tests**

Run: `go test ./internal/markdown/ -run TestPreprocessBlockMarkers -v`
Expected: PASS.

Run: `go test ./internal/markdown/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/markdown/links_render.go internal/markdown/links_render_test.go
git commit -m "feat(markdown): strip ^block-id markers before rendering"
```

---

### Task 6: Carry block anchors through wikilink rewrite + render broken anchors

**Files:**
- Modify: `internal/markdown/links_render.go`
- Modify: `internal/markdown/links_render_test.go`

- [ ] **Step 1: Write failing tests for wikilink anchor rewrite + broken anchor**

Add to `internal/markdown/links_render_test.go`:

```go
type anchorResolver struct {
	pathByName map[string]string
	lines      map[string]map[string]int // path → anchor key → line; key="^id" or "#slug"
}

func (a anchorResolver) Resolve(_, name, _, _ string) (string, bool) {
	p, ok := a.pathByName[strings.ToLower(name)]
	return p, ok
}

func (a anchorResolver) ResolveAnchor(path, heading, block string) (int, bool) {
	m, ok := a.lines[path]
	if !ok {
		return 0, false
	}
	if block != "" {
		l, ok := m["^"+block]
		return l, ok
	}
	if heading != "" {
		l, ok := m["#"+slugify(heading)]
		return l, ok
	}
	return 0, false
}

func TestPreprocessWikilinks_BlockAnchorPreserved(t *testing.T) {
	r := newTestRenderer(t)
	r.SetResolver(anchorResolver{
		pathByName: map[string]string{"note": "/notes/note.md"},
		lines:      map[string]map[string]int{"/notes/note.md": {"^foo": 7}},
	})
	got := r.preprocessWikilinks("See [[Note#^foo]]")
	if !strings.Contains(got, "/notes/note.md#^foo") {
		t.Errorf("expected block anchor in href; got %q", got)
	}
}

func TestPreprocessWikilinks_BrokenAnchor_RendersBroken(t *testing.T) {
	r := newTestRenderer(t)
	r.SetResolver(anchorResolver{
		pathByName: map[string]string{"note": "/notes/note.md"},
		lines:      map[string]map[string]int{"/notes/note.md": {}}, // no anchors
	})
	got := r.preprocessWikilinks("See [[Note#missing]]")
	if !strings.Contains(got, "?") {
		t.Errorf("expected broken marker '?' in output; got %q", got)
	}
	// Sanity: file exists, but anchor doesn't → still broken.
}
```

(`newTestRenderer` already exists in the test file. If not, see other tests in `links_render_test.go` for the construction pattern and reuse it.)

- [ ] **Step 2: Run, expect failures**

Run: `go test ./internal/markdown/ -run TestPreprocessWikilinks_BlockAnchorPreserved -v`
Run: `go test ./internal/markdown/ -run TestPreprocessWikilinks_BrokenAnchor -v`
Expected: FAIL.

- [ ] **Step 3: Update `preprocessWikilinks`**

In `internal/markdown/links_render.go`, modify the `replace` function in `preprocessWikilinks`:

```go
replace := func(match string) string {
    body := match[2 : len(match)-2]
    w := wikilink.Parse(body)
    if w == nil {
        return match
    }
    display := w.Alias
    if display == "" {
        display = w.Name
        if w.Heading != "" {
            display = w.Name + " > " + w.Heading
        } else if w.Block != "" {
            display = w.Name + " > ^" + w.Block
        }
    }
    path, ok := r.resolver.Resolve(r.fromFile, w.Name, w.Heading, w.Block)
    if !ok {
        return display + "?"
    }
    // If an anchor is specified but doesn't resolve, render broken.
    if w.Heading != "" || w.Block != "" {
        if _, anchorOK := r.resolver.ResolveAnchor(path, w.Heading, w.Block); !anchorOK {
            return display + "?"
        }
    }
    href := path
    switch {
    case w.Block != "":
        href = path + "#^" + w.Block
    case w.Heading != "":
        href = path + "#" + slugify(w.Heading)
    }
    return "[" + display + "](" + href + ")"
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/markdown/ -run TestPreprocessWikilinks -v`
Expected: PASS.

Run: `go test ./internal/markdown/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/markdown/links_render.go internal/markdown/links_render_test.go
git commit -m "feat(markdown): carry block anchors through wikilinks; broken anchors render broken"
```

---

### Task 7: Include broken anchors in the footer broken-link tally

**Files:**
- Modify: `internal/markdown/links_render.go` (`CountUnresolvedWikilinks`)
- Modify: `internal/markdown/links_render_test.go`

- [ ] **Step 1: Failing test**

Add to `internal/markdown/links_render_test.go`:

```go
func TestCountUnresolvedWikilinks_BrokenAnchorCounts(t *testing.T) {
	r := newTestRenderer(t)
	r.SetResolver(anchorResolver{
		pathByName: map[string]string{"note": "/notes/note.md"},
		lines:      map[string]map[string]int{"/notes/note.md": {"^foo": 7}},
	})
	src := "[[Note#^foo]] and [[Note#^missing]] and [[Note#unknown-heading]]"
	if got := r.CountUnresolvedWikilinks(src); got != 2 {
		t.Errorf("CountUnresolvedWikilinks = %d, want 2", got)
	}
}
```

- [ ] **Step 2: Run, expect failure**

Run: `go test ./internal/markdown/ -run TestCountUnresolvedWikilinks_BrokenAnchorCounts -v`
Expected: FAIL (likely got 0 — current impl only flags missing files).

- [ ] **Step 3: Update `CountUnresolvedWikilinks`**

In `internal/markdown/links_render.go`, update the `check` function:

```go
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
    path, ok := r.resolver.Resolve(r.fromFile, w.Name, w.Heading, w.Block)
    if !ok {
        count++
        return match
    }
    if w.Heading != "" || w.Block != "" {
        if _, aok := r.resolver.ResolveAnchor(path, w.Heading, w.Block); !aok {
            count++
        }
    }
    return match
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/markdown/ -run TestCountUnresolvedWikilinks -v`
Expected: PASS.

Run: `go test ./internal/markdown/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/markdown/links_render.go internal/markdown/links_render_test.go
git commit -m "feat(markdown): broken anchors count toward unresolved-wikilinks tally"
```

---

### Task 8: TUI follows anchored links by scrolling to the anchor's line

**Files:**
- Modify: `internal/tui/links.go` (`followLink`)
- Modify: `internal/tui/model.go` (Model needs access to vault for anchor lookup — likely already there)
- Modify: `internal/tui/links_test.go` or `model_test.go`

- [ ] **Step 1: Confirm the Model holds a vault reference**

Run: `grep -n "vault" internal/tui/model.go | head -10`

Expected: `m.vault` (or similar) of type `*vault.Vault`. If absent, this Step requires plumbing; check the backlinks modal code (`internal/tui/backlinks.go`) — it already uses the vault, so the field exists. Confirm the field name; the plan below assumes `m.vault`.

- [ ] **Step 2: Write failing TUI test**

Append to `internal/tui/model_test.go` (or `links_test.go`, whichever already houses link-follow tests — `grep -l "followLink\|followCurrentLink" internal/tui/*_test.go`):

```go
func TestFollowLink_HeadingAnchor_SetsPreselectRange(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	src := "# Intro\n\nbody\n\n## Deep Dive\n\nmore\n"
	if err := os.WriteFile(target, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.md")
	if err := os.WriteFile(source, []byte("[link](target.md#deep-dive)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestModelOn(t, dir, source) // helper used by existing tests
	// Position the link cursor at the only link.
	m.cycleLink(1)
	m.followCurrentLink()

	cur := m.history.Current()
	if cur != target {
		t.Fatalf("history.Current() = %s, want %s", cur, target)
	}
	// After navigation, pendingPreselectRange should have been consumed;
	// the rangeHighlight on content reflects it.
	if m.content.rangeHighlight == nil || m.content.rangeHighlight.Start != 5 {
		t.Errorf("expected rangeHighlight at line 5 (the ## Deep Dive line); got %+v", m.content.rangeHighlight)
	}
}
```

(Adapt the helper name and Model field access to match what other tests in the file already use.)

- [ ] **Step 3: Run, expect failure**

Run: `go test ./internal/tui/ -run TestFollowLink_HeadingAnchor -v`
Expected: FAIL — anchor currently produces "anchor navigation not implemented" status.

- [ ] **Step 4: Update `followLink`**

Edit `internal/tui/links.go`. Replace the `case markdown.LinkLocalFile` block and remove `LinkAnchor`'s no-op status:

```go
func (m *Model) followLink(l markdown.Link) {
	switch l.Resolved.Kind {
	case markdown.LinkLocalFile:
		switch {
		case l.Resolved.Range != nil:
			m.content.rangeHighlight = l.Resolved.Range
		case l.Resolved.Anchor != "":
			heading, block := splitAnchor(l.Resolved.Anchor)
			if line, ok := m.vault.ResolveAnchor(l.Resolved.Target, heading, block); ok {
				m.pendingPreselectRange = &markdown.LineRange{Start: line, End: line}
			}
			m.content.rangeHighlight = nil
		default:
			m.content.rangeHighlight = nil
		}
		m.navigateTo(l.Resolved.Target)
	case markdown.LinkExternal:
		m.pendingExternal = l.Href
		m.status = "press Enter again to open: " + l.Href
	case markdown.LinkAnchor:
		// Same-document anchor — still unimplemented (out of spec scope).
		m.status = "same-document anchor not supported: #" + l.Resolved.Anchor
	default:
		m.status = "unrecognized link: " + l.Href
	}
}

// splitAnchor splits a URL fragment into (heading, block). A leading '^'
// means block-id; otherwise the fragment is a heading slug.
func splitAnchor(anchor string) (heading, block string) {
	if strings.HasPrefix(anchor, "^") {
		return "", strings.TrimPrefix(anchor, "^")
	}
	return anchor, ""
}
```

Add `"strings"` to the imports if missing.

Guard against `m.vault == nil` (the watcher / vault is best-effort):

```go
case l.Resolved.Anchor != "":
    if m.vault != nil {
        heading, block := splitAnchor(l.Resolved.Anchor)
        if line, ok := m.vault.ResolveAnchor(l.Resolved.Target, heading, block); ok {
            m.pendingPreselectRange = &markdown.LineRange{Start: line, End: line}
        }
    }
    m.content.rangeHighlight = nil
```

**Note on heading slug:** `l.Resolved.Anchor` is whatever the URL fragment was — for the wikilink path it's already a slug (e.g. `deep-dive`), and `vault.ResolveAnchor` re-slugifies via `slugifyAnchor(heading)`. A pre-slugified heading like `deep-dive` round-trips through `slugifyAnchor` unchanged (it's idempotent). For plain markdown `[text](foo.md#Heading Name)`, callers typically already slug; either way the re-slug is safe.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tui/ -run TestFollowLink_HeadingAnchor -v`
Expected: PASS.

Run: `go test ./internal/tui/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/links.go internal/tui/*_test.go
git commit -m "feat(tui): follow #heading and #^block anchors with scroll-to-line"
```

---

### Task 9: Test block-anchor follow end-to-end

**Files:**
- Modify: `internal/tui/model_test.go` (or `links_test.go`)

- [ ] **Step 1: Failing test**

Add:

```go
func TestFollowLink_BlockAnchor_SetsPreselectRange(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	src := "Intro paragraph.\n\nMiddle paragraph. ^mid\n\nAnother.\n"
	if err := os.WriteFile(target, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.md")
	if err := os.WriteFile(source, []byte("See [[target#^mid]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestModelOn(t, dir, source)
	m.cycleLink(1)
	m.followCurrentLink()

	if m.history.Current() != target {
		t.Fatalf("Current = %s, want %s", m.history.Current(), target)
	}
	if m.content.rangeHighlight == nil || m.content.rangeHighlight.Start != 3 {
		t.Errorf("expected rangeHighlight at line 3, got %+v", m.content.rangeHighlight)
	}
}

func TestFollowLink_BrokenAnchor_StillNavigates(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	if err := os.WriteFile(target, []byte("just text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.md")
	if err := os.WriteFile(source, []byte("See [[target#^missing]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestModelOn(t, dir, source)
	// Broken anchors render as broken via `?` suffix — so they're
	// excluded from the link cursor (matching unresolved wikilinks).
	// Just assert the renderer counted it broken.
	if got := m.content.brokenCount; got != 1 {
		t.Errorf("brokenCount = %d, want 1", got)
	}
}
```

(If `m.content.brokenCount` is a different field name, adjust to whatever `internal/tui/content.go` uses for the footer tally.)

- [ ] **Step 2: Run, expect failure or pass**

Run: `go test ./internal/tui/ -run TestFollowLink_BlockAnchor -v`
Expected: PASS (Tasks 6 + 8 already implemented the behavior).

Run: `go test ./internal/tui/ -run TestFollowLink_BrokenAnchor -v`
Expected: PASS.

If a test fails, investigate — likely a small interface gap.

- [ ] **Step 3: Run full suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/*_test.go
git commit -m "test(tui): cover block-anchor follow and broken-anchor render"
```

---

### Task 10: Duplicate block-id diagnostic

**Files:**
- Modify: `internal/vault/anchors.go` (`extractAnchors` accepts diag sink)
- Modify: callers of `extractAnchors` to pass the vault's diag
- Modify: `internal/vault/anchors_test.go`

- [ ] **Step 1: Failing test**

Add:

```go
type captureDiag struct {
	warns []string
}

func (c *captureDiag) Warn(s string)  { c.warns = append(c.warns, s) }
func (c *captureDiag) Info(s string)  {}
func (c *captureDiag) Error(s string) {}

func TestExtractAnchors_DuplicateBlockID_EmitsDiagnostic(t *testing.T) {
	src := "First. ^dup\n\nSecond. ^dup\n"
	diag := &captureDiag{}
	extractAnchorsWithDiag(src, "/tmp/note.md", diag)
	if len(diag.warns) != 1 {
		t.Fatalf("want 1 warn, got %d (%v)", len(diag.warns), diag.warns)
	}
	if !strings.Contains(diag.warns[0], "dup") || !strings.Contains(diag.warns[0], "note.md") {
		t.Errorf("warn message lacks expected substrings: %s", diag.warns[0])
	}
}
```

- [ ] **Step 2: Run, expect failure**

Run: `go test ./internal/vault/ -run TestExtractAnchors_DuplicateBlockID_EmitsDiagnostic -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Add the diag-aware variant**

In `internal/vault/anchors.go`, refactor:

```go
// extractAnchors is the no-diagnostics convenience.
func extractAnchors(src string) anchors {
	return extractAnchorsWithDiag(src, "", NopDiagnostics{})
}

// extractAnchorsWithDiag also reports duplicate block ids via diag.
func extractAnchorsWithDiag(src, path string, diag Diagnostics) anchors {
	source := []byte(src)
	md := goldmark.New(goldmark.WithExtensions(WikilinkExtension))
	doc := md.Parser().Parse(text.NewReader(source))
	out := newAnchors()

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch nn := n.(type) {
		case *ast.Heading:
			line := lineForNode(nn, source)
			slug := slugifyAnchor(headingText(nn, source))
			if slug != "" {
				if _, dup := out.headings[slug]; !dup {
					out.headings[slug] = line
				}
			}
		case *ast.Paragraph, *ast.ListItem, *ast.Blockquote:
			if id, ok := trailingBlockID(nn, source); ok {
				line := lineForNode(nn, source)
				if prev, dup := out.blocks[id]; dup {
					diag.Warn(fmt.Sprintf("vault: duplicate block id ^%s in %s (lines %d and %d)", id, path, prev, line))
				} else {
					out.blocks[id] = line
				}
			}
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return out
}
```

Add `"fmt"` to imports.

Update the caller (where Task 3 placed `extractAnchors(string(src))`) to pass diag:

```go
anchors: extractAnchorsWithDiag(string(src), abs, v.diag),
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/vault/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/vault/anchors.go internal/vault/anchors_test.go internal/vault/vault.go internal/vault/extract.go
git commit -m "feat(vault): warn on duplicate block ids in same file"
```

---

### Task 11: Update CLAUDE.md and docs index for shipped state

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/index.md`
- Modify: `docs/superpowers/specs/2026-05-25-block-references-design.md` (mark shipped)

- [ ] **Step 1: Update the docs index entry**

In `docs/index.md`, change the block-references entry:

```markdown
- [Block references and heading anchors](superpowers/specs/2026-05-25-block-references-design.md) — shipped — `[[Note#Heading]]`, `[[Note#^block]]`, and `[text](note.md#heading)` now scroll the destination. Broken anchors render with the `?` suffix and count in the footer tally.
```

In the Wikilinks Phase 2 entry, drop "block references" from the remaining list:

```markdown
- [Wikilinks and backlinks](...) — Phase 1 shipped: ... Phase 2 partially shipped (auto-scroll, inline-link pre-select, broken-link tally, block references); configurable vault root remains. ...
```

- [ ] **Step 2: Update CLAUDE.md gotchas**

In `CLAUDE.md`, find the section that says "anchor navigation is not implemented" or similar references. There isn't an explicit gotcha for it today, but the "What's not built yet" / Phase 2 section mentions block references — update it:

Find:
```
**Wikilinks and backlinks — Phase 2 in progress:** ... Remaining: block references (`[[note#^blockid]]`) and configurable vault root.
```

Change to:
```
**Wikilinks and backlinks — Phase 2 in progress:** inline-link pre-selection on backlink-follow / Back / Forward shipped. Broken-link tally in the footer shipped. Heading and block anchors shipped (see [block-references](docs/superpowers/specs/2026-05-25-block-references-design.md)). Remaining: configurable vault root.
```

Add a new gotcha (alphabetical-ish near other vault gotchas):

```markdown
- **`^block-id` markers are stripped from rendered output.** The vault index sees the raw marker (so `[[Note#^foo]]` resolves), but `preprocessBlockMarkers` removes the trailing ` ^id` from every non-fence line before Glamour renders. Markers inside fenced code blocks render verbatim. Duplicate ids in the same file: first wins, with a `warn` diagnostic.
```

- [ ] **Step 3: Mark the spec shipped**

In `docs/superpowers/specs/2026-05-25-block-references-design.md`, change:

```
**Status:** spec — not yet implemented.
```

to:

```
**Status:** shipped.
```

- [ ] **Step 4: Run full test suite + build**

Run: `go build ./... && go vet ./... && go test -race ./...`
Expected: PASS — no races, no failures, builds cleanly.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md docs/index.md docs/superpowers/specs/2026-05-25-block-references-design.md
git commit -m "docs: mark block references shipped"
```

---

## Self-review notes

- **Spec coverage:** all components in the spec (anchors index, ResolveAnchor, preprocessBlockMarkers, broken-anchor render, broken-anchor tally, anchor follow path, duplicate-id diagnostic) map to tasks 1–10. Docs task closes out.
- **No placeholders:** every step has the code or commands the executor needs. Where adaptation is required (Task 8's `m.vault` field name, the test-helper name `newTestModelOn`), the step explicitly tells the executor to grep and adjust.
- **Type consistency:** `extractAnchors` / `extractAnchorsWithDiag` / `ResolveAnchor` / `slugifyAnchor` / `blockMarkerRegex` / `blockMarkerLineRegex` / `splitAnchor` all named consistently across tasks. `anchors` struct fields (`headings`, `blocks`) match across tasks 1, 2, 3, 4.
- **Open risk:** Task 8 assumes `m.vault` exists on the Model. The spec relied on this from the wikilinks Phase 1 work — `internal/tui/backlinks.go` and the diagnostics modal already reach `m.vault`. If the field name is different, Task 8 Step 1 catches that.
