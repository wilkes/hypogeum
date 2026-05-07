package tui

import (
	"github.com/charmbracelet/bubbles/filepicker"

	"github.com/wilkes/hypogeum/internal/tree"
)

func newPicker(root string) filepicker.Model {
	fp := filepicker.New()
	fp.CurrentDirectory = root
	fp.AllowedTypes = tree.MarkdownExts
	return fp
}

func (m *Model) resizePicker() {
	_, _, _, h := modalGeometry(m.width, m.height)
	picker := h - 2
	if picker < 1 {
		picker = 1
	}
	m.picker.Height = picker
}
