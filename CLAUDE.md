# CLAUDE.md

Guidance for Claude Code working in this repo. Keep this file short and accurate — out-of-date guidance is worse than no guidance.

## What this is

`hypogeum` is a terminal markdown browser. Point it at a directory of `.md` files; rendered content fills the screen, `^p` opens a fuzzy file finder, `^b` opens the directory tree in a modal, and `h`/`l` navigate browser-style history.

Built on the Charm stack: Bubble Tea (Elm-style update loop), Bubbles (widgets — viewport, key bindings), Lip Gloss (styling), Glamour (markdown → ANSI).

## Build, test, run

```sh
go build ./...                          # compiles every package
go test ./...                           # runs the full test suite
go run ./cmd/hypogeum [path]            # run against a directory or .md file
go install ./cmd/hypogeum               # install to $GOBIN
```

The TUI requires a real terminal — `go run` from inside a non-TTY harness will produce nothing useful. Use the model-level tests in `internal/tui/model_test.go` to exercise behavior without a terminal.

## Layout

```
cmd/hypogeum/main.go     CLI entrypoint: parses argv, hands off to tui.New
internal/tree/           Walks the filesystem, returns a *Node tree of markdown files
internal/markdown/       Glamour wrapper + link resolution (relative paths, anchors, external URLs)
internal/nav/            Browser-style back/forward history stack, no I/O
internal/watch/          fsnotify-backed live-update watcher, debounced and markdown-aware
internal/tui/            Bubble Tea Model that wires the four above into the content-first UI (tree opens as a modal)
```

The packages are layered: `tui` depends on `tree`, `markdown`, `nav`, `watch`; the lower layers know nothing about the TUI.

## Conventions

- **One package, one job.** `nav` is a pure stack — adding filesystem awareness to it is the wrong move; resolve paths in `markdown` or `tui` instead.
- **Pre-flatten for keystroke performance.** The tree is walked into `[]treeRow` once in `New`; cursor movement just updates an index. Don't re-walk the tree on keystrokes.
- **Tree is scrolled by `m.tree.vp`, not lipgloss.** `renderTree()` produces all rows; `m.tree.vp.View()` clips them to a visible window and scrolls so `m.tree.cursor` stays in view. Any code path that writes `m.tree.flat` or `m.tree.cursor` must call `m.refreshTreeVP()` afterward; otherwise the rendered viewport stays stale or the cursor can scroll out of frame.
- **Re-render on resize.** `WindowSizeMsg` rebuilds the Glamour renderer at the new wrap width and re-renders the current file. Anything that changes content width must do the same.
- **CLI argument shape:** zero args = cwd; one dir = browse it; one file = open it with the tree rooted at its parent. Anything else is a usage error.
- **Tests live next to the code they test** (`internal/nav/history_test.go`, `internal/tui/model_test.go`).

## Gotchas

