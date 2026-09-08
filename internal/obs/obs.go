// Package obs provides tiny structured observability primitives on top of
// log/slog: span timers and size counters. No external deps.
package obs

import (
	"log/slog"
	"time"
)

// Timer measures a named span. Use Start/Done around I/O or model calls.
type Timer struct {
	name  string
	start time.Time
}

// Start begins a named span.
func Start(name string) *Timer {
	return &Timer{name: name, start: time.Now()}
}

// Done logs the span duration at debug with optional extra attrs.
func (t *Timer) Done(extra ...any) {
	attrs := append([]any{"span", t.name, "obs_ms", t.Millis()}, extra...)

	slog.Debug("obs span done", attrs...)
}

// Millis returns elapsed milliseconds.
func (t *Timer) Millis() int64 {
	return time.Since(t.start).Milliseconds()
}

// LogSizes logs prompt input sizes at debug for token/cost reasoning.
func LogSizes(diffBytes, ctxBytes int) {
	slog.Debug("prompt sizes", "diff_bytes", diffBytes, "ctx_bytes", ctxBytes, "total_bytes", diffBytes+ctxBytes)
}
