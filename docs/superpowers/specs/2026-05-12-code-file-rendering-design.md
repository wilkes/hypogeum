# Code-file syntax highlighting — design

## Status

Approved 2026-05-12. Implementation plan pending.

## Problem

Hypogeum is a markdown browser. When the user points it at a `.go`, `.rb`, `.py`, or other source file — or follows a relative link from a markdown doc to one — the existing pipeline runs the source through Glamour as if it were prose. The result is unreadable: word-wrapping breaks indentation, no syntax colors, and the file appears as a wall of unstyled paragraphs.

We want code files to render with syntax highlighting and a line-number gutter, while keeping markdown the primary use case.

## Goals

- A `.go`/`.rb`/`.py`/etc. file passed on the CLI renders with syntax highlighting.
- A relative link from a `.md` file to a code file (e.g. `[main.go](./main.go)`) follows into a highlighted view via the existing `Enter`-on-selected-link flow.
- A line-number gutter precedes every source line.
- Long lines soft-wrap within the viewport; continuation rows show a blank gutter so each source line stays a single numbered unit.
- Live-reload: saving the open code file in an editor refreshes the view, preserving scroll offset.

## Non-goals

- Code files do **not** appear in the tree modal (`^b`) or the recency-ranked picker (`^p`).
- Wikilinks (`[[name]]`) do **not** resolve to code files.
- Vault/backlinks do **not** index code files.
- No code-aware navigation (jump to function/symbol), no folding, no editing.
- No user-configurable Chroma theme in v1.
- No "force render as markdown" override for code files that contain markdown syntax.

These keep `tree.IsMarkdown`, the vault index, and the picker untouched. Code rendering is a tactical upgrade of the per-file render path, not a redefinition of "what hypogeum browses."

## Architecture

### New package `internal/code`

A renderer parallel to `internal/markdown`, exposing:

```go
package code

type Renderer struct {
    width int
    style *chroma.Style
}

func NewRenderer(width int) *Renderer
func (r *Renderer) Render(path string, src []byte) (string, error)
```

