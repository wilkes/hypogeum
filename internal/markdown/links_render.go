package markdown

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/wilkes/hypogeum/internal/embed"
	"github.com/wilkes/hypogeum/internal/wikilink"
)

// Sentinel runes injected into the rendered output. They survive
// Glamour's word-wrap pass and get stripped out of the returned
// rendered string. Chosen as ASCII separator characters that are
// extremely unlikely to appear in user content.
//
// The link-text pair (sentinelStart/sentinelEnd) brackets the visible
// text of every link; stripSentinels records each pair as a sentinelSpan
// and the caller learns the link's row in the cleaned output.
//
// The url-suppress pair (urlSuppressStart/urlSuppressEnd) brackets the
// URL portion Glamour emits after every hyperlink. stripSentinels
// discards everything between the pair, plus the single space Glamour
// hardcodes immediately before urlSuppressStart, so the rendered prose
// reads as "[text]" instead of "[text] /path/to/target.md".
const (
	sentinelStart    = '\x1c' // FS (file separator)
	sentinelEnd      = '\x1e' // RS (record separator)
	urlSuppressStart = '\x1d' // GS (group separator)
	urlSuppressEnd   = '\x1f' // US (unit separator)
)

// Link is a renderable hyperlink: the visible text the user reads, the raw
// href as written in the source, where to navigate to, and which row of
// the rendered output its first character lives on.
type Link struct {
	Text     string       // visible text from the markdown source
	Href     string       // raw href (unresolved)
	Resolved ResolvedLink // classified + path-resolved target
	Row      int          // zero-indexed row in the rendered output
}

// LinkMarker brackets a link's visible text in the rendered output. The
// TUI uses this to inject BubbleZone Mark/Close pairs without coupling
// the markdown package to a specific zone library.
type LinkMarker func(linkIndex int) (open, close string)

// HighlightMarker returns a LinkMarker that wraps the link at index
// selected in SGR reverse-video (terminal-native selection highlight).
// All other links get empty open/close strings. Pass selected=-1 to
// highlight nothing (same as nil marker but explicit).
func HighlightMarker(selected int) LinkMarker {
	return func(i int) (string, string) {
		if i == selected {
			return "\x1b[7m", "\x1b[27m" // reverse-video on / off
		}
		return "", ""
	}
}

// RenderWithLinks renders src and returns both the rendered string and a
// list of every followable link in document order. base is the path of
// the file the source came from; it's used to resolve relative link
// targets to absolute paths.
//
// If marker is non-nil, the open/close strings it returns for each link
// are spliced around that link's visible text in the rendered output.
// They flow through downstream styling without changing visible width
// (caller's responsibility — typically zero-width sentinel sequences).
func (r *Renderer) RenderWithLinks(src, base string, marker LinkMarker) (string, []Link, []string, error) {
	src, embedDeps, embedLinks := r.preprocessEmbeds(src, base)
	src = r.preprocessWikilinks(src)
	raw, err := r.instrumented.Render(src)
	if err != nil {
		return "", nil, nil, fmt.Errorf("render markdown: %w", err)
	}

	asts := ExtractLinks(src)
	cleaned, spans := stripSentinels(raw, marker)
	links := make([]Link, 0, len(spans)+len(embedLinks))
	for i, s := range spans {
		l := Link{Row: s.row}
		if i < len(asts) {
			l.Text = asts[i].Text
			l.Href = asts[i].Href
			l.Resolved = ResolveLink(base, asts[i].Href)
		} else {
			l.Text = s.text
		}
		links = append(links, l)
	}
	links = append(links, embedLinks...)
	return cleaned, links, embedDeps, nil
}

// wikilinkRegex matches the wikilink syntax for the source-rewrite pass.
// We use a regex rather than goldmark for the rewrite because goldmark's
// AST → source round-trip is lossy; rewriting strings preserves
// everything else about the source unchanged.
var wikilinkRegex = regexp.MustCompile(`\[\[([^\]\n]+)\]\]`)

