package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wilkes/hypogeum/internal/recent"
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
		return formatSearchHits(m.modals.search.hits, m.root, m.modals.search.vp.Width, m.modals.search.cursor)
	}
}

// handleSearchTick fires when a debounce tick lands. If the modal has
// closed, the tick is dropped. If the user has typed more characters
// since this tick was scheduled, the tick's query won't match the
// current input value and we drop it (the latest keystroke scheduled
// its own tick).
func (m *Model) handleSearchTick(msg searchTickMsg) (tea.Model, tea.Cmd) {
	if m.modals.kind != modalSearch {
		return *m, nil
	}
	if msg.query != m.modals.search.input.Value() {
		return *m, nil
	}
	if len(msg.query) < searchMinQuery {
		return *m, nil
	}
	// Allocate a new ctx for this scan. Previous ctxs (if any) have
	// been cancelled by handleSearchKey.
	ctx, cancel := context.WithCancel(context.Background())
	m.modals.search.scanCtx = ctx
	m.modals.search.scanStop = cancel
	m.modals.search.inFlight = true
	m.refreshSearchVP()
	return *m, runSearchCmd(ctx, m.modals.search.paths, msg.query)
}

// handleSearchResults consumes the scan's output. Stale results — from
// a cancelled scan whose query no longer matches the input — are
// discarded. Otherwise hits are recency-ranked and stored.
func (m *Model) handleSearchResults(msg searchResultsMsg) (tea.Model, tea.Cmd) {
	if m.modals.kind != modalSearch {
		return *m, nil
	}
	if msg.query != m.modals.search.input.Value() {
		// Stale: user has typed more since this scan started.
		return *m, nil
	}
	m.modals.search.inFlight = false
	m.modals.search.scanCtx = nil
	m.modals.search.scanStop = nil
	if msg.err != nil && msg.err != context.Canceled {
		if m.diag != nil {
			m.diag.Info(fmt.Sprintf("search %q: %v", msg.query, msg.err))
		}
	}
	m.modals.search.hits = rerankByRecency(m.recent, msg.hits)
	m.modals.search.cursor = 0
	m.refreshSearchVP()
	if m.diag != nil {
		m.diag.Info(fmt.Sprintf("search %q: %d hits", msg.query, len(msg.hits)))
	}
	return *m, nil
}

// rerankByRecency reorders hits so files visited more recently come
// first. Hits from the same file keep their (line) order. Hits whose
// path doesn't appear in any recent.Ranked entry sort last in file-
// alphabetical order.
//
// store may be nil — happens in tests; we degrade to file-then-line
// order (input order).
func rerankByRecency(store recentStore, hits []search.Hit) []search.Hit {
	if store == nil || len(hits) == 0 {
		return hits
	}
	// Unique paths in stable input order.
	seen := map[string]int{}
	var uniquePaths []string
	for _, h := range hits {
		if _, ok := seen[h.Path]; !ok {
			seen[h.Path] = len(uniquePaths)
			uniquePaths = append(uniquePaths, h.Path)
		}
	}
	ranked := store.Rank(uniquePaths)
	// Group hits by path, then emit groups in priority order.
	byPath := map[string][]search.Hit{}
	for _, h := range hits {
		byPath[h.Path] = append(byPath[h.Path], h)
	}
	out := make([]search.Hit, 0, len(hits))
	for _, r := range ranked {
		out = append(out, byPath[r.Path]...)
	}
	return out
}

// recentStore is the subset of *recent.Store that rerankByRecency uses.
// Defined as an interface so tests can swap in a nil-tolerant fake.
type recentStore interface {
	Rank(paths []string) []recent.Ranked
}

// formatSearchHits renders each hit as a two-row entry:
//
//	▌ <relative-path>:<line>
//	  <snippet with \x11/\x12 → bold yellow>
//
// The cursor marker appears only on the selected hit. width is the
// viewport's visible width; snippets are truncated to width-4.
//
// applyHighlight (internal/tui/backlinks.go) handles the \x11/\x12 →
// SGR conversion; this function delegates to it for the snippet row.
func formatSearchHits(hits []search.Hit, root string, width, cursor int) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	for i, h := range hits {
		marker := "  "
		if i == cursor {
			marker = cursorMarkerStyle.Render("▌") + " "
		}
		header := marker + relativeTo(root, h.Path) + ":" + strconv.Itoa(h.Line)
		snippet := "  " + truncateOneLine(applyHighlight(h.Snippet), width-4)
		if i == cursor {
			header = lipgloss.NewStyle().Reverse(true).Render(header)
			snippet = lipgloss.NewStyle().Reverse(true).Render(snippet)
		}
		b.WriteString(header)
		b.WriteByte('\n')
		b.WriteString(snippet)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// resizeSearch fits the search modal's viewport into the modal interior,
// reserving rows for the query prompt and separator on top.
func (m *Model) resizeSearch() {
	_, _, w, h := modalGeometry(m.width, m.height)
	pw := w - 2
	ph := h - 2 - 2 // border (2) + prompt+separator (2)
	if pw < 1 {
		pw = 1
	}
	if ph < 1 {
		ph = 1
	}
	m.modals.search.vp.Width = pw
	m.modals.search.vp.Height = ph
	m.modals.search.input.Width = pw - 2 // leave room for "> " prefix
	m.refreshSearchVP()
}

// searchView returns the modal's renderable body — prompt, separator,
// viewport, and an optional overflow footer.
func (m *Model) searchView() string {
	p := &m.modals.search
	prompt := "> " + p.input.View()
	sepW := p.vp.Width
	if sepW < 1 {
		sepW = 1
	}
	sep := strings.Repeat("─", sepW)
	body := prompt + "\n" + sep + "\n" + p.vp.View()
	if len(p.hits) >= searchMaxHits {
		body += "\n" + lipgloss.NewStyle().Faint(true).
			Render("… results truncated at "+strconv.Itoa(searchMaxHits)+", refine the query")
	}
	return body
}
