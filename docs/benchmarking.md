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

## Extreme-scale findings (100k–1M files)

A one-off sweep at 100k and 1M files (flat directory — a filesystem worst case;
real nested vaults are kinder). Same machine. The interesting column is
**cost per file**: under clean O(n) it would be constant. It is not — most ops
get 6–25× *more expensive per file* from 100k to 1M, the signature of falling
off an OS cache cliff (see below).

| Operation | 100k | 1M | per-file 100k → 1M |
|-----------|------|-----|--------------------|
| `tree.Walk` | 173 ms | 2.99 s | 1.7 → 3.0 µs (1.8×) |
| `search.SearchAll` (worst case) | 1.64 s | 94 s | 16 → 94 µs (6×) |
| `recent.Rank` | 526 ms | 135 s | 5.3 → 135 µs (**25×**) |
| `vault.Build` | 7.8 s | 447 s (7.4 min) | 78 → 447 µs (6×) |
| `vault.RefreshFile` | 87 µs | 74 µs | **flat** |

- **`vault.Build` is the wall past ~100k files** — a 7.4-minute startup at 1M.
  This is the regime (and the first target) where a *persisted on-disk index*
  — load a prebuilt graph instead of rebuilding — would actually pay off. It
  stays YAGNI for realistic vaults (tens of thousands of notes).
- **`vault.RefreshFile` is dead flat from 1k → 1M files** (~75 µs), independent
  of vault size. That's the payoff of scoping per-save resolution to the
  changed file (PR #77 / [[vault-index]]). Without it, a save at 1M would run a
  full `resolveAllRefs` over ~5M refs — seconds of lag per keystroke-save.

### The vnode-cache cliff (why `recent.Rank` scaled 256×)

`recent.Rank` does one **serial** `os.Stat` per file, then a sort (the sort is
negligible). Two compounding factors explain its disproportionate blow-up:

1. **macOS caps the vnode (inode) cache** — `kern.maxvnodes` was **263,168** on
   the test machine. Below it, every `os.Stat` is a warm memory hit; above it
   each stat forces a vnode reclaim + APFS B-tree re-resolution. Confirmed with
   empty-file stat passes: serial cost went **4.0 µs/file at 100k → 12.7 µs/file
   at 500k** as N crossed the limit (and 135 µs at 1M with content competing for
   cache).
2. **`Rank` is single-threaded**, so it eats every cliff-induced miss latency
   back-to-back with zero overlap. `search` does *more* per file (open+read) yet
   scaled better because its `numWorkers = 4` fan-out hides the latency. A
   16-worker parallel stat pass beat the serial loop **3.1–3.3×** at 100k/500k.

So it is not an algorithmic bug — it is linear work meeting a platform limit,
made worse by a serial loop. **Fixes, if 100k+ vaults ever matter:** (a) mirror
`search`'s worker fan-out in `Rank`'s stat loop (~3×); (b) the bigger win — a
**persisted mtime cache** so `Rank` skips `os.Stat` for unchanged files and
sidesteps the cliff entirely. Both YAGNI today: under ~263k files `Rank` is warm
and sub-second, and it only runs on picker open.

### File size vs file count — two separate axes

The corpus uses tiny ~650 B files, so all the numbers above vary *file count*,
not *file size*. They are independent axes, and they split the hot paths cleanly.
Holding count fixed at N=2000 and varying only average file size (1 → 8 → 64 KB,
a 64× byte increase):

| Operation | 1 KB | 8 KB | 64 KB | reads… |
|-----------|------|------|-------|--------|
| `search.SearchAll` | 16.1 ms | 28.8 ms | 87.4 ms | **content → scales** (5.4×) |
| `vault.Build` | 132 ms | 229 ms | 966 ms | **content → scales** (7.3×) |
| `recent.Rank` | 9.0 ms | 7.6 ms | 9.1 ms | metadata → **flat** |
| `tree.Walk` | 3.0 ms | 2.7 ms | 3.1 ms | metadata → **flat** |

- **`tree.Walk` and `recent.Rank` don't read file contents** (`readdir` + `os.Stat`
  only), so a 64× size increase moves them 0%. They are purely *file-count*-bound —
  and `recent.Rank`'s vnode cliff is about *inode count*, so it's immune to file size
  too. The 1M-file numbers above hold regardless of how big the notes are.
