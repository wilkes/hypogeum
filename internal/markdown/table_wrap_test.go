package markdown

import (
	"strings"
	"testing"
)

// TestRender_TableCellWraps pins the contract: a long table cell wraps
// to multiple lines rather than being character-truncated.
func TestRender_TableCellWraps(t *testing.T) {
	r := rendererForTest(t)
	src := "" +
		"| Field | Description |\n" +
		"| ----- | ----------- |\n" +
		"| name | The full canonical name of the user including any honorifics and suffixes and other ceremonial modifiers |\n"

	out, err := r.Render(src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	visible := stripANSI(out)

	if !strings.Contains(visible, "suffixes") {
		t.Errorf("expected the cell's trailing word 'suffixes' to survive; cell was truncated:\n%s", visible)
	}

	var rowsWithBorder int
	for _, line := range strings.Split(visible, "\n") {
		if strings.ContainsRune(line, '│') {
			rowsWithBorder++
		}
	}
	if rowsWithBorder < 3 {
		t.Errorf("expected >=3 rows containing │ (header + wrapped body), got %d:\n%s", rowsWithBorder, visible)
	}
}
