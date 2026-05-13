package embed

import (
	"strconv"
	"strings"
)

// RenderToFence builds the markdown source we'll splice into the document.
//
// absPath        — the source file's path (only its basename is shown in the header).
// lines          — the actual source-line content (no trailing newlines).
// startLine      — 1-indexed line number of lines[0] in the original file.
// displayRange   — the range the user *asked for*, formatted for the header
//                  (e.g. "42–58" or "42"). Distinct from len(lines)/startLine
//                  because context-line padding expands the body but not the
//                  header.
// leadContext    — count of lines at the head of `lines` that are context
//                  (rendered with the ~ gutter).
// tailContext    — count of lines at the tail that are context.
// softWarning    — optional header annotation, e.g. "file ends at line 50".
func RenderToFence(absPath string, lines []string, startLine int, displayRange string, leadContext, tailContext int, softWarning string) string {
	lang := LanguageFromPath(absPath)

	// Gutter width: largest line number that will appear, with a floor so
	// the context marker (`~`) and single-digit line numbers stay padded
	// to the same width as multi-digit numbers in typical ranges.
	maxLine := startLine + len(lines) - 1
	gw := len(strconv.Itoa(maxLine))
	if gw < 3 {
		gw = 3
	}

	var b strings.Builder
	// Provenance header. Use the basename so an absolute path like
	// /Users/foo/bar/main.go renders as "main.go:42–58".
	header := basename(absPath) + ":" + displayRange
	if softWarning != "" {
		header += " (" + softWarning + ")"
	}
	b.WriteString("> `")
	b.WriteString(header)
	b.WriteString("`\n")

	// Open fence.
	b.WriteString("```")
	b.WriteString(lang) // empty for unknown — yields ``` alone, which is valid.
	b.WriteByte('\n')

	// Body with gutter.
	primaryEnd := len(lines) - tailContext
	for i, line := range lines {
		isContext := i < leadContext || i >= primaryEnd
		if isContext {
			b.WriteString(strings.Repeat(" ", gw-1))
			b.WriteByte('~')
		} else {
			n := strconv.Itoa(startLine + i)
			b.WriteString(strings.Repeat(" ", gw-len(n)))
			b.WriteString(n)
		}
		b.WriteString(" │ ")
		b.WriteString(line)
		b.WriteByte('\n')
	}

	// Close fence.
	b.WriteString("```\n")
	return b.String()
}

func basename(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}
