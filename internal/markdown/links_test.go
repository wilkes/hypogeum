package markdown

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExtractLinks_None(t *testing.T) {
	got := ExtractLinks("# Heading\n\nJust some text, no links.\n")
	if len(got) != 0 {
		t.Errorf("expected zero links, got %d: %+v", len(got), got)
	}
}

func TestExtractLinks_InlineLinks_InDocumentOrder(t *testing.T) {
	src := "See [first](one.md) and [second](two.md), then [third](three.md).\n"
	got := ExtractLinks(src)
	want := []ASTLink{
		{Text: "first", Href: "one.md"},
		{Text: "second", Href: "two.md"},
		{Text: "third", Href: "three.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractLinks mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestExtractLinks_NestedAndListItems(t *testing.T) {
	src := "" +
		"# Doc\n\n" +
		"- bullet [a](a.md)\n" +
		"- bullet with **bold [b](b.md)**\n" +
		"\n> Quote with [c](c.md)\n"
	got := ExtractLinks(src)
	want := []ASTLink{
		{Text: "a", Href: "a.md"},
		{Text: "b", Href: "b.md"},
		{Text: "c", Href: "c.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractLinks mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestExtractLinks_ExternalAndAnchor(t *testing.T) {
	src := "[charm](https://charm.sh) and [section](#intro) and [path](./other.md)\n"
	got := ExtractLinks(src)
	want := []ASTLink{
		{Text: "charm", Href: "https://charm.sh"},
		{Text: "section", Href: "#intro"},
		{Text: "path", Href: "./other.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractLinks mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestExtractLinks_AutolinkUsesURLAsText(t *testing.T) {
	// goldmark surfaces <https://example.com> as a link with the URL as text.
	src := "Visit <https://example.com> today.\n"
	got := ExtractLinks(src)
	want := []ASTLink{
		{Text: "https://example.com", Href: "https://example.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractLinks mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestExtractLinks_IgnoresImages(t *testing.T) {
	// Images use the same brackets-and-paren syntax but should not be
	// surfaced as followable links.
	src := "![alt](img.png) and a real [link](real.md)\n"
	got := ExtractLinks(src)
	want := []ASTLink{
		{Text: "link", Href: "real.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractLinks mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestResolveLink_LineRange(t *testing.T) {
	got := ResolveLink("/base/notes.md", "code/main.go#L10-L20")
	if got.Kind != LinkLocalFile {
		t.Fatalf("kind = %v, want LinkLocalFile", got.Kind)
	}
	if got.Range == nil {
		t.Fatalf("range is nil")
	}
	if got.Range.Start != 10 || got.Range.End != 20 {
		t.Fatalf("range = %+v", got.Range)
	}
	if got.Anchor != "" {
		t.Fatalf("anchor = %q, want empty (line range claims the fragment)", got.Anchor)
	}
}

func TestResolveLink_SingleLine(t *testing.T) {
	got := ResolveLink("/base/notes.md", "main.go#L5")
	if got.Range == nil || got.Range.Start != 5 || got.Range.End != 5 {
		t.Fatalf("range = %+v", got.Range)
	}
}

func TestResolveLink_NonLineFragmentIsStillAnchor(t *testing.T) {
	got := ResolveLink("/base/notes.md", "page.md#some-heading")
	if got.Range != nil {
		t.Fatalf("range = %+v, want nil", got.Range)
	}
	if got.Anchor != "some-heading" {
		t.Fatalf("anchor = %q", got.Anchor)
	}
}

func TestIsBrokenLocalLink(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.md")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if IsBrokenLocalLink(existing) {
		t.Errorf("existing file reported broken")
	}
	if !IsBrokenLocalLink(filepath.Join(dir, "ghost.md")) {
		t.Errorf("missing file reported not-broken")
	}
	if !IsBrokenLocalLink("") {
		t.Errorf("empty path reported not-broken")
	}
}
