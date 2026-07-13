package diagnostics

import (
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const traceSchemaVersion = "motion_trace.v3"

// MotionTraceRow is the stable JSON row schema for motion and transport traces.
type MotionTraceRow struct {
	Sequence         uint64                   `json:"sequence"`
	Timestamp        string                   `json:"timestamp"`
	Source           string                   `json:"source"`
	Reason           string                   `json:"reason"`
	Target           *MotionTraceTarget       `json:"target,omitempty"`
	Sample           *MotionTraceSample       `json:"sample,omitempty"`
	Retarget         *MotionTraceRetarget     `json:"retarget,omitempty"`
	Planner          *MotionTracePlanner      `json:"planner,omitempty"`
	TransportCommand *transport.Command       `json:"transport_command,omitempty"`
	TransportResult  *transport.CommandResult `json:"transport_result,omitempty"`
	Annotation       string                   `json:"annotation,omitempty"`
}

// MotionTracePlanner records one deterministic mode-planner decision so a
// stopped device is diagnosable as planner-wait versus transport failure.
type MotionTracePlanner struct {
	Mode              string         `json:"mode"`
	Event             string         `json:"event"`
	Style             string         `json:"style,omitempty"`
	Seed              int64          `json:"seed,omitempty"`
	PatternIdentifier string         `json:"pattern_id,omitempty"`
	SpeedPercent      int            `json:"speed_percent,omitempty"`
	DriftToPercent    int            `json:"drift_to_percent,omitempty"`
	DurationMillis    int64          `json:"duration_ms,omitempty"`
	SegmentIndex      int            `json:"segment_index,omitempty"`
	Scores            []PlannerScore `json:"scores,omitempty"`
	Note              string         `json:"note,omitempty"`
}

// PlannerScore records one candidate's deterministic score.
type PlannerScore struct {
	PatternIdentifier string  `json:"pattern_id"`
	Score             float64 `json:"score"`
	Chosen            bool    `json:"chosen"`
}

// MotionTraceTarget records semantic target context without raw transport shape.
type MotionTraceTarget struct {
	Label                     string `json:"label,omitempty"`
	SpeedPercent              int    `json:"speed_percent,omitempty"`
	StrokeMinPercent          int    `json:"stroke_min_percent,omitempty"`
	StrokeMaxPercent          int    `json:"stroke_max_percent,omitempty"`
	ReverseDirection          bool   `json:"reverse_direction,omitempty"`
	PatternIdentifier         string `json:"pattern_id,omitempty"`
	ProgramIdentifier         string `json:"program_id,omitempty"`
	AreaMinPercent            int    `json:"area_min_percent,omitempty"`
	AreaMaxPercent            int    `json:"area_max_percent,omitempty"`
	SoftAnchorPositionPercent int    `json:"soft_anchor_position_percent,omitempty"`
	SoftAnchorWeightPercent   int    `json:"soft_anchor_weight_percent,omitempty"`
}

// MotionTraceSample records a sampled transport-neutral motion point.
type MotionTraceSample struct {
	PositionPercent float64 `json:"position_percent"`
	TimeMillis      int64   `json:"time_ms"`
}

// MotionTraceRetarget records retarget decision details required for real-device review.
type MotionTraceRetarget struct {
	PreviousPlanID                  string             `json:"previous_plan_id,omitempty"`
	NextPlanID                      string             `json:"next_plan_id,omitempty"`
	PreviousPatternIdentifier       string             `json:"previous_pattern_id,omitempty"`
	NextPatternIdentifier           string             `json:"next_pattern_id,omitempty"`
	PreviousProgramIdentifier       string             `json:"previous_program_id,omitempty"`
	NextProgramIdentifier           string             `json:"next_program_id,omitempty"`
	PreviousTarget                  *MotionTraceTarget `json:"previous_target,omitempty"`
	NextTarget                      *MotionTraceTarget `json:"next_target,omitempty"`
	EstimatedCurrentPositionPercent float64            `json:"estimated_current_position_percent,omitempty"`
	EstimatedCurrentStreamMillis    int64              `json:"estimated_current_stream_ms,omitempty"`
	SelectedHandoffMillis           int64              `json:"selected_handoff_ms,omitempty"`
	SelectedLeadMillis              int64              `json:"selected_lead_ms,omitempty"`
	RecentCommandLatencyMillis      int64              `json:"recent_command_latency_ms,omitempty"`
	PhasePreserved                  bool               `json:"phase_preserved"`
	BridgePointsInserted            bool               `json:"bridge_points_inserted"`
	Recovery                        string             `json:"recovery,omitempty"`
}

// TraceExport is the stable JSON envelope returned by trace export endpoints.
type TraceExport struct {
	SchemaVersion             string                             `json:"schema_version"`
	Rows                      []MotionTraceRow                   `json:"rows"`
	DroppedRows               uint64                             `json:"dropped_rows"`
	IntifaceDispatches        []transport.IntifaceDispatchStatus `json:"intiface_dispatches,omitempty"`
	IntifaceDispatchesDropped uint64                             `json:"intiface_dispatches_dropped"`
	IntifaceLinearSentCount   uint64                             `json:"intiface_linear_sent_count"`
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
	row = cloneTraceRow(row)

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

	return cloneTraceRows(r.rows)
}

// Export returns the stable trace export envelope.
func (r *TraceRing) Export() TraceExport {
	r.mu.Lock()
	defer r.mu.Unlock()

	return TraceExport{
		SchemaVersion: traceSchemaVersion,
		Rows:          cloneTraceRows(r.rows),
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

func cloneTraceRows(rows []MotionTraceRow) []MotionTraceRow {
	clones := make([]MotionTraceRow, len(rows))
	for index, row := range rows {
		clones[index] = cloneTraceRow(row)
	}
	return clones
}

func cloneTraceRow(row MotionTraceRow) MotionTraceRow {
	if row.Target != nil {
		row.Target = cloneTraceTarget(row.Target)
	}
	if row.Sample != nil {
		sample := *row.Sample
		row.Sample = &sample
	}
	if row.Retarget != nil {
		retarget := *row.Retarget
		retarget.PreviousTarget = cloneTraceTarget(row.Retarget.PreviousTarget)
		retarget.NextTarget = cloneTraceTarget(row.Retarget.NextTarget)
		row.Retarget = &retarget
	}
	if row.Planner != nil {
		planner := *row.Planner
		planner.Scores = append([]PlannerScore(nil), row.Planner.Scores...)
		row.Planner = &planner
	}
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

func cloneTraceTarget(target *MotionTraceTarget) *MotionTraceTarget {
	if target == nil {
		return nil
	}
	clone := *target
	return &clone
}
