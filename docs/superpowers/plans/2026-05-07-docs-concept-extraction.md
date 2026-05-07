# Docs Concept Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract six cross-cutting concepts from `docs/` into their own short files under `docs/concepts/`, then update existing specs and package docs to reference them — so the backlinks pane (`b` in hypogeum) is meaningfully populated when reading any doc.

**Architecture:** Markdown-only changes. Each concept doc follows a fixed template (one-liner intro, "See also" hybrid line, Why/How/Gotchas sections). The two dated specs and the package docs replace material that's moving into a concept with a 2–3 sentence summary plus a `[[concept]]` wikilink pointer. The dated `superpowers/plans/` files are untouched (they're scratchpads). After all edits, run hypogeum against `docs/` to verify the backlinks pane shows the expected referrers and no `[[wikilink]]` is broken.

**Tech Stack:** Markdown. No code changes. Verification uses `go build`, `go test`, and a manual run of `go run ./cmd/hypogeum docs/`.

**Spec:** [`docs/superpowers/specs/2026-05-07-docs-concept-extraction-design.md`](../specs/2026-05-07-docs-concept-extraction-design.md).

---

## File Structure

**New files (under `docs/concepts/`):**
- `sentinel-render.md` — how link positions are recovered from Glamour's ANSI output.
- `vault-index.md` — forward + reverse reference index, basename resolution, proximity tiebreak.
- `diagnostics.md` — warn/error stream, footer transient, log file, `?` modal.
- `modal-geometry.md` — single-modal invariant, layout recompute on `B`/`?`, auto-collapse below height 20.
- `return-cursor.md` — path-keyed cursor restoration on Back navigation.
- `link-cursor.md` — content-pane link selection state model (`n`/`p`/`Enter`/`Esc`).

**Modified files:**
- `docs/index.md` — add Concepts section.
- `docs/architecture.md` — link to concepts where it currently mentions them inline.
- `docs/link-following.md` — replace approach section with a summary + `[[sentinel-render]]` and `[[link-cursor]]` pointers.
- `docs/packages/markdown.md` — replace sentinel-trick section with a summary + `[[sentinel-render]]` pointer; cross-link `[[link-cursor]]`.
- `docs/packages/tui.md` — cross-link `[[link-cursor]]`, `[[diagnostics]]`, `[[modal-geometry]]`, `[[return-cursor]]`.
- `docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md` — replace Diagnostics, Geometry, and Vault internals sections with summaries + concept pointers.
- `docs/superpowers/specs/2026-05-07-backlinks-navigation-design.md` — replace return-cursor and modal-layout sections with summaries + concept pointers.
- `CLAUDE.md` — no changes required; the spec list at the top of CLAUDE.md is already concise.

**Untouched:** `docs/packages/{tree,nav,watch}.md` (no cross-cutting concept material to extract); `docs/superpowers/plans/*.md` (scratchpads).

---

## Task ordering rationale

Concept docs are written **before** existing docs are edited to point at them, so the references aren't briefly broken. Verification is at the end. Each task ends with a commit so the work is bisectable.

---

## Task 1: Create `concepts/sentinel-render.md`

**Files:**
- Create: `docs/concepts/sentinel-render.md`

- [ ] **Step 1: Verify the directory does not exist yet**

Run: `ls docs/concepts 2>&1 || echo "NOT EXIST"`
Expected: `NOT EXIST` (or "No such file or directory"). The Write tool will create the directory.

- [ ] **Step 2: Write the concept doc**

Create `docs/concepts/sentinel-render.md` with this exact content:

```markdown
# sentinel-render

The trick that lets `internal/markdown` recover link byte positions from Glamour's ANSI output, even though Glamour itself emits no positional metadata.

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/markdown`](../packages/markdown.md) and [link-following](../link-following.md); press `b` for the full backlinks list.

## Why it exists

Glamour produces ANSI-styled text with theme-dependent SGR codes, no OSC 8 hyperlinks, and no offset information. To make links followable from the right pane, the TUI needs to know where each link's visible text *ended up* in the rendered string — both the byte range (so future phases can splice highlight SGR around it) and the row (so the viewport can scroll to it).

