package tree

import (
	"path/filepath"
	"strings"
)

// Merge overlays multiple root directories into a single virtual tree, as if
// the roots were superimposed onto one another. Directories that share a
// relative path are merged into one node; files that share a relative path
// are all kept ("union, keep both"), with their display Name disambiguated by
// source root so the user can tell them apart in the tree.
//
// With zero roots Merge returns an empty synthesized root. With exactly one
// root it is identical to Walk, so single-directory behavior is unchanged
// (including the absolute-path root node).
//
// In a merged (>=2-root) tree the synthesized virtual root has an empty Path
// and a Name of the roots' base names joined with " + ". Merged directory
// nodes are keyed by their slash-joined relative path (e.g. "sub/deep")
// rather than an absolute path, because a merged directory can back onto
// several real directories. File nodes keep their real absolute Path so they
// can be opened, watched, and resolved.
func Merge(roots []string) (*Node, error) {
	switch len(roots) {
	case 0:
		return &Node{IsDir: true}, nil
	case 1:
		return Walk(roots[0])
	}

	abs := make([]string, 0, len(roots))
	subtrees := make([]*Node, 0, len(roots))
	for _, r := range roots {
		a, err := filepath.Abs(r)
		if err != nil {
			return nil, err
		}
		n, err := walk(a)
		if err != nil {
			return nil, err
		}
		abs = append(abs, a)
		if n != nil {
			subtrees = append(subtrees, n)
		}
	}

	root := mergeDirs("", subtrees, abs)
	names := make([]string, len(abs))
	for i, a := range abs {
		names[i] = filepath.Base(a)
	}
	root.Name = strings.Join(names, " + ")
	return root, nil
}

// mergeDirs merges the children of every directory in dirs into a single node
// keyed by relPath (slash-joined, "" for the virtual root). roots is the list
// of absolute source roots, used only to label colliding files.
func mergeDirs(relPath string, dirs []*Node, roots []string) *Node {
	merged := &Node{Path: relPath, Name: filepath.Base(relPath), IsDir: true}
	if relPath == "" {
		merged.Name = ""
	}

	// Group children by name across all contributing directories. Insertion
	// order doesn't matter here — sortChildren re-sorts the merged result.
	dirGroups := map[string][]*Node{}
	fileGroups := map[string][]*Node{}
	for _, d := range dirs {
		for _, c := range d.Children {
			if c.IsDir {
				dirGroups[c.Name] = append(dirGroups[c.Name], c)
			} else {
				fileGroups[c.Name] = append(fileGroups[c.Name], c)
			}
		}
	}

	for name, group := range dirGroups {
		childRel := name
		if relPath != "" {
			childRel = relPath + "/" + name
		}
		sub := mergeDirs(childRel, group, roots)
		// Walk already prunes markdown-empty directories, so this guard
		// mirrors Walk's contract for the merged subtree.
		if len(sub.Children) > 0 {
			merged.Children = append(merged.Children, sub)
		}
	}

	for name, group := range fileGroups {
		group = dedupeByPath(group)
		if len(group) == 1 {
			merged.Children = append(merged.Children, group[0])
			continue
		}
		// Collision at the same relative path: keep every file, disambiguating
		// the display Name by source root. Nodes are cloned so the disambiguated
		// Name doesn't leak back into the per-root subtrees.
		for _, f := range group {
			clone := *f
			clone.Name = name + " (" + sourceLabel(f.Path, roots) + ")"
			merged.Children = append(merged.Children, &clone)
		}
	}

	sortChildren(merged)
	return merged
}

// dedupeByPath drops duplicate file nodes that share an absolute Path, which
// happens when roots overlap or the same directory is passed twice.
func dedupeByPath(nodes []*Node) []*Node {
	if len(nodes) < 2 {
		return nodes
	}
	seen := make(map[string]struct{}, len(nodes))
	out := make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		if _, ok := seen[n.Path]; ok {
			continue
		}
		seen[n.Path] = struct{}{}
		out = append(out, n)
	}
	return out
}

// RelWithin returns p expressed relative to root, and true, when p lies inside
// root; otherwise it returns "", false. It centralizes the filepath.Rel
// ".."-prefix check that distinguishes an inside path from an outside one.
func RelWithin(root, p string) (string, bool) {
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// sourceLabel returns a short label identifying which root p came from,
// preferring the longest matching root so nested roots still disambiguate.
// Falls back to the parent directory's base name when no root matches.
func sourceLabel(p string, roots []string) string {
	best := ""
	for _, r := range roots {
		if _, ok := RelWithin(r, p); !ok {
			continue
		}
		if best == "" || len(r) > len(best) {
			best = r
		}
	}
	if best != "" {
		return filepath.Base(best)
	}
	return filepath.Base(filepath.Dir(p))
}
