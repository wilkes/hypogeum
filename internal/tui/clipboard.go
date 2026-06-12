package tui

import "github.com/muesli/termenv"

// clipboardWriter copies text to the system clipboard. Injected on the
// Model (mirroring openExternal) so tests can record calls instead of
// emitting a real OSC 52 escape sequence to the terminal.
type clipboardWriter func(text string)

// defaultClipboardWriter copies via termenv.Copy, which emits an OSC 52
// escape. OSC 52 copies through the terminal, so it works over SSH and
// inside tmux with no pbcopy/xclip dependency. It returns no error;
// the persistent selection highlight and footer toast are the
// user-visible confirmation that a copy was attempted.
func defaultClipboardWriter(text string) { termenv.Copy(text) }
