# hypogeum

A terminal browser for markdown directories. Point it at a folder of `.md` files and wander through them — the rendered file fills the screen, `^p` opens a fuzzy file finder, `t` opens the directory tree in a modal, and links between files navigate with `Enter`. `h` and `l` go back and forward through your history, like a browser.

The name is the Greek word for an underground chamber or network of chambers (*hupó* "under" + *gê* "earth"). It shares a root with *hyperlink* (*hupér* "above") — the references float above the text, the chambers wait below.

## Status

Renders GitHub Flavored Markdown via [Glamour](https://github.com/charmbracelet/glamour), walks directory trees, navigates file-to-file. Wikilinks (`[[note]]`) and standard markdown links resolve across the whole vault; press `b` to see what links to the current file (and `B` for the same view as a centered modal). The selected inline link is highlighted in reverse-video on the page; following a backlink, going back, or going forward leaves the link you came from pre-selected, so `n`/`p` cycling resumes from a meaningful position.

## Install

Pre-built binaries are on the [releases page](https://github.com/wilkes/hypogeum/releases) — download the archive for your platform, extract, and put the `hypogeum` binary on your `$PATH`. Or:

```sh
go install github.com/wilkes/hypogeum/cmd/hypogeum@latest
```

Run `hypogeum --version` to confirm which build you're on.

## Usage

```sh
hypogeum                  # browse the current directory
hypogeum ~/notes          # browse a specific directory
hypogeum ~/notes/index.md # open a specific file; tree roots at its directory
```

## Keys

| Key | Action |
|-----|--------|
| `↑` / `k`, `↓` / `j` | Move within the focused pane |
| `Enter` | Open the selected file / follow selected link |
| `h` / `←` | Back (collapse folder when tree modal is open) |
| `l` / `→` | Forward (expand folder when tree modal is open) |
| `n` / `N` | Cycle to next / previous link |
| `v` | Start keyboard selection (then `Space` to anchor, motion to extend, `y` to copy) |
| `y` | Copy current file path / yank selection (in visual mode) |
| `Esc` | Clear link selection / cancel visual mode |
| `b` | Open backlinks (modal) |
| `t` | Open directory tree (modal) |
| `^p` / `o` | Open file finder (type to fuzzy-filter; `^j`/`^k` cursor) |
| `/` | Full-text search across vault markdown (type to search; `^j`/`^k` cursor) |
| `^l` | Log viewer |
| `?` | Help (cheat sheet) |
| `q` | Quit |

## Scripting / query mode

The binary also works in non-interactive mode: pass a reserved verb as the first argument to emit JSON and exit instead of launching the TUI. JSON goes to stdout, errors to stderr. Exit 0 on success (including empty results), exit 1 on failure.

| Verb | Command | Output |
|------|---------|--------|
| `search` | `hypogeum search "term" [-n 50] [--vault dir]` | `[{path, line, snippet}]` — recency-ranked full-text hits across vault markdown |
| `links` | `hypogeum links <file> [--vault dir]` | `[{text, target, path, kind, broken}]` — outbound links (kind ∈ wikilink/relative/external) |
| `recent` | `hypogeum recent [-n 20] [--vault dir]` | `[{path, score, mtime, visited}]` — recency-ranked notes |
| `neighbors` | `hypogeum neighbors <file> [--vault dir]` | `{file, outbound: [...], backlinks: [...]}` — outbound links and 1-hop backlinks with line/snippet |

The `--vault` flag defaults to the current directory. Use `./` prefix to refer to a file literally named `search`, `links`, `recent`, or `neighbors` in the TUI.

## Inspiration

The design owes an obvious debt to [Frogmouth](https://github.com/Textualize/frogmouth), which does the same job in Python on top of Textual. hypogeum is a clean-room reimplementation in Go with no shared code, written to feel native in environments where a single static binary beats a Python install.

## License

MIT.
