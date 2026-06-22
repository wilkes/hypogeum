package tui

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wilkes/hypogeum/internal/markdown"
	"github.com/wilkes/hypogeum/internal/recent"
	"github.com/wilkes/hypogeum/internal/search"
)

// searchMaxFiles caps how many matching files the grouped modal will hold.
// The cap is on files, not hits — far more coverage than the old per-hit
// cap, since one row per file is cheap. A runaway short-query result list
// still shouldn't lag rendering, so we bound it.
const searchMaxFiles = 200

// searchExpandedMatchCap bounds how many match rows an expanded file shows
// inline, so expanding a file with hundreds of matches doesn't reflood the
// modal. The remainder is summarized by a faint "… N more in this file" row;
// read the rest by opening the file.
const searchExpandedMatchCap = 50

// searchDebounce is how long after the latest keystroke the scan fires.
// 150ms is the established UX threshold for "the system feels responsive
// without firing on every character."
const searchDebounce = 150 * time.Millisecond

// searchMinQuery is the minimum query length below which no scan fires.
const searchMinQuery = 2

// searchBarWidth is the fixed column width of the per-file match-count bar.
const searchBarWidth = 8

// barRunes are the Unicode left-block partials, index 0 = 1/8 (▏) … 7 = 8/8 (█).
var barRunes = []rune("▏▎▍▌▋▊▉█")

// searchRowKind distinguishes the flattened row variants the grouped modal
// renders: a file header, one of a file's matches (only when expanded), or a
// non-navigable "… N more" overflow summary.
type searchRowKind int

const (
	rowFile searchRowKind = iota
	rowMatch
	rowMore
)

// searchRow is one visible, flattened row. file indexes searchState.files;
// for rowMatch, line indexes that file's Lines; for rowMore, more is the
// count of hidden matches. The cursor is an index into the flattened slice —
// the same pattern the tree modal uses.
type searchRow struct {
	kind searchRowKind
	file int
	line int
	more int
}

// searchState bundles full-text search modal state.
//
// paths is a snapshot taken at modal-open time so a mid-search watcher event
// doesn't yank files out from under in-flight workers. New files appear only
// on the next open — deliberate; rescanning on every fsnotify event would be
// expensive AND yank cursor focus.
//
// files holds the grouped, sorted (count desc, then recency) results. expanded
// is the per-file fold state (path → open), defaulting to collapsed and reset
// on every new scan — it is a render cache, not user state to preserve. rows is
// the flattened view of files+expanded that the cursor and renderer index into.
//
// scanStop is the CancelFunc for the currently-running (or most-recently
// scheduled) scan; each keystroke that fires a new tick calls it first.
type searchState struct {
	input    textinput.Model
	paths    []string
	files    []search.FileMatches
	expanded map[string]bool
	rows     []searchRow
	cursor   int
	vp       viewport.Model
	scanStop context.CancelFunc
	// inFlight is true between dispatch of a scan and the landing (or stale
	// discard) of its searchResultsMsg. Drives the "(searching…)" placeholder.
	inFlight bool
}

// newSearch returns a zero-valued search state with a fresh textinput.
func newSearch() searchState {
	ti := textinput.New()
	ti.Prompt = "" // we render our own "> " prefix
	ti.Placeholder = ""
	ti.CharLimit = 256
	return searchState{
		vp:       viewport.New(0, 0),
		input:    ti,
		expanded: map[string]bool{},
	}
}

// reset clears every transient field and re-focuses the textinput. Called on
// every modal-open. paths is the snapshot of vault files captured at open time.
func (s *searchState) reset(paths []string) {
	if s.scanStop != nil {
		s.scanStop()
	}
	s.paths = paths
	s.files = nil
	s.rows = nil
	s.expanded = map[string]bool{}
	s.cursor = 0
	s.scanStop = nil
	s.inFlight = false
	s.input.SetValue("")
	s.input.Focus()
}

