# Wikilinks and backlinks — design

**Status:** spec — not yet implemented.
**Scope:** add Obsidian-style `[[wikilinks]]` and a backlinks index to hypogeum, surfaced as a persistent bottom pane and a modal overlay. The backlinks index also covers standard markdown links (`[text](path.md)`) so a vault that uses GitHub-compatible links gets the same backlinks coverage.
**Out of scope:** tags, frontmatter views, full-text search, graph view, wikilink autocomplete, rename-and-rewrite. These are independent designs.

See also: [docs index](../../index.md), [architecture](../../architecture.md), [link-following](../../link-following.md).

## Motivation

`hypogeum` already renders a directory of markdown and follows `[text](path.md)` links. The next jump in usefulness is treating that directory as a *vault* — a set of cross-referencing notes — and surfacing the cross-references both inline (`[[Note]]` becomes a clickable link) and reflexively ("what links to *this* note?").

The use case is asymmetric: an LLM (Claude) generates and maintains the content; a human (the user) reads and navigates it. This shapes several design decisions:
- No authoring affordances (no autocomplete, no "create note from broken link"). The tool is a viewer.
- Broken `[[links]]` are a feedback signal *to* the content generator, not a UX flow to design around. Surface them; don't hide them.
- Lenient name matching matters less than it would for a human writer — Claude can produce exact names. We still implement Obsidian's canonical resolution rules so vaults written for Obsidian work without modification.

### Multi-tool compatibility

Vaults written for `hypogeum` are also expected to be readable in GitHub's web UI and in Obsidian without modification. This shapes the cross-reference strategy:

- **Standard markdown links (`[text](path.md)`)** work everywhere — GitHub, hypogeum, Obsidian. Claude is expected to prefer this syntax for most cross-references.
- **Wikilinks (`[[Note]]`)** work in hypogeum and Obsidian, but render as literal text on GitHub. Useful where name-based lookup is more natural than path-based.

The indexer covers both syntaxes uniformly: a backlink to `notes/foo.md` shows up regardless of whether the linking file used `[[Foo]]` or `[Foo](notes/foo.md)`. This means a vault that sticks to standard links gets full hypogeum backlinks coverage *and* full GitHub navigation, while a vault that mixes both gets the same backlinks coverage with the wikilink subset surviving in Obsidian.

## Architecture

A new `internal/vault` package becomes a peer of `tree`, `markdown`, `nav`, `watch`. It owns wikilink parsing, the forward and reverse indexes (covering both wikilinks *and* standard markdown links), and watcher integration for incremental maintenance. `internal/markdown` gains a small `Resolver` dependency it uses to turn `[[Name]]` AST nodes into either a real link (resolved) or a styled placeholder (unresolved). `internal/tui` consumes the vault for the backlinks UI.

```
cmd/hypogeum
        │
        ▼
internal/tui                 (still the only package that knows about the UI)
   │      │      │      │      │
   ▼      ▼      ▼      ▼      ▼
   tree   markdown   nav   watch   vault   (lower layers, mutually independent)
            │                       │
            └────── Resolver ───────┘      (markdown depends on a small interface,
                                            implemented by vault)
```

The dependency edge `markdown → vault` is via interface only. `markdown` does not import `vault`; it defines `Resolver` itself, which `vault.Vault` happens to satisfy. Tests of `markdown` pass a fake.

## Components

### `internal/vault` (new)

```
vault.go      Vault struct, Build, RefreshFile, Rebuild, Backlinks, indexes
resolver.go   Resolver interface impl, name lookup rules
parser.go     goldmark extension: [[...]] → Wikilink AST node
snippet.go    AST → nearest-enclosing-block text extraction
```

Public surface:

