// Package benchcorpus generates a deterministic synthetic markdown vault on
// disk for benchmarking. Same (seed, n, linkDensity) produces byte-identical
// files, so benchmark numbers stay comparable run to run.
package benchcorpus

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

// SearchToken is embedded once in every generated file. A full-text search
// for it therefore yields exactly one hit per file — a predictable corpus
// for search benchmarks.
const SearchToken = "hypogeumtoken"

// vocab is the fixed word pool for generated prose. Kept small and themed so
// generated files read like real notes without pulling in a dictionary.
var vocab = []string{
	"vault", "render", "cursor", "modal", "glamour", "sentinel",
	"backlink", "wikilink", "terminal", "markdown", "viewport", "fuzzy",
}

// Corpus is a generated synthetic vault on disk.
type Corpus struct {
	Root   string   // directory holding the .md files
	Files  []string // absolute paths in generation order
	Target string   // a pre-picked file for single-doc benchmarks (Files[n/2])
}

// Generate writes n markdown files into dir using an RNG seeded by seed and
// returns the Corpus. dir is expected to be a testing TempDir. linkDensity is
// the number of [[wikilinks]] emitted per file. It panics on a write error —
// acceptable in a benchmark/test helper.
func Generate(dir string, seed int64, n, linkDensity int) Corpus {
	rng := rand.New(rand.NewSource(seed))
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("note-%04d", i)
	}
	c := Corpus{Root: dir, Files: make([]string, n)}
	for i := 0; i < n; i++ {
		var b strings.Builder
		fmt.Fprintf(&b, "# %s\n\n%s\n\n", names[i], SearchToken)
		for s := 0; s < 3; s++ {
			fmt.Fprintf(&b, "## Section %d\n\n%s\n\n", s, paragraph(rng))
		}
		for l := 0; l < linkDensity; l++ {
			fmt.Fprintf(&b, "See [[%s]].\n", names[rng.Intn(n)])
		}
		b.WriteString("\n```go\nfunc main() {}\n```\n")
		path := filepath.Join(dir, names[i]+".md")
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			panic(err)
		}
		c.Files[i] = path
	}
	c.Target = c.Files[n/2]
	return c
}

// paragraph builds a deterministic 24-word sentence from vocab.
func paragraph(rng *rand.Rand) string {
	words := make([]string, 24)
	for i := range words {
		words[i] = vocab[rng.Intn(len(vocab))]
	}
	return strings.Join(words, " ") + "."
}
