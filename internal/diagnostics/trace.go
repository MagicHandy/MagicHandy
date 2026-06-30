package diagnostics

import (
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const traceSchemaVersion = "motion_trace.v1"

// MotionTraceRow is the stable JSON row schema for motion and transport traces.
type MotionTraceRow struct {
	Sequence         uint64                   `json:"sequence"`
	Timestamp        string                   `json:"timestamp"`
	Source           string                   `json:"source"`
	Reason           string                   `json:"reason"`
	Target           *MotionTraceTarget       `json:"target,omitempty"`
	Sample           *MotionTraceSample       `json:"sample,omitempty"`
	TransportCommand *transport.Command       `json:"transport_command,omitempty"`
	TransportResult  *transport.CommandResult `json:"transport_result,omitempty"`
	Annotation       string                   `json:"annotation,omitempty"`
}

// MotionTraceTarget records semantic target context without raw transport shape.
type MotionTraceTarget struct {
	Label             string `json:"label,omitempty"`
	SpeedPercent      int    `json:"speed_percent,omitempty"`
	StrokeMinPercent  int    `json:"stroke_min_percent,omitempty"`
	StrokeMaxPercent  int    `json:"stroke_max_percent,omitempty"`
	ReverseDirection  bool   `json:"reverse_direction,omitempty"`
	PatternIdentifier string `json:"pattern_id,omitempty"`
}

// MotionTraceSample records a sampled transport-neutral motion point.
type MotionTraceSample struct {
	PositionPercent int   `json:"position_percent"`
	TimeMillis      int64 `json:"time_ms"`
}

// TraceExport is the stable JSON envelope returned by trace export endpoints.
type TraceExport struct {
	SchemaVersion string           `json:"schema_version"`
	Rows          []MotionTraceRow `json:"rows"`
	DroppedRows   uint64           `json:"dropped_rows"`
}

// TraceSummary is a compact app-state view of trace availability.
type TraceSummary struct {
	SchemaVersion string `json:"schema_version"`
	RowCount      int    `json:"row_count"`
	Capacity      int    `json:"capacity"`
	DroppedRows   uint64 `json:"dropped_rows"`
}

// TraceRing is a fixed-capacity, oldest-first motion trace ring.
type TraceRing struct {
	mu       sync.Mutex
	capacity int
	next     uint64
	dropped  uint64
	rows     []MotionTraceRow
}

// NewTraceRing creates an in-memory trace ring with at least one row capacity.
func NewTraceRing(capacity int) *TraceRing {
	if capacity < 1 {
		capacity = 1
	}
	return &TraceRing{
		capacity: capacity,
		rows:     make([]MotionTraceRow, 0, capacity),
	}
}

// Add stores a sanitized trace row and returns the stored row.
func (r *TraceRing) Add(row MotionTraceRow) MotionTraceRow {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.next++
	row.Sequence = r.next
	if row.Timestamp == "" {
		row.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	row = sanitizeTraceRow(row)

	if len(r.rows) == r.capacity {
		copy(r.rows, r.rows[1:])
		r.rows[len(r.rows)-1] = row
		r.dropped++
		return row
	}

	r.rows = append(r.rows, row)
	return row
}

// Rows returns the trace rows in oldest-first order.
func (r *TraceRing) Rows() []MotionTraceRow {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows := make([]MotionTraceRow, len(r.rows))
	copy(rows, r.rows)
	return rows
}

// Export returns the stable trace export envelope.
func (r *TraceRing) Export() TraceExport {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows := make([]MotionTraceRow, len(r.rows))
	copy(rows, r.rows)
	return TraceExport{
		SchemaVersion: traceSchemaVersion,
		Rows:          rows,
		DroppedRows:   r.dropped,
	}
}

// Summary returns trace metadata for app state snapshots.
func (r *TraceRing) Summary() TraceSummary {
	r.mu.Lock()
	defer r.mu.Unlock()

	return TraceSummary{
		SchemaVersion: traceSchemaVersion,
		RowCount:      len(r.rows),
		Capacity:      r.capacity,
		DroppedRows:   r.dropped,
	}
}

func sanitizeTraceRow(row MotionTraceRow) MotionTraceRow {
	if row.TransportCommand != nil {
		command := transport.SafeCommand(*row.TransportCommand)
		row.TransportCommand = &command
	}
	if row.TransportResult != nil {
		result := transport.SafeCommandResult(*row.TransportResult)
		row.TransportResult = &result
	}
	return row
}
