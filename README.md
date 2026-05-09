# hypogeum

A terminal browser for markdown directories. Point it at a folder of `.md` files and wander through them — the left pane is the directory tree, the right pane renders the file you're on, and links between files navigate with `Enter`. `h` and `l` go back and forward through your history, like a browser.

The name is the Greek word for an underground chamber or network of chambers (*hupó* "under" + *gê* "earth"). It shares a root with *hyperlink* (*hupér* "above") — the references float above the text, the chambers wait below.

## Status

Renders GitHub Flavored Markdown via [Glamour](https://github.com/charmbracelet/glamour), walks directory trees, navigates file-to-file. Wikilinks (`[[note]]`) and standard markdown links resolve across the whole vault; press `b` to see what links to the current file (and `B` for the same view as a centered modal). The selected inline link is highlighted in reverse-video on the page; following a backlink, going back, or going forward leaves the link you came from pre-selected, so `n`/`p` cycling resumes from a meaningful position.

## Install

```sh
go install github.com/wilkes/hypogeum/cmd/hypogeum@latest
```

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
| `Enter` | Open the selected file |
| `h` / `←` | Back |
| `l` / `→` | Forward |
| `Tab` | Switch between tree and content |
| `n` / `p` | Cycle to next / previous link |
| `Enter` | Follow selected link |
| `Esc` | Clear link selection |
| `b` | Toggle backlinks pane |
| `B` | Backlinks modal |
| `^b` | Toggle tree pane |
| `^p` | Open file picker (modal) |
| `^l` | Log viewer |
| `?` | Help (cheat sheet) |
| `q` | Quit |

## Inspiration

The design owes an obvious debt to [Frogmouth](https://github.com/Textualize/frogmouth), which does the same job in Python on top of Textual. hypogeum is a clean-room reimplementation in Go with no shared code, written to feel native in environments where a single static binary beats a Python install.

## License

MIT.
