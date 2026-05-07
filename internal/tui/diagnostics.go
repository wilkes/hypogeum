package tui

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// diagnosticsRingCap is the in-memory ring buffer size — what the log
// viewer modal displays. The on-disk log is unbounded and accepts
// everything that passes through Push.
const diagnosticsRingCap = 200

// severity is plumbed through so future diagnostics (render times,
// rebuild durations) can land at sevInfo without changing the API.
type severity int

const (
	sevInfo severity = iota
	sevWarn
	sevError
)

func (s severity) String() string {
	switch s {
	case sevInfo:
		return "info"
	case sevWarn:
		return "warn"
	case sevError:
		return "error"
	}
	return "unknown"
}

// diagEntry is one record in both the ring buffer and the JSON log.
type diagEntry struct {
	Timestamp time.Time `json:"ts"`
	Severity  severity  `json:"severity"`
	Message   string    `json:"message"`
}

// diagOpts configures the diagnostics sink.
type diagOpts struct {
	// LogPath is the file to append JSON-line records to. Empty disables
	// file logging without otherwise affecting the sink.
	LogPath string
	// Now is the time source — injected for tests.
	Now func() time.Time
}

// diagnostics is the TUI's diagnostic sink. It satisfies vault.Diagnostics
// (in spirit — the TUI imports vault, so the type assertion happens there).
type diagnostics struct {
	mu        sync.Mutex
	ring      []diagEntry
	ringStart int // index of oldest entry when ring is full
	ringFull  bool
	logFile   *os.File
	now       func() time.Time
	transient diagEntry // most recent — read by the footer
	hasTrans  bool
}

func newDiagnostics(opts diagOpts) *diagnostics {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	d := &diagnostics{
		ring: make([]diagEntry, 0, diagnosticsRingCap),
		now:  opts.Now,
	}
	if opts.LogPath != "" {
		// Best-effort file open. Failure leaves logFile nil — diagnostics
		// continue working in-memory. This is the "no writable path"
		// branch from the spec.
		if f, err := os.OpenFile(opts.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			d.logFile = f
		}
	}
	return d
}

func (d *diagnostics) Info(msg string)  { d.push(sevInfo, msg) }
func (d *diagnostics) Warn(msg string)  { d.push(sevWarn, msg) }
func (d *diagnostics) Error(msg string) { d.push(sevError, msg) }

func (d *diagnostics) push(sev severity, msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry := diagEntry{Timestamp: d.now(), Severity: sev, Message: msg}

	if len(d.ring) < diagnosticsRingCap {
		d.ring = append(d.ring, entry)
	} else {
		d.ring[d.ringStart] = entry
		d.ringStart = (d.ringStart + 1) % diagnosticsRingCap
		d.ringFull = true
	}

	d.transient = entry
	d.hasTrans = true

	if d.logFile != nil {
		if data, err := json.Marshal(entry); err == nil {
			d.logFile.Write(data)
			d.logFile.Write([]byte("\n"))
		}
	}
}

// snapshot returns a copy of the ring buffer in oldest-to-newest order.
func (d *diagnostics) snapshot() []diagEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]diagEntry, 0, len(d.ring))
	if !d.ringFull {
		out = append(out, d.ring...)
		return out
	}
	out = append(out, d.ring[d.ringStart:]...)
	out = append(out, d.ring[:d.ringStart]...)
	return out
}

// transientStatus returns the most recent entry, if any, for footer display.
// The caller decides when to clear it.
func (d *diagnostics) transientStatus() (diagEntry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.transient, d.hasTrans
}

func (d *diagnostics) clearTransient() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hasTrans = false
}

func (d *diagnostics) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.logFile != nil {
		err := d.logFile.Close()
		d.logFile = nil
		return err
	}
	return nil
}
