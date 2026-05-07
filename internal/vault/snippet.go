package vault

import (
	"strings"

	"github.com/yuin/goldmark/ast"
)

// snippetHighlightOpen / Close bracket the display text of a reference
// inside its snippet. The TUI applies SGR around these markers when
// rendering snippets so the wikilink target stands out.
//
// Using ASCII control chars keeps the markers invisible to plain-text
// processing while distinguishable from anything in user content.
const (
	snippetHighlightOpen  = "\x11" // DC1
	snippetHighlightClose = "\x12" // DC2
)

// snippetForNode walks up from n to the smallest enclosing block-level
// node, then renders that subtree as plain text. Within the result,
// the original n's text is wrapped in highlight markers.
func snippetForNode(n ast.Node, source []byte, displayText string) string {
	block := enclosingBlock(n)
	if block == nil {
		return wrapHighlight(displayText)
	}
	plain := nodeText(block, source)
	plain = strings.TrimSpace(plain)

	if displayText != "" {
		i := strings.Index(plain, displayText)
		if i >= 0 {
			plain = plain[:i] + wrapHighlight(displayText) + plain[i+len(displayText):]
		}
	}
	return plain
}

func wrapHighlight(s string) string {
	return snippetHighlightOpen + s + snippetHighlightClose
}

// enclosingBlock returns the smallest block-level ancestor of n.
// Block-level means: paragraph, heading, list item, blockquote, fenced
// code, etc. Returns nil if n is itself the document root.
func enclosingBlock(n ast.Node) ast.Node {
	for cur := n.Parent(); cur != nil; cur = cur.Parent() {
		if cur.Type() == ast.TypeBlock {
			return cur
		}
	}
	return nil
}

// nodeText recursively concatenates every Text segment under n.
// Block-level structure (list bullets, blockquote markers) is not
// included — snippets are plain text by design.
func nodeText(n ast.Node, source []byte) string {
	switch v := n.(type) {
	case *ast.Text:
		return string(v.Segment.Value(source))
	case *wikilinkNode:
		// Render the display text — the alias if set, else the name.
		disp := v.Alias
		if disp == "" {
			disp = v.Name
		}
		return disp
	}
	var out []byte
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		piece := nodeText(c, source)
		if len(out) > 0 && len(piece) > 0 {
			// Insert a space between sibling block-level pieces so
			// "Heading\nparagraph" doesn't render as "Headingparagraph".
			if c.Type() == ast.TypeBlock {
				out = append(out, ' ')
			}
		}
		out = append(out, piece...)
	}
	return string(out)
}
