package vault

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestOutboundFor_MatchesFullBuild is the correctness bar for the links fast
// path: for every markdown file in a real vault, OutboundFor must return
// exactly what Build(root).Outbound(file) returns.
func TestOutboundFor_MatchesFullBuild(t *testing.T) {
	root := filepath.Join("..", "..", "docs")
	full, err := Build(root, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var files int
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !tree_IsMarkdown(p) {
			return nil
		}
		files++
		abs, _ := filepath.Abs(p)
		want := full.Outbound(abs)
		got, err := OutboundFor(root, p, NopDiagnostics{})
		if err != nil {
			t.Fatalf("OutboundFor(%s): %v", p, err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s: outbound mismatch\n want %d refs: %+v\n got  %d refs: %+v",
				p, len(want), want, len(got), got)
		}
		return nil
	})
	if files == 0 {
		t.Fatal("no markdown files found in docs/ — corpus missing")
	}
	t.Logf("verified %d files match full Build", files)
}

// tree_IsMarkdown mirrors the markdown filter without importing tree into the
// test (tree is already a vault dependency; this keeps the test self-contained).
func tree_IsMarkdown(p string) bool {
	ext := filepath.Ext(p)
	return ext == ".md" || ext == ".markdown"
}
