package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/wilkes/hypogeum/internal/recent"
)

// pickerMaxVisible caps how many rows render at once. The cursor is
// clamped to this range; refining the query is the way to reach hidden
// rows.
const pickerMaxVisible = 200

// pickerState is the flat, recency-ranked file finder rendered as a modal.
// Replaces the previous tree-rooted picker; cursor indexes into ranked.
type pickerState struct {
	all     []recent.Ranked  // full ranked list captured at open time
	ranked  []recent.Ranked  // currently visible (filtered or all)
	matches []fuzzy.Match    // parallel to ranked when query non-empty
	cursor  int
	vp      viewport.Model
	roots   []string // vault roots, used to render relative paths
	input   textinput.Model
}

func newPicker() pickerState {
	ti := textinput.New()
	ti.Prompt = ""      // we render our own "> " prefix
	ti.Placeholder = ""
	ti.CharLimit = 256
	return pickerState{
		vp:    viewport.New(0, 0),
		input: ti,
	}
}

// reset populates the picker with a fresh ranked list, resets the cursor
// and query, and focuses the textinput. Called on every picker open.
func (p *pickerState) reset(ranked []recent.Ranked, roots []string) {
	p.all = ranked
	p.ranked = ranked
	p.matches = nil
	p.cursor = 0
	p.roots = roots
	p.input.SetValue("")
	p.input.Focus()
	p.refreshVP()
}

// refilter recomputes p.ranked and p.matches from p.all and the current
// query. Empty query → ranked == all, matches == nil. Otherwise: run
// sahilm/fuzzy over a lowercased copy of the paths, then stable-sort by
// score descending with the source-order index (i.e. recency rank) as
// the tiebreaker. Cursor resets to 0 on every call.
func (p *pickerState) refilter() {
	q := strings.ToLower(p.input.Value())
	if q == "" {
		p.ranked = p.all
		p.matches = nil
		p.cursor = 0
		p.refreshVP()
		return
	}
	src := make([]string, len(p.all))
	for i, r := range p.all {
		src[i] = strings.ToLower(relPathForRoots(p.roots, r.Path))
	}
	raw := fuzzy.Find(q, src)
	sort.SliceStable(raw, func(i, j int) bool {
		if raw[i].Score != raw[j].Score {
			return raw[i].Score > raw[j].Score
		}
		return raw[i].Index < raw[j].Index
	})
	p.ranked = make([]recent.Ranked, len(raw))
	p.matches = make([]fuzzy.Match, len(raw))
	for i, m := range raw {
		p.ranked[i] = p.all[m.Index]
		p.matches[i] = m
	}
	p.cursor = 0
	p.refreshVP()
}

// refreshVP regenerates the viewport content and scrolls so the cursor row
// is in view.
func (p *pickerState) refreshVP() {
	p.vp.SetContent(p.renderRows())
	viewportClamp(&p.vp, p.cursor, 1)
}

func (p *pickerState) renderRows() string {
	width := p.vp.Width
	if width < 20 {
		width = 20
	}
	if p.input.Value() != "" && len(p.ranked) == 0 {
		return lipgloss.NewStyle().Faint(true).
			Render(`(no match for "` + p.input.Value() + `")`)
	}

	now := time.Now()
	var b strings.Builder
	visible := len(p.ranked)
	if visible > pickerMaxVisible {
		visible = pickerMaxVisible
	}
	for i := 0; i < visible; i++ {
		r := p.ranked[i]
		rel := relPathForRoots(p.roots, r.Path)
		recencyLabel, edited := pickRecencyLabel(now, r.MTime, r.Visit)
		suffix := recencyLabel
		if edited {
			suffix += " · edited"
		}
		pathDisplay := preTruncatePath(rel, suffix, width)
		if p.input.Value() != "" {
			pathDisplay = highlightMatch(pathDisplay, p.input.Value())
		}
		line := formatPickerRow(pathDisplay, suffix, width)
		if i == p.cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// renderOverflowFooter returns the "… N more" faint footer when ranked
// exceeds pickerMaxVisible, or an empty string otherwise.
func (p *pickerState) renderOverflowFooter() string {
	if overflow := len(p.ranked) - pickerMaxVisible; overflow > 0 {
		return lipgloss.NewStyle().Faint(true).
			Render("… " + strconv.Itoa(overflow) + " more")
	}
	return ""
}

// relPathForRoots renders p relative to whichever root yields the shortest
// relative path (the best containing root), falling back to the absolute path
// when p lives under none of the roots. With a single root it behaves like a
// plain filepath.Rel; with overlaid roots it picks the owning one so display
// paths stay short and unambiguous.
func relPathForRoots(roots []string, p string) string {
	best := ""
	for _, root := range roots {
		if root == "" {
			continue
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if best == "" || len(rel) < len(best) {
			best = rel
		}
	}
	if best == "" {
		return p
	}
	return best
}

// pickRecencyLabel returns the human-friendly recency label and a flag
// indicating whether the time used was mtime (true) or visit (false).
func pickRecencyLabel(now, mtime, visit time.Time) (label string, isMTime bool) {
	t := mtime
	isMTime = true
	if !visit.IsZero() && visit.After(mtime) {
		t = visit
		isMTime = false
	}
	return humanRecency(now, t), isMTime
}

// humanRecency formats a duration since t in one-glance form.
// Beyond ~6 weeks it falls back to YYYY-MM-DD.
func humanRecency(now, t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	case d < 6*7*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d/(7*24*time.Hour)))
	default:
		return t.Format("2006-01-02")
	}
}

