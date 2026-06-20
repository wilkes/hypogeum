# vault-index

The forward + reverse reference index that powers wikilink resolution and backlinks. Lives in `internal/vault`; read at startup, refreshed on watcher events.

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by the [wikilinks-and-backlinks design](../superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md) and [`internal/tui`](../packages/tui.md); press `b` for the full backlinks list.

## Why it exists

A vault is a set of cross-referencing notes. Two questions need to be answered fast and consistently:

1. **Forward (resolve):** given `[[Foo]]` in note A, which file does that point at?
2. **Reverse (backlinks):** given note B, which other notes link *to* it?

Both questions span the whole vault, both are needed on every render of every file with cross-references, and both must include standard markdown links (`[text](path.md)`) as well as wikilinks — the parent spec settled on uniform handling so vaults stay GitHub-compatible.

## How it works

`internal/vault` indexes the root once at startup (`Build`) and parses every `.md` file with goldmark's wikilink-extension-equipped parser. Each file becomes a `fileEntry` with a slice of `reference` records (kind = wikilink or stdlink, target name or href, resolved absolute path, optional heading/block/alias).

`Build` is parallel, not a single serial pass. `walkAndIndex` (`internal/vault/vault.go`) does a serial directory walk to collect markdown *paths* only, then fans the read+parse across `runtime.GOMAXPROCS(0)` workers — a profile showed serial `os.Open` syscalls and goldmark-allocation GC dominated a serial build. Each worker holds its own `goldmark.Markdown` (via `newMarkdownParser()`, since goldmark instances aren't safe to share across goroutines); map writes into `v.files`/`v.names` are serialized by `v.mu` inside `indexFile`. The result is independent of completion order because resolution sorts candidates (`scoreProximity`), so worker scheduling can't change which file a wikilink resolves to.

Forward index: `map[string]*fileEntry` keyed by absolute path.
Name index: `map[string][]string`, lowercased basename → list of absolute paths. Built once during `Build`; consulted during `Resolve`.

Reverse index is computed on demand from the forward index — `Backlinks(path)` iterates `files`, filters references whose `resolved == path`, and returns them in document order. At 1000 files × 20 refs/file = 20k iterations per call, which is invisible at terminal latency. If profiling ever shows it hurts, materialize a reverse map; YAGNI for now.

Refresh is incremental: `RefreshFile(path)` re-parses one file's outgoing references on `watch.FileModified`; `Rebuild()` re-walks the whole root on `watch.StructureChanged`. Both happen synchronously inside the TUI's fsEvent handler — vault sizes are small enough the work is invisible.

See also: a single file's *outbound* links don't need a full `Build`. `OutboundFor` (`internal/vault/outbound_fast.go`) indexes basenames only (no file reads) plus parses the one target file — enough to resolve its wikilinks — and is used by `query.Links`. Backlinks still need the full forward graph, so they go through `Build`.

### Resolution rules

In order of precedence:

1. **Exact basename match, case-insensitive.** `[[Foo]]` matches `Foo.md`, `foo.md`, `notes/FOO.md`.
2. **Proximity tiebreaker.** For each candidate, compute the relative path from `filepath.Dir(fromFile)` to the candidate and score it by `len(rel)` — the *byte length of that relative-path string* (`scoreProximity` in `internal/vault/resolver.go`). Smallest wins; lexical `path < path` order breaks exact-length ties. (This is string length, not a count of shared leading path components.)
3. **No-match → unresolved.** Renderer emits the broken-style placeholder (`?` suffix, dim red SGR).

The "name" stored in the index is `strings.ToLower(basenameWithoutExt(path))`. The forms `[[Foo|alias]]`, `[[Foo#Heading]]`, and `[[Foo^block]]` all resolve by the `Foo` portion; alias/heading/block are recorded separately.

## Invariants / gotchas

- **Vault is best-effort.** If `vault.Build` fails, `tui.New` continues with a nil vault — wikilinks render as broken, the backlinks modal opens empty. Same graceful-degradation rule as the watcher.
- **`markdown` does not import `vault`.** It defines a `Resolver` interface that `*vault.Vault` happens to satisfy. Tests of `markdown` use a fake. Keeps the package layering clean.
- **Mixed-syntax indexing is uniform.** A backlink to `notes/foo.md` shows up regardless of whether the linking file used `[[Foo]]` or `[Foo](notes/foo.md)`. The `Kind` field on `Backlink` lets the UI optionally render a small badge.
- **Renames are not auto-rewritten.** A rename that breaks `[[Old Name]]` in other files surfaces as broken links. This is the desired feedback loop — Claude owns content, hypogeum is a viewer.
- **Case-insensitive matching is locale-naive.** `strings.ToLower` is ASCII-safe in practice. Vaults with non-ASCII filenames may have surprising matches. Acceptable.
- **Reverse index is recomputed per call.** A backlinks modal that re-queried on every cursor move would re-iterate. The TUI caches `m.backlinks.items` after the modal opens to avoid this.
