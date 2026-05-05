package markdown

import (
	"fmt"
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

// RenderWithLinks renders src and returns both the rendered string and a
// list of every followable link in document order. base is the path of
// the file the source came from; it's used to resolve relative link
// targets to absolute paths.
func (r *Renderer) RenderWithLinks(src, base string) (string, []Link, error) {
	raw, err := r.instrumented.Render(src)
	if err != nil {
		return "", nil, fmt.Errorf("render markdown: %w", err)
	}

	asts := ExtractLinks(src)
	cleaned, spans := stripSentinels(raw)
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

// sentinelSpan records where a sentinel-bracketed link landed in the
// cleaned (sentinel-free) output.
type sentinelSpan struct {
	row  int    // zero-indexed line number in cleaned output
	text string // visible text inside the sentinel pair, ANSI stripped
}

// stripSentinels removes every sentinel byte from raw and returns the
// cleaned string plus a list of (row, visible-text) spans, one per link.
func stripSentinels(raw string) (string, []sentinelSpan) {
	var (
		out      strings.Builder
		spans    []sentinelSpan
		row      int
		inLink   bool
		linkText strings.Builder
		linkRow  int
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
			i++
		case sentinelEnd:
			if inLink {
				spans = append(spans, sentinelSpan{row: linkRow, text: linkText.String()})
				inLink = false
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