```go
type Vault struct { /* private */ }

// Diagnostics is the interface vault uses to surface non-fatal issues
// (parse failures, refresh races, etc.) to the user. Implemented by the TUI;
// see "Diagnostics" under Error handling.
type Diagnostics interface {
    Info(msg string)
    Warn(msg string)
    Error(msg string)
}

// Build walks root and indexes every .md file's wikilinks and standard
// markdown links. The diag sink receives non-fatal issues (e.g. parse
// failures); pass a no-op implementation for tests that don't care.
// Returns a Vault with both indexes populated.
func Build(root string, diag Diagnostics) (*Vault, error)

// RefreshFile re-parses one file's outgoing references (wikilinks and
// standard markdown links) and updates both indexes.
// Called on watch.FileModified.
func (v *Vault) RefreshFile(path string) error

// Rebuild re-walks the entire root. Called on watch.StructureChanged.
func (v *Vault) Rebuild() error

// Resolve implements markdown.Resolver. Returns the absolute path the
// wikilink target resolves to, or ("", false) if no file matches.
// fromFile is the file containing the wikilink (used for proximity tiebreaking).
func (v *Vault) Resolve(fromFile, name, heading, block string) (path string, ok bool)

// Backlinks returns every reference *to* path in document order
// across files. Used by the TUI backlinks pane. Includes both
// wikilink and standard-markdown-link references uniformly.
func (v *Vault) Backlinks(path string) []Backlink

type Backlink struct {
    SourceFile  string // absolute path of the linking file
    DisplayText string // alias/wikilink target, or standard link's [text]
    Snippet     string // smallest enclosing block, plain text, with the
                       // link's display text wrapped in SGR for highlight
    Line        int    // 1-indexed line in SourceFile where the link appears
    Kind        BacklinkKind // wikilink or stdlink — exposed so the UI
                             // can optionally render a small badge
}

type BacklinkKind int

const (
    BacklinkWikilink BacklinkKind = iota
    BacklinkStdLink
)
```

Internal state:

```go
type Vault struct {
    root  string
    files map[string]*fileEntry  // keyed by absolute path
    names map[string][]string    // lowercase basename → []absolute path
    mu    sync.RWMutex
}

type fileEntry struct {
    path string
    refs []reference  // outgoing references (wikilinks + standard markdown links)
}

type referenceKind int

const (
    refWikilink referenceKind = iota // [[Target]] — name-based lookup
    refStdLink                       // [text](path.md) — path-based lookup
)

type reference struct {
    kind        referenceKind
    target      string // raw [[Target]] name (wikilink) or href (stdlink)
    resolved    string // absolute path of the target file, "" if unresolved
    heading     string // [[Target#Heading]] or [text](path.md#heading)
    block       string // [[Target^block-id]] (wikilinks only)
    alias       string // [[Target|alias]] (wikilinks only)
    displayText string // wikilink: alias if present else target; stdlink: link text
    snippet     string // pre-rendered nearest-enclosing-block plain text
    line        int
}
```

Reverse index is computed on demand from the forward index (cheap — iterate `files`, filter by target). If profiling shows this hurts at scale, we materialize it; YAGNI for now.

### `internal/markdown` (changes)

Add a `Resolver` interface and an optional resolver field on `Renderer`:

```go
type Resolver interface {
    Resolve(fromFile, name, heading, block string) (path string, ok bool)
}

func NewRenderer(width int, opts ...Option) (*Renderer, error)
func WithResolver(r Resolver) Option
func WithFromFile(path string) Option  // set per-render before RenderWithLinks
```

The wikilink goldmark extension is registered when a resolver is present. It parses `[[...]]` into a private `wikilinkNode` AST type. The renderer walks AST nodes; for `wikilinkNode`:
- Call `resolver.Resolve(fromFile, name, heading, block)`.
- **Resolved:** emit the same byte sequence Glamour would produce for `[displayText](resolvedPath#heading)`. The existing sentinel pair brackets it; `RenderWithLinks` picks it up like any other link.
- **Unresolved:** emit `displayText?` with dim-red SGR, sentinel-bracketed. The link list records it as a `Link` with `Resolved = false` so the TUI footer can show "broken: [[Name]]" when it's selected.

The renderer needs the calling file's path because resolution is proximity-aware. `tui.refreshContent` calls `WithFromFile(path)` on the renderer before each render — this sets a field on `Renderer` that is read by the wikilink emit logic and passed as the first arg to `Resolver.Resolve`. (Renderer is per-width, mutable for this field; not shared across goroutines.)

### `internal/tui` (changes)

State additions:

```go
backlinksOpen   bool           // persistent bottom-split toggled by 'b'
modalOpen       modalKind      // none | backlinks | logs (single-modal invariant)
backlinksVP     viewport.Model // independent scroll for the backlinks bottom split
modalVP         viewport.Model // independent scroll for whichever modal is open
backlinkCursor  int            // selected row in whichever backlinks surface is active
vault           *vault.Vault   // injected at New time

// Diagnostic sink — owned by the TUI, passed to vault.Build via the
// vault.Diagnostics interface. Pushes to a 200-entry ring buffer
// (read by the log viewer modal) and to the footer transient status
// (cleared after ~3s via a tea.Tick).
diag            *diagnostics
status          string  // existing field; now also driven by transient diag
```

