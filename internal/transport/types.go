package transport

import (
	"context"
	"time"
)

// CommandKind identifies the physical transport command family.
type CommandKind string

const (
	// CommandKindConnectionCheck checks Cloud REST/HSP availability without moving the device.
	CommandKindConnectionCheck CommandKind = "connection_check"
	// CommandKindHSPState reads Cloud REST HSP state without moving the device.
	CommandKindHSPState CommandKind = "hsp_state"
	// CommandKindHSPEvents reads Cloud REST HSP state events without moving the device.
	CommandKindHSPEvents CommandKind = "hsp_events"
	// CommandKindHSPSetup prepares an HSP stream before timed points are sent.
	CommandKindHSPSetup CommandKind = "hsp_setup"
	// CommandKindStop stops active transport playback.
	CommandKindStop CommandKind = "stop"
	// CommandKindStrokeWindow applies a physical stroke envelope.
	CommandKindStrokeWindow CommandKind = "stroke_window"
	// CommandKindPointsAdd appends timed points to a stream.
	CommandKindPointsAdd CommandKind = "points_add"
	// CommandKindPointsPlay starts or resumes a timed-point stream.
	CommandKindPointsPlay CommandKind = "points_play"
)

// Transport is the deterministic boundary used by motion code to command
// playback without knowing the concrete dispatch owner.
type Transport interface {
	DiagnosticsProvider

	Stop(context.Context, StopCommand) (CommandResult, error)
	SetStrokeWindow(context.Context, StrokeWindowCommand) (CommandResult, error)
	AppendPoints(context.Context, AppendPointsCommand) (CommandResult, error)
	Play(context.Context, PlayCommand) (CommandResult, error)
}

// DiagnosticsProvider exposes a safe transport diagnostics snapshot.
type DiagnosticsProvider interface {
	Diagnostics() TransportDiagnostics
}

// MotionTimingCapabilities describes transport timing limits that the shared
// engine must honor before producing its transport-neutral frame.
type MotionTimingCapabilities struct {
	MinimumPointInterval time.Duration
}

// MotionTimingCapabilitiesProvider exposes optional device timing constraints.
type MotionTimingCapabilitiesProvider interface {
	MotionTimingCapabilities() MotionTimingCapabilities
}

// PlaybackStartTimeProvider reports when transport playback actually began.
// Owners that perform a pre-roll use this to align the shared engine clock.
type PlaybackStartTimeProvider interface {
	PlaybackStartTime() time.Time
}

// TimedPoint is a single transport-neutral timed point. Position is expressed as 0..100.
type TimedPoint struct {
	PositionPercent float64 `json:"position_percent"`
	TimeMillis      int64   `json:"time_ms"`
}

// StopCommand requests transport stop.
type StopCommand struct {
	Reason string `json:"reason"`
}

// StrokeWindowCommand applies the physical stroke envelope at transport level.
type StrokeWindowCommand struct {
	MinPercent       int  `json:"min_percent"`
	MaxPercent       int  `json:"max_percent"`
	ReverseDirection bool `json:"reverse_direction,omitempty"`
}

// AppendPointsCommand appends timed points to a stream.
type AppendPointsCommand struct {
	StreamID string       `json:"stream_id"`
	Points   []TimedPoint `json:"points"`
}

// PlayCommand starts or resumes playback of a timed-point stream.
type PlayCommand struct {
	StreamID         string `json:"stream_id"`
	StartTimeMillis  int64  `json:"start_time_ms"`
	ServerTimeMillis int64  `json:"server_time_ms,omitempty"`
}

// HSPSetupCommand prepares the active HSP stream on API v3 transports.
type HSPSetupCommand struct {
	StreamID uint32 `json:"stream_id"`
}

// Command is the safe serialized transport command shape used for diagnostics.
type Command struct {
	ID           string               `json:"id"`
	Kind         CommandKind          `json:"kind"`
	IssuedAt     string               `json:"issued_at"`
	Stop         *StopCommand         `json:"stop,omitempty"`
	StrokeWindow *StrokeWindowCommand `json:"stroke_window,omitempty"`
	HSPSetup     *HSPSetupCommand     `json:"hsp_setup,omitempty"`
	PointsAdd    *AppendPointsCommand `json:"points_add,omitempty"`
	PointsPlay   *PlayCommand         `json:"points_play,omitempty"`
}

// CommandResult captures safe transport command outcome details.
type CommandResult struct {
	CommandID     string      `json:"command_id"`
	Kind          CommandKind `json:"kind"`
	Transport     string      `json:"transport"`
	OK            bool        `json:"ok"`
	Status        string      `json:"status"`
	LatencyMillis int64       `json:"latency_ms"`
	Error         string      `json:"error,omitempty"`
	CompletedAt   string      `json:"completed_at"`
}

// TransportDiagnostics is a safe snapshot of transport command and playback state.
//
//revive:disable-next-line:exported -- Phase 3 explicitly names this contract.
type TransportDiagnostics struct {
	Name              string         `json:"name"`
	Connected         bool           `json:"connected"`
	PlaybackState     string         `json:"playback_state"`
	CommandCount      int            `json:"command_count"`
	LastLatencyMillis int64          `json:"last_latency_ms"`
	LastCommand       *Command       `json:"last_command,omitempty"`
	LastResult        *CommandResult `json:"last_result,omitempty"`
	LastError         string         `json:"last_error,omitempty"`
}

// SafeCommand returns a diagnostics-safe copy of command.
func SafeCommand(command Command) Command {
	command = cloneCommand(command)
	if command.Stop != nil && command.Stop.Reason != "" {
		command.Stop.Reason = "redacted"
	}
	return command
}

// SafeCommandResult returns a diagnostics-safe copy of result.
func SafeCommandResult(result CommandResult) CommandResult {
	if result.Error != "" {
		result.Error = "redacted"
	}
	return result
}
