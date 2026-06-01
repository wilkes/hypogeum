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

// TestRenderWithLinks_WikilinkInInlineCodeIsVerbatim guards the rule
// that `[[Name]]` inside a single-backtick inline-code span renders
// verbatim instead of being rewritten to a resolved markdown link.
// Without the skip, the rewritten "[Name](/abs/...)" sits inside a
// code span and Glamour faithfully prints the URL bytes — the leak
// shows up most visibly inside wrapped table cells.
func TestRenderWithLinks_WikilinkInInlineCodeIsVerbatim(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{
		answers: map[string]string{"Foo": "/abs/foo.md"},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	r.SetFromFile("/abs/source.md")

	out, links, _, err := r.RenderWithLinks("see `[[Foo]]` above.", "/abs/source.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	visible := stripANSI(out)
	if strings.Contains(visible, "/abs/foo.md") {
		t.Fatalf("URL leaked through inline-code span: %q", visible)
	}
	if !strings.Contains(visible, "[[Foo]]") {
		t.Fatalf("expected literal [[Foo]] in inline-code span, got: %q", visible)
	}
	if len(links) != 0 {
		t.Fatalf("inline-code wikilinks must not produce Link entries, got %d", len(links))
	}
}

// TestCountUnresolvedWikilinks_SkipsInlineCode mirrors the skip on the
// broken-link tally: a wikilink the user wrote as code shouldn't count
// against the document's broken-link score, even when the resolver
// would otherwise fail to find it.
func TestCountUnresolvedWikilinks_SkipsInlineCode(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	if got := r.CountUnresolvedWikilinks("see `[[Missing]]` above"); got != 0 {
		t.Fatalf("CountUnresolvedWikilinks = %d, want 0", got)
	}
	if got := r.CountUnresolvedWikilinks("see [[Missing]] above"); got != 1 {
		t.Fatalf("CountUnresolvedWikilinks = %d, want 1 (sanity check live form)", got)
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
