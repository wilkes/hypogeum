package vault

import (
	"strings"
	"testing"

	"github.com/wilkes/hypogeum/internal/highlight"
)

func TestSnippet_ParagraphContext(t *testing.T) {
	src := "before. Here we link to [[Foo]] in a sentence. after."
	refs := extractReferences(src, "/x.md")
	if len(refs) != 1 {
		t.Fatalf("refs: got %d want 1", len(refs))
	}
	if !strings.Contains(refs[0].snippet, "Here we link to") {
		t.Fatalf("snippet missing surrounding text: %q", refs[0].snippet)
	}
	if !strings.Contains(refs[0].snippet, "Foo") {
		t.Fatalf("snippet missing display text: %q", refs[0].snippet)
	}
}

func TestSnippet_ListItemContext(t *testing.T) {
	src := `- first item
- item with [[Foo]] inside
- last item`
	refs := extractReferences(src, "/x.md")
	if len(refs) != 1 {
		t.Fatalf("refs: got %d want 1", len(refs))
	}
	// The snippet should be the list item's text only — not the whole list.
	if strings.Contains(refs[0].snippet, "first item") {
		t.Fatalf("snippet leaked sibling list items: %q", refs[0].snippet)
	}
	if !strings.Contains(refs[0].snippet, "item with") {
		t.Fatalf("snippet missing item text: %q", refs[0].snippet)
	}
}

func TestSnippet_HighlightWrapping(t *testing.T) {
	src := "see [[Foo]] now."
	refs := extractReferences(src, "/x.md")
	if len(refs) != 1 {
		t.Fatalf("refs: got %d want 1", len(refs))
	}
	// The snippet wraps the display text in the snippet highlight markers.
	if !strings.Contains(refs[0].snippet, highlight.Open+"Foo"+highlight.Close) {
		t.Fatalf("snippet not wrapped with highlight markers: %q", refs[0].snippet)
	}
}