// preprocessWikilinks rewrites [[...]] occurrences in src into either
// standard markdown links (resolved) or styled placeholder text
// (unresolved). The resulting string is then handed to Glamour as
// normal markdown. Fenced code blocks and inline-code backtick spans
// are skipped so wikilink demos written as `[[Name]]` render verbatim.
func (r *Renderer) preprocessWikilinks(src string) string {
	if r.resolver == nil || !strings.Contains(src, "[[") {
		return src
	}
	replace := func(match string) string {
		body := match[2 : len(match)-2]
		w := wikilink.Parse(body)
		if w == nil {
			return match
		}
		display := w.Alias
		if display == "" {
			display = w.Name
			if w.Heading != "" {
				display = w.Name + " > " + w.Heading
			}
		}
		path, ok := r.resolver.Resolve(r.fromFile, w.Name, w.Heading, w.Block)
		if !ok {
			return display + "?"
		}
		href := path
		if w.Heading != "" {
			href = path + "#" + slugify(w.Heading)
		}
		return "[" + display + "](" + href + ")"
	}
	var b strings.Builder
	b.Grow(len(src))
	for _, seg := range splitOutsideFences(src) {
		if seg.isFence {
			b.WriteString(seg.text)
			continue
		}
		b.WriteString(replaceOutsideInlineCode(seg.text, wikilinkRegex, replace))
	}
	return b.String()
}

// CountUnresolvedWikilinks counts every [[...]] in src that the
// configured resolver does NOT resolve. Fences are skipped (matches
// preprocessWikilinks). When no resolver is configured, every
// well-formed wikilink counts as unresolved.
//
// Used by the TUI footer's broken-link tally; the count complements
// the per-document link list (which intentionally excludes unresolved
// wikilinks, since they can't be followed).
func (r *Renderer) CountUnresolvedWikilinks(src string) int {
	if !strings.Contains(src, "[[") {
		return 0
	}
	count := 0
	check := func(match string) string {
		body := match[2 : len(match)-2]
		w := wikilink.Parse(body)
		if w == nil {
			return match
		}
		if r.resolver == nil {
			count++
			return match
		}
		if _, ok := r.resolver.Resolve(r.fromFile, w.Name, w.Heading, w.Block); !ok {
			count++
		}
		return match
	}
	for _, seg := range splitOutsideFences(src) {
		if seg.isFence {
			continue
		}
		replaceOutsideInlineCode(seg.text, wikilinkRegex, check)
	}
	return count
}

// slugify is the same heading-slug rule used by anchor-style links.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// sentinelSpan records where a sentinel-bracketed link landed in the
// cleaned (sentinel-free) output.
type sentinelSpan struct {
	row  int    // zero-indexed line number in cleaned output
	text string // visible text inside the sentinel pair, ANSI stripped
}

// stripSentinels removes every sentinel byte from raw and returns the
// cleaned string plus a list of (row, visible-text) spans, one per link.
// If marker is non-nil, each link's text is wrapped with the open/close
// strings it returns, in place of the link-text sentinels.
//
// Sentinel pairs handled:
//   - sentinelStart..sentinelEnd: link visible text. Recorded as a span.
//   - urlSuppressStart..urlSuppressEnd: URL portion. Discarded along with
//     the single space Glamour hardcodes before it, so "[text] /url"
//     collapses to "[text]" in the cleaned output.
func stripSentinels(raw string, marker LinkMarker) (string, []sentinelSpan) {
	var (
		out        strings.Builder
		spans      []sentinelSpan
		row        int
		inLink     bool
		openEmit   bool // true once openMark has been written for the current link
		linkText   strings.Builder
		linkRow    int
		linkIdx    int
		openMark   string
		closeMark  string
	)
	out.Grow(len(raw))

	i := 0
	for i < len(raw) {
		c := raw[i]
		// Pass through CSI escape sequences untouched, but track them so
		// they don't pollute link text.
		if c == 0x1b && i+1 < len(raw) && raw[i+1] == '[' {
			j := i + 2
			for j < len(raw) && raw[j] != 'm' {
				j++
			}
			out.WriteString(raw[i : j+1])
			i = j + 1
			continue
		}
		switch c {
		case sentinelStart:
			inLink = true
			openEmit = false
			linkText.Reset()
			linkRow = row
			openMark, closeMark = "", ""
			if marker != nil {
				openMark, closeMark = marker(linkIdx)
			}
			i++
		case sentinelEnd:
			if inLink {
				if !openEmit {
					// Span contained only escapes — emit openMark now so
					// closeMark has a matching open.
					out.WriteString(openMark)
				}
				out.WriteString(closeMark)
				spans = append(spans, sentinelSpan{row: linkRow, text: linkText.String()})
				inLink = false
				openEmit = false
				linkIdx++
			}
			i++
		case urlSuppressStart:
			// Drop everything until urlSuppressEnd, including any ANSI
			// styling Glamour applied to the URL. In prose mid-sentence
			// we peel back the leading space too; in table cells / at
			// end of a wrapped line we replace the range with spaces of
			// equivalent visible width so column borders stay aligned.
			// Never strip a newline — that would join paragraphs.
			i = urlSuppressStrip(&out, raw, i)
		case '\n':
			if inLink && openEmit {
				// Close the highlight before the newline so Glamour's
				// per-row \e[0m tail doesn't suppress reverse-video on
				// continuation rows. The lazy emit below re-opens on
				// the next row's first content byte.
				out.WriteString(closeMark)
				openEmit = false
			}
			row++
			out.WriteByte(c)
			if inLink {
				linkText.WriteByte(c)
			}
			i++
		default:
			if inLink && !openEmit {
				out.WriteString(openMark)
				openEmit = true
			}
			out.WriteByte(c)
			if inLink {
				linkText.WriteByte(c)
			}
			i++
		}
	}
	return out.String(), spans
}

