package markdown

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

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
// styling. Philosophy: only color what the user clicks. Prose elements
// get weight, glyph, or whitespace differentiation — never color
// rotation. Headings keep restrained color because they're navigation
// targets; links keep color because they're action targets; everything
// else inherits the document's body color so prose reads as prose.
// width is the renderer wrap width, used to span the horizontal rule.
func applyHypogeumOverrides(cfg *ansi.StyleConfig, width int) {
	yes := true
	bold := &yes
	italic := &yes

	// Body text: light gray. Most everything inherits this implicitly.
	// Setting it on Document makes the inheritance explicit.
	body := "252"
	cfg.Document.Color = &body

	// H1: extra vertical breathing room. Color/style left to the base
	// theme (its pink-on-purple block is already distinctive).
	cfg.H1.BlockPrefix = "\n\n"
	cfg.H1.BlockSuffix = "\n"

	// H2/H3/H4: restrained blue ramp. Same hue family, decreasing
	// saturation as we go deeper — gives navigation hierarchy without
	// shouting. Bars stay because they're *structural* (left-edge
	// anchors for the eye), not decorative.
	h2 := "75" // muted sky blue
	cfg.H2.Color = &h2
	cfg.H2.Bold = bold
	cfg.H2.Prefix = "▌ "
	cfg.H2.BlockPrefix = "\n"
	cfg.H2.BlockSuffix = "\n"

	h3 := "67" // dimmer steel blue
	cfg.H3.Color = &h3
	cfg.H3.Bold = bold
	cfg.H3.Prefix = "│ "
	cfg.H3.BlockPrefix = "\n"

	h4 := "66" // dimmer still
	cfg.H4.Color = &h4
	cfg.H4.Prefix = "▸ "

	// Inline code: barely-different. Same body color, just slightly
	// faint and with single-space pads so they read as inline tokens
	// without becoming polka dots across the page. The eye learns to
	// recognize the rhythm without being pulled to it.
	codeColor := "250"
	cfg.Code.Color = &codeColor
	cfg.Code.BackgroundColor = nil
	cfg.Code.Prefix = ""
	cfg.Code.Suffix = ""

	// Strong: pure bold, no color. Reads as emphatic prose, not a
	// special token. Strip the ** markers since bold alone carries the
	// signal in ANSI terminals.
	cfg.Strong.Color = &body
	cfg.Strong.Bold = bold
	cfg.Strong.BlockPrefix = ""
	cfg.Strong.BlockSuffix = ""

	// Emph: italic only, body color, * markers stripped.
	cfg.Emph.Color = &body
	cfg.Emph.Italic = italic
	cfg.Emph.BlockPrefix = ""
	cfg.Emph.BlockSuffix = ""

	// Horizontal rule: a real spanning line, dim. Replaces the literal
	// "--------" with a row of box-drawing characters sized to the
	// renderer's wrap width.
	hrColor := "238"
	cfg.HorizontalRule.Color = &hrColor
	cfg.HorizontalRule.Format = "\n" + strings.Repeat("─", max(width-4, 8)) + "\n"

	// List bullet: body color. The • glyph alone is enough marker; a
	// brighter color would compete with the inline-code rhythm and the
	// heading color for attention.
	cfg.Item.BlockPrefix = "• "
	cfg.Item.Color = &body

	// Task list: proper checkbox glyphs.
	cfg.Task.Ticked = "☑ "
	cfg.Task.Unticked = "☐ "

	// Blockquote: keep the structural left bar, drop the text color +
	// faint. The bar tells you it's a quote; the text reads as prose.
	bqColor := "240" // dim gray, just for the bar — text inherits body
	cfg.BlockQuote.Color = &bqColor
	bqIndent := "│ "
	cfg.BlockQuote.IndentToken = &bqIndent

	// Code block: dim-near-black background card. This one element gets
	// to be visually distinct because code blocks ARE structurally
	// different from prose — they're transcluded artifacts.
	cbBg := "235"
	cfg.CodeBlock.BackgroundColor = &cbBg

	// Link: kept loud (color + underline) because links are clickable
	// targets — the one element we WANT the eye to find. Already set
	// by the base theme; no override needed.
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
