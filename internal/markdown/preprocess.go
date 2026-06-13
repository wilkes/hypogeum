package markdown

import (
	"errors"
	"strconv"
	"strings"

	"regexp"

	"github.com/wilkes/hypogeum/internal/embed"
	"github.com/wilkes/hypogeum/internal/pathutil"
	"github.com/wilkes/hypogeum/internal/wikilink"
)

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

		absPath, _ := pathutil.ResolveRelativeTo(base, em.Path)

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