Modal kinds:

```go
type modalKind int
const (
    modalNone modalKind = iota
    modalBacklinks
    modalLogs
)
```

The single-modal invariant means `B` and `?` are mutually exclusive: opening one closes the other. This keeps geometry simple (one modal viewport, swap content) and avoids stacking semantics.

Geometry:
- `b` toggles `backlinksOpen`. When open *and* `m.height >= 20`, the content viewport's height is reduced by `backlinksHeight` (8 rows including its border). When `m.height < 20`, `backlinksOpen` is honored as state but the pane is suppressed in `View()` — when the terminal grows again, the pane reappears.
- `B` toggles the backlinks modal (`modalOpen = modalBacklinks` ↔ `modalNone`). While any modal is open, geometry is recomputed as if `backlinksOpen` were false — the content viewport reclaims the bottom-split's space, and the modal renders centered on top. When the modal is dismissed, if `backlinksOpen` is still true, the bottom split reappears. Modal size is fixed at 60% width × 60% height (clamped to min 40 cols × 12 rows, max 120 cols × 40 rows).
- `?` toggles the log viewer modal (`modalOpen = modalLogs` ↔ `modalNone`). Same modal infrastructure and geometry as the backlinks modal. Mutually exclusive with the backlinks modal — pressing `?` while backlinks modal is open swaps the modal's content; pressing `B` while logs modal is open does the same.
- `Esc` dismisses the modal (any kind) first, then clears the link cursor as today, then is a no-op.

Backlink row rendering (used by both surfaces):

```
relative/path/to/source.md:42
  ...This snippet has the [[Note Name]] highlighted in here...
```

The basename and line on row 1; one-line snippet (truncated to viewport width with ellipsis) on row 2. The `[[Note Name]]` display text within the snippet is wrapped in bright SGR. Two visible rows per backlink; navigation keys (`j`/`k`) move by *backlink*, not by row.

Following a backlink: `Enter` calls `openFile(SourceFile)` (history records the visit). The link cursor is reset on the new page. Cursor navigation, scroll-to-line, and back-restores-cursor are detailed in the follow-on spec [backlinks-navigation-design](2026-05-07-backlinks-navigation-design.md). Phase 2 still includes pre-selecting the matching inline link in `m.links`.

Watcher integration:
- `fsEventMsg` handler now also calls `vault.RefreshFile(path)` (on `FileModified`) or `vault.Rebuild()` (on `StructureChanged`) before refreshing content. The refresh is synchronous; vault sizes are small enough that the extra work is invisible to the user. If profiling later proves otherwise, this becomes a `tea.Cmd`.

### Wikilink resolution rules

In order of precedence:

1. **Exact basename match, case-insensitive.** `[[Foo]]` matches `Foo.md`, `foo.md`, `notes/FOO.md`.
2. **Proximity tiebreaker on multiple matches.** Compute the relative path from `fromFile` to each candidate; pick the shortest. Lexical path order breaks ties.
3. **No-match → unresolved.** Renderer emits the styled placeholder.

Forms:
- `[[Foo]]` — name is `Foo`, display is `Foo`.
- `[[Foo|display]]` — name is `Foo`, display is `display`.
- `[[Foo#Heading]]` — name is `Foo`, anchor is `slug(Heading)`, display is `Foo > Heading` if no alias else alias.
- `[[Foo^block]]` — name is `Foo`, block is `block`. Phase 1 lands the user at the file; the block ID is recorded but not located. Documented limitation.

The "name" stored in the index for lookups is `strings.ToLower(basenameWithoutExt(path))`. The `names` index is `map[string][]string` (lowercased basename → list of absolute paths), so disambiguation can iterate candidates.

## Data flow

