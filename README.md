# hypogeum

A terminal browser for markdown directories. Point it at a folder of `.md` files and wander through them — the rendered file fills the screen, `^p` opens a fuzzy file finder, `^b` opens the directory tree in a modal, and links between files navigate with `Enter`. `h` and `l` go back and forward through your history, like a browser.

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

Pager dialect (default) — see [Configuration](#configuration) to switch to modern.

| Key | Action |
|-----|--------|
| `↑` / `k`, `↓` / `j` | Move within the focused pane |
| `Enter` | Open the selected file |
| `h` / `←` | Back (collapse folder when tree modal is open) |
| `l` / `→` | Forward (expand folder when tree modal is open) |
| `n` / `N` | Cycle to next / previous link |
| `Enter` | Follow selected link |
| `Esc` | Clear link selection |
| `b` | Open backlinks (modal) |
| `^b` | Open directory tree (modal) |
| `^p` | Open file finder (type to fuzzy-filter; `^j`/`^k` cursor) |
| `/` | Full-text search across vault markdown (type to search; `^j`/`^k` cursor) |
| `^l` | Log viewer |
| `?` | Help (cheat sheet) |
| `q` | Quit |

## Configuration

Hypogeum reads an optional config file from your platform's user-config
directory. Missing or malformed configs are not fatal — hypogeum always
starts with sensible defaults.

| OS      | Path                                                         |
| ------- | ------------------------------------------------------------ |
| Linux   | `$XDG_CONFIG_HOME/hypogeum/config.toml` (or `~/.config/...`) |
| macOS   | `~/Library/Application Support/hypogeum/config.toml`         |
| Windows | `%AppData%\hypogeum\config.toml`                             |

### Available settings

````toml
# dialect selects the keybinding preset.
#   "pager"  (default): vim/less idioms — h/l history, n/N link cycle,
#                       j/k motion, / for search, g/G top/bottom.
#   "modern":           browser/editor idioms — Alt+←/→ history,
#                       Tab/Shift+Tab link cycle, arrows for motion,
#                       Ctrl+F for search, Alt+b/Alt+l for modals.
dialect = "pager"
````

Press `?` in hypogeum to see the active dialect's full keybinding list.
Errors loading the config file appear in the `^l` log modal (or `Alt+l`
in modern dialect).

## Inspiration

The design owes an obvious debt to [Frogmouth](https://github.com/Textualize/frogmouth), which does the same job in Python on top of Textual. hypogeum is a clean-room reimplementation in Go with no shared code, written to feel native in environments where a single static binary beats a Python install.

## License

MIT.
