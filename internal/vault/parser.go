package vault

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// wikilinkNode is the AST type produced by the wikilink inline parser.
// It carries the raw components of a [[Name#Heading^Block|Alias]]
// syntax as parsed; resolution happens later in the renderer/indexer.
type wikilinkNode struct {
	ast.BaseInline
	Name    string
	Heading string
	Block   string
	Alias   string
}

var kindWikilink = ast.NewNodeKind("Wikilink")

func (w *wikilinkNode) Kind() ast.NodeKind { return kindWikilink }
func (w *wikilinkNode) Dump(source []byte, level int) {
	ast.DumpHelper(w, source, level, map[string]string{
		"Name":    w.Name,
		"Heading": w.Heading,
		"Block":   w.Block,
		"Alias":   w.Alias,
	}, nil)
}

// wikilinkParser is a goldmark inline parser that triggers on '[' and
// matches the full [[...]] form. Goldmark's standard link parser also
// triggers on '[' but only for a single bracket; by registering this
// parser at a higher priority, ours runs first and consumes [[ ... ]]
// before the standard parser can mistake it for two adjacent links.
type wikilinkParser struct{}

// Trigger returns the bytes that activate this parser.
func (wikilinkParser) Trigger() []byte { return []byte{'['} }

// Parse implements the inline parser interface. It returns nil if the
// input at the current position isn't a [[...]] — leaving the standard
// link parser free to handle a normal [link].
func (wikilinkParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, segment := block.PeekLine()
	if len(line) < 4 || line[0] != '[' || line[1] != '[' {
		return nil
	}
	// Find closing ]]. Don't cross newlines — wikilinks are inline only.
	end := -1
	for i := 2; i+1 < len(line); i++ {
		if line[i] == '\n' {
			break
		}
		if line[i] == ']' && line[i+1] == ']' {
			end = i
			break
		}
	}
	if end < 0 {
		return nil
	}

	body := string(line[2:end])
	w := parseWikilinkBody(body)
	if w == nil {
		return nil
	}

	// Advance the reader past the closing ]].
	block.Advance(end + 2)
	_ = segment
	return w
}

// parseWikilinkBody splits the inside of [[...]] into its components.
// Returns nil if the body is empty or otherwise malformed.
func parseWikilinkBody(body string) *wikilinkNode {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}

	w := &wikilinkNode{}

	// Pipe splits "name-with-extras|alias" first.
	if i := strings.Index(body, "|"); i >= 0 {
		w.Alias = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}

	// "^block" is allowed after the name, with or without a heading.
	if i := strings.Index(body, "^"); i >= 0 {
		w.Block = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}

	// "#heading" is everything between name and ^/| boundaries.
	if i := strings.Index(body, "#"); i >= 0 {
		w.Heading = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}

	w.Name = strings.TrimSpace(body)
	if w.Name == "" {
		return nil
	}
	return w
}

// wikilinkExt registers wikilinkParser with the parser at a high priority.
// The priority value is chosen to run *before* goldmark's built-in link
// parser (priority 200) so [[...]] is consumed before [link].
type wikilinkExt struct{}

func (wikilinkExt) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(wikilinkParser{}, 102),
	))
}

// WikilinkExtension is the goldmark.Extender that adds [[wikilink]]
// support to a goldmark instance.
var WikilinkExtension = wikilinkExt{}
