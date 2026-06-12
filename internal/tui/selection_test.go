package tui

import "testing"

func TestModel_CopyToClipboard_DefaultIsSet(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	if m.copyToClipboard == nil {
		t.Fatal("copyToClipboard should default to a non-nil writer")
	}
}
