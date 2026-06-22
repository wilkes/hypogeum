package main

import (
	"context"
	"fmt"

	"github.com/wilkes/hypogeum/internal/mcp"
)

// runMCP serves the vault over the Model Context Protocol on stdin/stdout. The
// optional single positional argument is the vault root (default: cwd). Unlike
// the query verbs this is a long-lived server, so it lives outside runQuery's
// flag/JSON machinery and dispatches straight to the internal/mcp package.
func runMCP(args []string, version string) error {
	root := "."
	switch len(args) {
	case 0:
		// root stays "." — internal/mcp.New resolves it to an absolute cwd.
	case 1:
		root = args[0]
	default:
		return fmt.Errorf("usage: hypogeum mcp [vault]")
	}

	srv, err := mcp.New(root, version)
	if err != nil {
		return err
	}
	defer srv.Close()
	return srv.Run(context.Background())
}
