package tui

import "github.com/charmbracelet/bubbles/filepicker"

// markdownPickerExts is the extension allow-list passed to filepicker.
// Mirrors the set in internal/tree but kept local so the tui package
// doesn't reach into tree internals.
var markdownPickerExts = []string{".md", ".markdown", ".mdown", ".mkd"}

// newPicker constructs a filepicker rooted at root and filtered to
// markdown extensions. The picker is reused across opens; resetting
// CurrentDirectory before each open ensures predictable startup.
func newPicker(root string) filepicker.Model {
	fp := filepicker.New()
	fp.CurrentDirectory = root
	fp.AllowedTypes = markdownPickerExts
	return fp
}

// resizePicker fits the picker into the modal interior. filepicker only
// exposes a Height knob; its rows render to whatever width the surrounding
// frame allots, which renderModal already sizes correctly.
func (m *Model) resizePicker() {
	_, _, _, h := modalGeometry(m.width, m.height)
	picker := h - 4 // border + breathing room around the file list
	if picker < 1 {
		picker = 1
	}
	m.picker.Height = picker
}
