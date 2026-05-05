// Package markdown wraps Glamour for rendering and provides utilities for
// resolving links in a markdown file relative to its location on disk.
package markdown

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
)

// Renderer renders markdown to ANSI-styled terminal output.
// It is safe to reuse across files; the underlying Glamour renderer holds
// no per-document state.
type Renderer struct {
	g *glamour.TermRenderer
}

// NewRenderer constructs a Renderer with the given output width.
// The "auto" style follows the terminal's light/dark setting.
func NewRenderer(width int) (*Renderer, error) {
	if width < 20 {
		width = 80
	}
	g, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
	if err != nil {
		return nil, fmt.Errorf("init glamour: %w", err)
	}
	return &Renderer{g: g}, nil
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

// LinkKind classifies a markdown link target so the navigation layer can
// decide how to handle it.
type LinkKind int

const (
	// LinkLocalFile is a file on the local filesystem (resolved absolute path).
	LinkLocalFile LinkKind = iota
	// LinkExternal is a URL with an http(s) or other non-file scheme.
	LinkExternal
	// LinkAnchor is a same-document anchor (begins with '#').
	LinkAnchor
	// LinkInvalid means the target could not be classified or resolved.
	LinkInvalid
)

// ResolvedLink describes a link target after resolution against a base file.
type ResolvedLink struct {
	Kind   LinkKind
	Target string // absolute path for LinkLocalFile, raw URL otherwise
	Anchor string // fragment, if any (without leading '#')
}

// ResolveLink interprets the href of a link found inside the file at base.
// It does not check that the target exists; callers handle missing files.
func ResolveLink(base, href string) ResolvedLink {
	href = strings.TrimSpace(href)
	if href == "" {
		return ResolvedLink{Kind: LinkInvalid}
	}

	// Pure fragment: same-document anchor.
	if strings.HasPrefix(href, "#") {
		return ResolvedLink{Kind: LinkAnchor, Anchor: strings.TrimPrefix(href, "#")}
	}

	// Try parsing as URL to detect schemes. Note that bare paths parse
	// successfully with an empty Scheme, so we check that explicitly.
	u, err := url.Parse(href)
	if err == nil && u.Scheme != "" && u.Scheme != "file" {
		return ResolvedLink{Kind: LinkExternal, Target: href}
	}

	// Local path. Strip any fragment for the file path; preserve it separately.
	target := href
	anchor := ""
	if u != nil {
		if u.Path != "" {
			target = u.Path
		}
		anchor = u.Fragment
	}

	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(base), target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return ResolvedLink{Kind: LinkInvalid}
	}
	return ResolvedLink{Kind: LinkLocalFile, Target: abs, Anchor: anchor}
}
