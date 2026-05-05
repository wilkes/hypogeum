package markdown

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

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