Three approaches were considered: OSC 8 hyperlinks (terminal support is uneven and Glamour doesn't emit them), coordinate mapping during render (would require forking Glamour), and instrumenting Glamour's style with sentinel byte sequences. The third is what shipped.

## How it works

The instrumented renderer is a second `glamour.TermRenderer` whose `link_text` style has `block_prefix = "\x1c"` (ASCII FS — file separator) and `block_suffix = "\x1e"` (ASCII RS — record separator) grafted on. Glamour writes these literally around every link's visible text and they survive the word-wrap pass. After render, a single linear scan over the output records each `(byteStart, byteEnd, row)` pair and strips the sentinels. The cleaned output is byte-equivalent to a plain `Render(src)` on the same terminal — there's a regression test (`TestRenderWithLinks_OutputIsCleanRender` in `internal/markdown/render_test.go`) that catches drift.

The instrumented style is a JSON deep clone of whichever environment default Glamour's `WithAutoStyle` would resolve to (NoTTY / dark / light), with sentinels grafted onto the `LinkText` primitive. **Do not** pass a partial config to `WithStyles` — it's replace-only, not merge, and silently drops everything else (headings, code blocks, margins). The first instrumented render came out unstyled because of this; the deep-clone approach restored visual parity.

Order is preserved by AST cross-reference: the Nth sentinel pair corresponds to the Nth `ASTLink` from `markdown.ExtractLinks`. If the two diverge (e.g. Glamour stops rendering some link form), `RenderWithLinks` falls back to a `Link` with empty `Resolved` rather than failing.

## Invariants / gotchas

- **Sentinels are single-byte ASCII control characters.** Multi-byte sentinels (`\x00LS\x01` etc.) leaked an extra byte into spans during the word-wrap pass. Single bytes survive cleanly.
- **The instrumented `Renderer` is per-width.** Glamour bakes wrap width into the renderer; `WindowSizeMsg` in the TUI rebuilds both the plain and instrumented renderers. Don't cache one across width changes.
- **`\x11` and `\x12` are reserved for snippet highlight.** The vault's snippet extraction wraps the matched display text with these bytes so the formatter can colorize them. Don't reuse `\x1c`/`\x1e` for anything else, and don't run snippets through any pipeline that strips ASCII control characters.
- **The clean-strip is byte-equivalent, not visually identical.** That's checked by a golden test, not a visual assertion. If you change the instrumented style, that test catches drift.
```

- [ ] **Step 3: Commit**

```bash
git add docs/concepts/sentinel-render.md
git commit -m "docs: extract sentinel-render concept"
```

---

## Task 2: Create `concepts/vault-index.md`

**Files:**
- Create: `docs/concepts/vault-index.md`

- [ ] **Step 1: Write the concept doc**

Create `docs/concepts/vault-index.md` with this exact content:

```markdown
# vault-index

The forward + reverse reference index that powers wikilink resolution and backlinks. Lives in `internal/vault`; read at startup, refreshed on watcher events.

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by the [wikilinks-and-backlinks design](../superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md) and [`internal/tui`](../packages/tui.md); press `b` for the full backlinks list.

## Why it exists

A vault is a set of cross-referencing notes. Two questions need to be answered fast and consistently:

1. **Forward (resolve):** given `[[Foo]]` in note A, which file does that point at?
2. **Reverse (backlinks):** given note B, which other notes link *to* it?

Both questions span the whole vault, both are needed on every render of every file with cross-references, and both must include standard markdown links (`[text](path.md)`) as well as wikilinks — the parent spec settled on uniform handling so vaults stay GitHub-compatible.

## How it works

`internal/vault` walks the root once at startup (`Build`) and parses every `.md` file with goldmark's wikilink-extension-equipped parser. Each file becomes a `fileEntry` with a slice of `reference` records (kind = wikilink or stdlink, target name or href, resolved absolute path, optional heading/block/alias).

Forward index: `map[string]*fileEntry` keyed by absolute path.
Name index: `map[string][]string`, lowercased basename → list of absolute paths. Built once during `Build`; consulted during `Resolve`.

Reverse index is computed on demand from the forward index — `Backlinks(path)` iterates `files`, filters references whose `resolved == path`, and returns them in document order. At 1000 files × 20 refs/file = 20k iterations per call, which is invisible at terminal latency. If profiling ever shows it hurts, materialize a reverse map; YAGNI for now.

Refresh is incremental: `RefreshFile(path)` re-parses one file's outgoing references on `watch.FileModified`; `Rebuild()` re-walks the whole root on `watch.StructureChanged`. Both happen synchronously inside the TUI's fsEvent handler — vault sizes are small enough the work is invisible.

### Resolution rules

In order of precedence:

1. **Exact basename match, case-insensitive.** `[[Foo]]` matches `Foo.md`, `foo.md`, `notes/FOO.md`.
2. **Proximity tiebreaker.** Compute relative path from `fromFile` to each candidate; pick the shortest. Lexical path order breaks ties.
3. **No-match → unresolved.** Renderer emits the broken-style placeholder (`?` suffix, dim red SGR).

The "name" stored in the index is `strings.ToLower(basenameWithoutExt(path))`. The forms `[[Foo|alias]]`, `[[Foo#Heading]]`, and `[[Foo^block]]` all resolve by the `Foo` portion; alias/heading/block are recorded separately.

## Invariants / gotchas

- **Vault is best-effort.** If `vault.Build` fails, `tui.New` continues with a nil vault — wikilinks render as broken, backlinks pane stays empty. Same graceful-degradation rule as the watcher.
- **`markdown` does not import `vault`.** It defines a `Resolver` interface that `*vault.Vault` happens to satisfy. Tests of `markdown` use a fake. Keeps the package layering clean.
- **Mixed-syntax indexing is uniform.** A backlink to `notes/foo.md` shows up regardless of whether the linking file used `[[Foo]]` or `[Foo](notes/foo.md)`. The `Kind` field on `Backlink` lets the UI optionally render a small badge.
- **Renames are not auto-rewritten.** A rename that breaks `[[Old Name]]` in other files surfaces as broken links. This is the desired feedback loop — Claude owns content, hypogeum is a viewer.
- **Case-insensitive matching is locale-naive.** `strings.ToLower` is ASCII-safe in practice. Vaults with non-ASCII filenames may have surprising matches. Acceptable.
- **Reverse index is recomputed per call.** A backlinks pane that re-renders on every cursor move would re-iterate. The TUI caches `m.backlinks` after `refreshBacklinks` to avoid this.
```

- [ ] **Step 2: Commit**

```bash
git add docs/concepts/vault-index.md
git commit -m "docs: extract vault-index concept"
```

---

## Task 3: Create `concepts/diagnostics.md`

**Files:**
- Create: `docs/concepts/diagnostics.md`

- [ ] **Step 1: Write the concept doc**

Create `docs/concepts/diagnostics.md` with this exact content:

```markdown
# diagnostics

The single internal stream of `info`/`warn`/`error` events that surfaces non-fatal issues to the user via three observers: a transient footer status, an append-only log file, and an in-app log viewer modal (`?`).

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/tui`](../packages/tui.md) and the [wikilinks-and-backlinks design](../superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md); press `b` for the full backlinks list.

## Why it exists

Several subsystems can fail non-fatally — a parse failure on one vault file, a `RefreshFile` race when a file is deleted between event and read, a watcher that exhausts inotify limits. The TUI must surface these without aborting and without forcing the user to know to look. Three observers cover the three usage modes:

- **Real-time.** Footer transient: the most recent diagnostic appears for ~3 seconds, then clears.
- **Audit.** JSON-line log file the user can `tail` from another terminal.
- **In-session review.** `?` opens a modal showing the last 200 entries from an in-memory ring buffer.

A single severity-tagged stream means new diagnostics added later (render times, rebuild durations) land at `info` without any plumbing changes.

## How it works

The TUI owns the diagnostic sink. It implements a `vault.Diagnostics` interface (`Info(string)`, `Warn(string)`, `Error(string)`) and passes itself to `vault.Build`. The vault calls back through the interface; `internal/tui` adds UI-side issues directly. All three calls fan out to the three observers.

**Footer transient:** the latest diagnostic populates `m.status`. A `tea.Tick` clears it after ~3s. Severity is shown via color cue (warn = yellow, error = red, info = dim).

**Log file:** appended to `$XDG_STATE_HOME/hypogeum/hypogeum.log` (Linux) or `~/Library/Logs/hypogeum/hypogeum.log` (macOS). One JSON line per entry: `{ts, severity, source, message}`. Path resolution falls back to `~/.local/state/hypogeum/` if `XDG_STATE_HOME` isn't set. If no path is writable, file logging silently disables — the in-memory buffer and footer still work.

**In-app log viewer modal (`?`):** reuses the modal infrastructure built for backlinks. The 200-entry ring buffer is the source. Severity color cues match the footer. `Esc` closes; `j`/`k` scroll. See [[modal-geometry]] for layout rules.

## Invariants / gotchas

- **Phase 1 emits only `warn` and `error` (and one `info` for `RefreshFile` races).** Severity is plumbed through so future diagnostics can land at `info` without API changes.
- **The log file is unbounded.** No rotation in Phase 1. Volume is low (one warn per parse failure, etc.); long-running sessions over many days could grow the file. If this becomes a problem, add a 10MB cap with single-file rotation. The user can `rm` the log file at any time without affecting the running session — the in-memory ring buffer is independent.
- **The diag sink is required by `vault.Build`.** Tests pass a no-op implementation. Don't make `Diagnostics` optional or nil-handling will leak into vault code.
- **Single-modal invariant.** Pressing `?` while the backlinks modal is open swaps content — the two modals share one viewport. See [[modal-geometry]].
- **Don't log secrets.** No content of vault files goes through diagnostics — only paths and short error messages. The log file is plain-text on disk.
```

- [ ] **Step 2: Commit**

```bash
git add docs/concepts/diagnostics.md
git commit -m "docs: extract diagnostics concept"
```

---

## Task 4: Create `concepts/modal-geometry.md`

**Files:**
- Create: `docs/concepts/modal-geometry.md`

- [ ] **Step 1: Write the concept doc**

Create `docs/concepts/modal-geometry.md` with this exact content:

```markdown
# modal-geometry

The single-modal invariant and layout rules that govern the backlinks pane (`b`), backlinks modal (`B`), and log viewer modal (`?`).

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/tui`](../packages/tui.md), the [wikilinks-and-backlinks design](../superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md), and the [backlinks-navigation design](../superpowers/specs/2026-05-07-backlinks-navigation-design.md); press `b` for the full backlinks list.

