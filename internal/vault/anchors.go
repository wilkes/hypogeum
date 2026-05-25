package vault

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// blockMarkerRegex matches a trailing block-id marker ` ^id` at end of
// text. The id is alphanumerics + hyphens, matching Obsidian's syntax.
var blockMarkerRegex = regexp.MustCompile(`(?:^| )\^([a-zA-Z0-9-]+)\s*$`)

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

// blockText returns the concatenated text content of block n.
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
