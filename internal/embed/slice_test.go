package embed

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeTmp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestSliceFile_WholeFile(t *testing.T) {
	p := writeTmp(t, "x.go", "a\nb\nc\n")
	lines, start, err := SliceFile(p, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != 1 {
		t.Fatalf("start = %d, want 1", start)
	}
	if len(lines) != 3 || lines[0] != "a" || lines[1] != "b" || lines[2] != "c" {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestSliceFile_Range(t *testing.T) {
	p := writeTmp(t, "x.go", "1\n2\n3\n4\n5\n")
	lines, start, err := SliceFile(p, &LineRange{Start: 2, End: 4}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != 2 {
		t.Fatalf("start = %d, want 2", start)
	}
	if len(lines) != 3 || lines[0] != "2" || lines[2] != "4" {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestSliceFile_Context(t *testing.T) {
	p := writeTmp(t, "x.go", "1\n2\n3\n4\n5\n6\n7\n8\n")
	lines, start, err := SliceFile(p, &LineRange{Start: 4, End: 5}, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != 2 {
		t.Fatalf("start = %d, want 2 (4 - 2 context)", start)
	}
	if len(lines) != 6 { // 2..7 inclusive
		t.Fatalf("len = %d, want 6", len(lines))
	}
}

func TestSliceFile_ContextClampedAtStart(t *testing.T) {
	p := writeTmp(t, "x.go", "1\n2\n3\n4\n5\n")
	lines, start, err := SliceFile(p, &LineRange{Start: 2, End: 2}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != 1 {
		t.Fatalf("start = %d, want 1 (clamped)", start)
	}
	if len(lines) != 5 {
		t.Fatalf("len = %d, want 5 (whole file)", len(lines))
	}
}

func TestSliceFile_RangeEndPastEOF(t *testing.T) {
	p := writeTmp(t, "x.go", "1\n2\n3\n")
	lines, start, err := SliceFile(p, &LineRange{Start: 2, End: 100}, 0)
	if !errors.Is(err, ErrRangePastEOF) {
		t.Fatalf("err = %v, want ErrRangePastEOF", err)
	}
	if start != 2 || len(lines) != 2 || lines[0] != "2" || lines[1] != "3" {
		t.Fatalf("lines = %#v, start = %d", lines, start)
	}
}

func TestSliceFile_StartPastEOF(t *testing.T) {
	p := writeTmp(t, "x.go", "1\n2\n3\n")
	_, _, err := SliceFile(p, &LineRange{Start: 100, End: 200}, 0)
	if !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("err = %v, want ErrInvalidRange", err)
	}
}

func TestSliceFile_Binary(t *testing.T) {
	p := writeTmp(t, "x.bin", "abc\x00def")
	_, _, err := SliceFile(p, nil, 0)
	if !errors.Is(err, ErrBinary) {
		t.Fatalf("err = %v, want ErrBinary", err)
	}
}

func TestSliceFile_OversizeWholeFile(t *testing.T) {
	big := make([]byte, 5*1024*1024+1)
	for i := range big {
		big[i] = 'x'
	}
	p := writeTmp(t, "x.txt", string(big))
	_, _, err := SliceFile(p, nil, 0)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestSliceFile_Missing(t *testing.T) {
	_, _, err := SliceFile("/no/such/path", nil, 0)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
