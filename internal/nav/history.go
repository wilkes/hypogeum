// Package nav provides a back/forward history stack for the file currently
// being viewed. It is independent of the TUI and the filesystem, dealing
// only in opaque path strings.
package nav

// History is a back/forward stack modeled after a browser's history.
// The current entry is always at index cursor; entries before it are "back"
// and entries after it are "forward". Visiting a new entry truncates anything
// after the cursor, matching browser semantics.
type History struct {
	entries []string
	cursor  int // index into entries; -1 when empty
}

// New returns an empty History.
func New() *History {
	return &History{cursor: -1}
}

// Visit records a new entry at the current position. Any forward history is
// discarded. Visiting the same path as the current entry is a no-op so that
// re-rendering the same file doesn't pollute history.
func (h *History) Visit(path string) {
	if h.cursor >= 0 && h.entries[h.cursor] == path {
		return
	}
	// Truncate forward history.
	h.entries = append(h.entries[:h.cursor+1], path)
	h.cursor = len(h.entries) - 1
}

// Current returns the path at the cursor, or "" if history is empty.
func (h *History) Current() string {
	if h.cursor < 0 {
		return ""
	}
	return h.entries[h.cursor]
}

// CanBack reports whether there is a previous entry.
func (h *History) CanBack() bool { return h.cursor > 0 }

// CanForward reports whether there is a next entry.
func (h *History) CanForward() bool {
	return h.cursor >= 0 && h.cursor < len(h.entries)-1
}

// Back moves the cursor one step back and returns the new current entry.
// It returns "" and false if there is no previous entry.
func (h *History) Back() (string, bool) {
	if !h.CanBack() {
		return "", false
	}
	h.cursor--
	return h.entries[h.cursor], true
}

// Forward moves the cursor one step forward and returns the new current entry.
// It returns "" and false if there is no next entry.
func (h *History) Forward() (string, bool) {
	if !h.CanForward() {
		return "", false
	}
	h.cursor++
	return h.entries[h.cursor], true
}
