package vault

import (
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func parseWith(src string) ast.Node {
	md := goldmark.New(goldmark.WithExtensions(WikilinkExtension))
	return md.Parser().Parse(text.NewReader([]byte(src)))
}

func findFirstWikilink(n ast.Node) *wikilinkNode {
	var found *wikilinkNode
	_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if w, ok := n.(*wikilinkNode); ok {
			found = w
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

func TestWikilinkParse_Bare(t *testing.T) {
	doc := parseWith("see [[Foo]] for more.")
	w := findFirstWikilink(doc)
	if w == nil {
		t.Fatalf("no wikilink parsed")
	}
	if w.Name != "Foo" || w.Alias != "" || w.Heading != "" || w.Block != "" {
		t.Fatalf("got %+v", w)
	}
}

func TestWikilinkParse_Aliased(t *testing.T) {
	doc := parseWith("see [[Foo|the foo]] for more.")
	w := findFirstWikilink(doc)
	if w == nil {
		t.Fatalf("no wikilink parsed")
	}
	if w.Name != "Foo" || w.Alias != "the foo" {
		t.Fatalf("got %+v", w)
	}
}

func TestWikilinkParse_Heading(t *testing.T) {
	doc := parseWith("see [[Foo#Section Two]] for more.")
	w := findFirstWikilink(doc)
	if w == nil {
		t.Fatalf("no wikilink parsed")
	}
	if w.Name != "Foo" || w.Heading != "Section Two" {
		t.Fatalf("got %+v", w)
	}
}

func TestWikilinkParse_Block(t *testing.T) {
	doc := parseWith("see [[Foo^abc123]] for more.")
	w := findFirstWikilink(doc)
	if w == nil {
		t.Fatalf("no wikilink parsed")
	}
	if w.Name != "Foo" || w.Block != "abc123" {
		t.Fatalf("got %+v", w)
	}
}

func TestWikilinkParse_HeadingWithAlias(t *testing.T) {
	doc := parseWith("see [[Foo#Section|that section]] for more.")
	w := findFirstWikilink(doc)
	if w == nil {
		t.Fatalf("no wikilink parsed")
	}
	if w.Name != "Foo" || w.Heading != "Section" || w.Alias != "that section" {
		t.Fatalf("got %+v", w)
	}
}

func TestWikilinkParse_NotAWikilink(t *testing.T) {
	// A single-bracket link must not be parsed as a wikilink.
	doc := parseWith("see [Foo](bar.md) for more.")
	if w := findFirstWikilink(doc); w != nil {
		t.Fatalf("standard link parsed as wikilink: %+v", w)
	}
}

func TestWikilinkParse_UnclosedNotConsumed(t *testing.T) {
	// "[[Foo" with no closing brackets is left to the standard parser
	// (which renders it as text). The wikilink parser must not consume
	// it greedily.
	doc := parseWith("see [[Foo for more.")
	if w := findFirstWikilink(doc); w != nil {
		t.Fatalf("unclosed wikilink parsed: %+v", w)
	}
}

func TestStandardLinksUnchangedByWikilinkExtension(t *testing.T) {
	src := `# Title

A paragraph with a [normal link](other.md) and a [link with title](x.md "title").

- list with [a link](y.md)
- list with [[Wikilink]]

[autolink test](https://example.com).
`
	withoutExt := goldmark.New().Parser().Parse(text.NewReader([]byte(src)))
	withExt := goldmark.New(goldmark.WithExtensions(WikilinkExtension)).Parser().Parse(text.NewReader([]byte(src)))

	stdLinks := func(n ast.Node) []string {
		var out []string
		_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			if l, ok := n.(*ast.Link); ok {
				out = append(out, string(l.Destination))
			}
			return ast.WalkContinue, nil
		})
		return out
	}

	got := stdLinks(withExt)
	want := stdLinks(withoutExt)
	if len(got) != len(want) {
		t.Fatalf("standard link count changed: got %d want %d (got=%v want=%v)", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("link %d destination changed: got %q want %q", i, got[i], want[i])
		}
	}
}
