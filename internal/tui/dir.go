package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wilkes/hypogeum/internal/tree"
)

// renderDirListing synthesizes a markdown document that lists the
// non-hidden entries of dir. The result is fed to the standard markdown
// renderer so all link-following plumbing applies: n/p cycles entries,
// Enter follows them, Back works.
//
// dir must be an absolute path; callers (refreshContent, applyLinkHighlight)
// already pass absolute paths from history.
//
// Entries are sorted directories-first, then alphabetical within each group,
// matching the tree pane's sort. Hidden entries (dotfiles) are skipped to
// match tree.IsHidden. Hrefs are absolute so ResolveLink doesn't depend on
// the listing's own base path.
func renderDirListing(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	type item struct {
		name  string
		path  string
		isDir bool
	}
	items := make([]item, 0, len(entries))
	for _, e := range entries {
		if tree.IsHidden(e.Name()) {
			continue
		}
		items = append(items, item{
			name:  e.Name(),
			path:  filepath.Join(dir, e.Name()),
			isDir: e.IsDir(),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.isDir != b.isDir {
			return a.isDir
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})

	var b strings.Builder
	header := filepath.Base(dir)
	if dir == string(filepath.Separator) {
		header = "/"
	}
	b.WriteString("# ")
	b.WriteString(header)
	b.WriteString("\n\n`")
	b.WriteString(dir)
	b.WriteString("`\n\n")

	parent := filepath.Dir(dir)
	if parent != dir {
		b.WriteString("- [..](")
		b.WriteString(parent)
		b.WriteString(")\n")
	}
	for _, it := range items {
		b.WriteString("- [")
		b.WriteString(it.name)
		if it.isDir {
			b.WriteByte('/')
		}
		b.WriteString("](")
		b.WriteString(it.path)
		b.WriteString(")\n")
	}
	return b.String(), nil
}
