// Package pathutil holds shared filesystem-path helpers.
package pathutil

import "path/filepath"

// ResolveRelativeTo resolves target against the directory of base and
// returns an absolute path. An already-absolute target is made absolute
// directly (base is ignored).
func ResolveRelativeTo(base, target string) (string, error) {
	if filepath.IsAbs(target) {
		return filepath.Abs(target)
	}
	return filepath.Abs(filepath.Join(filepath.Dir(base), target))
}
