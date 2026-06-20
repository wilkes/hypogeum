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
