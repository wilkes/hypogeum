package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

// handleSearchKey forwards printable runes to the textinput, then
// decides whether to schedule a debounced scan tick. Called only when
// modalSearch is open and msg.Type is tea.KeyRunes.
func (m *Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.modals.search.input, cmd = m.modals.search.input.Update(msg)
	query := m.modals.search.input.Value()
	if len(query) < searchMinQuery {
		// Below the minimum — clear any prior results and don't fire a
		// scan. A previous tick may still be in flight from a longer
		// query; its result will be discarded by the stale-query check.
		// Discard the textinput's cursor-blink cmd too: no async work
		// should be scheduled when the query is too short.
		m.modals.search.hits = nil
		m.modals.search.cursor = 0
		m.modals.search.inFlight = false
		m.refreshSearchVP()
		return *m, nil
	}
	// Cancel any prior in-flight scan immediately. The tick may not
	// have fired yet, but if a scan is mid-flight cancelling now lets
	// workers return early.
	if m.modals.search.scanStop != nil {
		m.modals.search.scanStop()
		m.modals.search.scanStop = nil
		m.modals.search.scanCtx = nil
	}
	tick := scheduleSearchTick(query)
	if cmd != nil {
		return *m, tea.Batch(cmd, tick)
	}
	return *m, tick
}

// refreshSearchVP regenerates the search modal's viewport content
// from the current hits / cursor / input value. Called whenever any
// of those change.
func (m *Model) refreshSearchVP() {
	m.modals.search.vp.SetContent(m.renderSearchRows())
	viewportClamp(&m.modals.search.vp, m.modals.search.cursor, 2)
}

// renderSearchRows produces the viewport body: the empty-state /
// loading / no-match placeholders when no hits should display, or the
// hit list otherwise. Each branch returns faint-styled text via lipgloss
// so the visual hierarchy stays consistent with the picker.
func (m *Model) renderSearchRows() string {
	q := m.modals.search.input.Value()
	faint := lipgloss.NewStyle().Faint(true)
	switch {
	case len(m.modals.search.paths) == 0:
		return faint.Render("(no markdown files in vault)")
	case len(q) < searchMinQuery:
		return faint.Render("(type 2+ chars to search)")
	case m.modals.search.inFlight && len(m.modals.search.hits) == 0:
		return faint.Render("(searching…)")
	case !m.modals.search.inFlight && len(m.modals.search.hits) == 0:
		return faint.Render(`(no match for "` + q + `")`)
	default:
		return strings.Join(formatHitsPlaceholder(m.modals.search.hits), "\n")
	}
}

// formatHitsPlaceholder is the minimal one-row-per-hit renderer.
// A later commit swaps it for the two-row-per-entry path/snippet layout
// once the hit-formatting helpers are in place.
func formatHitsPlaceholder(hits []search.Hit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Path)
	}
	return out
}
