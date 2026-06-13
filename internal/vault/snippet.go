package vault

import (
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/wilkes/hypogeum/internal/highlight"
)

// Snippets bracket the display text of a reference with highlight.Open /
// highlight.Close (defined in internal/highlight as the single source of
// truth for the wire format). The TUI applies SGR around these markers
// when rendering snippets so the wikilink target stands out.

// snippetForNode walks up from n to the smallest enclosing block-level
// node, then renders that subtree as plain text. Within the result,
// the original n's text is wrapped in highlight markers.
func snippetForNode(n ast.Node, source []byte, displayText string) string {
	block := enclosingBlock(n)
	if block == nil {
		return highlight.Wrap(displayText)
	}
	plain := nodeText(block, source)
	plain = strings.TrimSpace(plain)

	if displayText != "" {
		i := strings.Index(plain, displayText)
		if i >= 0 {
			plain = plain[:i] + highlight.Wrap(displayText) + plain[i+len(displayText):]
		}
	}
	return plain
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