// urlSuppressStrip handles the urlSuppressStart..urlSuppressEnd range
// at raw[startIdx]. Returns the new index past the range and the bytes
// to emit in place of the stripped range.
//
// In prose mid-sentence (the next non-space content byte is a letter,
// digit, etc.), we strip cleanly: the leading space before urlSuppressStart
// is peeled back, and the entire sentinel range is dropped. Result:
// "[text] /url body" collapses to "[text] body" without doubling the space.
//
// In tables and at end-of-line, Glamour sized the cell or wrap line
// counting the URL bytes as visible content. Dropping them mid-rendered
// output shortens that column or line by exactly the URL's display width,
// which is why tables with link cells render with ragged right edges and
// misaligned column borders. We keep the leading space and replace the
// sentinel range with spaces of equivalent width so the visible column /
// padding stays exactly what Glamour intended.
//
// Used by stripSentinels and stripURLSentinels — keep both call sites in
// sync.
func urlSuppressStrip(out *strings.Builder, raw string, startIdx int) int {
	end := startIdx + 1
	for end < len(raw) && raw[end] != urlSuppressEnd {
		end++
	}
	if end < len(raw) {
		end++ // consume urlSuppressEnd
	}

	if isPaddingContextAfter(raw, end) {
		// Don't trim the leading space; replace the URL range with spaces
		// of equivalent visible width so column borders stay aligned.
		width := ansi.StringWidth(raw[startIdx+1 : end-1])
		for k := 0; k < width; k++ {
			out.WriteByte(' ')
		}
		return end
	}

	trimTrailingSpace(out)
	return end
}

// isPaddingContextAfter reports whether the bytes after idx — up to the
// next newline — are entirely spaces, ANSI escapes, and (optionally) a
// table column-border character. This is true when the URL sits inside a
// table cell or at the visible end of a wrapped/padded line.
func isPaddingContextAfter(raw string, idx int) bool {
	for idx < len(raw) {
		c := raw[idx]
		switch {
		case c == '\n':
			return true
		case c == ' ':
			idx++
		case c == 0x1b:
			j := skipEscape(raw, idx)
			if j == idx {
				return false
			}
			idx = j
		case isTableBorderByte(raw, idx):
			return true
		default:
			return false
		}
	}
	return true
}

// skipEscape returns the index past the ANSI escape sequence at s[i], or
// i unchanged if no escape starts there. Recognizes CSI (\x1b[...m).
func skipEscape(s string, i int) int {
	if i+1 >= len(s) || s[i] != 0x1b || s[i+1] != '[' {
		return i
	}
	j := i + 2
	for j < len(s) && s[j] != 'm' {
		j++
	}
	if j < len(s) {
		return j + 1
	}
	return i
}

// isTableBorderByte reports whether s[i] starts a UTF-8 box-drawing
// character used by Glamour's table renderer. Currently only │ (U+2502,
// 0xE2 0x94 0x82) appears in cell rows; the horizontal rules (── ┼ ─) are
// on their own line and don't interfere with width math.
func isTableBorderByte(s string, i int) bool {
	return i+2 < len(s) && s[i] == 0xE2 && s[i+1] == 0x94 && s[i+2] == 0x82
}

