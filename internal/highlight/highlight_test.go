package highlight

import "testing"

// TestMarkerBytes guards the wire format: the DC1/DC2 control characters
// are a data contract shared by the producers (search, vault) and the
// TUI snippet renderer. Changing them silently would break highlighting.
func TestMarkerBytes(t *testing.T) {
	if Open != "\x11" {
		t.Fatalf("Open: got %q want \\x11 (DC1)", Open)
	}
	if Close != "\x12" {
		t.Fatalf("Close: got %q want \\x12 (DC2)", Close)
	}
}

func TestWrap(t *testing.T) {
	got := Wrap("needle")
	want := "\x11needle\x12"
	if got != want {
		t.Fatalf("Wrap: got %q want %q", got, want)
	}
}

func TestWrapEmpty(t *testing.T) {
	if got := Wrap(""); got != "\x11\x12" {
		t.Fatalf("Wrap(\"\"): got %q want %q", got, "\x11\x12")
	}
}

func TestStrip(t *testing.T) {
	// Strip is the inverse of Wrap for the marker bytes: the wrapped
	// content comes back clean.
	if got := Strip(Wrap("hello")); got != "hello" {
		t.Fatalf("Strip(Wrap(\"hello\")): got %q want %q", got, "hello")
	}
	// Interleaved markers (multiple spans, including stray/unbalanced
	// markers) are all removed; non-marker content is untouched.
	in := "a" + Open + "b" + Close + "c" + Open + "d" + Open + Close + "e"
	if got := Strip(in); got != "abcde" {
		t.Fatalf("Strip(interleaved): got %q want %q", got, "abcde")
	}
	// A string with no markers is returned unchanged.
	if got := Strip("plain"); got != "plain" {
		t.Fatalf("Strip(\"plain\"): got %q want %q", got, "plain")
	}
}
