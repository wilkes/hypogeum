package code

import (
	"strconv"
	"strings"
)

// addGutter prepends a faint right-aligned line-number gutter to each
// source line of formatted. The gutter width is fixed for the whole
// file — derived from the total line count so all numbers align —
// followed by a single-space separator column.
//
// formatted is the Chroma terminal256 output. Chroma emits a final
// SGR reset *after* the last newline, so a raw HasSuffix("\n") check
// would always be false and produce a phantom trailing row. We instead
// measure trailing-newline-ness against the SGR-stripped tail.
func addGutter(formatted string) string {
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
	var b strings.Builder
	b.Grow(len(formatted) + total*(gutterWidth+9))

	lineNum := 1
	start := 0
	emitted := 0
	for i := 0; i <= len(formatted); i++ {
		if i == len(formatted) || formatted[i] == '\n' {
			row := formatted[start:i]
			// Skip the trailing SGR-only tail that Chroma emits after
			// the final newline — it has no source content and would
			// otherwise show as a phantom blank-guttered row.
			if emitted == total && stripSGR(row) == "" {
				if i < len(formatted) {
					b.WriteByte('\n')
				}
				start = i + 1
				continue
			}
			b.WriteString(formatLineNumber(lineNum, gutterWidth))
			b.WriteByte(' ')
			b.WriteString(row)
			if i < len(formatted) {
				b.WriteByte('\n')
			}
			lineNum++
			emitted++
			start = i + 1
		}
	}
	return b.String()
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