## Why it exists

Three surfaces compete for screen space and input focus: the persistent backlinks bottom split (`b`), the backlinks modal overlay (`B`), and the log viewer modal (`?`). Without a coordination rule they'd stack, conflict over `Esc`, and require per-surface geometry calculations. The single-modal invariant and shared modal viewport collapse this to one decision per keypress: open / swap / close.

## How it works

**Persistent pane (`b`):** `m.backlinksOpen` toggles. When open *and* `m.height >= 20`, the content viewport's height shrinks by `backlinksHeight` (8 rows including border). When `m.height < 20`, the pane is suppressed in `View()` but `m.backlinksOpen` stays true — when the terminal grows again, the pane reappears.

**Modals (`B`, `?`):** `m.modalOpen` is a single enum (`modalNone`/`modalBacklinks`/`modalLogs`). Pressing `B` while `modalLogs` is up swaps to `modalBacklinks` (and vice versa). The two modals share one viewport (`m.modalVP`) and one set of geometry — content is the only thing that changes. While any modal is open, geometry is recomputed as if `backlinksOpen` were false; the content viewport reclaims the bottom split's space and the modal renders centered on top.

**Modal size:** fixed at 60% width × 60% height, clamped to min 40 cols × 12 rows, max 120 cols × 40 rows.

**`Esc` priority** (extending the existing chain):
1. If a modal is open → close it.
2. Else if `m.focus == focusBacklinks` → restore `prevFocus` (pane stays open).
3. Else if `m.linkCursor >= 0` → clear it.
4. Else → no-op.

## Invariants / gotchas

- **`B` and `?` are mutually aware.** They can never render simultaneously. Pressing one while the other is open swaps content, doesn't stack. Tests assert this.
- **The persistent pane and a modal can coexist as state.** When a modal opens with the pane open, the pane is hidden in `View()` for that frame; closing the modal brings the pane back. State and rendering are decoupled.
- **`prevFocus` is saved on modal open.** Opening from the backlinks pane saves `focusBacklinks` so `Esc` returns there, not to `focusContent`. There's a subtle bug in this area (recently fixed in commit `3df72c0`) — opening a modal from the backlinks pane must not stomp `prevFocus` if it was already set during the pane open.
- **Below height 20, the pane is suppressed but not closed.** The user's intent (`backlinksOpen = true`) is honored; only the rendering is conditional. This is the same graceful-degradation rule as the watcher and vault.
- **Modal viewport is shared.** Don't add per-modal scroll state — both modals scroll the same `modalVP`, with content swapped on open.
```

- [ ] **Step 2: Commit**

```bash
git add docs/concepts/modal-geometry.md
git commit -m "docs: extract modal-geometry concept"
```

---

## Task 5: Create `concepts/return-cursor.md`

**Files:**
- Create: `docs/concepts/return-cursor.md`

- [ ] **Step 1: Write the concept doc**

Create `docs/concepts/return-cursor.md` with this exact content:

```markdown
# return-cursor