**Startup:**
1. `tree.Walk(root)` produces the tree (existing).
2. `vault.Build(root)` walks the same root and, for each `.md` file, runs *one* goldmark parse (with the wikilink extension registered) and walks the resulting AST collecting *both* `wikilinkNode`s and standard `ast.Link` nodes. Each becomes a `reference` entry tagged with its `kind`. Standard links resolve via `markdown.ResolveLink` (existing); wikilinks resolve via the basename index. Populates `files` and `names`.
3. `markdown.NewRenderer(width, WithResolver(vault))` wires the resolver in.
4. `tui.New` stores the vault on the model.

**Render of a file:**
1. `m.refreshContent(path)` reads the source.
2. Sets `WithFromFile(path)` on the renderer.
3. `RenderWithLinks` parses + renders. Wikilink nodes are resolved during render.
4. Resolved wikilinks become regular `Link` entries in the returned link list; unresolved ones are `Link{Resolved: false, Href: "[[...]]"}`.
5. Footer/cursor behavior unchanged.

**`b` pressed:**
1. `m.backlinksOpen = !m.backlinksOpen`.
2. `m.refreshBacklinks(currentPath)` populates `m.backlinksVP` content from `vault.Backlinks(currentPath)`.
3. Re-layout: viewport heights recomputed.

**`B` pressed:**
1. `m.modalOpen` toggles between `modalBacklinks` and `modalNone`. If currently `modalLogs`, it switches to `modalBacklinks` (single-modal swap).
2. `refreshBacklinks` populates the shared modal viewport.
3. View renders modal as the top layer; all other input except modal nav and `Esc` is dropped while modal is open.

**`?` pressed:**
1. `m.modalOpen` toggles between `modalLogs` and `modalNone`. If currently `modalBacklinks`, it switches to `modalLogs`.
2. `refreshLogs` populates the shared modal viewport from the diagnostic ring buffer.
3. Same modal rendering and input rules as `B`.

**Filesystem event:**
1. `watch.FileModified(p)` arrives.
2. `vault.RefreshFile(p)` re-parses `p`'s outgoing references.
3. If `p == m.history.Current()` or `p` is the source of any backlink to current → `m.refreshContent(currentPath)` and `m.refreshBacklinks(currentPath)`.
4. `watch.StructureChanged` does `vault.Rebuild()` then `tree.Walk(root)` (existing) then refreshes both panes.

## Error handling

| Failure | Behavior |
|---|---|
| Goldmark parse failure on one file during `Build` | Emit a `warn` diagnostic with the file path and parse error. Skip that file's references. Index is still built. |
| `vault.Build` returns error (e.g. root unreadable) | Propagate to `tui.New` which fatals — same as today. |
| `RefreshFile` fails (file deleted between event and read) | Emit an `info` diagnostic. Drop the file's entry from indexes; no error to caller. |
| Watcher `nil` | Emit a `warn` diagnostic at startup. Vault built once, never refreshed. Same graceful-degradation as the rest of the TUI. |
| Wikilink target unresolved | Inline styled placeholder (dim red, `?` suffix). No footer message unless the user selects the link. Selecting shows `broken: [[Name]]`. |
| Wikilink resolved but anchor missing | File opens; anchor scroll is a no-op (same as a regular link). |
| Multiple basename matches | Proximity rule (above). Resolution is deterministic given the vault state. |
| Diagnostic log path unwritable | File logging silently disabled. In-memory ring buffer and footer still work. |

### Diagnostics

Errors and warnings during indexing feed a single internal stream with three observers: a transient footer status, an append-only log file, and an in-app log viewer modal (`?`). Severity levels are `info`/`warn`/`error`; Phase 1 emits only `warn` and `error` (plus one `info` for `RefreshFile` races). The vault accepts a `Diagnostics` interface (`Warn(string)`, `Error(string)`, `Info(string)`) implemented by the TUI, so the stream is a TUI concern without coupling `vault` to the UI. Full design and rationale: [[diagnostics]].

## Testing

