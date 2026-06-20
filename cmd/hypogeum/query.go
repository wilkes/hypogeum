package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wilkes/hypogeum/internal/query"
)

// queryVerbs is the set of reserved first-arg verbs that route to
// non-interactive query mode instead of launching the TUI.
var queryVerbs = map[string]bool{
	"search":    true,
	"links":     true,
	"recent":    true,
	"neighbors": true,
}

func isQueryVerb(s string) bool { return queryVerbs[s] }

// runQuery parses args (verb + flags + positional) and writes the verb's
// result as JSON to stdout. The first arg must be a query verb.
func runQuery(args []string, stdout io.Writer) error {
	verb := args[0]
	fs := flag.NewFlagSet(verb, flag.ContinueOnError)
	fs.SetOutput(io.Discard) // we surface flag errors ourselves
	vault := fs.String("vault", "", "vault root (default: current directory)")
	// -n (max results) only applies to verbs that actually cap output.
	// links/neighbors take no max, so leaving -n unregistered makes
	// `links foo -n 5` a parse error (unknown flag) instead of a silent
	// no-op.
	var n *int
	switch verb {
	case "search":
		n = fs.Int("n", 50, "max results")
	case "recent":
		n = fs.Int("n", 20, "max results")
	}

	// Go's flag package stops parsing at the first non-flag token, so a
	// flag placed *after* the positional arg (e.g. `search term -n 5`)
	// would otherwise be silently dropped into fs.Args(). Reorder the
	// tokens so flags precede the single positional before parsing.
	if err := fs.Parse(reorderFlagsFirst(fs, args[1:])); err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}

	root := *vault
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		root = cwd
	}

	var result any
	switch verb {
	case "search":
		term := fs.Arg(0)
		if term == "" {
			return fmt.Errorf("search: missing query term")
		}
		hits, err := query.Search(root, term, *n)
		if err != nil {
			return err
		}
		result = hits
	case "recent":
		entries, err := query.Recent(root, *n)
		if err != nil {
			return err
		}
		result = entries
	case "links":
		file := fs.Arg(0)
		if file == "" {
			return fmt.Errorf("links: missing file argument")
		}
		links, err := query.Links(root, file)
		if err != nil {
			return err
		}
		result = links
	case "neighbors":
		file := fs.Arg(0)
		if file == "" {
			return fmt.Errorf("neighbors: missing file argument")
		}
		nb, err := query.Neighbors(root, file)
		if err != nil {
			return err
		}
		result = nb
	default:
		return fmt.Errorf("unknown query verb: %s", verb)
	}

	enc := json.NewEncoder(stdout)
	return enc.Encode(result)
}

// reorderFlagsFirst permutes args so every flag token precedes the lone
// positional, working around the stdlib flag package halting at the first
// non-flag token. Each query verb accepts at most one positional, so the
// strategy is simply: emit flag tokens (and the values they consume) first,
// then the positionals.
//
// A token is treated as flag-consuming-a-value when it is "-name" / "--name"
// (no "=") and name is a flag registered on fs that is NOT a bool flag — in
// that case the following token is its value and must travel with it. The
// "-name=value" form is self-contained. "--" terminates flag scanning: it and
// everything after it are positionals.
func reorderFlagsFirst(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			// Everything after "--" is positional, "--" included so
			// flag.Parse stops there too.
			positionals = append(positionals, args[i:]...)
			break
		}
		if len(tok) > 1 && tok[0] == '-' {
			flags = append(flags, tok)
			// A "-name value" (space-separated) flag consumes the next
			// token as its value, unless it's "-name=value" or a bool flag.
			name := strings.TrimLeft(tok, "-")
			if !strings.Contains(tok, "=") && i+1 < len(args) && takesValue(fs, name) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positionals = append(positionals, tok)
	}
	return append(flags, positionals...)
}

// takesValue reports whether the flag named name is registered on fs and
// expects a separate value token (i.e. it is not a boolean flag).
func takesValue(fs *flag.FlagSet, name string) bool {
	f := fs.Lookup(name)
	if f == nil {
		return false
	}
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return !(ok && bf.IsBoolFlag())
}
