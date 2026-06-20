package markdown_test

import (
	"os"
	"testing"

	"github.com/wilkes/hypogeum/internal/benchcorpus"
	"github.com/wilkes/hypogeum/internal/markdown"
)

func BenchmarkRenderWithLinks(b *testing.B) {
	c := benchcorpus.Generate(b.TempDir(), 7, 50, 4)
	src, err := os.ReadFile(c.Target)
	if err != nil {
		b.Fatal(err)
	}
	r, err := markdown.NewRenderer(80)
	if err != nil {
		b.Fatal(err)
	}
	marker := markdown.HighlightMarker(-1) // no link selected
	for b.Loop() {
		if _, _, _, err := r.RenderWithLinks(string(src), c.Target, marker); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWithHighlight(b *testing.B) {
	c := benchcorpus.Generate(b.TempDir(), 7, 50, 4)
	body, err := os.ReadFile(c.Target)
	if err != nil {
		b.Fatal(err)
	}
	// Prepend an inline link to a local file so there is a cyclable link for
	// the highlight to land on (the corpus body uses wikilinks, which don't
	// enter the link cycler).
	src := "See [anchor](other.md) for details.\n\n" + string(body)

	r, err := markdown.NewRenderer(80)
	if err != nil {
		b.Fatal(err)
	}
	rr, err := r.RenderDocument(src, c.Target, markdown.HighlightMarker(-1))
	if err != nil {
		b.Fatal(err)
	}
	if len(rr.Links) == 0 {
		b.Fatal("expected at least the prepended inline link")
	}

	for b.Loop() {
		_ = rr.WithHighlight(0)
	}
}
