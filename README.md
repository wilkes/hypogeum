# hypogeum

A terminal browser for markdown directories. Point it at a folder of `.md` files and wander through them — the rendered file fills the screen, `^p` opens a fuzzy file finder, `/` opens full-text search, `t` opens the directory tree in a modal, and links between files navigate with `Enter`. `h` and `l` go back and forward through your history, like a browser. `y` copies the current file's path, and a visual-mode caret (`v`) or a mouse drag selects text to the clipboard.

The name is the Greek word for an underground chamber or network of chambers (*hupó* "under" + *gê* "earth"). It shares a root with *hyperlink* (*hupér* "above") — the references float above the text, the chambers wait below.

## Status

Actively developed and usable day-to-day. The core:

- **Rendering.** GitHub Flavored Markdown via [Glamour](https://github.com/charmbracelet/glamour). Non-markdown files (code, config) render with syntax highlighting and a line-number gutter; pointing at a directory shows a navigable listing.
- **Navigation.** Walks directory trees, navigates file-to-file, with browser-style back/forward history (`h`/`l`). Wikilinks (`[[note]]`) and standard markdown links resolve across the whole vault; press `b` to see what links to the current file. The selected inline link is highlighted in reverse-video, and following a backlink or moving through history leaves the link you came from pre-selected, so `n`/`p` cycling resumes from a meaningful position. Broken links are tallied in the footer.
- **Finding things.** A recency-ranked fuzzy finder (`^p` / `o`), full-text search across the vault (`/`), a recently-opened list (`r`), and the directory tree (`t`) all open as modals.
- **Embeds.** Source embeds (`![[file.go#L10-L20]]`) inline a slice of another file as a fenced code block; range links scroll to and highlight the target lines.
- **Selection.** Copy the current file's path with `y`, or select text — keyboard visual mode (`v`) or mouse drag — to the system clipboard (and OSC 52 for SSH/tmux).
- **Live reload.** An fsnotify watcher re-renders the open file and re-walks the tree as files change on disk.
- **Scripting.** Non-interactive JSON query mode (`search`, `links`, `recent`, `neighbors`, `graph`) for piping the link graph into other tools — see below.

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
hypogeum --help           # usage, query verbs, and global flags
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
| `r` | Open recently-opened files (modal) |
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
| `recent` | `hypogeum recent [-n 20] [--vault dir]` | `[{path, visited}]` — notes you've opened, most-recently-visited first |
| `neighbors` | `hypogeum neighbors <file> [--vault dir]` | `{file, outbound: [...], backlinks: [...]}` — outbound links and 1-hop backlinks with line/snippet |
| `graph` | `hypogeum graph [--vault dir]` | `{nodes, edges}` — whole-vault link graph; nodes are every markdown doc (orphans included), edges are every link with `{from, to, kind, broken}`. Example: `hypogeum graph --vault docs \| jq '.edges \| length'` |

The `--vault` flag defaults to the current directory. Use `./` prefix to refer to a file literally named `search`, `links`, `recent`, `neighbors`, or `graph` in the TUI.

- **Agent skill:** [`.claude/skills/hypogeum-vault/`](.claude/skills/hypogeum-vault/SKILL.md) teaches Claude Code (or any skill-aware agent) to explore and audit a markdown vault with the query CLI. Symlink it into `~/.claude/skills/` to use it in any repo.

## MCP server

`hypogeum mcp [vault]` serves the same vault to Claude and other agents over the [Model Context Protocol](https://modelcontextprotocol.io) on stdio, exposing five tools: `search_vault`, `outbound_links`, `neighbors`, `vault_graph`, and `read_note`. Their JSON output is identical to the matching query verbs; the server adds a warm, watcher-refreshed vault index so repeated `neighbors`/`vault_graph` calls don't rebuild the graph from cold. Read-only — no write tools. `vault` defaults to the current directory.

Register it with Claude (e.g. in `~/.claude.json` or a project `.mcp.json`):

```json
{
  "mcpServers": {
    "hypogeum": { "command": "hypogeum", "args": ["mcp", "/path/to/vault"] }
  }
}
```

## License

MIT.
