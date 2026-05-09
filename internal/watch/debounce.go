package watch

import "time"

// pending holds the staged structural and write events accumulated during
// one debounce window. Both maps act as sets keyed by absolute path.
type pending struct {
	structPaths map[string]struct{}
	writePaths  map[string]struct{}
}

func (p *pending) addStruct(path string) {
	if p.structPaths == nil {
		p.structPaths = make(map[string]struct{})
	}
	p.structPaths[path] = struct{}{}
}

func (p *pending) addWrite(path string) {
	if p.writePaths == nil {
		p.writePaths = make(map[string]struct{})
	}
	p.writePaths[path] = struct{}{}
}

// resetTimer (re)arms the debounce timer and returns its channel for the
// caller's select. The first call allocates the timer; subsequent calls
// stop and reset it, draining any spuriously-fired tick.
func (w *Watcher) resetTimer(timer **time.Timer) <-chan time.Time {
	if *timer == nil {
		*timer = time.NewTimer(w.debounceWindow)
	} else {
		if !(*timer).Stop() {
			select {
			case <-(*timer).C:
			default:
			}
		}
		(*timer).Reset(w.debounceWindow)
	}
	return (*timer).C
}

// flush emits one or two events covering the staged paths and clears them.
// Sends are guarded by w.done so a Close mid-flush returns promptly.
func (w *Watcher) flush(p *pending) {
	if len(p.structPaths) > 0 {
		paths := drainSet(p.structPaths)
		select {
		case w.out <- Event{Kind: StructureChanged, Paths: paths}:
		case <-w.done:
			return
		}
	}
	if len(p.writePaths) > 0 {
		paths := drainSet(p.writePaths)
		select {
		case w.out <- Event{Kind: FileModified, Paths: paths}:
		case <-w.done:
			return
		}
	}
}

func drainSet(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
		delete(m, k)
	}
	return out
}
