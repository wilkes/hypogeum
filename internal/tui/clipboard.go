package tui

import (
	"github.com/atotto/clipboard"
	"github.com/muesli/termenv"
)

// clipboardWriter copies text to the clipboard. Injected on the Model
// (mirroring openExternal) so tests record calls instead of touching a
// real clipboard or emitting an escape sequence.
type clipboardWriter func(text string)

// defaultClipboardWriter copies via two transports so a selection lands
// regardless of how hypogeum is being viewed:
//
//   - atotto/clipboard writes the OS clipboard (pbcopy on macOS, xclip/
//     xsel/wl-copy on Linux). This is the reliable path for a local
//     terminal — notably macOS Terminal.app, which has no OSC 52 support
//     at all and silently drops the escape below.
//   - termenv.Copy emits an OSC 52 escape, which carries the copy through
//     the terminal over SSH/tmux, where the OS-clipboard call would
//     target the wrong (remote) machine.
//
// Both are best-effort. atotto returns an error when no clipboard
// utility is reachable (e.g. a headless Linux box over SSH); we ignore
// it and rely on OSC 52 in that case. termenv.Copy returns nothing. The
// persistent selection highlight and the "Copied N chars" footer toast
// remain the user-visible confirmation that a copy was attempted.
func defaultClipboardWriter(text string) {
	_ = clipboard.WriteAll(text)
	termenv.Copy(text)
}