The single-slot cursor restoration that lets the user follow a backlink with `Enter`, navigate back with `h`, and resume scanning the backlinks list at the entry they followed from.

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/tui`](../packages/tui.md) and the [backlinks-navigation design](../superpowers/specs/2026-05-07-backlinks-navigation-design.md); press `b` for the full backlinks list.

## Why it exists

The "scan backlinks one at a time" workflow is: open the pane (`b`), move cursor (`j`/`k`) to an entry, follow it (`Enter`), read the source file briefly, return (`h`), move to the next entry (`j`), repeat. Without restoration, every return drops the user at backlink cursor 0 — they have to scroll back to where they were. With restoration, the loop is tight and the cursor matches their mental model.

The state to restore is small (which file, which cursor index, which surface — pane or modal), and it's only valid for *one* return — going back twice, forward, or to an unrelated file via the tree should discard it.

## How it works

Single-slot state on the model:

```go
type returnCursor struct {
    sourceFile string             // the file whose backlinks were being navigated
    cursor     int                // backlinkCursor at follow time
    surface    backlinksSurface   // surfacePane | surfaceModal
}

returnCursor *returnCursor  // nil when no follow is pending return
```

**Set on follow** (inside `followBacklink`, before `openFile` mutates history):

```go
m.returnCursor = &returnCursor{
    sourceFile: m.history.Current(),
    cursor:     m.backlinkCursor,
    surface:    m.activeBacklinksSurface(),
}
```

**Consumed on Back** (after `history.Back()` and `refreshContent`):

```go
if m.returnCursor != nil && path == m.returnCursor.sourceFile {
    m.refreshBacklinks(path)
    m.backlinkCursor = clamp(m.returnCursor.cursor, 0, len(m.backlinks)-1)
    switch m.returnCursor.surface {
    case surfacePane:
        if m.shouldShowBacklinks() {
            m.focus = focusBacklinks
        }
    case surfaceModal:
        m.modalOpen = modalBacklinks
        m.refreshBacklinksModal(path)
    }
    m.returnCursor = nil
}
```

The `path == m.returnCursor.sourceFile` check is path-keyed, not time-keyed: if the user navigates Back twice, the second Back lands on a *different* file, the check fails, and the slot is left untouched (it'll be consumed if they ever return to the original — though in practice the user has moved on by then). The slot is cleared either way on the next successful match-and-restore.

## Invariants / gotchas

- **Single-slot.** Only the most recent follow is remembered. Following a second backlink before returning overwrites the slot. This matches the user's mental model — "the last place I came from" is what `h` should restore.
- **Path-keyed lifetime, not time-keyed.** A stale `returnCursor` is harmless: it sits there until either the user returns to its `sourceFile` (consumed) or some unrelated path eventually matches `sourceFile` (rare; restoration would still be valid because the cached cursor was at that file's backlink list).
- **Cursor is clamped on restore.** If the vault refreshed between follow and return and the selected backlink no longer exists, the cursor lands on a neighbor. Test: `TestReturnCursor_ClampsToShrunkList` in `internal/tui/backlinks_test.go`.
- **Surface restoration matters.** A user who followed from a modal expects to land back in a modal, not in the pane. The slot records which surface was active at follow time; the restore branches on it.
- **The pane being closed at return time is fine.** If the user closed the pane between follow and return, `m.backlinksOpen` is false; we don't reopen it. Cursor is still restored in case they reopen later.
- **Vault refresh between follow and return is also fine.** `refreshBacklinks` re-queries; the clamp handles list-shrink.
```

- [ ] **Step 2: Commit**

```bash
git add docs/concepts/return-cursor.md
git commit -m "docs: extract return-cursor concept"
```

---

## Task 6: Create `concepts/link-cursor.md`

**Files:**
- Create: `docs/concepts/link-cursor.md`

- [ ] **Step 1: Write the concept doc**

Create `docs/concepts/link-cursor.md` with this exact content:

```markdown
# link-cursor

The integer index that tracks which link in the rendered content pane is currently selected. Bound to `n`/`p`/`Enter`/`Esc` while the right pane has focus.

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [link-following](../link-following.md), [`internal/markdown`](../packages/markdown.md), and [`internal/tui`](../packages/tui.md); press `b` for the full backlinks list.

## Why it exists

The user needs to follow a `[text](path.md)` or `[[wikilink]]` from the right pane without leaving the keyboard. Three approaches were considered:

- **OSC 8 hyperlinks.** Terminal support is uneven; Glamour doesn't emit them.
- **Numbered link picker (modal).** Cheaper but unbrowserlike. Rejected once the sentinel-render trick was proven.
- **Cursor over an in-order link list.** What shipped. `n` next, `p` previous, `Enter` follows, `Esc` clears. The cursor is footer-only in Phase 1; Phase 2 adds inline highlight by re-splicing SGR around the selected link's byte range.

The cursor is a single integer because [[sentinel-render]] guarantees the link list is in document order and every link has a known row in the rendered output.

## How it works

`Model.linkCursor int` holds the selection. `-1` means no link selected. `Model.links []markdown.Link` is the document's link list, refreshed on every render.

**Cycling:** `n` increments, `p` decrements, both wrap at the ends. After the move, `scrollToLink` adjusts `m.viewport.YOffset` so the selected link's row is in view.

**Following (`Enter` when `linkCursor >= 0`):** branches on `Resolved.Kind`:
- `LinkLocalFile` — `openFile(target)` plus `selectInTree(target)`. Records history; moves the tree cursor if the path is in the tree.
- `LinkExternal` — Status bar: `"external link not opened: <href>"`. Phase 3 will hand off to `xdg-open`/`open` after a confirm flow.
- `LinkAnchor` — Status bar: `"anchor navigation not implemented"`. Phase 2 will resolve to a heading row.
- `LinkInvalid` — Status bar: `"unrecognized link"`.

**Clearing (`Esc`):** sets `linkCursor = -1`. This is one step in the `Esc` priority chain; see [[modal-geometry]] for the full chain.

**Reset on refresh:** every call to `refreshContent` (history navigation, file open, watcher refresh, resize) resets `linkCursor` to `-1`. The link cursor is per-document; it doesn't survive a navigation. A link list from a document the user is no longer viewing would point at a dead row.

## Invariants / gotchas

- **Content-pane scoped.** `n`/`p`/`Esc` and link-aware `Enter` only fire when `focus == focusContent`. Tree-pane bindings are unaffected.
- **Reset on every `refreshContent`.** Pair `links` and `linkCursor` or accept stale UI.
- **Footer marker is `→ <target> [k/n]` when selected.** The constant `linkFooterMarker` is package-public for tests to assert on.
- **Unresolved wikilinks aren't in the cycler.** They render as plain text with a `?` suffix — visible to the user but not selectable with `n`/`p`. Intentional: a broken link can't be followed, so adding it to the cycler would be a confusing no-op.
- **Phase 1 has no inline highlight.** The cursor is footer-only. The rendered text doesn't change when the cursor moves. Phase 2 adds the highlight via SGR re-splicing.
```