// trimTrailingSpace removes the most recent printable space byte from b,
// skipping over any trailing ANSI escape sequences. Glamour writes the
// space between LinkText and Link as part of the URL element's Prefix
// using the parent style, which means the rendered byte order is
// "<space>\x1b[0m...\x1d". We need to remove the space, not the escape,
// so the post-strip prose reads cleanly.
//
// Used by stripSentinels and stripURLSentinels.
func trimTrailingSpace(b *strings.Builder) {
	s := b.String()
	end := len(s)
	for end > 0 {
		// Walk back over any complete trailing CSI/OSC escapes.
		if i := lastEscapeStart(s, end); i >= 0 {
			end = i
			continue
		}
		break
	}
	if end == 0 || s[end-1] != ' ' {
		return
	}
	rest := s[end:]
	b.Reset()
	b.WriteString(s[:end-1])
	b.WriteString(rest)
}

// lastEscapeStart returns the index of the start of an ANSI escape
// (\x1b...) that ends exactly at end, or -1 if none. Recognizes
// CSI ("...m") and OSC ("...\x1b\\" or "...\x07") forms.
func lastEscapeStart(s string, end int) int {
	if end < 2 {
		return -1
	}
	last := s[end-1]
	// CSI: ends in 'm'.
	if last == 'm' {
		for i := end - 2; i >= 0; i-- {
			if s[i] == 0x1b && i+1 < end && s[i+1] == '[' {
				return i
			}
			if s[i] == 0x1b {
				return -1
			}
		}
		return -1
	}
	// OSC ST: ends in "\x1b\\".
	if end >= 2 && s[end-2] == 0x1b && last == '\\' {
		for i := end - 3; i >= 0; i-- {
			if s[i] == 0x1b && i+1 < end && s[i+1] == ']' {
				return i
			}
		}
		return -1
	}
	// OSC BEL: ends in "\x07".
	if last == 0x07 {
		for i := end - 2; i >= 0; i-- {
			if s[i] == 0x1b && i+1 < end && s[i+1] == ']' {
				return i
			}
		}
		return -1
	}
	return -1
}

// embedTokenRegex matches ![[...]] outside of fenced code blocks.
// Fence detection is handled by splitOutsideFences below — inline
// `code` spans are NOT detected, so an embed inside a single-backtick
// span will still be processed. Order with preprocessWikilinks
// matters: this pass runs first so the ![[...]] form is consumed
// before the [[...]] regex sees it.
var embedTokenRegex = regexp.MustCompile(`!\[\[([^\]\n]+)\]\]`)

// preprocessEmbeds replaces every ![[...]] in src with a markdown fenced
// code block sliced from the referenced source file. Returns the rewritten
// src, the absolute paths of every successfully embedded source file
// (one entry per *distinct* path, deduped), and the synthetic Link entries
// that represent the embeds in the navigable link list.
//
// Failures (missing/binary/oversize/invalid range) render as a one-line
// blockquote warning in place of the embed.
func (r *Renderer) preprocessEmbeds(src, base string) (string, []string, []Link) {
	if !strings.Contains(src, "![[") {
		return src, nil, nil
	}

	var (
		deps  []string
		seen  = map[string]struct{}{}
		links []Link
	)
	replace := func(match string) string {
		body := match[3 : len(match)-2] // strip ![[ and ]]
		em, perr := embed.ParseEmbedToken(body)
		if perr != nil {
			return warningBlock(body, perr.Error())
		}

		absPath := em.Path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(filepath.Dir(base), absPath)
		}
		absPath, _ = filepath.Abs(absPath)

		lines, startLine, serr := embed.SliceFile(absPath, em.Range, em.ContextLines)
		soft := ""
		switch {
		case errors.Is(serr, embed.ErrNotFound):
			return warningBlock(em.Path, "file not found")
		case errors.Is(serr, embed.ErrBinary):
			return warningBlock(em.Path, "binary file, not embedded")
		case errors.Is(serr, embed.ErrTooLarge):
			return warningBlock(em.Path, "file too large to embed")
		case errors.Is(serr, embed.ErrInvalidRange):
			return warningBlock(em.Path, "invalid range")
		case errors.Is(serr, embed.ErrRangePastEOF):
			soft = "file ends at line " + strconv.Itoa(startLine+len(lines)-1)
			// keep going; lines and startLine are populated and valid
		case serr != nil:
			return warningBlock(em.Path, serr.Error())
		}

		displayRange := embedDisplayRange(em)
		leadCtx, tailCtx := 0, 0
		if em.Range != nil && em.ContextLines > 0 {
			leadCtx = em.Range.Start - startLine
			if leadCtx < 0 {
				leadCtx = 0
			}
			tailCtx = (startLine + len(lines) - 1) - em.Range.End
			if tailCtx < 0 {
				tailCtx = 0
			}
		}

		if _, ok := seen[absPath]; !ok {
			seen[absPath] = struct{}{}
			deps = append(deps, absPath)
		}
		l := Link{
			Text: em.Path,
			Href: body,
			// Row=-1 is the no-scroll sentinel honored by
			// (*Model).scrollToLink — embeds have no representative
			// single line, so cursor moves but viewport stays put.
			Row: -1,
			Resolved: ResolvedLink{
				Kind:   LinkLocalFile,
				Target: absPath,
			},
		}
		if em.Range != nil {
			l.Resolved.Range = em.Range
		}
		links = append(links, l)

		return embed.RenderToFence(absPath, lines, startLine, displayRange, leadCtx, tailCtx, soft)
	}

	var b strings.Builder
	b.Grow(len(src))
	for _, seg := range splitOutsideFences(src) {
		if seg.isFence {
			b.WriteString(seg.text)
			continue
		}
		b.WriteString(replaceOutsideInlineCode(seg.text, embedTokenRegex, replace))
	}
	out := b.String()
	return out, deps, links
}

