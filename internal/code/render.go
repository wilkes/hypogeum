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
// error for all user-facing problems (unrecognized syntax, tokenization
// failure). A non-nil error indicates a programming-level invariant
// violation (e.g. the terminal256 formatter not being registered).
//
// Soft-wrap for long lines will be added in a later task.
func (r *Renderer) Render(path string, src []byte) (string, error) {
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
		// Primary lexer failed; fall back to plain text. The Fallback
		// lexer is a no-op tokenizer and has never been observed to
		// fail in practice — if it ever does, that's an invariant
		// violation worth surfacing rather than swallowing.
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
	return addGutter(buf.String(), r.width), nil
}

// looksBinary reports whether src appears to be binary content using the
// same heuristic git uses: a NUL byte in the first 8 KB.
func looksBinary(src []byte) bool {
	n := len(src)
	if n > 8192 {
		n = 8192
	}
	return bytes.IndexByte(src[:n], 0) >= 0
}
