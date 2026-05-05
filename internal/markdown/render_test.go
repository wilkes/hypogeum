package markdown

import (
	"path/filepath"
	"strings"
	"testing"
)

// rendererForTest constructs a Renderer suitable for deterministic tests.
// Production callers go through NewRenderer which uses auto-style.
func rendererForTest(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer(80)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	return r
}

func TestRenderWithLinks_NoLinks(t *testing.T) {
	r := rendererForTest(t)
	out, links, err := r.RenderWithLinks("# Heading\n\nJust text.\n", "/base/dir/file.md")
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected zero links, got %d", len(links))
	}
	// Sentinel bytes must not leak into output.
	if strings.ContainsRune(out, sentinelStart) || strings.ContainsRune(out, sentinelEnd) {
		t.Errorf("sentinel bytes leaked into output: %q", out)
	}
}

func TestRenderWithLinks_LocalFileResolved(t *testing.T) {
	r := rendererForTest(t)
	base := "/notes/index.md"
	src := "See [first link](one.md) for more.\n"
	_, links, err := r.RenderWithLinks(src, base)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d: %+v", len(links), links)
	}
	got := links[0]
	if got.Text != "first link" {
		t.Errorf("Text = %q, want %q", got.Text, "first link")
	}
	if got.Href != "one.md" {
		t.Errorf("Href = %q, want %q", got.Href, "one.md")
	}
	wantTarget := filepath.Join("/notes", "one.md")
	if got.Resolved.Kind != LinkLocalFile || got.Resolved.Target != wantTarget {
		t.Errorf("Resolved = %+v, want LinkLocalFile %q", got.Resolved, wantTarget)
	}
}

func TestRenderWithLinks_OrderPreservedAcrossKinds(t *testing.T) {
	r := rendererForTest(t)
	src := "[a](a.md), [b](https://x.test), [c](#anchor), [d](sub/d.md)\n"
	_, links, err := r.RenderWithLinks(src, "/base/file.md")
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 4 {
		t.Fatalf("want 4 links, got %d: %+v", len(links), links)
	}

	wantHrefs := []string{"a.md", "https://x.test", "#anchor", "sub/d.md"}
	for i, want := range wantHrefs {
		if links[i].Href != want {
			t.Errorf("links[%d].Href = %q, want %q", i, links[i].Href, want)
		}
	}
	wantKinds := []LinkKind{LinkLocalFile, LinkExternal, LinkAnchor, LinkLocalFile}
	for i, want := range wantKinds {
		if links[i].Resolved.Kind != want {
			t.Errorf("links[%d].Resolved.Kind = %v, want %v", i, links[i].Resolved.Kind, want)
		}
	}
}

func TestRenderWithLinks_RowReflectsRenderedPosition(t *testing.T) {
	r := rendererForTest(t)
	// Two links with several lines between them; the second link's Row
	// must be greater than the first link's Row.
	src := "" +
		"# Doc\n\n" +
		"First paragraph with [first](one.md).\n\n" +
		"Second paragraph.\n\n" +
		"Third paragraph.\n\n" +
		"Final paragraph with [second](two.md).\n"
	_, links, err := r.RenderWithLinks(src, "/base/file.md")
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("want 2 links, got %d", len(links))
	}
	if links[0].Row >= links[1].Row {
		t.Errorf("expected links[0].Row (%d) < links[1].Row (%d)", links[0].Row, links[1].Row)
	}
	if links[0].Row < 0 {
		t.Errorf("links[0].Row = %d, want >= 0", links[0].Row)
	}
}

func TestRenderWithLinks_OutputIsCleanRender(t *testing.T) {
	// The instrumented render should be visually equivalent to the regular
	// render — same visible text, just with sentinels stripped. We verify
	// by stripping ANSI from both and comparing.
	r := rendererForTest(t)
	src := "# Doc\n\nSee [first](one.md) and [second](two.md).\n"

	plain, err := r.Render(src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	instrumented, _, err := r.RenderWithLinks(src, "/base/file.md")
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}

	if stripANSI(plain) != stripANSI(instrumented) {
		t.Errorf("instrumented render differs from plain render after ANSI strip\n plain: %q\ninstrumented: %q", stripANSI(plain), stripANSI(instrumented))
	}
}

// stripANSI removes CSI sequences. Used in tests to compare visible output.
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
