# Architecture Review — DDD Lens

> **Status (2026-06-20):** Findings 1–4 have shipped; only Finding #5 (Backlinks RLock full-graph scan) remains open. This doc is retained as a historical record of the refactor that followed. The LOC figures below predate that work and are no longer current.

A code and architectural review of hypogeum through the lens of Eric Evans' Domain-Driven Design. Conducted by reading every non-test `.go` file across all packages; every file:line reference below was verified against source.

## Headline

Hypogeum is a **well-layered codebase** that already embodies more DDD than most Go projects its size. The dependency graph is acyclic, lower layers genuinely know nothing about the TUI, and `internal/nav` is a textbook pure domain model.

The one structural tension is visible from line counts alone:

> **Note:** These figures predate the Findings 1–4 refactor and are no longer current — `links_render.go` in particular dropped from 820 LOC to ~100 after the split (Finding #4). Treat the table as historical.

```
internal/tui        8,604 LOC   ← more than half the codebase
internal/markdown   2,745 LOC   ← one file (links_render.go) is 820
everything else     ~4,500 LOC  spread across 11 focused packages
```

A fat application layer wrapped around thin domain packages usually means **domain logic has leaked upward into orchestration code**. The TUI should be the thin layer that wires domain services to a screen; when it's the thickest layer, some of that bulk is misplaced domain knowledge (path resolution, file reading, directory synthesis) that the lower layers should own.

## What's already right (and why it's DDD)

- **`nav.History` is a model pure domain object.** Pure stack semantics, zero I/O, no time/randomness, encapsulated internals. This is the gold standard the other packages should be measured against — and it's small, which is the point.
- **The `markdown.Resolver` interface is a real seam.** It's an interface that `vault.Vault` implements; `vault` imports nothing from `tui`, and `markdown` depends only on `embed` + `wikilink`. That inversion is a proper Anti-Corruption boundary — wikilink resolution can fail and the renderer degrades to broken-link rendering without knowing why.
- **Graceful degradation is principled, not lazy.** The `nil`-vault, `nil`-watcher, `nil`-recent fallbacks are documented invariants, not hidden bugs. `_ = clipboard.WriteAll(text)` has an OSC-52 fallback right below it.
- **Test placement mirrors the architecture.** 90% coverage on `markdown`, 94% on `nav`, 189 model-level test functions in `tui`. The tests respect the same layering as the code.

## Findings (priority order)

### 1. The `Model` god-object — `internal/tui/model.go:48-99`

> ✅ **Resolved.** The `pendingNav` value object and the `status` → `currentPath`/`footerMessage` split both shipped — see `internal/tui/model.go` (`pendingNav` struct, `currentPath`, `footerMessage`, `pending pendingNav`).

The `Model` struct has 18 fields with **partial, inconsistent grouping**. Four cohesive sub-structs exist (`tree`, `content`, `backlinks`, `modals`); the rest are loose. There are four latent concepts hiding in the loose fields:

| Concept | Fields | DDD framing |
|---|---|---|
| Presentation state | `width, height, focus, keys, status` | View-model |
| Service dependencies | `watcher, vault, recent, diag` | Injected domain services |
| In-flight navigation | `pendingPreselectTarget`, `pendingPreselectRange`, `pendingExternal` | A short-lived value object |
| Test seams | `openExternal, copyToClipboard` | Ports |

The three `pending*` fields are the clearest case: they're **set and cleared together** across Back, Forward, `followBacklink`, and search-Enter, then consumed in one place (`refreshContent`). That's a value object waiting to be born:

```go
// A navigation in flight — what link to pre-select at the destination,
// and an optional external URL awaiting confirmation.
type pendingNav struct {
    preselectTarget string
    preselectRange  *markdown.LineRange
    externalURL     string
}
```

The `status string` field (commented "last error or info message") is a **primitive-obsession smell**: across its call sites it holds the current file path, an error message, *and* a transient info toast at different times. One field meaning three things is an implicit state machine that lives only in the maintainer's head. Split into `currentPath` and `footerMessage`.

**Recommendation:** Do the `pendingNav` extraction and the `status` split — both low-risk, high-clarity. The "presentation state" struct ranks lower; `width`/`height`/`keys` on the Model are idiomatic Bubble Tea and churning ~40 call sites buys less.

### 2. Path resolution is triplicated (verified)

> ✅ **Resolved.** Extracted to a single domain service, `pathutil.ResolveRelativeTo` (`internal/pathutil/pathutil.go`); the three call sites now delegate to it.

The same "resolve relative to the base file's directory" rule appears in three packages with no single owner:

```
internal/markdown/links.go:77          target  = filepath.Join(filepath.Dir(base),     target)
internal/markdown/links_render.go:508  absPath = filepath.Join(filepath.Dir(base),     absPath)
internal/vault/vault.go:201            target  = filepath.Join(filepath.Dir(fromPath),  target)
```

In DDD terms this is a missing **domain service** — a domain rule ("what does a relative link mean?") copy-pasted three ways. It belongs in exactly one place:

```go
// One home for the rule.
func ResolveRelativeTo(base, target string) (string, error) {
    if filepath.IsAbs(target) {
        return filepath.Abs(target)
    }
    return filepath.Abs(filepath.Join(filepath.Dir(base), target))
}
```

**Recommendation:** Highest-value, lowest-risk change here. Extract the *function*. Skip the grander `AbsolutePath` newtype threaded through every signature — Go's lack of newtype ergonomics means constant `string(p)` conversions, and the function consolidation already captures ~90% of the benefit.

### 3. Duplicated highlight-marker protocol (verified, trivial)

> ✅ **Resolved.** Extracted into `internal/highlight` (`highlight.Open`/`highlight.Close` constants plus `Wrap`/`Strip` helpers); `search` and `vault` now share the one definition.

`internal/search/search.go:40-41` and `internal/vault/snippet.go:16-17` **independently define** the same control-character protocol:

```go
snippetHighlightOpen  = "\x11" // DC1
snippetHighlightClose = "\x12" // DC2
```

`search.go` even comments that it "mirrors the convention defined in" vault. That's a **Shared Kernel** that hasn't been extracted — two packages agreeing on a wire protocol by copy-paste.

**Recommendation:** Pull the constants (plus the strip/render helper that consumes them) into a tiny `internal/highlight` package. ~30-minute change that removes a real "change one, forget the other" hazard the CLAUDE.md already flags as load-bearing.

### 4. `markdown` is doing four jobs — `internal/markdown` (2,745 LOC)

> ✅ **Resolved.** `links_render.go` was split within the package — `sentinel.go` (strip/marker machinery) and `preprocess.go` (wikilink + embed rewriting) now carry the bulk, and `links_render.go` is down to ~100 LOC.

The package mixes four different *reasons to change*:

1. **Glamour rendering** (`render.go`, `style.go`)
2. **Link extraction & resolution** (`links.go`) — domain logic
3. **Sentinel instrumentation** (`links_render.go`, 820 LOC) — ANSI position recovery
4. **Source transformation** (wikilink + embed preprocessing, inside `links_render.go`)

A Glamour upgrade and a new `[[...]]` syntax feature currently touch the same 820-line file.

**Recommendation:** Split `links_render.go` into multiple files *within the same package* — `sentinel.go` (strip/marker machinery), `preprocess.go` (wikilink + embed rewriting), `render.go` (orchestration). Same package, same internal access; the 820-line monster becomes three units organized by reason-to-change. Avoid the heavier "split into 3-4 new packages" option — over-engineering for a solo-maintained tool. Reach for package boundaries only if you later want the sentinel logic tested in true isolation.

### 5. `Vault.Backlinks` holds a read lock across a full graph scan — `internal/vault/backlink.go`

> ⏳ **Open.** Still accurate — no reverse index yet; `Backlinks` continues to scan every file × every reference under `v.mu.RLock()`.

`Backlinks` takes `v.mu.RLock()` then iterates **every file × every reference** to find the handful pointing at one path. Correct, but O(files × refs) under the read lock, fired every time the backlinks modal opens. Invisible on small vaults; laggy on a 10k-note vault.

**Recommendation:** "Right but won't scale," not a bug. Don't add `context.Context` yet (premature). If it ever lags, maintain a reverse index incrementally during `RefreshFile` so `Backlinks` becomes an O(1) map lookup. Note it as a known limitation and move on.

## Suggestions deliberately *not* recommended

Filtering, not just relaying:

- **`ModalHandler` interface** — the ordered dispatch in `handleKey` is already covered by regression tests (picker-grabs-keys ordering, tree-arrow shadowing). An interface adds indirection without removing the underlying ordering constraint.
- **`AbsolutePath` newtype everywhere** — taxes Go ergonomics too heavily; the function extraction in Finding 2 captures the value.
- **Splitting `recent` into `Scorer` + `VisitStore`** — the scoring function is already pure and isolated (82% covered); a testability fix for a problem the tests don't have.

## Priority list

Each lands as its own small branch, matching the repo workflow.

| # | Change | Effort | Status |
|---|---|---|---|
| 1 | Extract `ResolveRelativeTo` — kill path triplication | ~1hr | ✅ shipped (`internal/pathutil`) |
| 2 | Extract shared highlight markers into `internal/highlight` | ~30min | ✅ shipped (`internal/highlight`) |
| 3 | Extract `pendingNav` value object + split `status` field | ~2hr | ✅ shipped (`internal/tui/model.go`) |
| 4 | Split `links_render.go` into 3 files (same package) | ~2hr | ✅ shipped (`sentinel.go`/`preprocess.go`) |
| 5 | (defer) Reverse-index for `Backlinks` | — | ⏳ open — only when a large vault makes it hurt |

Items 1-3 were the high-value, low-risk core: behavior-preserving, test-backed refactorings, each making a *named domain concept* explicit where there's currently an implicit one. All four have since shipped; only the deferred reverse-index (Finding #5) remains.
