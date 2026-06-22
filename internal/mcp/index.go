package mcp

import (
	"sync"

	"github.com/wilkes/hypogeum/internal/vault"
)

// index is the server's warm vault cache. A single MCP server instance serves
// one vault root, so it builds the forward graph (vault.Build) once and reuses
// it across tool calls instead of rebuilding from cold on every neighbors/graph
// query — the amortization that justifies a long-lived server over the
// per-call CLI verbs.
//
// Concurrency: this mutex guards only the i.v *pointer* — get() takes RLock to
// read it, rebuild()/refreshFile() take Lock to swap or mutate it. A reader
// copies the *vault.Vault under RLock and then releases it before calling
// v.Outbound()/v.Backlinks()/v.Files(), so the index lock does NOT serialize a
// reader's vault access against a concurrent refreshFile. What makes that
// race-free is the vault's *own* internal RWMutex: every vault read method
// takes v.mu.RLock and RefreshFile takes v.mu.Lock (see internal/vault). So
// there are two independent locks doing two jobs — this one orders pointer
// swaps/reads, the vault's orders in-place graph mutation against graph reads.
// TestWarmIndexConcurrentRefresh exercises both under -race.
type index struct {
	mu   sync.RWMutex
	root string       // absolute vault root
	v    *vault.Vault // nil until the first get(); rebuilt on StructureChanged
}

func newIndex(root string) *index { return &index{root: root} }

// get returns the warm vault, building it lazily on first use. Lazy (rather
// than eager-at-startup) keeps server startup instant and tolerates a vault
// that isn't fully present when the server launches; the cost is paid by the
// first neighbors/graph call. A build error is returned to the caller and not
// cached — the next call retries (e.g. after the vault becomes readable).
func (i *index) get() (*vault.Vault, error) {
	// Fast path: already warm.
	i.mu.RLock()
	if v := i.v; v != nil {
		i.mu.RUnlock()
		return v, nil
	}
	i.mu.RUnlock()

	i.mu.Lock()
	defer i.mu.Unlock()
	// Re-check under the write lock: another goroutine may have built it
	// between the RUnlock above and the Lock here.
	if i.v != nil {
		return i.v, nil
	}
	v, err := vault.Build(i.root, vault.NopDiagnostics{})
	if err != nil {
		return nil, err
	}
	i.v = v
	return v, nil
}

// rebuild re-walks the vault from scratch in response to a structural change
// (file/dir created, removed, or renamed). It no-ops when nothing has warmed
// the cache yet: there's no point building from a watch event for a vault no
// tool call has touched — the next get() will build it lazily. A failed
// rebuild leaves the previous good vault in place rather than blanking the
// cache, so a transient error mid-edit doesn't degrade later queries.
func (i *index) rebuild() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.v == nil {
		return
	}
	if v, err := vault.Build(i.root, vault.NopDiagnostics{}); err == nil {
		i.v = v
	}
}

// refreshFile re-parses a single modified file in place. Like rebuild it
// no-ops when the cache is cold (RefreshFile on a nil vault would panic, and a
// lazy build will pick up the change anyway).
func (i *index) refreshFile(path string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.v == nil {
		return
	}
	_ = i.v.RefreshFile(path) // best-effort; a parse error keeps the prior entry
}
