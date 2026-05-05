package markdown

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

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

// ASTLink is a single hyperlink as it appears in the markdown source.
// Text is the visible text the user reads (the link label, post-rendering
// of inline emphasis but pre-Glamour styling). Href is the raw target
// exactly as written in the source.
type ASTLink struct {
	Text string
	Href string
}

// ExtractLinks walks the markdown AST and returns every hyperlink in the
// order it appears in the document. Images are skipped; only followable
// links (inline links and autolinks) are returned.
func ExtractLinks(src string) []ASTLink {
	source := []byte(src)
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))

	var links []ASTLink
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			links = append(links, ASTLink{
				Text: nodeText(v, source),
				Href: string(v.Destination),
			})
			// Don't descend into the link's children — we already have
			// the label text, and any inner Link nodes (rare) would be
			// nonsense.
			return ast.WalkSkipChildren, nil
		case *ast.AutoLink:
			url := string(v.URL(source))
			links = append(links, ASTLink{Text: url, Href: url})
			return ast.WalkSkipChildren, nil
		case *ast.Image:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return links
}

// nodeText concatenates every Text segment under n. This collapses inline
// emphasis (e.g. **bold link text**) into its visible string form.
func nodeText(n ast.Node, source []byte) string {
	var out []byte
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			out = append(out, t.Segment.Value(source)...)
			continue
		}
		out = append(out, []byte(nodeText(c, source))...)
	}
	return string(out)
}
