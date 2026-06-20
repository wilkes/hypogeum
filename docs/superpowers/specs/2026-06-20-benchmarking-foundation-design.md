# Benchmarking foundation — design

Status: approved, pre-implementation.

## Goal

Establish a benchmarking foundation across hypogeum's hot paths to **measure
and find surprises**. This is pure exploration: the deliverable is a set of
benchmarks plus a written findings report. Optimization is explicitly out of
scope for this pass — anything worth fixing becomes its own follow-up branch,
justified by a `benchstat` before/after.

## Non-goals

- No production code changes. This pass is purely additive (one new package,
  benchmark test files, one doc).
- No optimization. We measure; we do not fix. Opportunistic fixes were
  considered and deliberately deferred to keep the PR clean and reviewable.
- No CI changes. Benchmarks require `-bench` and so never run in the default
  `go test` path; the pipeline is untouched and stays timing-flake-free.

## Hot-path candidates

A full sweep of the five performance-sensitive subsystems, grouped by the two
classic interactive-latency buckets:

**Cold start** (time-to-first-paint when a vault opens):

- `internal/tree` — `Walk`: filesystem walk + pre-flatten into `[]treeRow`.
- `internal/vault` — `Build(root, diag)`: parses every `.md`, builds the
  wikilink/backlink index. The prime O(n²) suspect as cross-link count grows.
  Takes a `Diagnostics`; the benchmark passes a no-op/discard stub.
- `internal/markdown` — `RenderWithLinks(src, base, marker)`: Glamour + sentinel
  preprocessing + link extraction of one document. This is the path the TUI
  actually runs for markdown (not the bare `Render`), so it's what we measure.
  Renderer is per-width — the benchmark constructs it at a fixed width.

**Hot loop** (per-keystroke cost, where GC pressure shows up as jank):

- `internal/search` — `Search`: case-insensitive full-text scan with worker
  fan-out, debounced at 150ms in the TUI.
- `internal/recent` — `(*Store).Rank` / `RankPaths`: re-ranks a result list by
  recency. The benchmark builds a `Store` over the corpus paths once (outside
  the timed region), then times the rank call.

For the hot-loop paths, **allocs/op is the headline metric**, not raw ns/op —
allocation churn during interactive use is what makes a terminal app feel
janky.

## Architecture

All additive. One new shared package plus benchmark files living next to the
code they test (matching the repo's "tests live next to the code" convention).

```
internal/benchcorpus/
  corpus.go                 deterministic synthetic-vault generator
  corpus_test.go            determinism assertion (same seed ⇒ identical bytes)
internal/search/search_bench_test.go     BenchmarkSearch/N=10|100|1000
internal/vault/vault_bench_test.go       BenchmarkBuild/N=...
internal/markdown/render_bench_test.go   BenchmarkRender (varies doc complexity)
internal/recent/recent_bench_test.go     BenchmarkRank/N=...
internal/tree/tree_bench_test.go         BenchmarkWalk/N=...
docs/benchmarking.md                     how-to-run + findings writeup
```

`docs/index.md` gains a link to `docs/benchmarking.md`.

## The corpus generator

`internal/benchcorpus` is the linchpin: a single deterministic generator that
every benchmark shares, so results are comparable across subsystems.

### API

```go
// Corpus is a generated synthetic vault on disk.
type Corpus struct {
    Root   string   // temp dir holding the .md files
    Files  []string // absolute paths, generation order
    Target string   // a pre-picked file for single-doc render/search benches
}

// Generate writes n markdown files into dir using a seeded RNG and returns
// the Corpus. dir is expected to be b.TempDir(). linkDensity controls the
// average number of [[wikilinks]] emitted per file.
func Generate(dir string, seed int64, n int, linkDensity int) Corpus
```

### Determinism

- Driven by a `*math/rand.Rand` constructed from `seed` — **never** the global
  source. Same `(seed, n, linkDensity)` ⇒ byte-identical files.
- `corpus_test.go` asserts this: generate twice into two temp dirs, compare
  bytes file-for-file.

### Generated file shape

Realistic markdown, not lorem-ipsum noise, so renderer and parser hit real
branches:

- A title heading plus a handful of section headings.
- Prose paragraphs assembled deterministically from a fixed word dictionary.
- `[[wikilinks]]` to other files in the corpus at the configured density —
  this is what stresses `vault.Build`'s cross-linking.
- Occasional inline `[text](relative.md)` links and a code fence.

### Scale parameterization

Benchmarks run at several N via sub-benchmarks:

```go
for _, n := range []int{10, 100, 1000} {
    b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) { ... })
}
```

Running across sizes is what turns "this takes 4ms" into a complexity curve and
surfaces algorithmic surprises (e.g. a `vault.Build` that bends superlinearly).
`benchstat` groups these sub-benchmarks automatically.

### Timing hygiene

Corpus generation and file I/O must sit **outside** the timed region. Generate
the corpus once before the measured loop, then `b.ResetTimer()` (or generate
before `b.Loop()`), so we measure the subsystem, not fixture setup. Files land
under `b.TempDir()` so cleanup is automatic and the real `docs/` is never
touched.

## The five benchmark suites

| Suite | Measures | Varies | Notes |
|-------|----------|--------|-------|
| `tree.Walk` | cold-start walk + flatten | N files | corpus on disk |
| `vault.Build` | wikilink/backlink index build | N + link density | O(n²) suspect |
| `markdown.RenderWithLinks` | Glamour + sentinel render + link extract | doc complexity | per-width, single file |
| `search.Search` | per-keystroke full-text scan | N files | worker fan-out; fixed query |
| `(*recent.Store).Rank` | result re-ranking | N hits | in-memory, no I/O; Store built outside timer |

All run with `-benchmem` for allocs/op alongside ns/op.

## Run & findings workflow

- `docs/benchmarking.md` documents the run commands:
  - Full sweep: `go test -bench=. -benchmem ./...`
  - Future A/B: capture `go test -bench=. -count=10` output and diff with
    `benchstat old.txt new.txt`.
  - Deeper digs: `-cpuprofile` / `-memprofile` → `go tool pprof`.
- After the benchmarks land, run the full sweep and write the **findings**
  section of `docs/benchmarking.md`: complexity curves, allocation hot spots,
  and any genuine surprises. This writeup is the payoff of the exploration.

## Testing

- `benchcorpus` gets a real unit test (`corpus_test.go`) asserting determinism
  and basic invariants (file count, non-empty content, links resolve within the
  corpus).
- The benchmark files themselves are exercised by a quick `go test -bench=.
  -benchtime=1x ./...` smoke run to confirm they compile and execute before the
  measured sweep.
