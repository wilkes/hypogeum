package vault_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wilkes/hypogeum/internal/benchcorpus"
	"github.com/wilkes/hypogeum/internal/vault"
)

// BenchmarkBuildLargeFiles isolates the per-BYTE cost of Build: fewer files,
// each a large prose document with a constant link count. With big files the
// fixed per-file overhead (os.Open) amortizes and goldmark parsing dominates,
// the opposite of BenchmarkBuild's small-file regime where open() syscalls do.
func BenchmarkBuildLargeFiles(b *testing.B) {
	const files = 300
	const paras = 300 // ~110 B/para ⇒ ~33 KB/file
	dir := b.TempDir()
	makeLargeDocs(b, dir, files, paras)
	for b.Loop() {
		if _, err := vault.Build(dir, vault.NopDiagnostics{}); err != nil {
			b.Fatal(err)
		}
	}
}

func makeLargeDocs(b *testing.B, dir string, files, paras int) {
	b.Helper()
	const para = "The vault renders cursor modals with glamour sentinels, backlinks, and wikilinks in terminal markdown viewports.\n\n"
	for i := 0; i < files; i++ {
		var sb strings.Builder
		fmt.Fprintf(&sb, "# note-%04d\n\n", i)
		for p := 0; p < paras; p++ {
			sb.WriteString(para)
		}
		for l := 0; l < 5; l++ {
			fmt.Fprintf(&sb, "See [[note-%04d]].\n", (i*7+l)%files)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("doc-%04d.md", i)), []byte(sb.String()), 0o644); err != nil {
			b.Fatal(err)
		}
	}
}

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
