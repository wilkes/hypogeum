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
