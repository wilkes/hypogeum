package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap collects every keybinding the model knows about. Centralizing them
// makes the help footer trivial to render and the bindings easy to change.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Open     key.Binding
	Back     key.Binding
	Forward  key.Binding
	FocusTog key.Binding
	Quit     key.Binding

	NextLink  key.Binding
	PrevLink  key.Binding
	ClearLink key.Binding

	ToggleBacklinks    key.Binding
	OpenBacklinksModal key.Binding
	OpenLogsModal      key.Binding
	OpenHelpModal      key.Binding

	ToggleTree   key.Binding
	ToggleFolder key.Binding

	OpenPicker key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Open:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:     key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "back")),
		Forward:  key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "forward")),
		FocusTog: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

		NextLink:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next link")),
		PrevLink:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev link")),
		ClearLink: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear link")),

		ToggleBacklinks:    key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "backlinks pane")),
		OpenBacklinksModal: key.NewBinding(key.WithKeys("B"), key.WithHelp("B", "backlinks modal")),
		OpenLogsModal:      key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("^l", "logs")),
		OpenHelpModal:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),

		ToggleTree:   key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("^b", "toggle tree")),
		ToggleFolder: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "expand/collapse")),

		OpenPicker: key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("^p", "open file…")),
	}
}
