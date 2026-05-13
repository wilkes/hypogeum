// Package embed turns ![[file.go#L10-L20+3]] source-embed tokens into
// renderable code-fence strings. It is pure: it does not know about
// Glamour, Bubble Tea, or the TUI. The markdown package's render
// pipeline calls into here to preprocess embeds before Glamour sees
// the source.
package embed

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// LineRange is an inclusive [Start, End] pair of 1-indexed source-file
// line numbers. End == Start represents a single line.
type LineRange struct {
	Start, End int
}

// Embed is the parsed body of a ![[…]] token.
type Embed struct {
	Path         string     // relative or absolute path as written
	Range        *LineRange // nil = whole file
	ContextLines int        // 0 unless +<c> suffix present
}

// lineSpec matches L<n> or L<n>-L<n> (the inner fragment after '#').
var lineSpec = regexp.MustCompile(`^L(\d+)(?:-L(\d+))?(?:\+(\d+))?$`)

// ParseEmbedToken parses the contents *between* the brackets of an
// embed token. Returns an error for any malformed body; the caller
// (markdown.preprocessEmbeds) renders these as a warning blockquote.
func ParseEmbedToken(body string) (*Embed, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("empty embed token")
	}
	hash := strings.IndexByte(body, '#')
	if hash < 0 {
		return &Embed{Path: body}, nil
	}
	path := strings.TrimSpace(body[:hash])
	frag := strings.TrimSpace(body[hash+1:])
	if path == "" {
		return nil, errors.New("empty path")
	}
	m := lineSpec.FindStringSubmatch(frag)
	if m == nil {
		return nil, errors.New("invalid line spec: " + frag)
	}
	start, _ := strconv.Atoi(m[1])
	if start < 1 {
		return nil, errors.New("line numbers are 1-indexed")
	}
	end := start
	if m[2] != "" {
		end, _ = strconv.Atoi(m[2])
		if end < start {
			return nil, errors.New("inverted range")
		}
	}
	ctx := 0
	if m[3] != "" {
		ctx, _ = strconv.Atoi(m[3])
	}
	return &Embed{Path: path, Range: &LineRange{Start: start, End: end}, ContextLines: ctx}, nil
}
