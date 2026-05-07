package markdown

import (
	"fmt"
	"regexp"
	"strings"
)

// Sentinel runes injected into the rendered output around every link's
// visible text. They survive Glamour's word-wrap pass and get stripped
// out of the returned rendered string. Chosen as ASCII separator
// characters that are extremely unlikely to appear in user content.
const (
	sentinelStart = '\x1c' // FS (file separator)
	sentinelEnd   = '\x1e' // RS (record separator)
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
	cleaned, spans := stripSentinels(raw, marker)
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
		w := parseWikilinkBodyForRender(body)
		if w == nil {
			return match
		}
		display := w.alias
		if display == "" {
			display = w.name
			if w.heading != "" {
				display = w.name + " > " + w.heading
			}
		}
		path, ok := r.resolver.Resolve(r.fromFile, w.name, w.heading, w.block)
		if !ok {
			return display + "?"
		}
		href := path
		if w.heading != "" {
			href = path + "#" + slugify(w.heading)
		}
		return "[" + display + "](" + href + ")"
	})
}

// parsedWikilink mirrors the vault's wikilinkNode without depending on
// it (markdown does not import vault). Names are kept lowercase here
// to make the source-rewrite logic readable.
type parsedWikilink struct {
	name    string
	heading string
	block   string
	alias   string
}

func parseWikilinkBodyForRender(body string) *parsedWikilink {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	w := &parsedWikilink{}
	if i := strings.IndexByte(body, '|'); i >= 0 {
		w.alias = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}
	if i := strings.IndexByte(body, '^'); i >= 0 {
		w.block = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}
	if i := strings.IndexByte(body, '#'); i >= 0 {
		w.heading = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}
	w.name = strings.TrimSpace(body)
	if w.name == "" {
		return nil
	}
	return w
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
// strings it returns, in place of the stripped sentinels.
func stripSentinels(raw string, marker LinkMarker) (string, []sentinelSpan) {
	var (
		out       strings.Builder
		spans     []sentinelSpan
		row       int
		inLink    bool
		linkText  strings.Builder
		linkRow   int
		linkIdx   int
		openMark  string
		closeMark string
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
			out.WriteString(openMark)
			i++
		case sentinelEnd:
			if inLink {
				out.WriteString(closeMark)
				spans = append(spans, sentinelSpan{row: linkRow, text: linkText.String()})
				inLink = false
				linkIdx++
			}
			i++
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