// flatten rebuilds rows from files + expanded. Called whenever results or
// fold state change. One rendered viewport line corresponds to one row.
func (s *searchState) flatten() {
	s.rows = s.rows[:0]
	for fi := range s.files {
		f := s.files[fi]
		s.rows = append(s.rows, searchRow{kind: rowFile, file: fi})
		if !s.expanded[f.Path] {
			continue
		}
		shown := len(f.Lines)
		if shown > searchExpandedMatchCap {
			shown = searchExpandedMatchCap
		}
		for li := 0; li < shown; li++ {
			s.rows = append(s.rows, searchRow{kind: rowMatch, file: fi, line: li})
		}
		if rest := len(f.Lines) - shown; rest > 0 {
			s.rows = append(s.rows, searchRow{kind: rowMore, file: fi, more: rest})
		}
	}
}

// moveCursor advances the cursor by delta over navigable rows, skipping the
// non-navigable rowMore overflow summaries. It clamps at the ends.
func (s *searchState) moveCursor(delta int) {
	if len(s.rows) == 0 || delta == 0 {
		return
	}
	i := s.cursor
	for {
		i += delta
		if i < 0 || i >= len(s.rows) {
			return // off the end — leave the cursor where it was
		}
		if s.rows[i].kind != rowMore {
			s.cursor = i
			return
		}
	}
}

// toggleSearchFold flips the fold state of the file under the cursor (or the
// parent file, if the cursor is on a match row), re-flattens, and re-parks the
// cursor on that file's header so expanding/collapsing never scrolls the cursor
// off the file you toggled.
func (m *Model) toggleSearchFold() {
	s := &m.modals.search
	if s.cursor < 0 || s.cursor >= len(s.rows) {
		return
	}
	fi := s.rows[s.cursor].file
	if fi < 0 || fi >= len(s.files) {
		return
	}
	path := s.files[fi].Path
	s.expanded[path] = !s.expanded[path]
	s.flatten()
	for i, r := range s.rows {
		if r.kind == rowFile && r.file == fi {
			s.cursor = i
			break
		}
	}
	m.refreshSearchVP()
}

// followSearchRow navigates on Enter: a file row jumps to that file's first
// match; a match row jumps to that match. Both reuse the existing scroll-to-line
// plumbing (pendingPreselectRange + navigateTo). A rowMore (or out-of-range
// cursor) is a no-op. Returns the closeModal Cmd, or nil if nothing was opened.
func (m *Model) followSearchRow() tea.Cmd {
	s := &m.modals.search
	if s.cursor < 0 || s.cursor >= len(s.rows) {
		return nil
	}
	r := s.rows[s.cursor]
	if r.file < 0 || r.file >= len(s.files) {
		return nil
	}
	f := s.files[r.file]
	var line int
	switch r.kind {
	case rowFile:
		if len(f.Lines) == 0 {
			return nil
		}
		line = f.Lines[0].Num // first match
	case rowMatch:
		line = f.Lines[r.line].Num
	default: // rowMore — not navigable
		return nil
	}
	cmd := m.closeModal()
	m.pending.preselectRange = &markdown.LineRange{Start: line, End: line}
	m.navigateTo(f.Path)
	return cmd
}

// searchTickMsg is delivered searchDebounce after each keystroke. query is the
// input value when the tick was scheduled; the handler drops it if newer.
type searchTickMsg struct {
	query string
}

// searchResultsMsg carries a grouped scan's output back into Update. query lets
// the handler discard results from a stale scan.
type searchResultsMsg struct {
	query string
	files []search.FileMatches
	err   error
}

// scheduleSearchTick returns a Cmd that fires a searchTickMsg after the debounce.
func scheduleSearchTick(query string) tea.Cmd {
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg {
		return searchTickMsg{query: query}
	})
}

// runSearchCmd returns a Cmd that runs the grouped scan in a goroutine and
// emits searchResultsMsg. paths is captured by value.
func runSearchCmd(ctx context.Context, paths []string, query string) tea.Cmd {
	return func() tea.Msg {
		files, err := search.SearchGrouped(ctx, paths, query, searchMaxFiles)
		return searchResultsMsg{query: query, files: files, err: err}
	}
}

