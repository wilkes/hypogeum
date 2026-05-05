// Package markdown wraps Glamour for rendering and provides utilities for
// resolving links in a markdown file relative to its location on disk.
package markdown

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// Renderer renders markdown to ANSI-styled terminal output.
// It is safe to reuse across files; the underlying Glamour renderer holds
// no per-document state.
type Renderer struct {
	g            *glamour.TermRenderer
	instrumented *glamour.TermRenderer // sentinel-injected style; used by RenderWithLinks
}

// NewRenderer constructs a Renderer with the given output width.
// Both the plain and instrumented renderers go through hypogeumStyle so
// they stay byte-equivalent and so the prose styling can evolve in one
// place.
func NewRenderer(width int) (*Renderer, error) {
	if width < 20 {
		width = 80
	}
	g, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithStyles(hypogeumStyle(width)),
	)
	if err != nil {
		return nil, fmt.Errorf("init glamour: %w", err)
	}

	instrumented, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithStyles(linkInstrumentationStyles(width)),
	)
	if err != nil {
		return nil, fmt.Errorf("init instrumented glamour: %w", err)
	}

	return &Renderer{g: g, instrumented: instrumented}, nil
}

// RenderFile reads and renders the markdown file at path.
func (r *Renderer) RenderFile(path string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return r.Render(string(src))
}

// Render renders a markdown string.
func (r *Renderer) Render(src string) (string, error) {
	out, err := r.g.Render(src)
	if err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return out, nil
}

// LinkKind classifies a markdown link target so the navigation layer can
// decide how to handle it.
type LinkKind int

const (
	// LinkLocalFile is a file on the local filesystem (resolved absolute path).
	LinkLocalFile LinkKind = iota
	// LinkExternal is a URL with an http(s) or other non-file scheme.
	LinkExternal
	// LinkAnchor is a same-document anchor (begins with '#').
	LinkAnchor
	// LinkInvalid means the target could not be classified or resolved.
	LinkInvalid
)

// ResolvedLink describes a link target after resolution against a base file.
type ResolvedLink struct {
	Kind   LinkKind
	Target string // absolute path for LinkLocalFile, raw URL otherwise
	Anchor string // fragment, if any (without leading '#')
}

// ResolveLink interprets the href of a link found inside the file at base.
// It does not check that the target exists; callers handle missing files.
func ResolveLink(base, href string) ResolvedLink {
	href = strings.TrimSpace(href)
	if href == "" {
		return ResolvedLink{Kind: LinkInvalid}
	}

	// Pure fragment: same-document anchor.
	if strings.HasPrefix(href, "#") {
		return ResolvedLink{Kind: LinkAnchor, Anchor: strings.TrimPrefix(href, "#")}
	}

	// Try parsing as URL to detect schemes. Note that bare paths parse
	// successfully with an empty Scheme, so we check that explicitly.
	u, err := url.Parse(href)
	if err == nil && u.Scheme != "" && u.Scheme != "file" {
		return ResolvedLink{Kind: LinkExternal, Target: href}
	}

	// Local path. Strip any fragment for the file path; preserve it separately.
	target := href
	anchor := ""
	if u != nil {
		if u.Path != "" {
			target = u.Path
		}
		anchor = u.Fragment
	}

	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(base), target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return ResolvedLink{Kind: LinkInvalid}
	}
	return ResolvedLink{Kind: LinkLocalFile, Target: abs, Anchor: anchor}
}

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

// hypogeumStyle returns the project's house style: a clone of the
// environment-detected default with heading bars, inline code,
// emphasis, lists, blockquotes, code blocks, and rules layered on top.
// Both the plain and instrumented renderers start from this so they
// remain byte-equivalent after sentinel-strip. width is the renderer's
// wrap width; used to size the horizontal rule.
func hypogeumStyle(width int) ansi.StyleConfig {
	cfg := cloneStyleConfig(defaultStyleConfig())
	applyHypogeumOverrides(&cfg, width)
	return cfg
}

// linkInstrumentationStyles returns hypogeumStyle with sentinel
// block_prefix/block_suffix grafted onto the LinkText primitive. The
// instrumented render is visually identical to the regular render after
// sentinels are stripped.
func linkInstrumentationStyles(width int) ansi.StyleConfig {
	cfg := hypogeumStyle(width)
	cfg.LinkText.BlockPrefix = string(sentinelStart) + cfg.LinkText.BlockPrefix
	cfg.LinkText.BlockSuffix = cfg.LinkText.BlockSuffix + string(sentinelEnd)
	return cfg
}

