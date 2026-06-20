# Benchmarking Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land a deterministic synthetic-vault corpus generator plus measure-only benchmarks across hypogeum's five hot paths, then write up the findings.

**Architecture:** One new shared package (`internal/benchcorpus`) generates a reproducible markdown vault on disk. Each subsystem gets a `*_bench_test.go` file next to the code it tests, all importing the corpus. Benchmarks run across input sizes (N=10/100/1000) via sub-benchmarks to expose complexity curves. No production code changes; CI is untouched because benchmarks only run under `-bench`.

**Tech Stack:** Go 1.24 (`testing.B`, `b.Loop()`), `math/rand` (seeded), `benchstat` for future A/B comparisons.

## Global Constraints

- **Go 1.24.5** — `for b.Loop()` is the loop idiom (setup before the first `b.Loop()` call is excluded from timing; no manual `b.ResetTimer()` needed).
- **Module path:** `github.com/wilkes/hypogeum`.
- **Measure-only.** No production code may change. No optimization. Additive test + doc files exclusively.
- **Determinism:** the corpus generator uses a seeded `*rand.Rand` (constructed via `rand.NewSource(seed)`) — never the global `math/rand` source.
- **Timing hygiene:** generate the corpus / build any `Store` / read any file *before* the `for b.Loop()` loop so fixture cost is excluded.
- **Tests live next to the code they test** (repo convention). Benchmark files use the same package as the code under test where they already have white-box tests; otherwise an external `_test` package is fine since all symbols used are exported.
- **All benchmarks run with `-benchmem`.** For `search` and `recent`, allocs/op is the headline metric.
- Commit messages end with the repo's Co-Authored-By / Claude-Session trailers (see existing history); merge via `gh pr merge --merge`, never squash.

---

### Task 1: Corpus generator (`internal/benchcorpus`)

**Files:**
- Create: `internal/benchcorpus/corpus.go`
- Test: `internal/benchcorpus/corpus_test.go`

**Interfaces:**
- Consumes: stdlib only (`fmt`, `math/rand`, `os`, `path/filepath`, `strings`).
- Produces:
  - `const SearchToken = "hypogeumtoken"` — embedded once per file; a full-text search for it yields exactly one hit per file.
  - `type Corpus struct { Root string; Files []string; Target string }`
  - `func Generate(dir string, seed int64, n, linkDensity int) Corpus` — writes `n` files into `dir`, returns the corpus. `Target` is `Files[n/2]`.

- [ ] **Step 1: Write the failing tests**

`internal/benchcorpus/corpus_test.go`:

```go
package benchcorpus

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestGenerate_Deterministic(t *testing.T) {
	a := Generate(t.TempDir(), 42, 20, 3)
	b := Generate(t.TempDir(), 42, 20, 3)
	if len(a.Files) != len(b.Files) {
		t.Fatalf("file count differs: %d vs %d", len(a.Files), len(b.Files))
	}
	for i := range a.Files {
		da, err := os.ReadFile(a.Files[i])
		if err != nil {
			t.Fatal(err)
		}
		db, err := os.ReadFile(b.Files[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(da, db) {
			t.Fatalf("file %d bytes differ between runs with same seed", i)
		}
	}
}

func TestGenerate_Invariants(t *testing.T) {
	c := Generate(t.TempDir(), 1, 15, 2)
	if len(c.Files) != 15 {
		t.Fatalf("want 15 files, got %d", len(c.Files))
	}
	if c.Target == "" {
		t.Fatal("Target unset")
	}
	for _, p := range c.Files {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), SearchToken) {
			t.Errorf("%s missing SearchToken", p)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/benchcorpus/`
Expected: FAIL — `undefined: Generate` / `undefined: SearchToken`.

- [ ] **Step 3: Write the generator**

