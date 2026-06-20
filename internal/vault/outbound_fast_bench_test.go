package vault_test

import (
	"fmt"
	"testing"

	"github.com/wilkes/hypogeum/internal/benchcorpus"
	"github.com/wilkes/hypogeum/internal/vault"
)

// BenchmarkOutboundFullBuild is the old links path: build the whole vault, then
// read one file's outbound links.
func BenchmarkOutboundFullBuild(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			c := benchcorpus.Generate(b.TempDir(), 7, n, 5)
			b.ReportAllocs()
			for b.Loop() {
				v, err := vault.Build(c.Root, vault.NopDiagnostics{})
				if err != nil {
					b.Fatal(err)
				}
				_ = v.Outbound(c.Target)
			}
		})
	}
}

// BenchmarkOutboundFast is the new links path: name-only walk + parse the one
// target file.
func BenchmarkOutboundFast(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			c := benchcorpus.Generate(b.TempDir(), 7, n, 5)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := vault.OutboundFor(c.Root, c.Target, vault.NopDiagnostics{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
