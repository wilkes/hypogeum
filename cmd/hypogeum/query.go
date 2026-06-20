package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

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
	n := fs.Int("n", defaultLimit(verb), "max results")
	if err := fs.Parse(args[1:]); err != nil {
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

// defaultLimit is the default -n cap per verb.
func defaultLimit(verb string) int {
	switch verb {
	case "search":
		return 50
	case "recent":
		return 20
	default:
		return 0 // links/neighbors ignore -n
	}
}
