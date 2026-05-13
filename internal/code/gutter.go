package code

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/wilkes/hypogeum/internal/markdown"
)

// addGutter prepends a faint right-aligned line-number gutter to each
// source line of formatted, soft-wrapping rows longer than contentWidth.
// Continuation rows get a blank (uncolored) gutter so per-source-line
// numbering stays one-per-source-line.
//
// formatted is the Chroma terminal256 output; each source line ends
// with "\n". contentWidth is the total renderer width (gutter + body).
//
// Chroma emits a final SGR reset *after* the last newline, so we
// measure trailing-newline-ness against the SGR-stripped tail to
// avoid an off-by-one line count.
func addGutter(formatted string, contentWidth int, highlight *markdown.LineRange) string {
	if formatted == "" {
		return ""
	}
	total := strings.Count(formatted, "\n")
	if !endsWithNewline(formatted) {
		total++
	}
	if total == 0 {
		return ""
	}

	gutterWidth := len(strconv.Itoa(total))
	bodyWidth := contentWidth - gutterWidth - 1 // -1 for the separator space
	if bodyWidth < 1 {
		bodyWidth = 1
	}

	var b strings.Builder
	b.Grow(len(formatted) + total*(gutterWidth+9))

	lineNum := 1
	start := 0
	emitted := 0
	emit := func(row string) {
		// Skip the trailing SGR-only tail Chroma emits after the
		// final newline — it has no source content.
		if emitted == total && stripSGR(row) == "" {
			return
		}
		wrapped := ansi.Wrap(row, bodyWidth, "")
		rows := strings.Split(wrapped, "\n")
		// Carry SGR state across wrap boundaries. ansi.Wrap won't split
		// inside an escape sequence but it also doesn't synthesize a
		// reset/restore at the seam, so a long colored token would
		// otherwise render its continuation rows in terminal default.
		carry := ""
		for i, sub := range rows {
			if i == 0 {
				b.WriteString(formatLineNumberFor(lineNum, gutterWidth, highlight))
			} else {
				b.WriteString(blankGutterFor(gutterWidth, lineNum, highlight))
			}
			if i > 0 && carry != "" {
				b.WriteString(carry)
			}
			b.WriteString(sub)
			// Reset at row end so the gutter on the next row never
			// inherits whatever color was active.
			if i < len(rows)-1 {
				b.WriteString("\x1b[0m")
			}
			b.WriteByte('\n')
			carry = lastUnclosedSGR(sub, carry)
		}
		lineNum++
		emitted++
	}

	for i := 0; i <= len(formatted); i++ {
		if i == len(formatted) || formatted[i] == '\n' {
			emit(formatted[start:i])
			start = i + 1
		}
	}

	// Drop the trailing newline we always emit so callers can append
	// their own framing without worrying about a duplicate.
	out := b.String()
	if strings.HasSuffix(out, "\n") {
		out = out[:len(out)-1]
	}
	return out
}

// endsWithNewline reports whether formatted's last non-SGR byte is '\n'.
// Chroma's terminal256 formatter appends an SGR reset after the trailing
// newline, so a naive HasSuffix check misclassifies real-trailing-newline
// inputs as "no trailing newline" and yields an off-by-one line count.
func endsWithNewline(formatted string) bool {
	// Walk backwards skipping any complete "\x1b[...m" sequences.
	i := len(formatted)
	for i > 0 {
		// A trailing SGR sequence ends in 'm'. Scan back to find its '\x1b['.
		if formatted[i-1] != 'm' {
			break
		}
		j := i - 2
		for j >= 0 && formatted[j] != '\x1b' {
			j--
		}
		if j < 0 || j+1 >= len(formatted) || formatted[j+1] != '[' {
			break
		}
		i = j
	}
	return i > 0 && formatted[i-1] == '\n'
}

// lastUnclosedSGR returns the SGR sequence that's still in effect at
// the end of sub, given that prev was the carry at the start. An
// "\x1b[0m" reset clears the carry; any other SGR replaces it. Used to
// restore color state at the start of wrapped continuation rows so
// long colored tokens don't lose their color mid-line.
func lastUnclosedSGR(sub, prev string) string {
	carry := prev
	for i := 0; i < len(sub); {
		if sub[i] != '\x1b' || i+1 >= len(sub) || sub[i+1] != '[' {
			i++
			continue
		}
		j := i + 2
		for j < len(sub) && sub[j] != 'm' {
			j++
		}
		if j >= len(sub) {
			break
		}
		seq := sub[i : j+1]
		if seq == "\x1b[0m" {
			carry = ""
		} else {
			carry = seq
		}
		i = j + 1
	}
	return carry
}

// stripSGR removes ANSI SGR sequences from s. Used to detect rows whose
// only content is style bytes (Chroma's trailing reset, for example).
func stripSGR(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// formatLineNumber right-aligns n in a field of width w, wrapped in a
// dim SGR sequence and reset, with a trailing separator space. The
// reset is critical — without it the dim attribute would bleed into
// the source-line tokens that follow.
func formatLineNumber(n, w int) string {
	return formatLineNumberFor(n, w, nil)
}

// blankGutter is formatLineNumber for continuation rows. Same width
// (w padding + 1 separator) as a numbered gutter so columns align;
// no SGR attribute applied so no color can leak into the column.
func blankGutter(w int) string {
	return blankGutterFor(w, 0, nil)
}

// formatLineNumberFor is formatLineNumber with an optional highlight
// range. When n falls inside hi, the gutter cell renders in
// reverse-video instead of dim so the eye can find the referenced
// span. The reverse attribute is closed with \x1b[27m (not the full
// 0m reset) so any SGR state in the source body that follows is
// preserved.
func formatLineNumberFor(n, w int, hi *markdown.LineRange) string {
	s := strconv.Itoa(n)
	pad := w - len(s)
	if pad < 0 {
		pad = 0
	}
	if inRange(n, hi) {
		// Reverse-video the whole gutter cell (padding + number + separator
		// space) so the band reads as a continuous bar. Reset at the end
		// so the source body that follows is not reverse-video.
		return "\x1b[7m" + strings.Repeat(" ", pad) + s + " \x1b[27m"
	}
	return "\x1b[2m" + strings.Repeat(" ", pad) + s + "\x1b[0m "
}

// blankGutterFor is blankGutter with an optional highlight range. When
// sourceLine falls inside hi, the blank gutter cell on a continuation
// row also renders in reverse-video so a wrapped highlighted line
// keeps its band across its continuation rows.
func blankGutterFor(w, sourceLine int, hi *markdown.LineRange) string {
	if inRange(sourceLine, hi) {
		return "\x1b[7m" + strings.Repeat(" ", w+1) + "\x1b[27m"
	}
	return strings.Repeat(" ", w+1)
}

// inRange reports whether n is within hi's inclusive [Start, End].
// A nil hi means no range is set, so nothing is in range.
func inRange(n int, hi *markdown.LineRange) bool {
	if hi == nil {
		return false
	}
	return n >= hi.Start && n <= hi.End
}
