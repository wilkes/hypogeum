package markdown

import (
	"strings"
	"testing"
)

// recordingResolver captures every Resolve call so tests can assert on the
// arguments passed (notably heading/block, which the simpler fakeResolver
// in wikilink_test.go drops).
type recordingResolver struct {
	answers map[string]string
	calls   []resolveCall
}

type resolveCall struct {
	fromFile string
	name     string
	heading  string
	block    string
}

func (r *recordingResolver) Resolve(fromFile, name, heading, block string) (string, bool) {
	r.calls = append(r.calls, resolveCall{fromFile: fromFile, name: name, heading: heading, block: block})
	v, ok := r.answers[name]
	return v, ok
}

func TestPreprocessWikilinks_NilResolverPassesThrough(t *testing.T) {
	r, err := NewRenderer(80)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	r.resolver = nil
	src := "see [[Foo]] above"
	got := r.preprocessWikilinks(src)
	if got != src {
		t.Fatalf("expected pass-through, got %q", got)
	}
}

func TestPreprocessWikilinks_SimpleResolved(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(&recordingResolver{
		answers: map[string]string{"Foo": "/abs/foo.md"},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	got := r.preprocessWikilinks("see [[Foo]] above")
	want := "see [Foo](/abs/foo.md) above"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPreprocessWikilinks_AliasIsDisplay(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(&recordingResolver{
		answers: map[string]string{"Foo": "/abs/foo.md"},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	got := r.preprocessWikilinks("[[Foo|the foo]]")
	want := "[the foo](/abs/foo.md)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPreprocessWikilinks_HeadingDisplayAndAnchor(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(&recordingResolver{
		answers: map[string]string{"Foo": "/abs/foo.md"},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	got := r.preprocessWikilinks("[[Foo#Some Heading]]")
	want := "[Foo > Some Heading](/abs/foo.md#some-heading)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPreprocessWikilinks_UnresolvedRendersBroken(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(&recordingResolver{}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	got := r.preprocessWikilinks("[[Missing]]")
	want := "Missing?"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPreprocessWikilinks_BlockReferencePassedToResolver(t *testing.T) {
	rec := &recordingResolver{answers: map[string]string{"Foo": "/abs/foo.md"}}
	r, err := NewRenderer(80, WithResolver(rec))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	r.SetFromFile("/abs/src.md")
	_ = r.preprocessWikilinks("[[Foo^block123]]")
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 Resolve call, got %d", len(rec.calls))
	}
	c := rec.calls[0]
	if c.name != "Foo" {
		t.Errorf("name: got %q want Foo", c.name)
	}
	if c.block != "block123" {
		t.Errorf("block: got %q want block123", c.block)
	}
	if c.fromFile != "/abs/src.md" {
		t.Errorf("fromFile: got %q want /abs/src.md", c.fromFile)
	}
}

func TestRenderWithLinks_EndToEndSentinelsStripped(t *testing.T) {
	r := rendererForTest(t)
	out, links, err := r.RenderWithLinks("See [the docs](docs.md) here.\n", "/base/index.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("links: got %d want 1", len(links))
	}
	if links[0].Text != "the docs" {
		t.Errorf("Text: got %q want %q", links[0].Text, "the docs")
	}
	if links[0].Href != "docs.md" {
		t.Errorf("Href: got %q want %q", links[0].Href, "docs.md")
	}
	if links[0].Row < 0 {
		t.Errorf("Row: got %d want >=0", links[0].Row)
	}
	if !strings.Contains(out, "the docs") {
		t.Errorf("output missing link text: %q", out)
	}
	if strings.ContainsRune(out, sentinelStart) || strings.ContainsRune(out, sentinelEnd) {
		t.Errorf("sentinels leaked into output: %q", out)
	}
}

func TestStripSentinels_NoSentinels(t *testing.T) {
	in := "hello world\nno markers here\n"
	cleaned, spans := stripSentinels(in, nil)
	if cleaned != in {
		t.Errorf("cleaned: got %q want %q", cleaned, in)
	}
	if len(spans) != 0 {
		t.Errorf("expected zero spans, got %d", len(spans))
	}
}

func TestStripSentinels_SingleLineLink(t *testing.T) {
	in := "hello \x1cworld\x1e end"
	cleaned, spans := stripSentinels(in, nil)
	wantClean := "hello world end"
	if cleaned != wantClean {
		t.Errorf("cleaned: got %q want %q", cleaned, wantClean)
	}
	if len(spans) != 1 {
		t.Fatalf("spans: got %d want 1", len(spans))
	}
	if spans[0].row != 0 {
		t.Errorf("row: got %d want 0", spans[0].row)
	}
	if spans[0].text != "world" {
		t.Errorf("text: got %q want world", spans[0].text)
	}
}

func TestStripSentinels_LinkWrappingTwoLines(t *testing.T) {
	in := "a\x1cone\ntwo\x1e b"
	cleaned, spans := stripSentinels(in, nil)
	wantClean := "aone\ntwo b"
	if cleaned != wantClean {
		t.Errorf("cleaned: got %q want %q", cleaned, wantClean)
	}
	if len(spans) != 1 {
		t.Fatalf("spans: got %d want 1", len(spans))
	}
	if spans[0].row != 0 {
		t.Errorf("row: got %d want 0", spans[0].row)
	}
	if !strings.Contains(spans[0].text, "\n") {
		t.Errorf("expected span text to contain newline, got %q", spans[0].text)
	}
	if spans[0].text != "one\ntwo" {
		t.Errorf("text: got %q want %q", spans[0].text, "one\ntwo")
	}
}

func TestStripSentinels_MultipleLinksOneLine(t *testing.T) {
	in := "\x1cfoo\x1e and \x1cbar\x1e"
	cleaned, spans := stripSentinels(in, nil)
	wantClean := "foo and bar"
	if cleaned != wantClean {
		t.Errorf("cleaned: got %q want %q", cleaned, wantClean)
	}
	if len(spans) != 2 {
		t.Fatalf("spans: got %d want 2", len(spans))
	}
	for i, s := range spans {
		if s.row != 0 {
			t.Errorf("spans[%d].row: got %d want 0", i, s.row)
		}
	}
	if spans[0].text != "foo" || spans[1].text != "bar" {
		t.Errorf("texts: got %q,%q want foo,bar", spans[0].text, spans[1].text)
	}
}

func TestStripSentinels_LinkInsideSGR(t *testing.T) {
	in := "\x1b[1m\x1cbold\x1e\x1b[0m"
	cleaned, spans := stripSentinels(in, nil)
	// SGR escapes survive intact, sentinels disappear.
	wantClean := "\x1b[1mbold\x1b[0m"
	if cleaned != wantClean {
		t.Errorf("cleaned: got %q want %q", cleaned, wantClean)
	}
	if strings.ContainsRune(cleaned, sentinelStart) || strings.ContainsRune(cleaned, sentinelEnd) {
		t.Errorf("sentinels leaked: %q", cleaned)
	}
	if len(spans) != 1 {
		t.Fatalf("spans: got %d want 1", len(spans))
	}
	if spans[0].text != "bold" {
		t.Errorf("text: got %q want bold (SGR should not pollute span text)", spans[0].text)
	}
}

func TestStripSentinels_MarkerBracketsLink(t *testing.T) {
	marker := func(i int) (string, string) {
		return "<<O" + string(rune('0'+i)) + ">>", "<<C" + string(rune('0'+i)) + ">>"
	}
	in := "\x1cone\x1e and \x1ctwo\x1e"
	cleaned, spans := stripSentinels(in, marker)
	if !strings.Contains(cleaned, "<<O0>>one<<C0>>") {
		t.Errorf("expected first marker pair in cleaned output: %q", cleaned)
	}
	if !strings.Contains(cleaned, "<<O1>>two<<C1>>") {
		t.Errorf("expected second marker pair in cleaned output: %q", cleaned)
	}
	if strings.ContainsRune(cleaned, sentinelStart) || strings.ContainsRune(cleaned, sentinelEnd) {
		t.Errorf("sentinel runes leaked despite marker: %q", cleaned)
	}
	if len(spans) != 2 {
		t.Fatalf("spans: got %d want 2", len(spans))
	}
}

// TestStripSentinels_URLSuppression covers the urlSuppressStart/End pair
// that hides Glamour's trailing "[text] /url" form. The leading space
// must come off too so prose reads as "[text]" without a hanging blank.
func TestStripSentinels_URLSuppression(t *testing.T) {
	in := "see \x1cdocs\x1e \x1d/path/to/file.md\x1f for more"
	cleaned, spans := stripSentinels(in, nil)
	want := "see docs for more"
	if cleaned != want {
		t.Errorf("cleaned = %q, want %q", cleaned, want)
	}
	if len(spans) != 1 || spans[0].text != "docs" {
		t.Errorf("spans = %+v, want one span with text %q", spans, "docs")
	}
}

// TestRender_HidesURLs confirms the plain renderer also honors the
// hidden-URL house style — Render runs through stripURLSentinels so a
// caller piping output to a file gets the same prose the TUI does.
func TestRender_HidesURLs(t *testing.T) {
	r := rendererForTest(t)
	out, err := r.Render("See [docs](https://example.com/long/path) for more.\n")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(out, "example.com") || strings.Contains(out, "/long/path") {
		t.Errorf("URL leaked into plain Render output: %q", out)
	}
	if !strings.Contains(out, "docs") {
		t.Errorf("link text missing from plain Render output: %q", out)
	}
	if strings.ContainsRune(out, urlSuppressStart) || strings.ContainsRune(out, urlSuppressEnd) {
		t.Errorf("url-suppress sentinels leaked: %q", out)
	}
}

// TestRender_DottedUnderlineSGR confirms the LinkText primitive emits
// the SGR 4:4 (dotted underline) sequence around link visible text.
// Terminals without 4:4 support fall back to a solid underline.
func TestRender_DottedUnderlineSGR(t *testing.T) {
	r := rendererForTest(t)
	out, err := r.Render("See [docs](https://example.com).\n")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "\x1b[4:4m") {
		t.Errorf("expected dotted-underline SGR \\x1b[4:4m in output: %q", out)
	}
	if !strings.Contains(out, "\x1b[24m") {
		t.Errorf("expected underline-reset SGR \\x1b[24m in output: %q", out)
	}
}

// TestRenderWithLinks_OSC8External verifies that external URLs flow
// into the OSC 8 hyperlink wrapper around link visible text. The URL
// portion stays hidden in the rendered prose; the click-target lives
// in the OSC 8 sequence.
func TestRenderWithLinks_OSC8External(t *testing.T) {
	r := rendererForTest(t)
	out, _, err := r.RenderWithLinks("See [docs](https://example.com/path).\n", "/base/file.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	want := "\x1b]8;;https://example.com/path\x1b\\"
	if !strings.Contains(out, want) {
		t.Errorf("expected OSC 8 hyperlink open %q in output: %q", want, out)
	}
	if !strings.Contains(out, "\x1b]8;;\x1b\\") {
		t.Errorf("expected OSC 8 hyperlink close in output: %q", out)
	}
}

// TestRenderWithLinks_OSC8LocalFile uses file:// for local paths so
// terminals with clickable-link support open the target.
func TestRenderWithLinks_OSC8LocalFile(t *testing.T) {
	r := rendererForTest(t)
	out, _, err := r.RenderWithLinks("See [docs](one.md).\n", "/notes/index.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	want := "\x1b]8;;file:///notes/one.md\x1b\\"
	if !strings.Contains(out, want) {
		t.Errorf("expected OSC 8 file:// for local link in output: %q", out)
	}
}

// TestRenderWithLinks_OSC8AnchorSkipped — anchor-only links have no
// useful click target, so we omit OSC 8 wrapping (the terminal would
// otherwise treat them as broken links).
func TestRenderWithLinks_OSC8AnchorSkipped(t *testing.T) {
	r := rendererForTest(t)
	out, _, err := r.RenderWithLinks("See [section](#anchor).\n", "/notes/index.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if strings.Contains(out, "\x1b]8;;") {
		t.Errorf("expected no OSC 8 sequences for anchor-only link: %q", out)
	}
}