// warningBlock formats an embed failure as a one-line blockquote that
// Glamour will style faintly, preserving the surrounding document flow.
func warningBlock(path, reason string) string {
	return "> ⚠ `" + path + "`: " + reason + "\n"
}

// replaceOutsideInlineCode applies pattern.ReplaceAllStringFunc(src, replace)
// to every region of src that is NOT inside an inline backtick code span.
// Code spans pass through verbatim so wikilink/embed demos written as
// `[[Name]]` or `![[file]]` render as code instead of being rewritten.
//
// Multi-line spans are out of scope: the wikilink and embed regexes both
// require their tokens on a single line, and our docs don't use
// multi-line backtick spans containing wikilink syntax. inlineCodeSpans
// only matches closing runs on the same line as the opener.
func replaceOutsideInlineCode(src string, pattern *regexp.Regexp, replace func(string) string) string {
	spans := inlineCodeSpans(src)
	if len(spans) == 0 {
		return pattern.ReplaceAllStringFunc(src, replace)
	}
	var b strings.Builder
	b.Grow(len(src))
	pos := 0
	for _, sp := range spans {
		if pos < sp.start {
			b.WriteString(pattern.ReplaceAllStringFunc(src[pos:sp.start], replace))
		}
		b.WriteString(src[sp.start:sp.end])
		pos = sp.end
	}
	if pos < len(src) {
		b.WriteString(pattern.ReplaceAllStringFunc(src[pos:], replace))
	}
	return b.String()
}

// codeSpanRange is the byte half-open range [start, end) of one inline
// code span in the source.
type codeSpanRange struct {
	start, end int
}

// inlineCodeSpans returns the byte ranges of every inline backtick code
// span in src. CommonMark's rule applies: a span opens with a run of N
// backticks and closes at the first matching run of N backticks before
// the next newline. Unclosed runs are treated as literal text. Backtick
// runs that appear after an already-matched span are scanned fresh.
func inlineCodeSpans(src string) []codeSpanRange {
	if !strings.ContainsRune(src, '`') {
		return nil
	}
	var spans []codeSpanRange
	i := 0
	for i < len(src) {
		if src[i] != '`' {
			i++
			continue
		}
		openStart := i
		for i < len(src) && src[i] == '`' {
			i++
		}
		openLen := i - openStart
		if end, ok := findClosingRun(src, i, openLen); ok {
			spans = append(spans, codeSpanRange{openStart, end})
			i = end
		}
	}
	return spans
}

// findClosingRun returns the byte index just past a run of exactly n
// backticks in src starting from start, searching only up to the next
// newline. Runs of a different length are skipped (they're content
// inside the span). Returns ok=false if no matching run exists before
// EOL.
func findClosingRun(src string, start, n int) (int, bool) {
	for j := start; j < len(src) && src[j] != '\n'; {
		if src[j] != '`' {
			j++
			continue
		}
		closeStart := j
		for j < len(src) && src[j] == '`' {
			j++
		}
		if j-closeStart == n {
			return j, true
		}
	}
	return 0, false
}

