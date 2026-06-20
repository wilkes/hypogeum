package search_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wilkes/hypogeum/internal/benchcorpus"
	"github.com/wilkes/hypogeum/internal/search"
)

// BenchmarkSearchLargeFiles stresses the per-LINE cost of scanFile: many
// short prose lines per file, the search token on exactly one of them. Every
// line is scanned (lowercased) whether or not it matches, so this isolates the
// per-line scan/allocation cost that grows with file size — the lever behind
// the "2× on realistic notes" finding in docs/benchmarking.md.
func BenchmarkSearchLargeFiles(b *testing.B) {
	const unlimited = 1 << 30
	const files = 500
	const lines = 400 // ~80 B/line ⇒ ~32 KB/file
	dir := b.TempDir()
	paths := makeLineHeavyCorpus(b, dir, files, lines)
	ctx := context.Background()
	for b.Loop() {
		hits, err := search.Search(ctx, paths, benchcorpus.SearchToken, unlimited)
		if err != nil {
			b.Fatal(err)
		}
		if len(hits) != files {
			b.Fatalf("want %d hits (one per file), got %d", files, len(hits))
		}
	}
}

// makeLineHeavyCorpus writes files of `lines` prose lines each, with the
// search token on the middle line (one hit per file). Deterministic.
func makeLineHeavyCorpus(b *testing.B, dir string, files, lines int) []string {
	b.Helper()
	// Mixed-case, like real prose: forces strings.ToLower off its
	// already-lowercase fast path so the per-line lowercasing cost is real.
	const prose = "The Vault Renders Cursor Modals With Glamour Sentinels, Backlinks, And Wikilinks In Terminal Markdown Viewports."
	paths := make([]string, files)
	for i := 0; i < files; i++ {
		var sb strings.Builder
		for l := 0; l < lines; l++ {
			if l == lines/2 {
				sb.WriteString(benchcorpus.SearchToken)
				sb.WriteByte('\n')
			}
			sb.WriteString(prose)
			sb.WriteByte('\n')
		}
		p := filepath.Join(dir, fmt.Sprintf("big-%04d.md", i))
		if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
			b.Fatal(err)
		}
		paths[i] = p
	}
	return paths
}

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
