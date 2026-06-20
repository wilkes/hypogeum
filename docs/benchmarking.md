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

## Findings (run on 2026-06-20 / darwin/amd64 Intel Core i9-9980HK @ 2.40GHz)

| Benchmark | N=10 | N=100 | N=1000 | allocs/op @ N=1000 | Note |
|-----------|------|-------|--------|--------------------|------|
| tree.Walk | 39,502 ns/op | 155,270 ns/op | 1,358,281 ns/op | 4,031 | sublinear 10→100 (3.9×), then near-linear 100→1000 (8.7×) — directory stat batching effect |
| vault.Build | 630,827 ns/op | 6,062,199 ns/op | 68,982,213 ns/op | 278,068 | near-linear across all sizes (~9.6× / ~11.4×); NOT the expected O(n²) |
| search.Search | 198,297 ns/op | 1,754,616 ns/op | 17,915,101 ns/op | 16,037 | linear with N; 63 MB/op at N=1000 (file reads per call) |
| recent.Rank | 35,534 ns/op | 350,943 ns/op | 4,058,654 ns/op | 2,009 | linear with N; cost includes one `os.Stat` per path — this is filesystem I/O, not pure memory |
| markdown.RenderWithLinks | 1,389,308 ns/op | — | — | 12,269 | single doc; Glamour is allocation-heavy (~1.4 ms, ~504 KB per render) |

**Surprises and findings:**

- **`vault.Build` is not quadratic.** The suspected O(n²) wikilink/backlink cross-indexing scales
  at ~9.6× (N=10→100) and ~11.4× (N=100→1000) — consistent with O(n log n) or a slightly
  superlinear O(n). With a 1,000-file vault, one build takes ~69 ms. This is well within
  acceptable startup latency and not a current concern.

- **`markdown.RenderWithLinks` allocates ~12,000 objects per document render (~504 KB).** Glamour's
  AST-to-ANSI pipeline is inherently allocation-heavy. At 1.4 ms per render this is only felt on
  rapid repeated re-renders (e.g. every `WindowSizeMsg`). The allocation count cannot be reduced
  without replacing Glamour.

- **`recent.Rank` cost is dominated by filesystem I/O, not memory.** The doc comment in
  `internal/recent/recent.go` (line 90–91) is explicit: mtime is intentionally not cached because
  the watcher may update files between calls. Each `Rank` call issues one `os.Stat` per path.
  At N=1000 this is ~4 ms and ~2,000 allocs. A naive reading of the alloc count would suggest
  a pure in-memory cost — the real bottleneck is the `os.Stat` fan-out.

**Follow-up candidates (separate branches, justified by benchstat):**

- **`search.Search` allocates proportionally to N** (~667 KB at N=10, ~63 MB at N=1000). Each call
  reads every file into memory. An index-based approach (pre-read + line table) would reduce
  per-call allocations, at the cost of staleness — measure whether the current latency is
  problematic in the real TUI before optimizing.

- **`markdown.RenderWithLinks` alloc reduction.** The only realistic lever is replacing or
  wrapping Glamour with a renderer that reuses buffers. Profile first (`-cpuprofile`) to confirm
  allocations are the bottleneck vs. the Goldmark parse step.

- **`tree.Walk` sublinear 10→100 transition.** The 3.9× ratio for a 10× N increase suggests
  per-call fixed overhead dominates at small N. Harmless — even 1,000-file walks take 1.4 ms —
  but worth noting if the corpus is ever extended to N=10,000+.