- [ ] **Step 2: Commit**

```bash
git add docs/concepts/link-cursor.md
git commit -m "docs: extract link-cursor concept"
```

---

## Task 7: Update `docs/index.md` with Concepts section

**Files:**
- Modify: `docs/index.md`

- [ ] **Step 1: Read the current index**

Read `docs/index.md` to confirm structure. Current sections: `## Architecture`, `## Active feature work`, `## Conventions for adding to this folder`.

- [ ] **Step 2: Insert the new Concepts section between Architecture and Active feature work**

Edit `docs/index.md` to add a new section after the Architecture block (which ends at the watch.md bullet) and before the `## Active feature work` heading. Use Edit with this `old_string`:

```
  - [`internal/watch`](packages/watch.md) — fsnotify-backed live-update watcher, debounced and tree-aware

## Active feature work
```

and this `new_string`:

```
  - [`internal/watch`](packages/watch.md) — fsnotify-backed live-update watcher, debounced and tree-aware

## Concepts

Cross-cutting ideas that show up in multiple specs and packages. Each is its own short file so the backlinks pane (`b`) shows everywhere it's referenced. Wikilink syntax is used here (and only here) to dogfood the feature; the rest of this index uses standard markdown links so GitHub renders them.

- [[sentinel-render]] — how link positions are recovered from Glamour's ANSI output
- [[vault-index]] — forward + reverse reference index, basename resolution, proximity tiebreak
- [[diagnostics]] — the warn/error stream, footer transient, log file, `?` modal
- [[modal-geometry]] — single-modal invariant and layout recompute on `B`/`?`
- [[return-cursor]] — path-keyed cursor restoration on Back navigation
- [[link-cursor]] — content-pane link selection (`n`/`p`/`Enter`/`Esc`) state model

## Active feature work
```

- [ ] **Step 3: Commit**

```bash
git add docs/index.md
git commit -m "docs: add Concepts section to docs/index.md"
```

---

## Task 8: Update `docs/architecture.md` to link concepts

**Files:**
- Modify: `docs/architecture.md`

- [ ] **Step 1: Read the architecture doc**

Read `docs/architecture.md`. Identify three places where it touches concept material:

1. The "Pre-flatten the tree" / "Re-render on resize" trade-offs section discusses width-rebuild — this is general, no concept link needed.
2. The "Cross-cutting concerns" bullet list mentions style detection, path resolution, history semantics, hidden-entry filtering, empty-directory pruning. None of these map to extracted concepts.
3. The doc currently does *not* mention the sentinel trick, vault, diagnostics, modals, return cursor, or link cursor — those are described in the per-package docs and specs.

So the architecture doc itself doesn't need concept-pointer edits; its role is the package map. Add a single new bullet to "Cross-cutting concerns" that points readers at the new concepts folder.

- [ ] **Step 2: Add a Concepts pointer to the cross-cutting list**

Edit `docs/architecture.md` to add a final bullet to the Cross-cutting concerns list. Use Edit with this `old_string`:

```
- **Empty-directory pruning** lives in `tree`. A directory with no `.md` anywhere underneath doesn't appear in the tree at all.

When you add a new concern, decide its owner first.
```

and this `new_string`:

```
- **Empty-directory pruning** lives in `tree`. A directory with no `.md` anywhere underneath doesn't appear in the tree at all.
- **Cross-cutting concepts** that span multiple packages or specs (the sentinel-render trick, the vault index, diagnostics, modal geometry, the return cursor, the link cursor) live in [`docs/concepts/`](concepts/). The docs index lists them; package docs and specs link to them by name.

When you add a new concern, decide its owner first.
```

- [ ] **Step 3: Commit**

```bash
git add docs/architecture.md
git commit -m "docs: cross-reference concepts/ from architecture overview"
```

---

## Task 9: Update `docs/link-following.md` to delegate to concepts

**Files:**
- Modify: `docs/link-following.md`

- [ ] **Step 1: Read the current file**

Read `docs/link-following.md`. The "Approach: instrumented render + status-line cursor" section (lines 15–24) currently inlines:
- The sentinel-render explanation (paragraphs 1–2 of that section).
- The link-cursor explanation (paragraph 3 of that section).

Replace this section with a 2–3 sentence summary plus pointers.

- [ ] **Step 2: Replace the Approach section body**

Edit `docs/link-following.md`. Use Edit with this `old_string`:

```
## Approach: instrumented render + status-line cursor

Glamour produces ANSI output with no positional metadata, no OSC 8 hyperlinks, and theme-dependent SGR codes for link styling. Three approaches were investigated; the chosen one is **instrument the renderer**: pass a custom style with sentinel byte sequences in `link_text.block_prefix` / `block_suffix`. Glamour writes them literally around every link's visible text. A post-pass over the rendered string finds each sentinel pair and records `(byteStart, byteEnd)`. Order-preserving cross-reference with goldmark's AST attaches the original `href`.

Verified working on word-wrapped multi-line links (the wrap splits the span across rows but the sentinels still bracket it).

The "cursor" itself is a single integer index into the link list. Phase 1 surfaces it only in the footer (`[3/7] notes/first.md`) — no inline highlight. Phase 2 adds the inline highlight by re-splicing SGR codes around the active link's byte range. Phase 3 handles external URLs via `os/exec`.

Alternative considered: numbered link picker (modal). Cheaper but less browser-like; rejected once the sentinel trick was proven.
```

