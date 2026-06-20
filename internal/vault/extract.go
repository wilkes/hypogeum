package vault

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type referenceKind int

const (
	refWikilink referenceKind = iota
	refStdLink
)

type reference struct {
	kind        referenceKind
	target      string // raw [[Target]] (wikilink) or href (stdlink)
	resolved    string // absolute path of the target file, "" if unresolved
	heading     string
	block       string
	alias       string
	displayText string
	snippet     string
	line        int
}

// extractReferences parses src as markdown (with the wikilink extension)
// and returns one reference per outgoing link, in document order.
// Standard ast.Link nodes become refStdLink entries; wikilinkNode
// instances become refWikilink entries.
func extractReferences(src, fromPath string) []reference {
	source := []byte(src)
	md := goldmark.New(goldmark.WithExtensions(WikilinkExtension))
	doc := md.Parser().Parse(text.NewReader(source))

	var refs []reference
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch nn := n.(type) {
		case *wikilinkNode:
			disp := nn.Alias
			if disp == "" {
				disp = nn.Name
			}
			refs = append(refs, reference{
				kind:        refWikilink,
				target:      nn.Name,
				heading:     nn.Heading,
				block:       nn.Block,
				alias:       nn.Alias,
				displayText: disp,
				snippet:     snippetForNode(nn, source, disp),
				line:        lineForNode(nn, source),
			})
			return ast.WalkSkipChildren, nil
		case *ast.Link:
			href := string(nn.Destination)
			disp := linkText(nn, source)
			refs = append(refs, reference{
				kind:        refStdLink,
				target:      href,
				resolved:    resolveStdLink(fromPath, href),
				displayText: disp,
				snippet:     snippetForNode(nn, source, disp),
				line:        lineForNode(nn, source),
			})
			return ast.WalkSkipChildren, nil
		case *ast.Image:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return refs
}

// lineForNode returns the 1-indexed line of the first segment of n
// within source. Returns 0 if no segment is found (rare — defensive).
//
// Wikilink nodes carry no ast.Text child, so they're handled by their
// stored byte offset (Pos) instead of by walking for a segment.
func lineForNode(n ast.Node, source []byte) int {
	if w, ok := n.(*wikilinkNode); ok {
		return lineAtOffset(source, w.Pos)
	}
	var seg *ast.Text
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := c.(*ast.Text); ok {
			seg = t
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if seg == nil {
		return 0
	}
	return lineAtOffset(source, seg.Segment.Start)
}

// lineAtOffset returns the 1-indexed line containing byte offset in
// source (counting the newlines before it). offset is clamped to the
// source length so out-of-range values degrade to the last line.
func lineAtOffset(source []byte, offset int) int {
	if offset > len(source) {
		offset = len(source)
	}
	line := 1
	for i := 0; i < offset; i++ {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}

// linkText returns the visible text under a *ast.Link.
func linkText(n ast.Node, source []byte) string {
	var out []byte
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			out = append(out, t.Segment.Value(source)...)
			continue
		}
		out = append(out, []byte(linkText(c, source))...)
	}
	return string(out)
}