`Render` returns a non-nil error only if a programming-level invariant fails (e.g. the `terminal256` formatter isn't registered). User-facing problems — binary input, oversized files, unrecognized syntax — return a renderable string with `nil` error, so the dispatcher in `refreshContent` doesn't need a special branch beyond what's already there for `os.ReadFile` failures.

`Render` runs the following pipeline:

1. **Binary check.** If the first 8 KB of `src` contains a NUL byte, return a single-line rendering `"binary file, not displayed"`. (Chroma will happily tokenize binary input as plain text and produce noise; the NUL heuristic is the same one `git diff` uses.)
2. **Size check.** If `len(src) > 5 * 1024 * 1024`, return `"file too large to display"`. Chroma's tokenizer is O(n) but the resulting ANSI string for a 200 MB file would exhaust the Bubbles viewport and slow every paint.
3. **Lexer selection:**
   - `lexer := lexers.Match(filepath.Base(path))` — covers both extension globs and filename globs (`Dockerfile`, `Makefile`, `Gemfile`) in one call.
   - If nil, `lexer = lexers.Analyse(string(src))` — Chroma scores every registered lexer's `Analyse` function and returns the best match.
   - If still nil, `lexer = lexers.Fallback` (the plain-text lexer; never nil, so the chain always terminates).
4. **Tokenize:** `iterator, err := lexer.Tokenise(nil, string(src))`. On error, fall back to plain-text rendering with line numbers.
5. **Format:** `formatter := formatters.Get("terminal256")`; emit ANSI to a `bytes.Buffer`.
6. **Gutter + wrap.** Split the formatted output on `\n`. For each source line:
   - Prepend a faint right-aligned line number (width = `len(strconv.Itoa(totalLines))`).
   - If the line's display width exceeds `width - gutterWidth - 1`, soft-wrap using `ansi.Wrap` from `github.com/charmbracelet/x/ansi` (already a direct requirement in `go.mod`).
   - Continuation rows get a blank gutter of identical width so numbering remains one-per-source-line.

`width` is `m.content.viewport.Width` and the renderer is rebuilt on every `WindowSizeMsg`, the same way `markdown.Renderer` already is.

### Dispatch in `internal/tui/content.go`

`refreshContent` becomes a two-branch dispatcher:

```go
if tree.IsMarkdown(path) {
    // existing path: SetFromFile + RenderWithLinks + populate m.content.links
} else {
    out, err := m.content.codeRenderer.Render(path, src)
    if err != nil {
        m.status = err.Error()
        m.content.viewport.SetContent(fmt.Sprintf("Error: %v", err))
    } else {
        m.status = path
        m.content.viewport.SetContent(out)
        m.content.viewport.GotoTop()
    }
    m.content.links = nil
    m.content.linkCursor = -1
}
```

Code files have no `markdown.Link` slice. Setting `m.content.links = nil` and `m.content.linkCursor = -1` makes `n`/`p`/`Enter` for link cycling natural no-ops — the existing handlers already tolerate an empty slice.

The `pendingPreselectTarget` flag is cleared at the top of `refreshContent` regardless of branch (matches existing behavior); it never applies to code files.

### Model wiring

`internal/tui/content.go::contentUIState` gains a field:

```go
codeRenderer *code.Renderer
```

It is built in `tui.New` alongside `m.content.renderer` and rebuilt on `WindowSizeMsg` in the same handler.

### CLI

`cmd/hypogeum/main.go::resolveTarget` already accepts any path that `os.Stat` succeeds on and routes a file to `(filepath.Dir(target), target)`. **No change.** The initial-file open path goes through `refreshContent`, which now dispatches by extension.

When `initialFile` is a code file inside a directory with markdown siblings, the tree modal still works (it walks the parent for `.md` regardless of the open file's type).

When `initialFile` is a code file inside a directory with **no** markdown, the tree modal opens empty. That matches existing behavior for a markdown-empty directory; we keep it. The content viewport shows the highlighted code, which is the user-facing point.

### Link following

`internal/markdown.ResolveLink` already classifies a relative path to *any* file (not just markdown) as `LinkLocalFile`. The `Enter`-on-link path calls `navigateTo`, which calls `openFile`, which calls `refreshContent`. With the new dispatcher in place, this works without changes to `internal/markdown`.

Recency tracking (`m.recent.Record`) already keys on the full path; recording a code-file visit is harmless because the picker still filters its candidate set to markdown via `allVaultMarkdownPaths`.

### Watcher / live reload

`internal/watch/classify.go` has two `tree.IsMarkdown` gates:

| Gate | Disposition |
|---|---|
| Structure-change classifier (create/delete/rename) | **Unchanged.** A new `.py` file does not trigger a tree re-walk. The tree is a markdown-only surface. |
| File-modification classifier | **Relax.** Emit `FileModified` for any path, not just markdown. |

`internal/tui/content.go::handleFSEvent` already filters modification events by "is this the currently open file?" (`p == cur`). Broadening the classifier is safe: non-md modifications get dropped at the TUI layer unless the user has that file open.

`refreshContent`'s scroll-offset preservation already works for any branch: `offset := viewport.YOffset`, refresh, `SetYOffset(offset)`. No special-casing for code files.

### Theming

Chroma style picked to match Glamour's existing code-fence palette. Glamour's `dark` and `auto` styles wrap Chroma's `monokai` family, so `internal/code/style.go` starts with `styles.Get("monokai")` and exposes a single `defaultStyle()` function so future theme work has one place to land.

The visual acceptance test: open a markdown file containing a fenced Go block, then open the same Go source standalone. The two should look the same. If colors drift, we tune by selecting a different built-in Chroma style — no new style language.

## Error handling

| Condition | Render |
|---|---|
| `os.ReadFile` fails | Existing path — status bar + `"Error: <err>"` in viewport. |
| File > 5 MB | `"file too large to display"` |
| File contains NUL in first 8 KB | `"binary file, not displayed"` |
| Chroma tokenization error | Plain-text fallback with gutter, no colors |
| Lexer lookup yields nil after `Match` + `Analyse` | Plain-text fallback with gutter |

No panics propagate out of `code.Render`. User-facing problems return a renderable string with nil error; only programmer-error conditions (e.g. missing `terminal256` formatter) surface as non-nil errors, and the dispatcher in `refreshContent` already handles those via the standard error path.

## Testing

### `internal/code/render_test.go` (new)

- `.go` source → output contains ANSI escape sequences (proof of highlighting) and starts each line with a numeric gutter.
- `.rb` source → same.
- `Dockerfile` (no extension, filename-globbed) → highlighted.
- Unknown extension `.xyz` with code-like content → `Analyse` either picks a lexer or falls back to plain text; either way, output has the gutter.
- Binary blob (`[]byte{'M','Z',0x00,...}`) → output is exactly the binary-file message.
- 6 MB buffer → output is exactly the too-large message.
- Long-line wrap: a 300-column line in a 80-column renderer produces multiple output rows where continuation rows have a blank gutter and no SGR escape precedes the gutter (guards against residual color from the wrapped source line bleeding into the gutter column).

Assertions are on shape (line count, presence of `\x1b[`, gutter prefix regex) rather than exact ANSI sequences. Coupling to Chroma's exact escape output would break tests on every Chroma update.

### `internal/tui/content_test.go` additions

- `refreshContent` with a `.go` path → `m.content.viewport.View()` is non-empty, `len(m.content.links) == 0`, `m.content.linkCursor == -1`, `m.status` equals the path.
- `refreshContent` with a non-existent code file path → status carries the read error, links are still cleared.

### `internal/watch/classify_test.go` additions

- A write event on a `.py` file produces a `FileModified` classifier output (currently dropped).
- A create event on a `.py` file produces no `StructureChanged` output (gate stays in place).

## Files touched

| File | Change |
|---|---|
| `internal/code/render.go` | new — pipeline, gutter, wrap |
| `internal/code/style.go` | new — Chroma style selection |
| `internal/code/render_test.go` | new |
| `internal/tui/content.go` | dispatch in `refreshContent` |
| `internal/tui/model.go` (or wherever the renderer is built) | construct `m.content.codeRenderer` in `New` and on resize |
| `internal/tui/content_test.go` | new tests |
| `internal/watch/classify.go` | drop `IsMarkdown` gate in modification classifier |
| `internal/watch/classify_test.go` | regression test |
| `CLAUDE.md` | brief note in "What's not built yet" → "Code file rendering" |
| `docs/index.md` | link this spec |

## Risks / open questions

- **Chroma style drift vs. Glamour's code fences.** Mitigation: the v1 acceptance test compares the two side-by-side. If monokai isn't a match, we pick a different built-in. No spec-level commitment to a specific named style — we settle that during implementation against actual rendered output.
- **Soft-wrap with ANSI escapes.** `ansi.Wrap` is ANSI-aware and won't split inside an SGR sequence, but the seam between a wrapped row's end-of-row SGR-reset and the next row's gutter has to be tested to ensure the gutter doesn't inherit a residual color. Covered by the long-line wrap test.
- **Recency tracking for code files.** Currently `openFile` calls `m.recent.Record(path)` for every visit. The picker filters its candidate set to markdown, so a recorded `.go` path is invisible — but it does occupy a row in the recency JSON file. Acceptable for v1; if storage growth is a concern later, gate the `Record` call on `tree.IsMarkdown`.
- **Initial-file in an empty-markdown directory.** The tree modal will be empty. The status bar still shows the path. This is consistent with existing behavior; no UI tweak needed.

## What's intentionally not in this design

- Picker support for code files — out of scope, can be a follow-up that toggles `allVaultMarkdownPaths` behind a flag.
- Wikilinks to code files — would require widening the vault index, which contradicts "vault is markdown-only."
- User-configurable Chroma theme — v2.
- Horizontal scrolling instead of soft-wrap — Bubbles' viewport doesn't support horizontal scroll without significant work; soft-wrap is the pragmatic v1.