// handleSearchKey forwards a key to the textinput, then decides whether to
// schedule a debounced scan tick.
func (m *Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	before := m.modals.search.input.Value()
	var cmd tea.Cmd
	m.modals.search.input, cmd = m.modals.search.input.Update(msg)
	query := m.modals.search.input.Value()
	if query == before {
		// Cursor/selection keystroke that didn't change the query.
		return *m, cmd
	}
	// Query changed — drop prior results so the user doesn't see stale rows
	// until the new scan returns.
	m.clearSearchResults()
	if len(query) < searchMinQuery {
		if m.modals.search.scanStop != nil {
			m.modals.search.scanStop()
			m.modals.search.scanStop = nil
		}
		m.modals.search.inFlight = false
		m.refreshSearchVP()
		return *m, nil
	}
	// Cancel any prior in-flight scan immediately.
	if m.modals.search.scanStop != nil {
		m.modals.search.scanStop()
		m.modals.search.scanStop = nil
	}
	m.refreshSearchVP()
	tick := scheduleSearchTick(query)
	// ClearScreen on every query-changing keystroke: under rapid typing with
	// slow scans, the diff renderer can leave stale prompt rows on screen.
	if cmd != nil {
		return *m, tea.Batch(cmd, tick, tea.ClearScreen)
	}
	return *m, tea.Batch(tick, tea.ClearScreen)
}

// clearSearchResults drops the current results, fold state, and flattened rows.
func (m *Model) clearSearchResults() {
	m.modals.search.files = nil
	m.modals.search.rows = nil
	m.modals.search.expanded = map[string]bool{}
	m.modals.search.cursor = 0
}

// refreshSearchVP regenerates the modal's viewport from current rows/cursor/query.
func (m *Model) refreshSearchVP() {
	m.modals.search.vp.SetContent(m.renderSearchRows())
	viewportClamp(&m.modals.search.vp, m.modals.search.cursor, 2)
}

// renderSearchRows produces the viewport body: placeholders when nothing should
// display, or the grouped file list otherwise.
func (m *Model) renderSearchRows() string {
	q := m.modals.search.input.Value()
	faint := lipgloss.NewStyle().Faint(true)
	switch {
	case len(m.modals.search.paths) == 0:
		return faint.Render("(no markdown files in vault)")
	case len(q) < searchMinQuery:
		return faint.Render("(type 2+ chars to search)")
	case m.modals.search.inFlight && len(m.modals.search.files) == 0:
		return faint.Render("(searching…)")
	case !m.modals.search.inFlight && len(m.modals.search.files) == 0:
		return faint.Render(`(no match for "` + q + `")`)
	default:
		return m.formatSearchFiles()
	}
}

// handleSearchTick fires when a debounce tick lands.
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
	ctx, cancel := context.WithCancel(context.Background())
	m.modals.search.scanStop = cancel
	m.modals.search.inFlight = true
	m.refreshSearchVP()
	return *m, runSearchCmd(ctx, m.modals.search.paths, msg.query)
}

// handleSearchResults consumes a grouped scan's output. Stale results — from a
// cancelled scan whose query no longer matches the input — are discarded.
func (m *Model) handleSearchResults(msg searchResultsMsg) (tea.Model, tea.Cmd) {
	if m.modals.kind != modalSearch {
		return *m, nil
	}
	m.modals.search.inFlight = false
	m.modals.search.scanStop = nil
	if msg.query != m.modals.search.input.Value() {
		m.refreshSearchVP()
		return *m, nil
	}
	if msg.err != nil && msg.err != context.Canceled {
		if m.diag != nil {
			m.diag.Info(fmt.Sprintf("search %q: %v", msg.query, msg.err))
		}
	}
	m.modals.search.files = sortSearchFiles(msg.files)
	m.modals.search.expanded = map[string]bool{} // fresh scan starts all-collapsed
	m.modals.search.cursor = 0
	m.modals.search.flatten()
	m.refreshSearchVP()
	if m.diag != nil {
		m.diag.Info(fmt.Sprintf("search %q: %d files", msg.query, len(msg.files)))
	}
	// Full repaint when results arrive: the modal frame may have shifted rows
	// since the scan started, and the diff renderer doesn't always clear them.
	return *m, tea.ClearScreen
}

