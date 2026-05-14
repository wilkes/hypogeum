package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/search"
)

// searchMaxHits caps how many full-text hits the modal will hold.
// Matches pickerMaxVisible's choice for the same reason: a runaway
// short-query result list shouldn't lag rendering.
const searchMaxHits = 200

// searchDebounce is how long after the latest keystroke the scan fires.
// 150ms is the established UX threshold for "the system feels responsive
// without firing on every character."
const searchDebounce = 150 * time.Millisecond

// searchMinQuery is the minimum query length below which no scan fires.
// 1 char is too noisy on a multi-hundred-file vault; 3 is frustrating
// on short words like "go". 2 is the established sweet spot.
const searchMinQuery = 2

// searchSnippetWindow is the displayed budget per snippet, after which
// the row gets truncated by the renderer.
const searchSnippetWindow = 60

// searchState bundles full-text search modal state.
//
// paths is a snapshot taken at modal-open time so a mid-search watcher
// event doesn't yank files out from under in-flight workers. New files
// appear only on the next ^s open — this is deliberate; rescanning on
// every fsnotify event would be expensive AND yank cursor focus.
//
// scanStop is the CancelFunc for the currently-running (or most-recently
// scheduled) scan. Each keystroke that fires a new tick calls scanStop
// first so workers from the prior scan return early.
type searchState struct {
	input    textinput.Model
	paths    []string
	hits     []search.Hit
	cursor   int
	vp       viewport.Model
	scanCtx  context.Context
	scanStop context.CancelFunc
	// inFlight is true between the moment a scan is dispatched and the
	// moment its searchResultsMsg lands (or is discarded as stale).
	// Drives the "(searching…)" placeholder.
	inFlight bool
}

// newSearch returns a zero-valued search state with a fresh textinput.
func newSearch() searchState {
	ti := textinput.New()
	ti.Prompt = ""      // we render our own "> " prefix
	ti.Placeholder = ""
	ti.CharLimit = 256
	return searchState{
		vp:    viewport.New(0, 0),
		input: ti,
	}
}

// reset clears every transient field and re-focuses the textinput.
// Called on every modal-open. paths is the snapshot of vault files
// captured at open time.
func (s *searchState) reset(paths []string) {
	if s.scanStop != nil {
		s.scanStop()
	}
	s.paths = paths
	s.hits = nil
	s.cursor = 0
	s.scanCtx = nil
	s.scanStop = nil
	s.inFlight = false
	s.input.SetValue("")
	s.input.Focus()
}

// searchTickMsg is delivered searchDebounce after each keystroke.
// query is the input value at the moment the tick was scheduled; the
// handler compares it to the current input to decide whether to honor
// or drop the tick (later keystrokes mean this one is stale).
type searchTickMsg struct {
	query string
}

// searchResultsMsg carries the output of a scan back into Update.
// query lets the handler discard results from a stale scan.
type searchResultsMsg struct {
	query string
	hits  []search.Hit
	err   error
}

// scheduleSearchTick returns a Cmd that fires a searchTickMsg with
// query after searchDebounce.
func scheduleSearchTick(query string) tea.Cmd {
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg {
		return searchTickMsg{query: query}
	})
}

// runSearchCmd returns a Cmd that runs the scan in a goroutine and
// emits searchResultsMsg with the result. paths is captured by value;
// the scan reads it without further synchronization.
func runSearchCmd(ctx context.Context, paths []string, query string) tea.Cmd {
	return func() tea.Msg {
		hits, err := search.Search(ctx, paths, query, searchMaxHits)
		return searchResultsMsg{query: query, hits: hits, err: err}
	}
}
