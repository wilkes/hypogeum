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

// RenderResult is a completed render plus the sentinel-instrumented Glamour
// output, so the highlighted link can be changed without re-running Glamour.
// raw is unexported: only WithHighlight (same package) re-strips it.
type RenderResult struct {
	Content   string   // rendered output with the marker passed to RenderDocument applied
	Links     []Link   // every followable link, document order
	EmbedDeps []string // absolute source paths sliced in by embeds
	raw       string   // Glamour output with sentinels intact — input to re-highlight
}

// WithHighlight re-derives the visible output with only link `selected`
// reverse-videoed (selected = -1 highlights nothing). Cheap: a single
// stripSentinels pass over raw, with no Glamour render.
func (rr *RenderResult) WithHighlight(selected int) string {
	cleaned, _ := stripSentinels(rr.raw, HighlightMarker(selected))
	return cleaned
}

// RenderDocument renders src and returns a reusable RenderResult. base is the
// path of the file the source came from; it resolves relative link targets.
// See RenderWithLinks for the marker semantics. The pipeline: preprocessEmbeds
// and preprocessWikilinks rewrite the source, the instrumented renderer injects
// sentinels, stripSentinels recovers link positions from the ANSI output.
func (r *Renderer) RenderDocument(src, base string, marker LinkMarker) (*RenderResult, error) {
	src, embedDeps, embedLinks := r.preprocessEmbeds(src, base)
	src = r.preprocessWikilinks(src)
	raw, err := r.instrumented.Render(src)
	if err != nil {
		return nil, fmt.Errorf("render markdown: %w", err)
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

	return &RenderResult{
		Content:   cleaned,
		Links:     links,
		EmbedDeps: embedDeps,
		raw:       raw,
	}, nil
}

// RenderWithLinks renders src and returns the rendered string, the links, and
// the embed dependency paths. It is a thin wrapper over RenderDocument kept for
// callers that don't need the reusable handle.
//
// If marker is non-nil, the open/close strings it returns for each link are
// spliced around that link's visible text. They flow through downstream styling
// without changing visible width (caller's responsibility — typically
// zero-width sentinel sequences).
func (r *Renderer) RenderWithLinks(src, base string, marker LinkMarker) (string, []Link, []string, error) {
	rr, err := r.RenderDocument(src, base, marker)
	if err != nil {
		return "", nil, nil, err
	}
	return rr.Content, rr.Links, rr.EmbedDeps, nil
}