`internal/benchcorpus/corpus.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/benchcorpus/`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/benchcorpus/
git commit -m "test(bench): deterministic synthetic-vault corpus generator"
```

---

### Task 2: `tree.Walk` benchmark

**Files:**
- Test: `internal/tree/tree_bench_test.go`

**Interfaces:**
- Consumes: `benchcorpus.Generate`, `Corpus.Root`; `tree.Walk(root string) (*Node, error)`.
- Produces: `BenchmarkWalk` (cold-start filesystem walk + flatten).

- [ ] **Step 1: Write the benchmark**

`internal/tree/tree_bench_test.go`:

```go
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
```

- [ ] **Step 2: Run it to verify it executes**

Run: `go test -run=^$ -bench=BenchmarkWalk -benchmem -benchtime=1x ./internal/tree/`
Expected: PASS — three sub-benchmark lines (`BenchmarkWalk/N=10`, `/N=100`, `/N=1000`) with ns/op and allocs/op.

- [ ] **Step 3: Commit**

```bash
git add internal/tree/tree_bench_test.go
git commit -m "test(bench): benchmark tree.Walk across vault sizes"
```

---

### Task 3: `vault.Build` benchmark

**Files:**
- Test: `internal/vault/vault_bench_test.go`

**Interfaces:**
- Consumes: `benchcorpus.Generate`, `Corpus.Root`; `vault.Build(root string, diag Diagnostics) (*Vault, error)`; `vault.NopDiagnostics{}`.
- Produces: `BenchmarkBuild` (wikilink/backlink index build; the O(n²) suspect — note the higher `linkDensity`).

- [ ] **Step 1: Write the benchmark**

`internal/vault/vault_bench_test.go`:

```go
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
```

- [ ] **Step 2: Run it to verify it executes**

Run: `go test -run=^$ -bench=BenchmarkBuild -benchmem -benchtime=1x ./internal/vault/`
Expected: PASS — three sub-benchmark lines.

- [ ] **Step 3: Commit**

```bash
git add internal/vault/vault_bench_test.go
git commit -m "test(bench): benchmark vault.Build across vault sizes"
```

---

### Task 4: `markdown.RenderWithLinks` benchmark

**Files:**
- Test: `internal/markdown/render_bench_test.go`

**Interfaces:**
- Consumes: `benchcorpus.Generate`, `Corpus.Target`; `markdown.NewRenderer(width int, opts ...Option) (*Renderer, error)`; `(*Renderer).RenderWithLinks(src, base string, marker LinkMarker) (string, []Link, []string, error)`; `markdown.HighlightMarker(selected int) LinkMarker`.
- Produces: `BenchmarkRenderWithLinks` (single-doc render; renderer built once at width 80; `HighlightMarker(-1)` selects nothing).

- [ ] **Step 1: Write the benchmark**

`internal/markdown/render_bench_test.go`:

```go
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
```

- [ ] **Step 2: Run it to verify it executes**

Run: `go test -run=^$ -bench=BenchmarkRenderWithLinks -benchmem -benchtime=1x ./internal/markdown/`
Expected: PASS — one benchmark line with ns/op and allocs/op.

- [ ] **Step 3: Commit**

```bash
git add internal/markdown/render_bench_test.go
git commit -m "test(bench): benchmark markdown.RenderWithLinks on a single doc"
```

---

### Task 5: `search.Search` benchmark

**Files:**
- Test: `internal/search/search_bench_test.go`

**Interfaces:**
- Consumes: `benchcorpus.Generate`, `Corpus.Files`, `benchcorpus.SearchToken`; `search.Search(ctx context.Context, paths []string, query string, maxHits int) ([]Hit, error)`.
- Produces: `BenchmarkSearch` (per-keystroke full-text scan). **`maxHits` must be large** (`1 << 30`) — `Search` returns `nil, nil` immediately when `maxHits <= 0`, which would measure an early return instead of the scan.

- [ ] **Step 1: Write the benchmark**

`internal/search/search_bench_test.go`:

```go
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
	for _, n := range []int{10, 100, 1000} {
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
```

- [ ] **Step 2: Run it to verify it executes**

Run: `go test -run=^$ -bench=BenchmarkSearch -benchmem -benchtime=1x ./internal/search/`
Expected: PASS — three sub-benchmark lines; the `len(hits) != n` guard confirms the scan actually ran (one hit per file via `SearchToken`).

- [ ] **Step 3: Commit**

```bash
git add internal/search/search_bench_test.go
git commit -m "test(bench): benchmark search.Search across vault sizes"
```

---

### Task 6: `recent.Store.Rank` benchmark

**Files:**
- Test: `internal/recent/recent_bench_test.go`

**Interfaces:**
- Consumes: `benchcorpus.Generate`, `Corpus.Files`; `recent.New(stateFile string) (*Store, error)` (pass `""` for an in-memory store — `saveLocked` no-ops on an empty state file); `(*Store).Record(path string) error`; `(*Store).Rank(paths []string) []Ranked`.
- Produces: `BenchmarkRank` (result re-ranking; `Store` built and populated outside the timed loop). `linkDensity` is `0` — links are irrelevant to ranking.

- [ ] **Step 1: Write the benchmark**

`internal/recent/recent_bench_test.go`:

```go
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
```

- [ ] **Step 2: Run it to verify it executes**

Run: `go test -run=^$ -bench=BenchmarkRank -benchmem -benchtime=1x ./internal/recent/`
Expected: PASS — three sub-benchmark lines.

- [ ] **Step 3: Commit**

```bash
git add internal/recent/recent_bench_test.go
git commit -m "test(bench): benchmark recent.Store.Rank across result sizes"
```

---

### Task 7: Run the sweep and write `docs/benchmarking.md`

**Files:**
- Create: `docs/benchmarking.md`
- Modify: `docs/index.md` (add a link under "Active feature work" or a new "Tooling" bullet)

**Interfaces:**
- Consumes: all benchmarks from Tasks 2–6.
- Produces: the how-to-run doc + the findings writeup (the deliverable of this measure-only pass).

- [ ] **Step 1: Confirm the whole suite compiles and the unit tests still pass**

Run: `go build ./... && go test ./...`
Expected: PASS — benchmarks don't run under plain `go test`, so this just confirms everything compiles and `benchcorpus`'s unit tests pass.

- [ ] **Step 2: Run the full sweep and capture output**

Run: `go test -run=^$ -bench=. -benchmem ./... | tee /tmp/bench.txt`
Expected: ns/op + allocs/op for every benchmark across N=10/100/1000 (single line for `RenderWithLinks`). Keep `/tmp/bench.txt` for the writeup.

- [ ] **Step 3: Write `docs/benchmarking.md`**

Create `docs/benchmarking.md` with these sections (fill the Findings table from `/tmp/bench.txt`):

````markdown
# Benchmarking

Measure-only benchmarks across hypogeum's five hot paths, over a deterministic
synthetic corpus (`internal/benchcorpus`). See the
[design spec](superpowers/specs/2026-06-20-benchmarking-foundation-design.md).

## Running

```sh
# Full sweep with allocations
go test -run=^$ -bench=. -benchmem ./...

# One subsystem
go test -run=^$ -bench=BenchmarkBuild -benchmem ./internal/vault/

# Stable A/B for an optimization: capture baseline, change code, compare
go test -run=^$ -bench=. -count=10 ./internal/vault/ > old.txt
# ...edit...
go test -run=^$ -bench=. -count=10 ./internal/vault/ > new.txt
benchstat old.txt new.txt   # go install golang.org/x/perf/cmd/benchstat@latest

# Profile a hot path
go test -run=^$ -bench=BenchmarkSearch -cpuprofile=cpu.out ./internal/search/
go tool pprof cpu.out
```

Benchmarks never run under plain `go test` (they need `-bench`), so CI is
unaffected.

## The corpus

`internal/benchcorpus.Generate(dir, seed, n, linkDensity)` writes `n`
markdown files into a temp dir with a fixed RNG seed — byte-identical across
runs. Each file carries headings, prose, `[[wikilinks]]` at the requested
density, an inline code fence, and one `SearchToken` (so a search yields
exactly one hit per file). Benchmarks vary `n` over 10/100/1000 to expose
complexity curves.

## Findings (run on <date / machine>)

| Benchmark | N=10 | N=100 | N=1000 | allocs/op @ N=1000 | Note |
|-----------|------|-------|--------|--------------------|------|
| tree.Walk | … | … | … | … | |
| vault.Build | … | … | … | … | scaling vs N? |
| search.Search | … | … | … | … | allocs/keystroke |
| recent.Rank | … | … | … | … | allocs/keystroke |
| markdown.RenderWithLinks | (single doc) | — | — | … | |

Surprises: <bullet list — e.g. superlinear scaling, allocation hot spots>.
Follow-up candidates (separate branches, justified by benchstat): <list>.
````

- [ ] **Step 4: Fill the Findings table from `/tmp/bench.txt`**

Transcribe the ns/op and allocs/op numbers into the table. Note the machine and date in the heading. Write 1–3 bullets on the most interesting surprise (scaling shape, allocation hot spot). List any optimization follow-ups — but do not implement them (measure-only pass).

- [ ] **Step 5: Link from `docs/index.md`**

Add under the existing doc index (a "Tooling" bullet or alongside Active feature work):

```markdown
- [Benchmarking](benchmarking.md) — how to run the hot-path benchmarks (`internal/benchcorpus` corpus + per-package `*_bench_test.go`) and the latest findings.
```

- [ ] **Step 6: Commit**

```bash
git add docs/benchmarking.md docs/index.md
git commit -m "docs(bench): run instructions and findings for the hot-path sweep"
```

---

## Self-Review

**Spec coverage:**
- Corpus generator (deterministic, scale-parameterized, fixed seed, wikilink density) → Task 1. ✓
- Five benchmark suites (tree.Walk, vault.Build, markdown.RenderWithLinks, search.Search, recent.Store.Rank) → Tasks 2–6. ✓
- `-benchmem` / allocs-as-headline for hot-loop paths → noted in each task + Global Constraints. ✓
- Timing hygiene (fixture outside the timed loop) → Global Constraints + each task generates before `b.Loop()`. ✓
- Run + findings workflow, `docs/benchmarking.md`, `benchstat`/pprof pointers → Task 7. ✓
- Measure-only, additive, no CI change → Global Constraints; Task 7 Step 1 confirms `go test ./...` stays green. ✓

**Placeholder scan:** The `<date>` / `…` cells in `docs/benchmarking.md` are intentional — they're filled at runtime in Task 7 Steps 3–4 from real benchmark output, which can't be known when writing the plan. Every code step has complete code. No "TBD"/"handle errors"/"similar to Task N".

**Type consistency:** `Corpus{Root, Files, Target}`, `Generate(dir, seed, n, linkDensity)`, and `SearchToken` are used identically in Tasks 2–6 as defined in Task 1. Verified against source: `tree.Walk(root)`, `vault.Build(root, diag)` + `vault.NopDiagnostics{}`, `markdown.NewRenderer(width)` + `(*Renderer).RenderWithLinks` (4 returns) + `HighlightMarker(int)`, `search.Search(ctx, paths, query, maxHits)` (returns `nil,nil` when `maxHits<=0` → uses `1<<30`), `recent.New("")` + `(*Store).Record` + `(*Store).Rank`.