and this `new_string`:

```
## Approach: instrumented render + status-line cursor

Two pieces, each with its own concept doc:

- **Recovering link positions** from Glamour's ANSI output: the renderer is instrumented with sentinel byte sequences that survive word-wrap. Full design and rationale: [[sentinel-render]].
- **Selecting and following a link** with `n`/`p`/`Enter`/`Esc`: a single integer cursor into the link list, footer-only in Phase 1. Full design and rationale: [[link-cursor]].

Alternative considered: numbered link picker (modal). Cheaper but less browser-like; rejected once the sentinel trick was proven.
```

- [ ] **Step 3: Commit**

```bash
git add docs/link-following.md
git commit -m "docs: delegate link-following approach to concept docs"
```

---

## Task 10: Update `docs/packages/markdown.md` to delegate sentinel-render

**Files:**
- Modify: `docs/packages/markdown.md`

- [ ] **Step 1: Read the file**

Read `docs/packages/markdown.md`. The "## The sentinel trick" section (lines 59–65) inlines the sentinel mechanism. Replace it with a summary plus pointer.

- [ ] **Step 2: Replace the sentinel-trick section**

Edit `docs/packages/markdown.md`. Use Edit with this `old_string`:

```
## The sentinel trick

Glamour produces ANSI output with no positional metadata, no OSC 8 hyperlinks, and theme-dependent SGR codes. To recover link positions, the instrumented renderer injects two ASCII separator characters (`\x1c` FS, `\x1e` RS) into Glamour's `link_text` style as `block_prefix` / `block_suffix`. Glamour writes them literally around every link's visible text and they survive word-wrap. A single pass over the rendered output strips the sentinels, records each pair's `(row, text)`, and cross-references with the AST.

The instrumented style is a JSON deep clone of whichever environment default Glamour's `WithAutoStyle` would resolve to (NoTTY / dark / light), with sentinels grafted onto the `LinkText` primitive. **Do not** pass a partial config to `WithStyles` — it's replace-only, not merge, and silently drops everything not specified. There's a regression test that catches this.

Full design rationale and the alternatives we rejected (OSC 8, coordinate mapping) are in [link-following.md](../link-following.md).
```

and this `new_string`:

```
## The sentinel trick

The instrumented renderer injects two ASCII separator characters (`\x1c` FS, `\x1e` RS) into Glamour's `link_text` style. Glamour writes them around every link's visible text; a post-pass strips them and records `(row, text)`. The cleaned output is byte-equivalent to a plain `Render` on the same terminal — verified by `TestRenderWithLinks_OutputIsCleanRender`. Full design and rationale (including the alternatives we rejected): [[sentinel-render]].
```

- [ ] **Step 3: Commit**

```bash
git add docs/packages/markdown.md
git commit -m "docs(markdown): delegate sentinel-trick details to concept doc"
```

---

## Task 11: Update `docs/packages/tui.md` to cross-link concepts

**Files:**
- Modify: `docs/packages/tui.md`

- [ ] **Step 1: Read the file**

Read `docs/packages/tui.md`. The doc covers the Model, key dispatch, link cursor invariants, and footer rendering, but predates the wikilinks/backlinks work — it doesn't mention diagnostics, modal geometry, or return cursor at all. The link-cursor invariant (line 51) and the "Why Model holds both `links` and `linkCursor`" section (lines 76–78) are where to add the `[[link-cursor]]` cross-link.

The other concepts ([[diagnostics]], [[modal-geometry]], [[return-cursor]]) aren't documented in this file; rather than inlining them now, add a short "Backlinks and modal surfaces" section after the existing content that points at the three concepts so a reader of `tui.md` arrives at them.

- [ ] **Step 2: Update the link-cursor invariant line**

Edit `docs/packages/tui.md`. Use Edit with this `old_string`:

```
- **Link bindings are content-pane scoped.** `n`/`p`/`Esc` and link-aware `Enter` only fire when `focus == focusContent`. The tree pane's bindings are unaffected.
```

and this `new_string`:

```
- **Link bindings are content-pane scoped.** `n`/`p`/`Esc` and link-aware `Enter` only fire when `focus == focusContent`. The tree pane's bindings are unaffected. Full state model: [[link-cursor]].
```

- [ ] **Step 3: Append a "Backlinks and modal surfaces" section at the end of the file**

Use Edit with this `old_string`:

```
## Footer rendering

`renderFooter` always shows the current file path (relative to the tree root) plus the help string. When a link is selected, it prepends `"→ "` and appends `[k/n] <target>`. The marker constant (`linkFooterMarker`) is package-public for tests to assert on.
```

and this `new_string`:

```
## Footer rendering

`renderFooter` always shows the current file path (relative to the tree root) plus the help string. When a link is selected, it prepends `"→ "` and appends `[k/n] <target>`. The marker constant (`linkFooterMarker`) is package-public for tests to assert on.

## Backlinks and modal surfaces

The TUI hosts three additional surfaces beyond the two-pane core: the persistent backlinks pane (`b`), the backlinks modal (`B`), and the log viewer modal (`?`). They share input rules, geometry, and a single `prevFocus` slot. Each has its own concept doc:

- [[modal-geometry]] — single-modal invariant, layout recompute on open, auto-collapse below height 20, `Esc` priority chain.
- [[diagnostics]] — the warn/error stream that feeds the footer transient and the `?` modal.
- [[return-cursor]] — single-slot cursor restoration that survives `Enter`-follow → `h`-back round trips on backlinks.

The vault that powers backlinks is documented at [[vault-index]].
```