- **`search` and `vault.Build` scale with total bytes read** — more lines to
  lowercase/match, more prose to tokenize through goldmark. `RefreshFile` and
  `markdown.RenderWithLinks` ride the same axis (one document, scales with its size).
- **Scaling is sub-linear in bytes** (64× bytes → ~5–7× time) because fixed per-file
  overhead (the `open` syscall, scanner/goldmark setup, map insert) costs the same at
  1 KB or 64 KB. At 1 KB that overhead dominates (~7.8 ns/byte effective); at 64 KB
  content work dominates (~0.67 ns/byte). Total cost is really
  `count × per-file-overhead + total-bytes × per-byte-cost` — count-bound ops feel
  only the first term, content ops feel both.

So the extreme-scale numbers, run on tiny files, *under*-state the content ops for a
realistic vault: with ~5–10 KB notes, `vault.Build` runs roughly 2× the reported times
(the 7.4-min build at 1M → ~12–15 min). `tree.Walk` and `recent.Rank` are unchanged.
One guard worth knowing: `search` caps per-file reads at `maxFileBytes` (1 MiB), so a
single giant note can't blow up a scan; `vault.Build` and `RefreshFile` have no such cap
and read the whole file.

`search`'s share of that 2× has since been removed (PR #80): `scanFile` no longer
allocates a `Text()` + `ToLower()` copy per line, so its per-line cost — the part that
grew with file size — dropped ~3.2× on a mixed-case large-file corpus (45 MB/op → 215 KB,
403k allocs → 3k). `vault.Build` got ~3× faster too (PR #82, see below), though it remains
the content op most sensitive to note size — its per-byte work is goldmark allocating a
full prose AST, which the #82 changes overlap but don't eliminate.

> Method note: at this scale `testing.B`'s regenerate-per-run model is
> impractical, so these came from a throwaway harness that generates one corpus
> and times each operation a single pass. Not committed — reconstruct from this
> note if needed.

### What a profile of `vault.Build` showed (PR #82)

`pprof` of a large-file build (300 files × 33 KB) put the cost in surprising places:

- **`os.Open` syscalls — ~44% of CPU.** Reads ran serially, and macOS syscalls are
  expensive; this is a per-*file* cost (scales with count, not size).
- **GC — ~25%**, driven by goldmark's allocations (`text.Segments.Append` alone was 38%
  of bytes; the AST it builds for prose is 82% of all allocations).
- **goldmark parse *compute* — only ~3%.** The per-byte cost is the *garbage*, not the parsing.

