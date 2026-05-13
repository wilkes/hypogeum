package code

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

// defaultStyle returns the Chroma style code rendering uses. Hardcoded
// to monokai (which matches Glamour's dark code-fence palette) for v1.
// User-configurable themes are deferred to v2; keep this the only call
// site for styles so the future hook has one place to land.
func defaultStyle() *chroma.Style {
	s := styles.Get("monokai")
	if s == nil {
		return styles.Fallback
	}
	return s
}
