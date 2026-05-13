package embed

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"strings"
)

// Sentinel errors for the four embed warnings the renderer surfaces.
// ErrRangePastEOF is *non-fatal*: SliceFile clamps and still returns
// a usable slice so the renderer can show what it has alongside the
// "file ends at line N" header note.
var (
	ErrNotFound     = errors.New("source file not found")
	ErrBinary       = errors.New("source file is binary")
	ErrTooLarge     = errors.New("source file too large to embed")
	ErrInvalidRange = errors.New("invalid range")
	ErrRangePastEOF = errors.New("range past EOF (clamped)")
)

// maxSize mirrors internal/code.maxSize so we don't embed anything we
// wouldn't render as a standalone code file.
const maxSize = 5 * 1024 * 1024

// SliceFile reads absPath, validates it, and returns the requested
// line slice expanded by ctx lines on each side. r == nil returns the
// whole file. Lines have no trailing newline. startLine is 1-indexed
// and reflects any context-expansion (so for a #L10-L20+3 embed on a
// long-enough file, startLine == 7).
func SliceFile(absPath string, r *LineRange, ctx int) ([]string, int, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	if len(data) > maxSize {
		return nil, 0, ErrTooLarge
	}
	if looksBinary(data) {
		return nil, 0, ErrBinary
	}

	// Split keeping line semantics intact. strings.Split on "\n" produces
	// a trailing empty element when the file ends with a newline; trim it
	// so line counts match what users see in their editor.
	all := strings.Split(string(data), "\n")
	if n := len(all); n > 0 && all[n-1] == "" {
		all = all[:n-1]
	}
	total := len(all)

	if r == nil {
		return all, 1, nil
	}
	if r.Start < 1 || r.Start > total {
		return nil, 0, ErrInvalidRange
	}
	// Only the *user-specified* end past EOF is a warning; context-line
	// padding that runs off either edge of the file is silently clamped.
	pastEOF := r.End > total
	start := r.Start - ctx
	if start < 1 {
		start = 1
	}
	end := r.End + ctx
	if end > total {
		end = total
	}
	lines := all[start-1 : end]
	if pastEOF {
		return lines, start, ErrRangePastEOF
	}
	return lines, start, nil
}

// looksBinary uses the same NUL-in-first-8KB rule as internal/code.
func looksBinary(src []byte) bool {
	n := len(src)
	if n > 8192 {
		n = 8192
	}
	return bytes.IndexByte(src[:n], 0) >= 0
}