// formatPickerRow lays out one row to fit width. Left column is path
// (truncated with leading ellipsis if too long), right column is suffix
// (right-aligned). One-space gap minimum between them.
func formatPickerRow(left, right string, width int) string {
	const gap = 2
	rightW := ansi.StringWidth(right)
	leftBudget := width - rightW - gap
	if leftBudget < 5 {
		return strings.Repeat(" ", width-rightW) + right
	}
	leftTrunc := truncateLeadingEllipsis(left, leftBudget)
	leftW := ansi.StringWidth(leftTrunc)
	pad := width - leftW - rightW
	if pad < gap {
		pad = gap
	}
	return leftTrunc + strings.Repeat(" ", pad) + right
}

// highlightStyle is the lipgloss style for matched characters in a row.
var highlightStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))

// highlightMatch wraps the bytes of display that match query (via a
// fresh fuzzy.Find pass) in highlightStyle. Returns display unchanged
// if query is empty or doesn't match. Called per visible row after
// truncation, so indices map to the truncated string.
func highlightMatch(display, query string) string {
	if query == "" {
		return display
	}
	src := strings.ToLower(display)
	matches := fuzzy.Find(strings.ToLower(query), []string{src})
	if len(matches) == 0 {
		return display
	}
	idx := matches[0].MatchedIndexes
	if len(idx) == 0 {
		return display
	}
	// sahilm/fuzzy MatchedIndexes are byte offsets (the inner loop variable j
	// in fuzzy.go advances by candidateSize bytes via utf8.DecodeRuneInString).
	// Iterate by byte offset so multibyte runes are addressed correctly.
	var b strings.Builder
	bytePos := 0
	for _, r := range display {
		runeLen := len(string(r))
		if containsInt(idx, bytePos) {
			b.WriteString(highlightStyle.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
		bytePos += runeLen
	}
	return b.String()
}

// containsInt reports whether n is in the (small) sorted slice s.
func containsInt(s []int, n int) bool {
	for _, v := range s {
		if v == n {
			return true
		}
		if v > n {
			return false
		}
	}
	return false
}

// preTruncatePath returns the path trimmed to whatever column budget
// formatPickerRow would have available for the left side. Centralizes
// the width math so highlightMatch operates on the actually-visible
// characters.
func preTruncatePath(path, suffix string, width int) string {
	const gap = 2
	rightW := ansi.StringWidth(suffix)
	leftBudget := width - rightW - gap
	if leftBudget < 5 {
		return path
	}
	return truncateLeadingEllipsis(path, leftBudget)
}

// truncateLeadingEllipsis truncates s to fit max, preferring to drop
// characters from the start (so the basename stays visible).
func truncateLeadingEllipsis(s string, max int) string {
	if ansi.StringWidth(s) <= max {
		return s
	}
	const ell = "…"
	keep := max - ansi.StringWidth(ell)
	if keep < 1 {
		return ell
	}
	return ell + ansi.TruncateLeft(s, ansi.StringWidth(s)-keep, "")
}

func (p *pickerState) renderQueryPrompt() string {
	return "> " + p.input.View()
}

func (p *pickerState) renderSeparator() string {
	w := p.vp.Width
	if w < 1 {
		w = 1
	}
	return strings.Repeat("─", w)
}

// View returns the picker's renderable string: prompt, separator, list,
// and an optional overflow footer outside the viewport.
func (p *pickerState) View() string {
	if len(p.all) == 0 {
		return p.renderQueryPrompt() + "\n" + p.renderSeparator() + "\n" +
			lipgloss.NewStyle().Faint(true).Render("(no markdown files in vault)")
	}
	out := p.renderQueryPrompt() + "\n" + p.renderSeparator() + "\n" + p.vp.View()
	if footer := p.renderOverflowFooter(); footer != "" {
		out += "\n" + footer
	}
	return out
}

// resizePicker fits the picker viewport into the modal interior, leaving
// two rows at the top for the query prompt and separator.
func (m *Model) resizePicker() {
	_, _, w, h := modalGeometry(m.width, m.height)
	pw := w - 2
	ph := h - 2 - 2 // border (2) + prompt+separator (2)
	if pw < 1 {
		pw = 1
	}
	if ph < 1 {
		ph = 1
	}
	m.modals.picker.vp.Width = pw
	m.modals.picker.vp.Height = ph
	m.modals.picker.input.Width = pw - 2 // leave room for "> " prefix
	m.modals.picker.refreshVP()
}

// selectedPath returns the path under the picker cursor, or ("", false)
// if the cursor is out of range or the list is empty.
func (p *pickerState) selectedPath() (string, bool) {
	if p.cursor < 0 || p.cursor >= len(p.ranked) {
		return "", false
	}
	return p.ranked[p.cursor].Path, true
}