- **Empty directories are pruned.** `tree.Walk` drops any directory whose subtree contains zero markdown files (`internal/tree/tree.go`). A user pointing at a folder with only PDFs in it will see an empty tree, not a wall of folders.
- **Auto-open is top-level only.** When no `initialFile` is given, the model picks the *first root-level* `.md` (`firstTopLevelFile` in `internal/tui/model.go`). It does *not* descend into subdirectories — earlier versions did, and the result was landing on the deepest leaf alphabetically. Don't change this back without a strong reason.
- **`tree.Walk` returns a synthesized empty root** when nothing matches, instead of nil — callers don't have to special-case nil. Keep that contract.
- **Hidden entries are skipped** (anything starting with `.`) — `.git`, dotfile notes directories, etc. If you ever expose a flag to include them, do it in `tree`, not `tui`.
- **Glamour renderer is per-width.** It's recreated on every `WindowSizeMsg`. Don't cache it across width changes or wrapping breaks silently.
- **The watcher is best-effort.** If `watch.New` fails (e.g. inotify limits exhausted), `tui.New` swallows the error and the browser runs without live updates rather than refusing to start. Consumers must tolerate `m.watcher == nil`.
- **Watcher events are debounced and coarse.** `internal/watch` collapses fsnotify ops into `StructureChanged` (re-walk the tree) or `FileModified` (re-read the open file). Don't try to plumb finer-grained ops through; the TUI doesn't need them and editors save in bursts that mean per-op handling would re-walk redundantly.
- **Vault is best-effort.** If `vault.Build` fails, `tui.New` continues with a nil vault — wikilinks render as broken (with a `?` suffix), the backlinks modal opens empty. Same graceful-degradation rule as the watcher.
- **Modals swap by default; `?` is anchored.** `b` (backlinks), `^l` (logs), `^p` (picker), and `^b` (tree) all open modals that swap with each other under the single-modal-swap invariant. `?` (help) is the exception: pressing it while another modal is open is a no-op, so the cheat sheet can't steal focus from a mid-task modal. `?` while help is already open still toggles it closed. `Esc` closes whichever modal is up before falling through to the link cursor's clear behavior.
- **Tree expansion state defaults to collapsed and is derived from the current file.** `m.tree.expanded[path] == true` means open; anything else means closed. The root is always treated as expanded by `isExpanded` so the top level is visible without seeding the map. Every navigation (`selectInTree`, called from `navigateTo` / Back / Forward / `followBacklink`) calls `expandAncestorsOf(path)` which **clears** the entire map and re-populates only the ancestor chain. This makes the map a *cache* derived from the current file rather than user state to preserve. Manual `Space`-toggles via `toggleFolder` persist only until the next navigation — a deliberate design choice (see [project memory](../../.claude/projects/-Users-wilkes-Projects-wilkes-hypogeum/memory/project_finder_first_navigation.md)).
- **The tree is a modal, not a side pane.** `^b` opens `modalTree` via the same `openModalWith` path as the picker/backlinks/logs. There is no `m.tree.visible` flag, no `shouldShowTree()`, no width-gated effective state — the modal sizes itself via `modalGeometry` and clamps to a minimum, so narrow terminals get a smaller modal rather than a force-hidden pane. `Enter` on a file row closes the modal (matching the picker); `Enter` on a directory row toggles collapse; `Esc` closes without opening anything. **`←` / `h` collapses an expanded directory; `→` / `l` expands a collapsed one** — these shadow their global `Back`/`Forward` history bindings only while the tree modal is open. The shadow relies on `handleTreeModalKey` running *before* the global `Back`/`Forward` switch in `handleKey`; if you reorder the dispatch blocks, the arrow keys will start stepping through history instead of toggling folders (covered by `TestModel_ArrowKeysShadowHistoryWhileTreeModalOpen`).
- **Tree-row click is gated on `modals.kind == modalTree`.** `handleMouse` only iterates tree-row zones when the tree modal is open. BubbleZone keeps zone bounds across re-renders, so a closed modal's stale row zones would otherwise still catch clicks. Any feature that *acts* on a tree row must verify the modal is open.
- **`View()` must zone-Scan the composed output including the modal.** The tree-row zones live inside `m.tree.vp.View()`, which gets spliced onto the base via `overlayModal` *after* the original Scan would have run. We Scan once at the end on `overlayModal(...)` so BubbleZone records the final screen coordinates. Other modals (picker, backlinks, logs, help) don't have zones inside them and are unaffected by the placement — but if you add zones inside any modal, they need this same Scan-last ordering.
- **Picker grabs printable keys before global modal-toggles.** `handleKey` routes `tea.KeyRunes` to `handlePickerKey` *before* the global modal-toggle switch when the picker is open. Otherwise `b` (now the backlinks modal binding) would swap the picker out the moment a user typed `b` into the fuzzy-filter query. Non-rune keys (`Esc`, `Enter`, `^P`, `^j`, `^k`, arrows) still flow through the normal modal block so the picker can be closed by retoggle and navigated by cursor keys. Add a regression test if you ever introduce another lowercase-letter modal toggle.
- **`^p` opens a flat recency-ranked finder with type-to-filter.** Opens as a `modalKind`. The textinput is focused from the moment the picker opens; printable keystrokes go to the query, and the result list re-filters via `sahilm/fuzzy` (subsequence, case-insensitive). Sort is match score first with the source-order index (i.e. recency rank in `p.all`) as a stable tiebreaker. Empty query falls back to the pure recency list. `^j` / `^k` move the cursor since `j` / `k` now type into the query; arrow keys also move the cursor. `Esc` clears a non-empty query before closing on the second press. Visible rows are capped at `pickerMaxVisible` (200); overflow shows a faint `… N more` footer. The hybrid recency score (filesystem mtime, 7-day half-life + persisted visits, 2-day half-life × 1.5) lives in `internal/recent`. See [finder-fuzzy-filter](docs/superpowers/specs/2026-05-12-finder-fuzzy-filter-design.md) and [unified-finder-recency](docs/superpowers/specs/2026-05-12-unified-finder-recency-design.md).
- **Snippet highlight uses ASCII control chars (`\x11` / `\x12`).** Don't use these bytes in user content (extremely unlikely) and don't rewrite snippets through any pipeline that would strip control chars.
- **Unresolved wikilinks aren't in the link cycler.** They render as plain text with a `?` suffix — visible to the user but not selectable with `n`/`p`. This is intentional: a broken link can't be followed, so adding it to the cycler would be a confusing no-op.
- **Non-markdown files render via `internal/code`, not Glamour.** `refreshContent` (`internal/tui/content.go`) branches on `tree.IsMarkdown(path)`. Markdown goes through `markdown.Renderer.RenderWithLinks`; everything else goes through `code.Renderer.Render`, which is a Chroma → 256-color ANSI → line-number gutter → soft-wrap pipeline. Code files have no `markdown.Link` slice — link cycling (`n`/`p`/`Enter`) is a natural no-op. Tree modal and the `^p` picker stay markdown-only; code files are reachable only via CLI arg or an inline relative link from a markdown file. The watcher's *write* classifier (`internal/watch/classify.go`) accepts any path so live-reload works for the open code file; the *structure* classifier (`stage()`) stays markdown-only so a new `.py` doesn't trigger a tree re-walk.
- **Source embeds (`![[file.go#L10-L20]]`) preprocess to fenced code blocks.** `markdown.Renderer.preprocessEmbeds` runs *before* `preprocessWikilinks` in `RenderWithLinks`. It slices the source file with `internal/embed.SliceFile`, formats a fenced code block with a literal-text gutter inside the fence body (no separate gutter pipeline — that's the deliberate Approach-A simplification), and synthesizes one `Link` per embed so `n`/`p`/`Enter` work on them. Line numbers are *literal*: editing the source shifts embeds to whatever the line numbers now point at. Named anchors are out of scope.
- **Embed live-sync uses `m.content.embedDeps`.** `RenderWithLinks` returns the list of absolute source paths sliced into the output; `refreshContent` persists them and calls `m.watcher.AddPath` for each parent directory. `handleFSEvent`'s `FileModified` branch checks `embedDeps` alongside the open path. A markdown file that *removes* an embed still leaves the prior source dir watched until the watcher is destroyed — cheap, churn-free.
- **Range-link Enter sets `m.content.rangeHighlight`** before `navigateTo`. The code renderer reads it via `RenderOptions.Highlight` and reverse-videos the gutter for those lines. Esc clears the highlight (handled at the *top* of the Esc cascade so it fires before link-cursor clear).

## What's not built yet

**Link following — Phases 1, 2, and 3 shipped:** `n`/`p` cycle through every link in the current document, `Enter` follows the selected one. Local files navigate (history-aware); external `http`/`https` URLs arm a one-keystroke confirm — a second `Enter` exec's `open` / `xdg-open` / `cmd start` depending on platform, any other key cancels. The selected link is highlighted in reverse-video on the rendered page via `markdown.HighlightMarker`. Implementation lives in `internal/markdown` (`ExtractLinks`, `RenderWithLinks`, `HighlightMarker`), `internal/tui/external.go` (the platform exec wrapper), and the content-key handler in `internal/tui/input.go`.

**External URL handoff details:** the opener (`m.openExternal`) is injected for tests; the default is `openExternalURL` which validates the scheme (http/https only — `javascript:`, `data:`, `file:`, `mailto:`, `ftp:` are rejected to avoid shell handoffs of executable URLs) and runs `exec.Cmd.Start()` so the browser detaches and hypogeum keeps responding immediately.

**Wrapped-link highlight:** `stripSentinels` closes reverse-video before each `\n` inside a sentinel-bracketed span and lets the lazy-open path re-emit `openMark` on the first content byte of the next row. This is required because Glamour writes a per-row `\e[0m` tail that would otherwise cancel a once-opened `\e[7m`. Continuation rows include the left-margin indent in the highlight; matches the `less` / `vim` visual-mode idiom.

Full plan and design rationale (including why we picked the sentinel-instrumented render approach over OSC 8 or coordinate mapping) lives in [docs/link-following.md](docs/link-following.md). Phase 2 design notes: [docs/superpowers/specs/2026-05-09-link-following-phase-2-design.md](docs/superpowers/specs/2026-05-09-link-following-phase-2-design.md).

**Wikilinks and backlinks — Phase 1 shipped:** `[[wikilinks]]` resolve via vault index, backlinks modal (`b`), log viewer (`^l`), and backlinks navigation (cursor `j`/`k`, `Enter` to follow with scroll-to-line, `h` restores cursor). Implementation lives in `internal/vault/` and the modal logic in `internal/tui/`.

**Wikilinks and backlinks — Phase 2 in progress:** inline-link pre-selection on backlink-follow / Back / Forward shipped (see [pre-select-inline-link](docs/superpowers/specs/2026-05-09-pre-select-inline-link-design.md)). Remaining: block references (`[[note#^blockid]]`), broken-link tally in the status bar, and configurable vault root. Design outlined in [docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md](docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md).

## Workflow

Every feature lives on its own branch. Before writing a design spec, an implementation plan, or any code for a new feature, `git checkout -b <topic>` off `main` (or use a worktree). The spec, plan, and implementation commits all land on that branch and ship together via PR. Don't commit feature work — including docs-only artifacts that originated from a brainstorming session — directly to `main`.

PRs merge with `gh pr merge --merge`, not squash. Squashing is disabled by repo settings.

## Documentation and plans

Write design docs, implementation plans, and investigation notes to `docs/` at the repo root. Start at [docs/index.md](docs/index.md) — it points at the [architecture overview](docs/architecture.md) (which links to per-package docs in `docs/packages/`) and to active feature plans like [link-following](docs/link-following.md).

One file per topic, kebab-cased filename, no date prefix. Update plans in place as work progresses — strike-through or "Status:" lines beat parallel files. README.md and CLAUDE.md stay at the repo root; everything longer-form goes in `docs/`. Update `docs/index.md` whenever you add a new file under `docs/`.