// applyHypogeumOverrides patches cfg in place with hypogeum's house
// styling. Targets the readability points that the default Glamour dark
// theme leaves flat. width is the renderer wrap width, used to span the
// horizontal rule.
func applyHypogeumOverrides(cfg *ansi.StyleConfig, width int) {
	yes := true // shared *bool for any boolean style toggle (bold, italic, faint, ...)
	bold := &yes
	italic := &yes
	faint := &yes

	// H1: extra vertical breathing room so it reads as a page-break
	// when scrolling between sections. Color/style left to the base
	// theme (its pink-on-purple block is already distinctive).
	cfg.H1.BlockPrefix = "\n\n"
	cfg.H1.BlockSuffix = "\n"

	// H2: bright cyan, vertical bar prefix. The bar gives the eye a
	// clear left-edge anchor for skimming long docs.
	h2 := "117"
	cfg.H2.Color = &h2
	cfg.H2.Bold = bold
	cfg.H2.Prefix = "▌ "
	cfg.H2.BlockPrefix = "\n"
	cfg.H2.BlockSuffix = "\n"

	// H3: a step quieter than H2 — thinner bar, steel-blue.
	h3 := "110"
	cfg.H3.Color = &h3
	cfg.H3.Bold = bold
	cfg.H3.Prefix = "│ "
	cfg.H3.BlockPrefix = "\n"

	// H4: caret marker, softer color, no bold. Mostly used for
	// sub-grouping inside H3 sections.
	h4 := "109"
	cfg.H4.Color = &h4
	cfg.H4.Prefix = "▸ "

	// Inline code: drop the background-color block and the surrounding
	// space pads. Default looks like a button; this looks like
	// emphasized prose. Color stays warm so it's still distinct from
	// regular text.
	codeColor := "173"
	cfg.Code.Color = &codeColor
	cfg.Code.BackgroundColor = nil
	cfg.Code.Prefix = ""
	cfg.Code.Suffix = ""

	// Strong: gold + bold. Default was bold-only, which is barely
	// distinguishable from regular text in many terminals. Strip the
	// markdown-source ** markers — once the span is colored the markers
	// are noise.
	strong := "222"
	cfg.Strong.Color = &strong
	cfg.Strong.Bold = bold
	cfg.Strong.BlockPrefix = ""
	cfg.Strong.BlockSuffix = ""

	// Emph: soft cyan + italic. Same reasoning — italic alone
	// disappears. Strip the * markers for the same reason as Strong.
	emph := "117"
	cfg.Emph.Color = &emph
	cfg.Emph.Italic = italic
	cfg.Emph.BlockPrefix = ""
	cfg.Emph.BlockSuffix = ""

	// Horizontal rule: a real spanning line, dim. Replaces the literal
	// "--------" with a row of box-drawing characters sized to the
	// renderer's wrap width. Reads as <hr>, not as ASCII art.
	hrColor := "240"
	cfg.HorizontalRule.Color = &hrColor
	cfg.HorizontalRule.Format = "\n" + strings.Repeat("─", max(width-4, 8)) + "\n"

	// List bullet: bright cyan accent. Each top-level bullet is now
	// visually distinct from prose, like a styled <ul> marker.
	bulletColor := "117"
	cfg.Item.BlockPrefix = "• "
	cfg.Item.Color = &bulletColor

	// Task list: replace ASCII brackets with proper checkbox glyphs.
	cfg.Task.Ticked = "☑ "
	cfg.Task.Unticked = "☐ "

	// Blockquote: lavender bar + faint quoted text. Default has the bar
	// but no color, so quotes blend into prose. Now they read as
	// quoted/lifted-out content.
	bqColor := "141"
	cfg.BlockQuote.Color = &bqColor
	cfg.BlockQuote.Faint = faint
	bqIndent := "│ "
	cfg.BlockQuote.IndentToken = &bqIndent

	// Code block: dim-near-black background so they look like <pre>
	// cards instead of loose indentation. Chroma syntax highlighting
	// inside is unaffected — we're only painting the frame.
	cbBg := "235"
	cfg.CodeBlock.BackgroundColor = &cbBg
}

// defaultStyleConfig mirrors Glamour's WithAutoStyle resolution: pick
// NoTTY when stdout isn't a terminal, otherwise dark or light by
// background detection.
func defaultStyleConfig() *ansi.StyleConfig {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return &styles.NoTTYStyleConfig
	}
	if termenv.HasDarkBackground() {
		return &styles.DarkStyleConfig
	}
	return &styles.LightStyleConfig
}

// cloneStyleConfig returns a deep copy of cfg by JSON round-trip.
// We never mutate the package-level styles.* configs, so deep copy is
// a correctness requirement, not a nicety.
func cloneStyleConfig(cfg *ansi.StyleConfig) ansi.StyleConfig {
	b, err := json.Marshal(cfg)
	if err != nil {
		// StyleConfig only contains JSON-serializable fields, so this
		// branch is effectively unreachable. Fall back to a shallow copy
		// rather than panicking.
		return *cfg
	}
	var out ansi.StyleConfig
	if err := json.Unmarshal(b, &out); err != nil {
		return *cfg
	}
	return out
}