`internal/vault`:
- `parser_test.go`: each wikilink form parses to expected node.
- `snippet_test.go`: AST fixtures for paragraph, list item, blockquote, nested list-in-blockquote → snippet text matches expected (link's display text wrapped in marker bytes for the highlight).
- `vault_test.go`:
  - Build on fixture tree → indexes match.
  - Build with mixed-syntax fixture (some wikilinks, some standard markdown links pointing at the same target) → backlinks include both, with `Kind` correctly set.
  - `RefreshFile` after editing one file → forward index updated, reverse-side derivation correct.
  - `Resolve` with case differences, disambiguation by proximity.
  - `Resolve` on miss returns `false`.
  - Parse failure on one file emits a `warn` diagnostic via the injected sink; Build still returns a populated vault.

`internal/markdown`:
- Golden test: a file with `[[Foo]]` where the resolver returns a real path produces the same byte structure as `[Foo](path/to/foo.md)`.
- Golden test: unresolved wikilink renders with the broken-style SGR, sentinel-bracketed, and appears in the link list with `Resolved = false`.

`internal/tui`:
- `backlinks_test.go`:
  - Toggling `b` changes geometry and populates the backlinks viewport.
  - `j`/`k` move the backlink cursor.
  - `Enter` on a selected backlink calls `openFile` and records history.
  - Modal: `B` opens; `Esc` closes; navigation works.
  - Auto-collapse: persistent pane suppressed when `m.height < 20`.
  - Single-modal invariant: pressing `?` while backlinks modal is open swaps content (logs modal renders, backlinks modal does not); pressing `B` while logs modal is open swaps the other way. Both never render simultaneously.
- `diagnostics_test.go`:
  - Footer transient: emitting a diagnostic populates the footer status; status clears after the timeout.
  - Log viewer: pressing `?` opens the modal, lists ring-buffer entries, `Esc` closes.
  - File logging falls back gracefully when the log path is unwritable.
- Existing tests must continue to pass (no regressions to tree-pane behavior).

## Phasing

**Phase 1 (this spec):**
- Wikilink parser, basename index, forward + on-demand reverse indexes.
- Mixed-syntax indexing: standard markdown links contribute to the same indexes as wikilinks.
- `Resolver` interface in `internal/markdown`; `vault.Vault` implements it.
- Renderer integration: resolved wikilinks render as standard links; unresolved get the broken-style SGR with `?` suffix.
- Vault build at startup, watcher-driven refresh (`RefreshFile` on `FileModified`, `Rebuild` on `StructureChanged`).
- Backlinks: persistent bottom pane (`b`) + modal (`B`).
- Snippet extraction (smallest enclosing block) with in-snippet highlight on the link's display text.
- Diagnostic infrastructure: `Diagnostics` interface, footer transient (~3s), JSON-line log file, in-app log viewer modal (`?`), single-modal invariant.

**Phase 2 (separate spec):**
- Block reference (`^block-id`) actual position resolution.
- Auto-scroll to wikilink occurrence in source file when following a backlink.
- "Broken links" tally view aggregating across the vault.

**Phase 3 (separate spec):**
- Tags, frontmatter views, full-text search, graph view — each its own spec.

## Open questions / accepted risks

- **Goldmark extension complexity.** The wikilink syntax overlaps with markdown's `[link]` and `![image]` patterns. The extension must run *before* the standard link parser and consume the `[[...]]` token cleanly. Reference: goldmark's [extension API](https://github.com/yuin/goldmark#extending-goldmark). Risk: a poorly-prioritized extension could break existing link rendering. Mitigation: golden-test existing markdown rendering before/after to confirm parity.
- **Snippet rendering preserves no markdown.** A wikilink inside a list item nested in a blockquote produces a plain-text snippet — bullet, indentation, and `>` are stripped. This is a deliberate choice (snippets are navigation aids, not render targets) but means the snippet may read slightly differently than the source. Acceptable.
- **Reverse index is computed on demand.** O(total references) per `Backlinks(path)` call. At 1000 files × 20 refs/file = 20k iterations per backlinks-pane refresh. Fine for terminal latency. If we ever support 100k-file vaults, materialize the reverse index.
- **Case-insensitive matching is locale-naive.** Uses `strings.ToLower` (ASCII-aware in practice for Go). Vaults with non-ASCII filenames may have surprising matches. Documented; acceptable for now.
- **Renames are not auto-rewritten.** Per the use case framing — Claude owns content, hypogeum is a viewer — a rename that breaks `[[Old Name]]` in other files surfaces as broken links rather than being silently fixed. This is the desired feedback loop.
- **Log file is unbounded.** No rotation in Phase 1. Volume should be low (only `warn` and `error` during normal operation, plus the occasional `info` for refresh races); long-running sessions over many days could still grow the file. If this ever becomes a problem, add a 10MB cap with single-file rotation. The user can also `rm` the log file at any time without affecting the running session — the in-memory ring buffer is independent.
