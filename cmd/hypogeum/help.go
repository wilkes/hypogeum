package main

// helpText is the usage shown by `hypogeum --help` / `-h`. It documents the
// three CLI modes — launching the TUI on a path, the non-interactive query
// verbs, and the global flags — but deliberately omits the in-app keybindings,
// which the running TUI lists under `?`.
func helpText() string {
	return `hypogeum — a terminal browser for markdown vaults

Usage:
  hypogeum [path]            Browse a directory, or open a single .md file
                             (the tree roots at its parent). No path = cwd.
  hypogeum <verb> [args]     Run a non-interactive query, printing JSON.
  hypogeum --version | -v    Print build version, commit, and date.
  hypogeum --help | -h       Show this help.

Query verbs (JSON to stdout; errors to stderr):
  search <term> [-n N]       Full-text search, edit-recency ranked (default -n 50).
  recent [-n N]              Recently opened files, most recent first (default -n 20).
  links <file>              Outbound links from <file>.
  neighbors <file>          Backlinks plus outbound links for <file>.
  graph                     The whole-vault link graph as {nodes, edges}.

  All verbs accept -vault <dir> to set the vault root (default: cwd). A first
  arg matching a verb always routes to query mode — pass ./search to open a
  file literally named "search" in the TUI instead.

Examples:
  hypogeum ~/notes
  hypogeum search "elm architecture" -n 10
  hypogeum neighbors -vault ~/notes ~/notes/index.md
  hypogeum graph | jq '.edges | length'

Press ? inside the browser for the full list of keybindings.`
}
