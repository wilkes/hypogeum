package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap collects every keybinding the model knows about. Centralizing
// them makes the help cheat sheet trivial to render and dialects easy
// to define as alternative factory functions.
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

	OpenBacklinksModal key.Binding
	OpenLogsModal      key.Binding
	OpenHelpModal      key.Binding

	ToggleTree   key.Binding
	ToggleFolder key.Binding

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

func pagerKeys() keyMap {
	return keyMap{
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Open:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:    key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "back")),
		Forward: key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "forward")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

		NextLink:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next link")),
		PrevLink:  key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev link")),
		ClearLink: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear link")),

		OpenBacklinksModal: key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "backlinks")),
		OpenLogsModal:      key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("^l", "logs")),
		OpenHelpModal:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),

		ToggleTree:   key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("^b", "open tree")),
		ToggleFolder: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "expand/collapse")),

		OpenPicker:       key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("^p", "open file…")),
		PickerCursorDown: key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("^j", "picker: next")),
		PickerCursorUp:   key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("^k", "picker: prev")),

		OpenSearch:       key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("^s", "search…")),
		SearchCursorDown: key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("^j", "search: next")),
		SearchCursorUp:   key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("^k", "search: prev")),

		Top:          key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:       key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "half-page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "half-page up")),
	}
}

func modernKeys() keyMap {
	return keyMap{
		Up:      key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		Down:    key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
		Open:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:    key.NewBinding(key.WithKeys("alt+left", "backspace"), key.WithHelp("alt+←/⌫", "back")),
		Forward: key.NewBinding(key.WithKeys("alt+right"), key.WithHelp("alt+→", "forward")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+q", "ctrl+c"), key.WithHelp("q/^q", "quit")),

		NextLink:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next link")),
		PrevLink:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev link")),
		ClearLink: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear link")),

		OpenBacklinksModal: key.NewBinding(key.WithKeys("alt+b"), key.WithHelp("alt+b", "backlinks")),
		OpenLogsModal:      key.NewBinding(key.WithKeys("alt+l"), key.WithHelp("alt+l", "logs")),
		OpenHelpModal:      key.NewBinding(key.WithKeys("?", "f1"), key.WithHelp("?/F1", "help")),

		ToggleTree:   key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("^b", "open tree")),
		ToggleFolder: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "expand/collapse")),

		OpenPicker: key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("^p", "open file…")),
		// Picker/SearchCursor fields intentionally zero-valued so the
		// dispatcher falls through to Up/Down (arrow-only in modern).
		// See TestModernKeys_AllActionsBound.

		OpenSearch: key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("^f", "search…")),

		Top:          key.NewBinding(key.WithKeys("ctrl+home"), key.WithHelp("^home", "top")),
		Bottom:       key.NewBinding(key.WithKeys("ctrl+end"), key.WithHelp("^end", "bottom")),
		HalfPageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
	}
}

// keysFor returns the keyMap for the named dialect. Unknown values fall
// back to pager — this is the runtime mirror of config.Load's validation
// fallback, so the binary stays usable even if a config slipped through.
func keysFor(dialect string) keyMap {
	switch dialect {
	case "modern":
		return modernKeys()
	default:
		return pagerKeys()
	}
}
