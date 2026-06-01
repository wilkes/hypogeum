package markdown

import (
	"strings"
	"testing"
)

// Tables with links inside cells used to render with ragged column edges
// because Glamour sized each cell counting the suppressed URL bytes as
// visible content. After we strip those bytes the cell ended up short by
// "<space>/url" worth of characters. urlSuppressStrip now keeps the
// visible width identical in padding contexts (tables, end-of-line).
//
// These tests pin that invariant: every row of a rendered table has the
// same visible width regardless of what kind of content sits in the cells.

// assertEqualWidths fails the test if the widths slice isn't uniform.
// Skips horizontal rule rows (they're decorated with box-drawing chars
// and aren't expected to match cell-row widths). The "rule row" heuristic
// is: contains the ┼ glyph.
func assertEqualWidths(t *testing.T, out string) {
	t.Helper()
	want := -1
	for _, line := range strings.Split(stripANSI(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.ContainsRune(trimmed, '┼') {
			continue
		}
		w := len([]rune(line))
		if want == -1 {
			want = w
			continue
		}
		if w != want {
			t.Errorf("row width %d, want %d:\n%q\nfull output:\n%s", w, want, line, stripANSI(out))
		}
	}
}

func TestRender_TablePlainAligns(t *testing.T) {
	r := rendererForTest(t)
	src := "" +
		"| Name | Doc          |\n" +
		"| ---- | ------------ |\n" +
		"| Foo  | Foo doc      |\n" +
		"| Bar  | Bar doc      |\n"
	out, err := r.Render(src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertEqualWidths(t, out)
}

// TestRender_TableWithLinkInLastColumn pins the original bug: a link in
// the final column used to render with the cell short by "<space>/url".
func TestRender_TableWithLinkInLastColumn(t *testing.T) {
	r := rendererForTest(t)
	src := "" +
		"| Name | Doc |\n" +
		"| ---- | --- |\n" +
		"| Foo  | [Foo](foo.md) |\n" +
		"| Bar  | [Bar](bar.md) |\n"
	out, err := r.Render(src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertEqualWidths(t, out)
}

// TestRender_TableWithLinkInFirstColumn pins the bug in a non-last
// column: the misalignment used to push the right border out of place.
func TestRender_TableWithLinkInFirstColumn(t *testing.T) {
	r := rendererForTest(t)
	src := "" +
		"| Name | Doc |\n" +
		"| ---- | --- |\n" +
		"| [Foo](foo.md) | Doc one |\n" +
		"| [Bar](bar.md) | Doc two |\n"
	out, err := r.Render(src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertEqualWidths(t, out)
}

func TestRender_TableWithLinksInEveryColumn(t *testing.T) {
	r := rendererForTest(t)
	src := "" +
		"| A | B | C |\n" +
		"| - | - | - |\n" +
		"| [a](a.md) | [b](b.md) | [c](c.md) |\n" +
		"| [d](dd.md) | [e](ee.md) | [f](ff.md) |\n"
	out, err := r.Render(src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertEqualWidths(t, out)
}

// TestRender_TableExternalAndLocalLinksAlign covers the case where one
// cell holds an external https URL (long) and another holds a short
// relative link.
func TestRender_TableExternalAndLocalLinksAlign(t *testing.T) {
	r := rendererForTest(t)
	src := "" +
		"| Name | Link |\n" +
		"| ---- | ---- |\n" +
		"| Foo  | [external](https://example.test/path) |\n" +
		"| Bar  | [local](bar.md) |\n"
	out, err := r.Render(src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertEqualWidths(t, out)
}

// TestRenderWithLinks_TableAligns mirrors TestRender_TableWithLinkInLastColumn
// for the instrumented path. The instrumented renderer was always meant to
// be byte-equivalent to the plain renderer after sentinel-strip, so any
// width regression on this path is a separate symptom of the same root cause.
func TestRenderWithLinks_TableAligns(t *testing.T) {
	r := rendererForTest(t)
	src := "" +
		"| Name | Doc |\n" +
		"| ---- | --- |\n" +
		"| Foo  | [Foo](foo.md) |\n" +
		"| Bar  | [Bar](bar.md) |\n"
	out, links, _, err := r.RenderWithLinks(src, "/base/file.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2", len(links))
	}
	assertEqualWidths(t, out)
}

// TestRender_TableWithLinks_PlainEqualsInstrumented guards the design
// promise that the plain and instrumented renderers produce identical
// visible output. A divergence here means our URL-strip logic drifted
// between the two paths.
func TestRender_TableWithLinks_PlainEqualsInstrumented(t *testing.T) {
	r := rendererForTest(t)
	src := "" +
		"| Col A | Col B |\n" +
		"| ----- | ----- |\n" +
		"| [one](one.md) | text |\n" +
		"| two | [three](three.md) |\n"

	plain, err := r.Render(src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	instr, _, _, err := r.RenderWithLinks(src, "/base/file.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if got, want := stripANSI(instr), stripANSI(plain); got != want {
		t.Errorf("instrumented differs from plain after ANSI strip\nplain:\n%s\ninstrumented:\n%s", want, got)
	}
}

// TestRender_ProseLinkUnaffected guards against regressing the in-line
// prose case while fixing the table case. The leading space before the
// URL should still get collapsed so "See [foo](url) bar" reads as
// "See foo bar", not "See foo  bar".
func TestRender_ProseLinkUnaffected(t *testing.T) {
	r := rendererForTest(t)
	out, err := r.Render("See [foo](foo.md) for more.\n")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	visible := stripANSI(out)
	if !strings.Contains(visible, "See foo for more.") {
		t.Errorf("expected clean prose, got: %q", visible)
	}
	if strings.Contains(visible, "foo  for") {
		t.Errorf("double space leaked through: %q", visible)
	}
}

// TestRender_ProseLinkWithWikilinkResolver covers the wikilink path:
// resolved wikilinks are rewritten to standard markdown links, so they
// go through the same URL-suppress logic and must behave the same.
func TestRender_ProseLinkWithWikilinkResolver(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{
		answers: map[string]string{"Notes": "/abs/notes.md"},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	r.SetFromFile("/abs/source.md")
	out, _, _, err := r.RenderWithLinks("See [[Notes]] above.\n", "/abs/source.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if !strings.Contains(stripANSI(out), "See Notes above.") {
		t.Errorf("expected clean wikilink output, got: %q", stripANSI(out))
	}
}

// TestRender_TableWithWikilinkAligns guards that resolved wikilinks
// inside a table cell preserve cell width too — the wikilink path
// rewrites to a markdown link before Glamour sees it, so it should
// follow exactly the same code path.
func TestRender_TableWithWikilinkAligns(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{
		answers: map[string]string{"Foo": "/abs/foo.md", "Bar": "/abs/bar.md"},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	r.SetFromFile("/abs/source.md")
	src := "" +
		"| Name | Doc |\n" +
		"| ---- | --- |\n" +
		"| n1   | [[Foo]] |\n" +
		"| n2   | [[Bar]] |\n"
	out, _, _, err := r.RenderWithLinks(src, "/abs/source.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	assertEqualWidths(t, out)
}

// TestRender_TableCellWithInlineCodeWikilinkHidesURL is the table-cell
// shape of TestRenderWithLinks_WikilinkInInlineCodeIsVerbatim: a cell
// whose content is a comma-separated list of `[[Name]]` tokens (the
// docs-concept-extraction table pattern) used to leak full absolute
// paths into the rendered cell once Glamour ≥0.10.0 started wrapping
// long cells across rows.
func TestRender_TableCellWithInlineCodeWikilinkHidesURL(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{
		answers: map[string]string{
			"markdown":       "/Users/wilkes/Projects/wilkes/hypogeum/docs/packages/markdown.md",
			"link-following": "/Users/wilkes/Projects/wilkes/hypogeum/docs/link-following.md",
		},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	r.SetFromFile("/abs/source.md")
	src := "" +
		"| Concept | Referrers |\n" +
		"| ------- | --------- |\n" +
		"| `sentinel-render.md` | `[[markdown]]`, `[[link-following]]` |\n"
	out, _, _, err := r.RenderWithLinks(src, "/abs/source.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	visible := stripANSI(out)
	if strings.Contains(visible, "/Users/wilkes") {
		t.Fatalf("URL leaked into wrapped table cell:\n%s", visible)
	}
	for _, want := range []string{"[[markdown]]", "[[link-following]]"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("expected literal %q in cell, got:\n%s", want, visible)
		}
	}
}

// TestIsPaddingContextAfter checks the discriminator that decides
// preserve-width vs strip-cleanly. The cases mirror the three real-world
// shapes we observed from Glamour: prose mid-sentence, prose EOL, and
// table cell (with and without a following column border).
func TestIsPaddingContextAfter(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		idx  int
		want bool
	}{
		{"prose mid-sentence", " for more.", 0, false},
		{"prose end of line", "   \n", 0, true},
		{"table cell next column", "   │next", 0, true},
		{"table cell with ansi padding then border", "\x1b[0m   │", 0, true},
		{"end of string", "   ", 0, true},
		{"immediate newline", "\n", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isPaddingContextAfter(c.raw, c.idx); got != c.want {
				t.Errorf("isPaddingContextAfter(%q, %d) = %v, want %v", c.raw, c.idx, got, c.want)
			}
		})
	}
}
