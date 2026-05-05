// Package markdown wraps Glamour for rendering and provides utilities for
// resolving links in a markdown file relative to its location on disk.
package markdown

import (
	"fmt"
	"os"

	"github.com/charmbracelet/glamour"
)

// Renderer renders markdown to ANSI-styled terminal output.
// It is safe to reuse across files; the underlying Glamour renderer holds
// no per-document state.
type Renderer struct {
	g            *glamour.TermRenderer
	instrumented *glamour.TermRenderer // sentinel-injected style; used by RenderWithLinks
}

// NewRenderer constructs a Renderer with the given output width.
// Both the plain and instrumented renderers go through hypogeumStyle so
// they stay byte-equivalent and so the prose styling can evolve in one
// place.
func NewRenderer(width int) (*Renderer, error) {
	if width < 20 {
		width = 80
	}
	g, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithStyles(hypogeumStyle(width)),
	)
	if err != nil {
		return nil, fmt.Errorf("init glamour: %w", err)
	}

	instrumented, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithStyles(linkInstrumentationStyles(width)),
	)
	if err != nil {
		return nil, fmt.Errorf("init instrumented glamour: %w", err)
	}

	return &Renderer{g: g, instrumented: instrumented}, nil
}

// RenderFile reads and renders the markdown file at path.
func (r *Renderer) RenderFile(path string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return r.Render(string(src))
}

// Render renders a markdown string.
func (r *Renderer) Render(src string) (string, error) {
	out, err := r.g.Render(src)
	if err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return out, nil
}
