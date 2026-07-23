package diagnostics

import (
	"sync"
	"time"
)

// HandyLogEntry is one device-motion diagnostic row for the diagnostics UI.
type HandyLogEntry struct {
	Timestamp      string         `json:"ts"`
	Event          string         `json:"event"`
	Source         string         `json:"source,omitempty"`
	PositionPct    *float64       `json:"position_pct,omitempty"`
	DurationMS     *int           `json:"duration_ms,omitempty"`
	Error          string         `json:"error,omitempty"`
	PlaybackState  string         `json:"playback_state,omitempty"`
	BufferAheadMS  *int64         `json:"buffer_ahead_ms,omitempty"`
	StreamElapsed  *int64         `json:"stream_elapsed_ms,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
}

// HandyLogRing stores recent device motion events in oldest-first order.
type HandyLogRing struct {
	mu       sync.Mutex
	capacity int
	entries  []HandyLogEntry
}

// NewHandyLogRing creates a fixed-capacity handy motion log ring.
func NewHandyLogRing(capacity int) *HandyLogRing {
	if capacity < 1 {
		capacity = 1
	}
	return &HandyLogRing{
		capacity: capacity,
		entries:  make([]HandyLogEntry, 0, capacity),
	}
}

// Add stores one handy motion log entry.
func (r *HandyLogRing) Add(entry HandyLogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if len(r.entries) == r.capacity {
		copy(r.entries, r.entries[1:])
		r.entries[len(r.entries)-1] = entry
		return
	}
	r.entries = append(r.entries, entry)
}

// Entries returns a copy of stored entries oldest-first.
func (r *HandyLogRing) Entries() []HandyLogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]HandyLogEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

// Tail returns the newest limit entries.
func (r *HandyLogRing) Tail(limit int) []HandyLogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 || len(r.entries) == 0 {
		return nil
	}
	start := 0
	if len(r.entries) > limit {
		start = len(r.entries) - limit
	}
	out := make([]HandyLogEntry, len(r.entries)-start)
	copy(out, r.entries[start:])
	return out
}
