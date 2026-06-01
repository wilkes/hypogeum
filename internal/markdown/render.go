// Package markdown wraps Glamour for rendering and provides utilities for
// resolving links in a markdown file relative to its location on disk.
package markdown

import (
	"fmt"
	"os"

	"github.com/charmbracelet/glamour"
)

// Option configures a Renderer.
type Option func(*Renderer)

// WithResolver makes wikilink AST nodes resolve via r. If unset,
// wikilinks always render as broken (which is fine for unit tests
// of the markdown package alone).
func WithResolver(r Resolver) Option {
	return func(rr *Renderer) { rr.resolver = r }
}

// Renderer renders markdown to ANSI-styled terminal output.
// Per-render state (fromFile) is mutated via SetFromFile; not safe
// for concurrent use across goroutines.
type Renderer struct {
	g            *glamour.TermRenderer
	instrumented *glamour.TermRenderer // sentinel-injected style; used by RenderWithLinks

	resolver Resolver
	fromFile string // set by SetFromFile before each RenderWithLinks
}

// NewRenderer constructs a Renderer with the given output width.
// Options can configure resolver and other behaviors; pass none to
// match the previous (resolver-less) behavior.
//
// Both the plain and instrumented renderers go through hypogeumStyle so
// they stay byte-equivalent and so the prose styling can evolve in one
// place.
func NewRenderer(width int, opts ...Option) (*Renderer, error) {
	if width < 20 {
		width = 80
	}
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
	if err != nil {
		return nil, fmt.Errorf("init glamour: %w", err)
	}

	instrumented, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithStyles(linkInstrumentationStyles(width)),
		glamour.WithInlineTableLinks(true),
	)
	if err != nil {
		return nil, fmt.Errorf("init instrumented glamour: %w", err)
	}

	r := &Renderer{
		g:            g,
		instrumented: instrumented,
		resolver:     nopResolver{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

// SetFromFile sets the file path used to resolve wikilink targets
// for the next render. Must be called before RenderWithLinks for
// each new file. The renderer is not safe for concurrent use across
// files; one renderer per goroutine.
func (r *Renderer) SetFromFile(path string) {
	r.fromFile = path
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
//
// The URL-suppression sentinels grafted onto cfg.Link by hypogeumStyle
// are stripped here so the plain renderer produces the same hidden-URL
// output as RenderWithLinks.
func (r *Renderer) Render(src string) (string, error) {
	out, err := r.g.Render(src)
	if err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return stripURLSentinels(out), nil
}
