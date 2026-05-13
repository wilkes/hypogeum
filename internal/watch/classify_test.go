package watch

import (
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		ev   fsnotify.Event
		want classifyResult
	}{
		{
			name: "create markdown file",
			ev:   fsnotify.Event{Name: "/tmp/note.md", Op: fsnotify.Create},
			want: classifyResult{Kind: StructureChanged, Path: "/tmp/note.md", MaybeNewDir: true},
		},
		{
			name: "create non-markdown file",
			ev:   fsnotify.Event{Name: "/tmp/note.txt", Op: fsnotify.Create},
			want: classifyResult{Kind: StructureChanged, Path: "/tmp/note.txt", MaybeNewDir: true},
		},
		{
			name: "write to markdown",
			ev:   fsnotify.Event{Name: "/tmp/note.md", Op: fsnotify.Write},
			want: classifyResult{Kind: FileModified, Path: "/tmp/note.md"},
		},
		{
			name: "write to non-markdown",
			ev:   fsnotify.Event{Name: "/tmp/note.txt", Op: fsnotify.Write},
			want: classifyResult{Kind: FileModified, Path: "/tmp/note.txt"},
		},
		{
			name: "remove",
			ev:   fsnotify.Event{Name: "/tmp/note.md", Op: fsnotify.Remove},
			want: classifyResult{Kind: StructureChanged, Path: "/tmp/note.md"},
		},
		{
			name: "rename",
			ev:   fsnotify.Event{Name: "/tmp/note.md", Op: fsnotify.Rename},
			want: classifyResult{Kind: StructureChanged, Path: "/tmp/note.md"},
		},
		{
			name: "hidden path on create",
			ev:   fsnotify.Event{Name: "/tmp/.git/HEAD", Op: fsnotify.Create},
			want: classifyResult{Path: "/tmp/.git/HEAD", Ignore: true},
		},
		{
			name: "hidden path on write",
			ev:   fsnotify.Event{Name: "/tmp/.config/note.md", Op: fsnotify.Write},
			want: classifyResult{Path: "/tmp/.config/note.md", Ignore: true},
		},
		{
			name: "chmod alone is ignored",
			ev:   fsnotify.Event{Name: "/tmp/note.md", Op: fsnotify.Chmod},
			want: classifyResult{Path: "/tmp/note.md", Ignore: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classify(tt.ev)
			if got != tt.want {
				t.Errorf("classify(%+v) = %+v, want %+v", tt.ev, got, tt.want)
			}
		})
	}
}

func TestClassify_WriteOnNonMarkdown_NotIgnored(t *testing.T) {
	r := classify(fsnotify.Event{Name: "/tmp/notes/main.go", Op: fsnotify.Write})
	if r.Ignore {
		t.Error("expected write on .go file to NOT be ignored (live-reload for code files)")
	}
	if r.Kind != FileModified {
		t.Errorf("expected Kind=FileModified, got %v", r.Kind)
	}
	if r.Path != "/tmp/notes/main.go" {
		t.Errorf("expected Path preserved, got %q", r.Path)
	}
}

func TestClassify_CreateOnNonMarkdown_StillStructureChange(t *testing.T) {
	// Structure changes stay markdown-only: a new .py file should not
	// trigger a tree re-walk. classify returns StructureChanged +
	// MaybeNewDir; the stage() wrapper does the IsMarkdown check.
	r := classify(fsnotify.Event{Name: "/tmp/notes/script.py", Op: fsnotify.Create})
	if r.Ignore {
		t.Error("classify should not ignore Create on .py — that's stage()'s job")
	}
	if !r.MaybeNewDir {
		t.Error("expected MaybeNewDir on Create event")
	}
}
