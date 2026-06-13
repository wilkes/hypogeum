package tui

import (
	"github.com/charmbracelet/bubbles/key"
)

// keyMap collects every keybinding the model knows about, so the help
// cheat sheet and the dialect factories share one source.
type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Open    key.Binding
	Back    key.Binding
	Forward key.Binding
	Quit    key.Binding

	NextLink  key.Binding
	PrevLink  key.Binding
	ClearLink key.Binding

	CopyPath key.Binding

	OpenBacklinksModal key.Binding
	OpenLogsModal      key.Binding
	OpenHelpModal      key.Binding

	ToggleTree   key.Binding
	ToggleFolder key.Binding
	EnterVisual  key.Binding
	BeginSelect  key.Binding

	OpenPicker       key.Binding
	PickerCursorDown key.Binding
	PickerCursorUp   key.Binding

	OpenSearch       key.Binding
	SearchCursorDown key.Binding
	SearchCursorUp   key.Binding

	Top          key.Binding
	Bottom       key.Binding
	HalfPageDown key.Binding
	HalfPageUp   key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Open:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:    key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("←/h", "back")),
		Forward: key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("→/l", "forward")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

		NextLink:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next link")),
		PrevLink:  key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev link")),
		ClearLink: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear link")),

		CopyPath: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy path")),

		OpenBacklinksModal: key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "backlinks")),
		OpenLogsModal:      key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("^l", "logs")),
		OpenHelpModal:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),

		ToggleTree:   key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "open tree")),
		ToggleFolder: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "expand/collapse")),
		EnterVisual:  key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "select mode")),
		BeginSelect:  key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "start selection")),

		OpenPicker:       key.NewBinding(key.WithKeys("ctrl+p", "o"), key.WithHelp("^p/o", "open file…")),
		PickerCursorDown: key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("^j", "picker: next")),
		PickerCursorUp:   key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("^k", "picker: prev")),

		OpenSearch:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search…")),
		SearchCursorDown: key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("^j", "search: next")),
		SearchCursorUp:   key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("^k", "search: prev")),

		Top:          key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:       key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "half-page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "half-page up")),
	}
}

