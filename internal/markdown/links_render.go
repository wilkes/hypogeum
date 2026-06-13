package markdown

import "fmt"

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
//
// The pipeline: preprocessEmbeds and preprocessWikilinks rewrite the
// source (see preprocess.go), the instrumented renderer injects sentinels
// (see style.go), stripSentinels recovers link positions from the ANSI
// output (see sentinel.go), and the visible-text segmentation that the
// preprocessors lean on lives in fences.go.
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
