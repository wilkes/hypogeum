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
	headings map[string]int
	blocks   map[string]int
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
// Copied to avoid an internal/vault → internal/markdown import cycle.
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
