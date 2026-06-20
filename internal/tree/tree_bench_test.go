package tree_test

import (
	"fmt"
	"testing"

	"github.com/wilkes/hypogeum/internal/benchcorpus"
	"github.com/wilkes/hypogeum/internal/tree"
)

func BenchmarkWalk(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			c := benchcorpus.Generate(b.TempDir(), 7, n, 3)
			for b.Loop() {
				if _, err := tree.Walk(c.Root); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
