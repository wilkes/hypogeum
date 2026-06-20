package search_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/wilkes/hypogeum/internal/benchcorpus"
	"github.com/wilkes/hypogeum/internal/search"
)

func BenchmarkSearch(b *testing.B) {
	const unlimited = 1 << 30 // large cap so the full fan-out scan runs
	for _, n := range []int{10, 100, 1000, 5000, 10000} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			c := benchcorpus.Generate(b.TempDir(), 7, n, 3)
			ctx := context.Background()
			for b.Loop() {
				hits, err := search.Search(ctx, c.Files, benchcorpus.SearchToken, unlimited)
				if err != nil {
					b.Fatal(err)
				}
				if len(hits) != n {
					b.Fatalf("want %d hits (one per file), got %d", n, len(hits))
				}
			}
		})
	}
}
