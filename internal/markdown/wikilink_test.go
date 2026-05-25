package markdown

import (
	"strings"
	"testing"
)

type fakeResolver struct {
	answers map[string]string
}

func (f fakeResolver) Resolve(fromFile, name, heading, block string) (string, bool) {
	v, ok := f.answers[name]
	return v, ok
}

func (f fakeResolver) ResolveAnchor(string, string, string) (int, bool) {
	return 0, false
}

func TestRenderWithLinks_ResolvedWikilinkBecomesLink(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{
		answers: map[string]string{"Foo": "/abs/foo.md"},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	r.SetFromFile("/abs/source.md")

	out, links, _, err := r.RenderWithLinks("see [[Foo]] above.", "/abs/source.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("links: got %d want 1 (%v)", len(links), links)
	}
	if links[0].Resolved.Kind != LinkLocalFile {
		t.Fatalf("expected LinkLocalFile, got %v", links[0].Resolved.Kind)
	}
	if links[0].Resolved.Target != "/abs/foo.md" {
		t.Fatalf("target: got %q want /abs/foo.md", links[0].Resolved.Target)
	}
	if !strings.Contains(out, "Foo") {
		t.Fatalf("rendered output missing display text: %q", out)
	}
}

func TestRenderWithLinks_UnresolvedWikilinkIsBroken(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	r.SetFromFile("/abs/source.md")

	out, links, _, err := r.RenderWithLinks("see [[Missing]] above.", "/abs/source.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 0 {
		// Unresolved wikilinks render as plain text (with a "?" suffix) —
		// they should not produce a Link entry. The user can't follow them.
		t.Fatalf("links: got %d want 0", len(links))
	}
	if !strings.Contains(out, "Missing") {
		t.Fatalf("rendered output missing display text: %q", out)
	}
	if !strings.Contains(out, "?") {
		t.Fatalf("expected '?' suffix on broken wikilink: %q", out)
	}
}

func TestRenderWithLinks_AliasUsedAsDisplayText(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{
		answers: map[string]string{"Foo": "/abs/foo.md"},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	out, links, _, err := r.RenderWithLinks("see [[Foo|the foo]] above.", "/abs/source.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 1 || links[0].Text != "the foo" {
		t.Fatalf("link text: got %v want 'the foo'", links)
	}
	if !strings.Contains(out, "the foo") {
		t.Fatalf("rendered output missing alias: %q", out)
	}
}
