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

func (r *recordingResolver) ResolveAnchor(string, string, string) (int, bool) {
	return 0, true
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
	out, links, _, err := r.RenderWithLinks("See [the docs](docs.md) here.\n", "/base/index.md", nil)
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

// TestStripSentinels_URLSuppression_SpaceInsideSGR covers the real-world
// case: Glamour writes the space between LinkText and Link's content
// using the parent style, so the bytes are "<space>\x1b[0m\x1d...".
// The trim must walk back past the trailing SGR reset to find and
// remove the space, but must preserve the SGR (it's a valid reset
// that the terminal will honor).
func TestStripSentinels_URLSuppression_SpaceInsideSGR(t *testing.T) {
	in := "see \x1cdocs\x1e \x1b[0m\x1d/path\x1f, more"
	cleaned, _ := stripSentinels(in, nil)
	// stripANSI for visual comparison (the SGR survives, but no extra
	// spaces between "docs" and ",").
	visible := stripANSI(cleaned)
	want := "see docs, more"
	if visible != want {
		t.Errorf("visible(cleaned) = %q, want %q (raw: %q)", visible, want, cleaned)
	}
	if strings.ContainsRune(cleaned, urlSuppressStart) || strings.ContainsRune(cleaned, urlSuppressEnd) {
		t.Errorf("url-suppress sentinels leaked: %q", cleaned)
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

// TestRender_LinkTextUnderlined confirms the LinkText primitive carries
// the underline attribute (Glamour's dark theme puts it on Link, not
// LinkText, so we layer it on explicitly to compensate for hiding the
// URL portion).
func TestRender_LinkTextUnderlined(t *testing.T) {
	r := rendererForTest(t)
	out, err := r.Render("See [docs](https://example.com).\n")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// termenv emits ";4" as a sub-parameter when underline is requested
	// alongside other attributes (color, bold). Look for either ";4m"
	// or "[4m" — both are valid SGR underline indicators.
	if !strings.Contains(out, ";4m") && !strings.Contains(out, "[4m") {
		t.Errorf("expected underline SGR (;4 or [4) in output: %q", out)
	}
}

func TestHighlightMarker_SelectedLinkGetsReverseVideo(t *testing.T) {
	// Two sentinel-bracketed spans: link 0 and link 1.
	// HighlightMarker(1) should wrap link 1 in reverse-video SGR and
	// leave link 0 unwrapped.
	in := "\x1cfoo\x1e and \x1cbar\x1e"
	marker := HighlightMarker(1)
	cleaned, _ := stripSentinels(in, marker)

	if strings.Contains(cleaned, "\x1b[7m") && strings.HasPrefix(cleaned, "\x1b[7m") {
		t.Errorf("link 0 (foo) should NOT be highlighted; got: %q", cleaned)
	}
	if !strings.Contains(cleaned, "\x1b[7mbar\x1b[27m") {
		t.Errorf("link 1 (bar) should be wrapped in reverse-video SGR; got: %q", cleaned)
	}
	if strings.ContainsRune(cleaned, sentinelStart) || strings.ContainsRune(cleaned, sentinelEnd) {
		t.Errorf("sentinels leaked into output: %q", cleaned)
	}
}

func TestHighlightMarker_NoneSelectedWhenIndexNegative(t *testing.T) {
	in := "\x1cfoo\x1e and \x1cbar\x1e"
	marker := HighlightMarker(-1)
	cleaned, _ := stripSentinels(in, marker)
	if strings.Contains(cleaned, "\x1b[7m") {
		t.Errorf("no link should be highlighted when selected=-1; got: %q", cleaned)
	}
}

func TestHighlightMarker_WrappedLinkHighlightsEverySegment(t *testing.T) {
	// A sentinel-bracketed link whose visible text spans three rows
	// (two embedded newlines, as Glamour would emit after word-wrap).
	// Each row's link contribution must be wrapped independently in
	// reverse-video SGR so Glamour's per-row \e[0m tail doesn't kill
	// the highlight on continuation rows.
	in := "a\x1cone\ntwo\nthree\x1e b"
	marker := HighlightMarker(0)
	cleaned, _ := stripSentinels(in, marker)
	want := "a\x1b[7mone\x1b[27m\n\x1b[7mtwo\x1b[27m\n\x1b[7mthree\x1b[27m b"
	if cleaned != want {
		t.Errorf("multi-segment highlight: got %q want %q", cleaned, want)
	}
}

func TestCountUnresolvedWikilinks(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{
		answers: map[string]string{"Found": "/abs/found.md"},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	r.SetFromFile("/abs/source.md")

	src := "see [[Found]], [[Missing]] and [[AlsoMissing|alias]]\n" +
		"```\n" +
		"[[InsideFence]]\n" +
		"```\n"

	got := r.CountUnresolvedWikilinks(src)
	if got != 2 {
		t.Fatalf("CountUnresolvedWikilinks = %d, want 2", got)
	}
}


// anchorResolver resolves wikilink names AND anchors (heading slugs or
// block ids), keyed per-path. Used by the block-anchor tests below.
type anchorResolver struct {
	pathByName map[string]string
	lines      map[string]map[string]int // path → anchor key → line; key="^id" or "#slug"
}

func (a anchorResolver) Resolve(_, name, _, _ string) (string, bool) {
	p, ok := a.pathByName[strings.ToLower(name)]
	return p, ok
}

func (a anchorResolver) ResolveAnchor(path, heading, block string) (int, bool) {
	m, ok := a.lines[path]
	if !ok {
		return 0, false
	}
	if block != "" {
		l, ok := m["^"+block]
		return l, ok
	}
	if heading != "" {
		l, ok := m["#"+slugify(heading)]
		return l, ok
	}
	return 0, false
}

func TestCountUnresolvedWikilinks_BrokenAnchorCounts(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(anchorResolver{
		pathByName: map[string]string{"note": "/notes/note.md"},
		lines:      map[string]map[string]int{"/notes/note.md": {"^foo": 7}},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	src := "[[Note^foo]] and [[Note^missing]] and [[Note#unknown-heading]]"
	if got := r.CountUnresolvedWikilinks(src); got != 2 {
		t.Errorf("CountUnresolvedWikilinks = %d, want 2", got)
	}
}

func TestPreprocessWikilinks_BlockAnchorPreserved(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(anchorResolver{
		pathByName: map[string]string{"note": "/notes/note.md"},
		lines:      map[string]map[string]int{"/notes/note.md": {"^foo": 7}},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	got := r.preprocessWikilinks("See [[Note^foo]]")
	if !strings.Contains(got, "/notes/note.md#^foo") {
		t.Errorf("expected block anchor in href; got %q", got)
	}
}

func TestPreprocessWikilinks_BrokenAnchor_RendersBroken(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(anchorResolver{
		pathByName: map[string]string{"note": "/notes/note.md"},
		lines:      map[string]map[string]int{"/notes/note.md": {}},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	got := r.preprocessWikilinks("See [[Note#missing]]")
	if !strings.Contains(got, "?") {
		t.Errorf("expected broken marker '?' in output; got %q", got)
	}
}

func TestPreprocessBlockMarkers_StripsOutsideFences(t *testing.T) {
	src := "First paragraph. ^p1\n\n```\nin code ^kept\n```\n\nSecond. ^p2\n"
	got := preprocessBlockMarkers(src)
	want := "First paragraph.\n\n```\nin code ^kept\n```\n\nSecond.\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}
