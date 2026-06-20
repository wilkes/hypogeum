package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutbound(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	foo := write("foo.md", "Links to [[bar]] and [missing](./nope.md) and [site](https://x.com)\n")
	write("bar.md", "# Bar\n")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	out := v.Outbound(foo)
	if len(out) != 3 {
		t.Fatalf("got %d outbound, want 3: %+v", len(out), out)
	}

	// First: resolved wikilink to bar.md.
	if out[0].Kind != OutboundWikilink {
		t.Errorf("out[0].Kind = %v, want OutboundWikilink", out[0].Kind)
	}
	if out[0].Resolved != filepath.Join(dir, "bar.md") {
		t.Errorf("out[0].Resolved = %q, want bar.md", out[0].Resolved)
	}

	// Second: relative std link. The vault surfaces the COMPUTED path
	// as-is; it does not check existence at the vault layer.
	if out[1].Kind != OutboundStdLink {
		t.Errorf("out[1].Kind = %v, want OutboundStdLink", out[1].Kind)
	}
	if out[1].Resolved != filepath.Join(dir, "nope.md") {
		t.Errorf("out[1].Resolved = %q, want computed nope.md path", out[1].Resolved)
	}

	// Third: external std link — raw target preserved, never resolved.
	if out[2].RawTarget != "https://x.com" {
		t.Errorf("out[2].RawTarget = %q, want https://x.com", out[2].RawTarget)
	}
	if out[2].Resolved != "" {
		t.Errorf("out[2].Resolved = %q, want empty (external)", out[2].Resolved)
	}
}

func TestOutboundUnknownFile(t *testing.T) {
	v, err := Build(t.TempDir(), NopDiagnostics{})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Outbound("/no/such/file.md"); len(got) != 0 {
		t.Errorf("Outbound(unknown) = %v, want empty", got)
	}
}
