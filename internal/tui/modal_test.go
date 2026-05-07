package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestSpliceLineANSIAware verifies that splicing a modal segment over a
// base line containing ANSI SGR escapes does not slice escapes mid-sequence
// and does not corrupt the visible width of the result.
func TestSpliceLineANSIAware(t *testing.T) {
	// Base: 25 visible cells, with SGR styling around columns 5-15.
	// Width is the same whether or not ANSI is present.
	base := "abcde" + "\x1b[38;5;252m" + "FFFFFFFFFF" + "\x1b[0m" + "ghijklmnop"
	if got := ansi.StringWidth(base); got != 25 {
		t.Fatalf("base width: got %d want 25", got)
	}

	// Modal segment: opaque, 8 cells wide, will be placed at column 8 —
	// straddling the styled range so naive byte slicing would cut into
	// "\x1b[38;5;252m".
	over := "[MODAL!]"
	got := spliceLine(base, over, 8)

	if w := ansi.StringWidth(got); w != 25 {
		t.Errorf("post-splice visible width: got %d want 25 (output: %q)", w, got)
	}

	// The plain text must show base[0:8] + over + base[16:25] when ANSI
	// is stripped.
	plainBase := ansi.Strip(base)
	wantPlain := plainBase[:8] + over + plainBase[16:]
	if gotPlain := ansi.Strip(got); gotPlain != wantPlain {
		t.Errorf("post-splice plain text: got %q want %q", gotPlain, wantPlain)
	}

	// No partial SGR introducer should appear: every "\x1b[" must have a
	// terminator within a few bytes. A naive splicer leaves dangling
	// bytes like "8;5;252m" or "[0m" without their leading "\x1b[".
	if strings.Contains(got, "8;5;252m") && !strings.Contains(got, "\x1b[38;5;252m") {
		t.Errorf("found bare SGR params without ESC[ introducer: %q", got)
	}
}

// TestSpliceLinePadsShortBase ensures we don't crash when the base line is
// shorter than the splice column (rare but possible during transient resizes).
func TestSpliceLinePadsShortBase(t *testing.T) {
	base := "abc"
	got := spliceLine(base, "XY", 6)
	if w := ansi.StringWidth(got); w != 8 {
		t.Errorf("got width %d want 8 (%q)", w, got)
	}
	if !strings.HasPrefix(ansi.Strip(got), "abc   XY") {
		t.Errorf("expected padding then segment, got %q", ansi.Strip(got))
	}
}

// TestOverlayModalPreservesGeometry verifies the composed output keeps the
// same line count and per-line cell width as the base — the modal must
// not stretch any line.
func TestOverlayModalPreservesGeometry(t *testing.T) {
	const termW, termH = 100, 30
	// Build a base of exactly termW × termH with SGR scattered through.
	row := strings.Repeat("\x1b[31ma\x1b[0m", termW) // 'a' x termW with red SGR
	if w := ansi.StringWidth(row); w != termW {
		t.Fatalf("base row width: got %d want %d", w, termW)
	}
	rows := make([]string, termH)
	for i := range rows {
		rows[i] = row
	}
	base := strings.Join(rows, "\n")

	// Build a modal of the right geometry filled with a marker glyph.
	_, _, w, h := modalGeometry(termW, termH)
	mrow := strings.Repeat("M", w)
	mrows := make([]string, h)
	for i := range mrows {
		mrows[i] = mrow
	}
	modal := strings.Join(mrows, "\n")

	out := overlayModal(base, modal, termW, termH)

	outLines := strings.Split(out, "\n")
	if len(outLines) != termH {
		t.Fatalf("line count: got %d want %d", len(outLines), termH)
	}
	for i, ln := range outLines {
		if got := ansi.StringWidth(ln); got != termW {
			t.Errorf("line %d width: got %d want %d (%q)", i, got, termW, ln)
		}
	}
}