// fenceSegment is a chunk of source paired with whether it lies inside
// a fenced code block. Used by preprocessEmbeds to skip embed scanning
// inside fences.
type fenceSegment struct {
	text    string
	isFence bool
}

// splitOutsideFences walks src line-by-line and returns alternating
// segments: false = embed-eligible prose, true = fenced code block
// (including the fence delimiters themselves). Trailing newlines are
// preserved so concatenating segments reproduces src exactly.
//
// Fence semantics (a subset of CommonMark sufficient for our docs):
//   - Opening fence: ≤3 leading spaces, then 3+ backticks OR 3+ tildes.
//   - Closing fence: same marker char, ≥ opening length, ≤3 leading
//     spaces, only optional whitespace after the marker run.
//   - Mismatched marker char or shorter run does NOT close the fence.
func splitOutsideFences(src string) []fenceSegment {
	if !strings.ContainsAny(src, "`~") {
		return []fenceSegment{{text: src, isFence: false}}
	}
	var segs []fenceSegment
	var cur strings.Builder
	inFence := false
	var fenceChar byte
	var fenceLen int

	lines := strings.SplitAfter(src, "\n")
	for _, line := range lines {
		if !inFence {
			if ch, n, ok := openingFence(line); ok {
				if cur.Len() > 0 {
					segs = append(segs, fenceSegment{text: cur.String(), isFence: false})
					cur.Reset()
				}
				inFence = true
				fenceChar = ch
				fenceLen = n
				cur.WriteString(line)
				continue
			}
			cur.WriteString(line)
			continue
		}
		// inside a fence
		cur.WriteString(line)
		if closingFence(line, fenceChar, fenceLen) {
			segs = append(segs, fenceSegment{text: cur.String(), isFence: true})
			cur.Reset()
			inFence = false
		}
	}
	if cur.Len() > 0 {
		segs = append(segs, fenceSegment{text: cur.String(), isFence: inFence})
	}
	return segs
}

// openingFence returns the marker char, run length, and ok=true if line
// is an opening code fence under our subset of CommonMark. Accepts up
// to 3 leading spaces; the run begins with the first non-space char.
func openingFence(line string) (byte, int, bool) {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) {
		return 0, 0, false
	}
	ch := line[i]
	if ch != '`' && ch != '~' {
		return 0, 0, false
	}
	start := i
	for i < len(line) && line[i] == ch {
		i++
	}
	n := i - start
	if n < 3 {
		return 0, 0, false
	}
	return ch, n, true
}

// closingFence reports whether line closes an open fence whose marker
// char is ch and whose opening run length is n. Closing requires ≥n
// markers of the same char, ≤3 leading spaces, and only optional
// whitespace afterward.
func closingFence(line string, ch byte, n int) bool {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) || line[i] != ch {
		return false
	}
	start := i
	for i < len(line) && line[i] == ch {
		i++
	}
	if i-start < n {
		return false
	}
	for i < len(line) {
		c := line[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
		i++
	}
	return true
}

// embedDisplayRange formats em.Range for the provenance header in the
// fence; matches what the user typed inside the brackets.
func embedDisplayRange(em *embed.Embed) string {
	if em.Range == nil {
		return "whole file"
	}
	if em.Range.Start == em.Range.End {
		return strconv.Itoa(em.Range.Start)
	}
	return strconv.Itoa(em.Range.Start) + "–" + strconv.Itoa(em.Range.End)
}

// stripURLSentinels removes urlSuppressStart..urlSuppressEnd ranges
// from raw, along with the single space immediately preceding each
// start sentinel. Used by Render (the plain path) to honor the
// hidden-URL house style without paying for the link-text bookkeeping
// stripSentinels does.
func stripURLSentinels(raw string) string {
	if !strings.ContainsRune(raw, urlSuppressStart) {
		return raw
	}
	var out strings.Builder
	out.Grow(len(raw))
	for i := 0; i < len(raw); {
		c := raw[i]
		if c == urlSuppressStart {
			i = urlSuppressStrip(&out, raw, i)
			continue
		}
		out.WriteByte(c)
		i++
	}
	return out.String()
}
