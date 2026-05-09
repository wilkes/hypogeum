package markdown

import (
	"fmt"
	"regexp"
	"strings"

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

// LinkMarker brackets a link's visible text in the rendered output.
// The TUI uses this to inject BubbleZone Mark/Close pairs (for click
// hit-testing) without coupling the markdown package to a specific
// zone library; the markdown package itself layers OSC 8 hyperlink
// sequences around the marker output so terminals that support it
// can click-to-open without the URL appearing in prose.
//
// linkIndex matches ASTLinks[linkIndex] for the same render — the
// caller can resolve href back to the original link if needed.
type LinkMarker func(linkIndex int) (open, close string)

// RenderWithLinks renders src and returns both the rendered string and a
// list of every followable link in document order. base is the path of
// the file the source came from; it's used to resolve relative link
// targets to absolute paths.
//
// If marker is non-nil, the open/close strings it returns for each link
// are spliced around that link's visible text in the rendered output.
// They flow through downstream styling without changing visible width
// (caller's responsibility — typically zero-width sentinel sequences).
func (r *Renderer) RenderWithLinks(src, base string, marker LinkMarker) (string, []Link, error) {
	src = r.preprocessWikilinks(src)
	raw, err := r.instrumented.Render(src)
	if err != nil {
		return "", nil, fmt.Errorf("render markdown: %w", err)
	}

	asts := ExtractLinks(src)
	osc8 := func(i int) string {
		if i < 0 || i >= len(asts) {
			return ""
		}
		return resolveOSC8Target(base, asts[i].Href)
	}
	cleaned, spans := stripSentinelsWithOSC8(raw, marker, osc8)
	links := make([]Link, 0, len(spans))
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
	return cleaned, links, nil
}

// resolveOSC8Target picks the hyperlink target embedded in an OSC 8
// sequence. Local files become file:// URLs (so click handlers in
// modern terminals open them); externals pass through; anchors are
// dropped (no target to click).
func resolveOSC8Target(base, href string) string {
	r := ResolveLink(base, href)
	switch r.Kind {
	case LinkLocalFile:
		return "file://" + r.Target
	case LinkExternal:
		return r.Target
	default:
		return ""
	}
}

// wikilinkRegex matches the wikilink syntax for the source-rewrite pass.
// We use a regex rather than goldmark for the rewrite because goldmark's
// AST → source round-trip is lossy; rewriting strings preserves
// everything else about the source unchanged.
var wikilinkRegex = regexp.MustCompile(`\[\[([^\]\n]+)\]\]`)

// preprocessWikilinks rewrites [[...]] occurrences in src into either
// standard markdown links (resolved) or styled placeholder text
// (unresolved). The resulting string is then handed to Glamour as
// normal markdown.
func (r *Renderer) preprocessWikilinks(src string) string {
	if r.resolver == nil {
		return src
	}
	return wikilinkRegex.ReplaceAllStringFunc(src, func(match string) string {
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
	})
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
	return stripSentinelsWithOSC8(raw, marker, nil)
}

// stripSentinelsWithOSC8 is stripSentinels with an additional hook that
// wraps each link's text in an OSC 8 hyperlink. osc8Target(i) returns
// the URL for the i-th link (empty string skips OSC 8 wrapping for that
// link). The OSC 8 wrappers go *outside* the marker pair so terminal
// hit-testing (BubbleZone Mark) and click-to-open (OSC 8) compose
// rather than conflict.
func stripSentinelsWithOSC8(raw string, marker LinkMarker, osc8Target func(int) string) (string, []sentinelSpan) {
	var (
		out         strings.Builder
		spans       []sentinelSpan
		row         int
		inLink      bool
		linkText    strings.Builder
		linkRow     int
		linkIdx     int
		openMark    string
		closeMark   string
		currentOSC8 string
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
			linkText.Reset()
			linkRow = row
			openMark, closeMark = "", ""
			if marker != nil {
				openMark, closeMark = marker(linkIdx)
			}
			currentOSC8 = ""
			if osc8Target != nil {
				currentOSC8 = osc8Target(linkIdx)
			}
			if currentOSC8 != "" {
				out.WriteString("\x1b]8;;" + currentOSC8 + "\x1b\\")
			}
			out.WriteString(openMark)
			i++
		case sentinelEnd:
			if inLink {
				out.WriteString(closeMark)
				if currentOSC8 != "" {
					out.WriteString("\x1b]8;;\x1b\\")
				}
				spans = append(spans, sentinelSpan{row: linkRow, text: linkText.String()})
				inLink = false
				linkIdx++
			}
			i++
		case urlSuppressStart:
			// Drop everything until urlSuppressEnd, including any ANSI
			// styling Glamour applied to the URL. Also peel back the
			// single space immediately before this sentinel so the
			// "[text] /url" form collapses cleanly. Never strip a
			// newline — that would join paragraphs.
			trimTrailingSpace(&out)
			j := i + 1
			for j < len(raw) && raw[j] != urlSuppressEnd {
				j++
			}
			if j < len(raw) {
				j++ // consume the urlSuppressEnd byte too
			}
			i = j
		case '\n':
			row++
			out.WriteByte(c)
			if inLink {
				linkText.WriteByte(c)
			}
			i++
		default:
			out.WriteByte(c)
			if inLink {
				linkText.WriteByte(c)
			}
			i++
		}
	}
	return out.String(), spans
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
			trimTrailingSpace(&out)
			j := i + 1
			for j < len(raw) && raw[j] != urlSuppressEnd {
				j++
			}
			if j < len(raw) {
				j++
			}
			i = j
			continue
		}
		out.WriteByte(c)
		i++
	}
	return out.String()
}