Two low-risk fixes followed (both shipped in #82): fan the read+parse across `GOMAXPROCS`
workers (overlaps the open syscalls + spreads GC), and reuse one goldmark parser per worker
instead of constructing one per file. Result (`benchstat`, n=6): `BuildLargeFiles` −70%,
`Build/N=1000` −68% time / −42% allocs. `BenchmarkBuildLargeFiles` guards the per-byte regime.

#### Rejected: replacing goldmark with a hand-rolled link scanner

The remaining per-byte allocation is goldmark building a full prose AST just to find a few
links. We prototyped an AST-free scanner (regex-free byte scan for `[[wikilinks]]` and
`[text](dest)`, fence/inline-code/escape aware, reusing `internal/wikilink.Parse`) and
**measured it against goldmark before deciding** — then discarded it. The numbers said no:

- **Upside was modest.** Versus `extractReferences` on a 300-link doc: allocations −48%
  (12.6k → 6.6k), bytes −55%, but **time only −8%** — because parse *compute* was never the
  cost (it's ~3%; the allocations bite via GC, which #82 already parallelized). On top of
  #82 the scanner bought little wall-clock.
- **Correctness cost was real and concrete.** A differential test over 84 files (the `docs/`
  vault + edge cases) matched goldmark on core fields (kind/target/heading/block/alias/line)
  **100% where ref counts aligned** — but counts diverged on **2/84 real files**, both from
  **indented code fences inside list items**. Fence state is global, so one missed fence
  *cascades* and mis-classifies every link after it in the file. `displayText` matched only
  80% (formatted link text isn't flattened) and the backlink `snippet` matched **0%** (it
  needs goldmark's inline→plaintext rendering).
- **Closing those gaps means reimplementing CommonMark** — list-aware fence tracking, lazy
  continuation, an inline renderer for snippets — i.e. re-growing the parser we set out to
  delete, and owning every edge case goldmark already handles.

Conclusion: the safe ~3× from #82 is the right stopping point. The last slice of per-byte
allocation isn't worth becoming a markdown-parser maintainer. Don't re-prototype this without
a vault large enough that Build's *allocation* (not its already-parallel wall-clock) is the
felt bottleneck.

## CLI command cold-start (`hypogeum search|recent|links|neighbors`)

The non-interactive query verbs are not benchmarked at the Go level — their *work* is already
covered one layer down (`query.Search`→`search.SearchAll`, `query.Links`/`Neighbors`→
`vault.Build`, `query.Recent`→`recent.Rank`), and the wrappers add nothing (`Neighbors` builds
the vault once and reads `Outbound`/`Backlinks` in memory — no double-parse). What package
benchmarks *can't* see is the process-level cost, measured here by timing the built binary
(30 runs, median) against `docs/` (68 small files):

| Invocation | Median | Above startup floor |
|------------|--------|---------------------|
| `--version` (startup floor) | 24.2 ms | — (process spawn + Go runtime init) |
| `links` / `neighbors` | ~24 ms | **~0** (a 68-file `vault.Build` is sub-ms post-#82) |
| `recent` | 31.9 ms | ~8 ms |
| `search` | 33.8 ms | ~10 ms |

Two things define CLI latency, and neither is a package-benchmark target:

- **A ~24 ms fixed floor** — process spawn + Go runtime init, paid by every invocation. On a
  small vault the commands are startup-bound (`links`/`neighbors` don't even clear the floor);
  you can't beat it from a cold binary, so there's nothing to optimize there.
- **Cold rebuild on every call.** Unlike the TUI (which builds the vault once and reuses it all
  session), each CLI invocation starts from zero. So at scale, per-call latency ≈ the package
  benchmark for that op: `search` over 100k files ≈ 1.6 s *every time*; `links`/`neighbors` ≈ a
  full `vault.Build` (~8 s at 100k). Scripting these in a loop over a large vault is the one
  place this bites.

The fix for the large-vault case is the **persisted on-disk index** noted below — load instead
of rebuild. It's the only lever that helps the CLI specifically: fs-event freshness can't,
because no process is alive between invocations to receive the events. Still YAGNI below ~10k
files, where the ~24 ms floor dominates anyway.

**Follow-up candidates (separate branches, justified by benchstat):**

- **`search.Search` allocation.** ✅ *Largely addressed across two passes.* PR #76 pooled the
  per-file scanner buffers (full-scan allocations −98%, time ~2.3×, 10k-file vault back under the
  150 ms debounce). PR #80 then removed the per-line `Text()` + `ToLower()` copies in favour of a
  no-alloc ASCII-fold scan (large-file search −68% time, −99.5% bytes; small files improved too).
  Remaining lever — a persisted index (pre-read + line table) — would cut the *remaining* per-scan
  file reads at the cost of staleness, and only matters past ~10k files (still YAGNI). Note PR #80
  narrows matching to ASCII case folding; non-ASCII case-insensitive matches (e.g. `É`/`é`) no
  longer match, a deliberate trade for the allocation win.

- **`markdown.RenderWithLinks` alloc reduction.** The only realistic lever is replacing or
  wrapping Glamour with a renderer that reuses buffers. Profile first (`-cpuprofile`) to confirm
  allocations are the bottleneck vs. the Goldmark parse step.

- **`tree.Walk` sublinear 10→100 transition.** The 3.9× ratio for a 10× N increase suggests
  per-call fixed overhead dominates at small N. Harmless — even 1,000-file walks take 1.4 ms —
  but worth noting if the corpus is ever extended to N=10,000+.