- [ ] **Step 4: Commit**

```bash
git add docs/packages/tui.md
git commit -m "docs(tui): cross-link concept docs from package doc"
```

---

## Task 12: Update wikilinks-and-backlinks spec — Diagnostics section

**Files:**
- Modify: `docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md`

- [ ] **Step 1: Read the Diagnostics subsection**

Read lines 289–299 of `docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md`. This is the Diagnostics subsection inside Error handling.

- [ ] **Step 2: Replace the Diagnostics subsection body with a summary**

Use Edit with this `old_string`:

```
### Diagnostics

Errors and warnings during indexing (parse failures, `RefreshFile` races, watcher errors) feed a single internal diagnostic stream with three observers:

1. **Transient footer status.** The most recent diagnostic appears in the footer for ~3 seconds, then clears. Surfaces problems in real time without requiring the user to know to look.
2. **Log file.** Appended to `$XDG_STATE_HOME/hypogeum/hypogeum.log` (Linux) or `~/Library/Logs/hypogeum/hypogeum.log` (macOS), one JSON line per entry (`{ts, severity, source, message}`). Path resolution falls back to `~/.local/state/hypogeum/` if `XDG_STATE_HOME` isn't set. If no path is writable, file logging is silently disabled (the in-memory buffer and footer still work).
3. **In-app log viewer.** Key `?` opens a modal showing the last 200 diagnostic entries from an in-memory ring buffer. Reuses the modal infrastructure built for backlinks. Severity is shown via color cue (warn = yellow, error = red, info = dim). `Esc` closes; `j`/`k` scroll.

Severity levels: `info`, `warn`, `error`. Phase 1 emits only `warn` and `error` (and the one `info` for `RefreshFile` races). Severity is plumbed through so future diagnostics (render times, rebuild durations) can land at `info` without changing the API.

Diagnostic emission point lives in `internal/vault` (`v.warn(format, args...)`, `v.errorf(...)`, etc.) and in `internal/tui` for UI-side issues. Both push to a shared diagnostic sink owned by the TUI model — the TUI hands the sink to the vault during `Build`. This keeps the diagnostic stream a TUI concern (which it is — it's user-facing) without coupling `vault` to the TUI; `vault` accepts a `Diagnostics` interface (`Warn(string)`, `Error(string)`, `Info(string)`) that the TUI implements.
```

and this `new_string`:

```
### Diagnostics

Errors and warnings during indexing feed a single internal stream with three observers: a transient footer status, an append-only log file, and an in-app log viewer modal (`?`). Severity levels are `info`/`warn`/`error`; Phase 1 emits only `warn` and `error` (plus one `info` for `RefreshFile` races). The vault accepts a `Diagnostics` interface (`Warn(string)`, `Error(string)`, `Info(string)`) implemented by the TUI, so the stream is a TUI concern without coupling `vault` to the UI. Full design and rationale: [[diagnostics]].
```

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md
git commit -m "docs(spec): summarize Diagnostics section, point at concept"
```

---

## Task 13: Update wikilinks-and-backlinks spec — Geometry section

**Files:**
- Modify: `docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md`

- [ ] **Step 1: Read the Geometry block**

The Geometry block lives inside the `internal/tui` (changes) section, around lines 204–208. It currently inlines the modal sizing rules, single-modal invariant, and `Esc` priority.

- [ ] **Step 2: Replace the Geometry block with a summary**

Use Edit with this `old_string`:

```
Geometry:
- `b` toggles `backlinksOpen`. When open *and* `m.height >= 20`, the content viewport's height is reduced by `backlinksHeight` (8 rows including its border). When `m.height < 20`, `backlinksOpen` is honored as state but the pane is suppressed in `View()` — when the terminal grows again, the pane reappears.
- `B` toggles the backlinks modal (`modalOpen = modalBacklinks` ↔ `modalNone`). While any modal is open, geometry is recomputed as if `backlinksOpen` were false — the content viewport reclaims the bottom-split's space, and the modal renders centered on top. When the modal is dismissed, if `backlinksOpen` is still true, the bottom split reappears. Modal size is fixed at 60% width × 60% height (clamped to min 40 cols × 12 rows, max 120 cols × 40 rows).
- `?` toggles the log viewer modal (`modalOpen = modalLogs` ↔ `modalNone`). Same modal infrastructure and geometry as the backlinks modal. Mutually exclusive with the backlinks modal — pressing `?` while backlinks modal is open swaps the modal's content; pressing `B` while logs modal is open does the same.
- `Esc` dismisses the modal (any kind) first, then clears the link cursor as today, then is a no-op.
```

and this `new_string`:

```
Geometry: `b` toggles the persistent bottom split; `B` and `?` toggle modals that share one viewport with single-modal-swap semantics. While any modal is open, the bottom split is suppressed; closing the modal restores it. Below `m.height = 20` the persistent pane is suppressed but its state is preserved. Full layout rules and `Esc` priority chain: [[modal-geometry]].
```

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md
git commit -m "docs(spec): summarize Geometry block, point at modal-geometry concept"
```

---

## Task 14: Update wikilinks-and-backlinks spec — Vault internals & resolution

**Files:**
- Modify: `docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md`

- [ ] **Step 1: Read the Wikilink resolution rules section**

The "### Wikilink resolution rules" subsection lives around lines 224–238. It inlines the precedence rules and forms.

- [ ] **Step 2: Replace the Wikilink resolution rules block with a summary**

Use Edit with this `old_string`:

```
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
```

and this `new_string`:

```
### Wikilink resolution rules

Basename-only, case-insensitive, with a proximity tiebreaker on multiple matches; no-match emits the broken-style placeholder. Forms: `[[Foo]]`, `[[Foo|display]]`, `[[Foo#Heading]]`, `[[Foo^block]]` — the latter records the block ID but Phase 1 only lands the user at the file. Full rules, internal index layout, and refresh semantics: [[vault-index]].
```

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md
git commit -m "docs(spec): summarize wikilink resolution, point at vault-index concept"
```

---

## Task 15: Update backlinks-navigation spec — return-cursor section

**Files:**
- Modify: `docs/superpowers/specs/2026-05-07-backlinks-navigation-design.md`

- [ ] **Step 1: Read the Return flow section**

Lines 152–173 cover the return flow with the full `returnCursor` struct and consumption code.

- [ ] **Step 2: Replace the Return flow body with a summary**

Use Edit with this `old_string`:

```
**Return flow (`h` / Back):**

`h` already calls `m.history.Back()` and `m.refreshContent(path)`. We add a check after the refresh:

```go
if m.returnCursor != nil && path == m.returnCursor.sourceFile {
    m.refreshBacklinks(path)
    m.backlinkCursor = clamp(m.returnCursor.cursor, 0, len(m.backlinks)-1)

    switch m.returnCursor.surface {
    case surfacePane:
        if m.shouldShowBacklinks() {
            m.focus = focusBacklinks
        }
    case surfaceModal:
        m.modalOpen = modalBacklinks
        m.refreshBacklinksModal(path)
    }
    m.returnCursor = nil
}
```

The single-slot `returnCursor` is consumed on use. If the user navigates Back twice, Forward, or to an unrelated file via the tree before returning, the slot is discarded — no stale cursor restoration.
```

and this `new_string`:

```
**Return flow (`h` / Back):**

After `m.history.Back()` and `m.refreshContent(path)`, if the slot's `sourceFile` matches `path`, the cached `backlinkCursor` is restored (clamped to the current list length) and the recorded surface — pane focus or modal — is reopened. The slot is single-slot and path-keyed: navigating Back twice, Forward, or to an unrelated file before returning leaves the slot in place but harmless (it'll only fire if the user ever lands back on the recorded `sourceFile`). Full design and rationale: [[return-cursor]].
```

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-05-07-backlinks-navigation-design.md
git commit -m "docs(spec): summarize return-cursor section, point at concept"
```

---

## Task 16: Verify build and tests still pass

**Files:** none (smoke check)

- [ ] **Step 1: Run go build**

Run: `go build ./...`
Expected: no output, exit code 0. (No Go code was changed.)

- [ ] **Step 2: Run go test**

Run: `go test ./...`
Expected: all tests pass. (No Go code was changed.)

- [ ] **Step 3: Manual hypogeum smoke test against docs/**

Run: `go run ./cmd/hypogeum docs/` (in a real terminal, not piped). Walk through each concept doc:

- `concepts/sentinel-render.md` — press `b`. Expected referrers: `index.md` (wikilink), `link-following.md` (wikilink), `packages/markdown.md` (wikilink).
- `concepts/vault-index.md` — press `b`. Expected referrers: `index.md`, the wikilinks-spec, `packages/tui.md`.
- `concepts/diagnostics.md` — press `b`. Expected referrers: `index.md`, the wikilinks-spec, `packages/tui.md`.
- `concepts/modal-geometry.md` — press `b`. Expected referrers: `index.md`, the wikilinks-spec, `packages/tui.md`, `concepts/diagnostics.md` (cross-concept), `concepts/link-cursor.md` (cross-concept).
- `concepts/return-cursor.md` — press `b`. Expected referrers: `index.md`, the backlinks-navigation spec, `packages/tui.md`.
- `concepts/link-cursor.md` — press `b`. Expected referrers: `index.md`, `link-following.md`, `packages/tui.md`, `packages/markdown.md` (if the cross-link in the markdown.md update is added — it is *not* in this plan; markdown.md only links sentinel-render).

Open `docs/index.md` and verify the new Concepts section's `[[wikilinks]]` resolve (no `?` suffix anywhere).

If a concept doc shows fewer referrers than expected, check whether the corresponding source-file edit landed; the path-based proximity rule should not affect this case (every `[[concept]]` is unambiguous given the unique basenames chosen).

Quit hypogeum (`q`) when done.

- [ ] **Step 4: GitHub render check (optional, post-push)**

After pushing the branch, open a couple of changed files on github.com:
- `docs/concepts/sentinel-render.md` — confirm `[architecture](../architecture.md)` renders as a clickable link.
- `docs/index.md` — confirm the `[[wikilink]]` lines render as literal text without breaking the surrounding list.

This is informational; no commit needed if it looks right.

- [ ] **Step 5: No commit needed for verification**

Verification produces no file changes. The work for this plan is complete after Task 15's commit.

---

## Self-review

Spec coverage check (against `docs/superpowers/specs/2026-05-07-docs-concept-extraction-design.md`):

- ✅ Six concept docs (Tasks 1–6) — Architecture: "Six initial concept files".
- ✅ Index updated with Concepts section using wikilinks (Task 7) — Index updates section.
- ✅ Architecture, link-following, package docs, both specs updated to point at concepts (Tasks 8–15) — Components: "Source-spec editing rule" + Phase 1 list.
- ✅ Verification (Task 16) — Testing section (build, hypogeum walk-through, GitHub render check).
- ✅ `superpowers/plans/*` untouched — Out of scope.
- ✅ Filenames are kebab-case, no date prefix, basenames unique — convention enforced.
- ✅ Hybrid "See also" line on every concept doc — concept-doc template.
- ✅ Source-spec edits keep the section heading and replace body with summary — Source-spec editing rule.

Type/name consistency check:

- Concept doc filenames in tasks (sentinel-render, vault-index, diagnostics, modal-geometry, return-cursor, link-cursor) match the spec's table and the index's wikilink list exactly.
- The `[[concept]]` syntax is used uniformly in all replacement bodies.
- Cross-concept links (e.g. `diagnostics.md` referencing `[[modal-geometry]]`) match filenames consistently.

Placeholder check: no "TBD", "implement later", or vague steps. Each Edit shows exact `old_string` and `new_string`.

Plan complete and saved to `docs/superpowers/plans/2026-05-07-docs-concept-extraction.md`.
