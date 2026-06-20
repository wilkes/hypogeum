package recent_test

import (
	"fmt"
	"testing"

	"github.com/wilkes/hypogeum/internal/benchcorpus"
	"github.com/wilkes/hypogeum/internal/recent"
)

func BenchmarkRank(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			c := benchcorpus.Generate(b.TempDir(), 7, n, 0)
			store, err := recent.New("") // in-memory, no state file
			if err != nil {
				b.Fatal(err)
			}
			for _, p := range c.Files {
				if err := store.Record(p); err != nil {
					b.Fatal(err)
				}
			}
			for b.Loop() {
				_ = store.Rank(c.Files)
			}
		})
	}
}
