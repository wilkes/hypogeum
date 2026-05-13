package code

import (
	"strconv"
	"strings"
)

// addGutter prepends a faint right-aligned line-number gutter to each
// line of formatted. The gutter width is fixed for the whole file —
// derived from the total line count so all numbers align — followed by
// a single-space separator column.
//
// formatted is the Chroma terminal256 output; each source line ends
// with "\n". A trailing newline (if any) is preserved.
func addGutter(formatted string) string {
	// Count source lines: number of '\n' plus 1 if the buffer doesn't
	// end with a newline. Using strings.Count avoids allocating a slice
	// just to measure.
	total := strings.Count(formatted, "\n")
	if !strings.HasSuffix(formatted, "\n") {
		total++
	}
	if total == 0 {
		return ""
	}

	gutterWidth := len(strconv.Itoa(total))
	var b strings.Builder
	b.Grow(len(formatted) + total*(gutterWidth+3))

	lineNum := 1
	start := 0
	for i := 0; i <= len(formatted); i++ {
		if i == len(formatted) || formatted[i] == '\n' {
			b.WriteString(formatLineNumber(lineNum, gutterWidth))
			b.WriteByte(' ')
			b.WriteString(formatted[start:i])
			if i < len(formatted) {
				b.WriteByte('\n')
			}
			lineNum++
			start = i + 1
		}
	}
	return b.String()
}

// formatLineNumber right-aligns n in a field of width w, wrapped in a
// dim SGR sequence and reset. The reset is critical — without it the
// dim attribute would bleed into the source-line tokens that follow.
func formatLineNumber(n, w int) string {
	s := strconv.Itoa(n)
	pad := w - len(s)
	if pad < 0 {
		pad = 0
	}
	return "\x1b[2m" + strings.Repeat(" ", pad) + s + "\x1b[0m"
}

// blankGutter is formatLineNumber for continuation rows. Same width and
// trailing space as a numbered gutter so columns align; no SGR attribute
// applied so no color can leak into the column.
func blankGutter(w int) string {
	return strings.Repeat(" ", w) + " "
}
