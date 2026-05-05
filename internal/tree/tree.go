// Package tree walks a directory and produces a tree of markdown files
// suitable for display in the left pane of the TUI. It is filesystem-aware
// but UI-unaware; the TUI converts these nodes into Bubble widgets.
package tree

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Node represents either a directory or a markdown file in the tree.
// Directories carry their children sorted alphabetically with directories
// listed before files.
type Node struct {
	Path     string  // absolute path
	Name     string  // basename for display
	IsDir    bool
	Children []*Node // populated for directories only
}

// Markdown extensions recognized by the walker. The TUI ignores everything
// else. Adding more is fine; just be sure Glamour can render them.
var markdownExts = map[string]struct{}{
	".md":       {},
	".markdown": {},
	".mdown":    {},
	".mkd":      {},
}

// Walk builds a Node tree rooted at root, including only directories that
// (transitively) contain at least one markdown file.
func Walk(root string) (*Node, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	n, err := walk(abs)
	if err != nil {
		return nil, err
	}
	if n == nil {
		// Empty result: synthesize an empty root so callers don't have to
		// special-case nil.
		return &Node{Path: abs, Name: filepath.Base(abs), IsDir: true}, nil
	}
	return n, nil
}

func walk(dir string) (*Node, error) {
	entries, err := readDir(dir)
	if err != nil {
		return nil, err
	}

	node := &Node{
		Path:  dir,
		Name:  filepath.Base(dir),
		IsDir: true,
	}

	for _, entry := range entries {
		full := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if isHidden(entry.Name()) {
				continue
			}
			child, err := walk(full)
			if err != nil {
				return nil, err
			}
			if child != nil && len(child.Children) > 0 {
				node.Children = append(node.Children, child)
			}
			continue
		}
		if isHidden(entry.Name()) {
			continue
		}
		if !isMarkdown(entry.Name()) {
			continue
		}
		node.Children = append(node.Children, &Node{
			Path:  full,
			Name:  entry.Name(),
			IsDir: false,
		})
	}

	sortChildren(node)
	if len(node.Children) == 0 {
		return nil, nil
	}
	return node, nil
}

// readDir is split out for testability and to centralize error handling.
func readDir(dir string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(rootedFS{}, dir)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// rootedFS satisfies fs.ReadDirFS using the OS filesystem. We use this rather
// than os.ReadDir directly so the package is easier to drive with a fake FS
// in tests later if we want to.
type rootedFS struct{}

func (rootedFS) Open(name string) (fs.File, error) { return nil, fs.ErrInvalid }
func (rootedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	// Fall back to os.ReadDir for absolute paths.
	return osReadDir(name)
}

func isMarkdown(name string) bool {
	_, ok := markdownExts[strings.ToLower(filepath.Ext(name))]
	return ok
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

func sortChildren(n *Node) {
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.IsDir != b.IsDir {
			return a.IsDir // directories first
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}
