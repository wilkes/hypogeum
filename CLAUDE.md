# CLAUDE.md

Guidance for Claude Code working in this repo. Keep this file short and accurate — out-of-date guidance is worse than no guidance.

## What this is

`hypogeum` is a terminal markdown browser. Point it at a directory of `.md` files; the left pane is a directory tree, the right pane renders the current file via Glamour, and `h`/`l` navigate browser-style history.

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
internal/tui/            Bubble Tea Model that wires the three above into the two-pane UI
```

The packages are layered: `tui` depends on `tree`, `markdown`, `nav`; the lower layers know nothing about the TUI.

## Conventions

- **One package, one job.** `nav` is a pure stack — adding filesystem awareness to it is the wrong move; resolve paths in `markdown` or `tui` instead.
- **Pre-flatten for keystroke performance.** The tree is walked into `[]treeRow` once in `New`; cursor movement just updates an index. Don't re-walk the tree on keystrokes.
- **Re-render on resize.** `WindowSizeMsg` rebuilds the Glamour renderer at the new wrap width and re-renders the current file. Anything that changes content width must do the same.
- **CLI argument shape:** zero args = cwd; one dir = browse it; one file = open it with the tree rooted at its parent. Anything else is a usage error.
- **Tests live next to the code they test** (`internal/nav/history_test.go`, `internal/tui/model_test.go`).

## Gotchas

- **Empty directories are pruned.** `tree.Walk` drops any directory whose subtree contains zero markdown files (`internal/tree/tree.go`). A user pointing at a folder with only PDFs in it will see an empty tree, not a wall of folders.
- **Auto-open is top-level only.** When no `initialFile` is given, the model picks the *first root-level* `.md` (`firstTopLevelFile` in `internal/tui/model.go`). It does *not* descend into subdirectories — earlier versions did, and the result was landing on the deepest leaf alphabetically. Don't change this back without a strong reason.
- **`tree.Walk` returns a synthesized empty root** when nothing matches, instead of nil — callers don't have to special-case nil. Keep that contract.
- **Hidden entries are skipped** (anything starting with `.`) — `.git`, dotfile notes directories, etc. If you ever expose a flag to include them, do it in `tree`, not `tui`.
- **Glamour renderer is per-width.** It's recreated on every `WindowSizeMsg`. Don't cache it across width changes or wrapping breaks silently.

## What's not built yet

The README calls out link-following from inside rendered content as the next milestone. The plumbing is there (`markdown.ResolveLink` already classifies local/external/anchor links) but nothing in the TUI surfaces clickable links or moves a cursor between them yet. When wiring that up: the resolver returns absolute paths for local files, which can be fed straight into `model.openFile`.