// sortSearchFiles orders files by match count descending, tie-broken by edit
// recency (mtime, newest first) — so the densest files surface first and ties
// fall back to the same recency signal the finder uses. Within a file, Lines
// stay ascending (the scan produces them in line order). Stateless.
func sortSearchFiles(files []search.FileMatches) []search.FileMatches {
	if len(files) < 2 {
		return files
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	rank := map[string]int{}
	for i, p := range recent.RankPathsByMTime(paths) {
		rank[p] = i // 0 = most recently edited
	}
	sort.SliceStable(files, func(a, b int) bool {
		if files[a].Count != files[b].Count {
			return files[a].Count > files[b].Count
		}
		return rank[files[a].Path] < rank[files[b].Path]
	})
	return files
}

// formatSearchFiles renders the flattened rows: file headers (cursor marker,
// count, proportional bar, relative path, fold caret) and, for expanded files,
// indented match rows (line number + highlighted snippet) plus an overflow tail.
func (m *Model) formatSearchFiles() string {
	s := &m.modals.search
	if len(s.files) == 0 {
		return ""
	}
	maxCount := s.files[0].Count // files are sorted count-descending
	countW := len(strconv.Itoa(maxCount))
	width := s.vp.Width
	rev := lipgloss.NewStyle().Reverse(true)
	faint := lipgloss.NewStyle().Faint(true)

	lines := make([]string, 0, len(s.rows))
	for i, r := range s.rows {
		var line string
		switch r.kind {
		case rowFile:
			f := s.files[r.file]
			caret := "▸"
			if s.expanded[f.Path] {
				caret = "▾"
			}
			count := fmt.Sprintf("%*d", countW, f.Count)
			bar := renderBar(f.Count, maxCount, searchBarWidth)
			path := relativeTo(m.root, f.Path)
			line = truncateOneLine(caret+" "+count+" "+bar+"  "+path, width)
		case rowMatch:
			ln := s.files[r.file].Lines[r.line]
			num := fmt.Sprintf("%*d", countW+2, ln.Num)
			snip := applyHighlight(search.RenderSnippet(ln, search.SnippetBudget))
			line = truncateOneLine("    "+num+"  "+snip, width)
		case rowMore:
			line = faint.Render(truncateOneLine("    … "+strconv.Itoa(r.more)+" more in this file", width))
		}
		if i == s.cursor {
			line = rev.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// renderBar draws a fixed-width bar whose filled length encodes count relative
// to max. The scale is sqrt (so a single-match file isn't invisible next to a
// dominant one) with a one-eighth floor for any nonzero count — the numeric
// count remains the precise source of truth; the bar is a scannability aid.
func renderBar(count, max, width int) string {
	if max <= 0 || count <= 0 {
		return strings.Repeat(" ", width)
	}
	total := width * 8 // capacity in eighths
	frac := math.Sqrt(float64(count)) / math.Sqrt(float64(max))
	eighths := int(math.Round(frac * float64(total)))
	if eighths < 1 {
		eighths = 1 // floor: any match shows ▏
	}
	if eighths > total {
		eighths = total
	}
	full := eighths / 8
	rem := eighths % 8
	var b strings.Builder
	used := 0
	for ; used < full; used++ {
		b.WriteRune('█')
	}
	if rem > 0 && used < width {
		b.WriteRune(barRunes[rem-1])
		used++
	}
	for ; used < width; used++ {
		b.WriteByte(' ')
	}
	return b.String()
}

// resizeSearch fits the modal's viewport into the modal interior, reserving
// rows for the query prompt and separator on top.
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
	// Reserve 2 cols for "> " AND 1 for the textinput's cursor block, which
	// renders past the value's end (see the prompt-fit regression tests).
	m.modals.search.input.Width = pw - 3
	m.refreshSearchVP()
}

// searchView returns the modal's renderable body — prompt, separator, viewport,
// and an optional file-overflow footer.
func (m *Model) searchView() string {
	p := &m.modals.search
	sepW := p.vp.Width
	if sepW < 1 {
		sepW = 1
	}
	// Force the prompt to exactly one row of width sepW so cursor overhang or a
	// width miscount can't wrap it onto a second row (the stacked-prompts bug).
	prompt := lipgloss.NewStyle().
		Width(sepW).
		MaxHeight(1).
		Render("> " + p.input.View())
	sep := strings.Repeat("─", sepW)
	body := prompt + "\n" + sep + "\n" + p.vp.View()
	if len(p.files) >= searchMaxFiles {
		body += "\n" + lipgloss.NewStyle().Faint(true).
			Render("… results truncated at "+strconv.Itoa(searchMaxFiles)+" files, refine the query")
	}
	return body
}
