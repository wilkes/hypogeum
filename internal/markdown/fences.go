package markdown

import (
	"regexp"
	"strings"
)

// replaceOutsideInlineCode applies pattern.ReplaceAllStringFunc(src, replace)
// to every region of src that is NOT inside an inline backtick code span.
// Code spans pass through verbatim so wikilink/embed demos written as
// `[[Name]]` or `![[file]]` render as code instead of being rewritten.
//
// Multi-line spans are out of scope: the wikilink and embed regexes both
// require their tokens on a single line, and our docs don't use
// multi-line backtick spans containing wikilink syntax. inlineCodeSpans
// only matches closing runs on the same line as the opener.
func replaceOutsideInlineCode(src string, pattern *regexp.Regexp, replace func(string) string) string {
	spans := inlineCodeSpans(src)
	if len(spans) == 0 {
		return pattern.ReplaceAllStringFunc(src, replace)
	}
	var b strings.Builder
	b.Grow(len(src))
	pos := 0
	for _, sp := range spans {
		if pos < sp.start {
			b.WriteString(pattern.ReplaceAllStringFunc(src[pos:sp.start], replace))
		}
		b.WriteString(src[sp.start:sp.end])
		pos = sp.end
	}
	if pos < len(src) {
		b.WriteString(pattern.ReplaceAllStringFunc(src[pos:], replace))
	}
	return b.String()
}

// codeSpanRange is the byte half-open range [start, end) of one inline
// code span in the source.
type codeSpanRange struct {
	start, end int
}

// inlineCodeSpans returns the byte ranges of every inline backtick code
// span in src. CommonMark's rule applies: a span opens with a run of N
// backticks and closes at the first matching run of N backticks before
// the next newline. Unclosed runs are treated as literal text. Backtick
// runs that appear after an already-matched span are scanned fresh.
func inlineCodeSpans(src string) []codeSpanRange {
	if !strings.ContainsRune(src, '`') {
		return nil
	}
	var spans []codeSpanRange
	i := 0
	for i < len(src) {
		if src[i] != '`' {
			i++
			continue
		}
		openStart := i
		for i < len(src) && src[i] == '`' {
			i++
		}
		openLen := i - openStart
		if end, ok := findClosingRun(src, i, openLen); ok {
			spans = append(spans, codeSpanRange{openStart, end})
			i = end
		}
	}
	return spans
}

// findClosingRun returns the byte index just past a run of exactly n
// backticks in src starting from start, searching only up to the next
// newline. Runs of a different length are skipped (they're content
// inside the span). Returns ok=false if no matching run exists before
// EOL.
func findClosingRun(src string, start, n int) (int, bool) {
	for j := start; j < len(src) && src[j] != '\n'; {
		if src[j] != '`' {
			j++
			continue
		}
		closeStart := j
		for j < len(src) && src[j] == '`' {
			j++
		}
		if j-closeStart == n {
			return j, true
		}
	}
	return 0, false
}

// fenceSegment is a chunk of source paired with whether it lies inside
// a fenced code block. Used by preprocessEmbeds to skip embed scanning
// inside fences.
type fenceSegment struct {
	text    string
	isFence bool
}

// splitOutsideFences walks src line-by-line and returns alternating
// segments: false = embed-eligible prose, true = fenced code block
// (including the fence delimiters themselves). Trailing newlines are
// preserved so concatenating segments reproduces src exactly.
//
// Fence semantics (a subset of CommonMark sufficient for our docs):
//   - Opening fence: ≤3 leading spaces, then 3+ backticks OR 3+ tildes.
//   - Closing fence: same marker char, ≥ opening length, ≤3 leading
//     spaces, only optional whitespace after the marker run.
//   - Mismatched marker char or shorter run does NOT close the fence.
func splitOutsideFences(src string) []fenceSegment {
	if !strings.ContainsAny(src, "`~") {
		return []fenceSegment{{text: src, isFence: false}}
	}
	var segs []fenceSegment
	var cur strings.Builder
	inFence := false
	var fenceChar byte
	var fenceLen int

	lines := strings.SplitAfter(src, "\n")
	for _, line := range lines {
		if !inFence {
			if ch, n, ok := openingFence(line); ok {
				if cur.Len() > 0 {
					segs = append(segs, fenceSegment{text: cur.String(), isFence: false})
					cur.Reset()
				}
				inFence = true
				fenceChar = ch
				fenceLen = n
				cur.WriteString(line)
				continue
			}
			cur.WriteString(line)
			continue
		}
		// inside a fence
		cur.WriteString(line)
		if closingFence(line, fenceChar, fenceLen) {
			segs = append(segs, fenceSegment{text: cur.String(), isFence: true})
			cur.Reset()
			inFence = false
		}
	}
	if cur.Len() > 0 {
		segs = append(segs, fenceSegment{text: cur.String(), isFence: inFence})
	}
	return segs
}

// openingFence returns the marker char, run length, and ok=true if line
// is an opening code fence under our subset of CommonMark. Accepts up
// to 3 leading spaces; the run begins with the first non-space char.
func openingFence(line string) (byte, int, bool) {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) {
		return 0, 0, false
	}
	ch := line[i]
	if ch != '`' && ch != '~' {
		return 0, 0, false
	}
	start := i
	for i < len(line) && line[i] == ch {
		i++
	}
	n := i - start
	if n < 3 {
		return 0, 0, false
	}
	return ch, n, true
}

// closingFence reports whether line closes an open fence whose marker
// char is ch and whose opening run length is n. Closing requires ≥n
// markers of the same char, ≤3 leading spaces, and only optional
// whitespace afterward.
func closingFence(line string, ch byte, n int) bool {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) || line[i] != ch {
		return false
	}
	start := i
	for i < len(line) && line[i] == ch {
		i++
	}
	if i-start < n {
		return false
	}
	for i < len(line) {
		c := line[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
		i++
	}
	return true
}
