package vault_test

import (
	"fmt"
	"testing"

	"github.com/wilkes/hypogeum/internal/benchcorpus"
	"github.com/wilkes/hypogeum/internal/vault"
)

func BenchmarkBuild(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			c := benchcorpus.Generate(b.TempDir(), 7, n, 5)
			for b.Loop() {
				if _, err := vault.Build(c.Root, vault.NopDiagnostics{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkRefreshFile measures the cost of a single FileModified refresh
// against vault size — the work done on every editor save. The re-read and
// re-parse of the one changed file is constant; what scales with N is the
// wikilink re-resolution, so growth across N exposes that cost.
func BenchmarkRefreshFile(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			c := benchcorpus.Generate(b.TempDir(), 7, n, 5)
			v, err := vault.Build(c.Root, vault.NopDiagnostics{})
			if err != nil {
				b.Fatal(err)
			}
			for b.Loop() {
				if err := v.RefreshFile(c.Target); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
