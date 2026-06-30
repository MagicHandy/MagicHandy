package transport

import "context"

// CommandKind identifies the physical transport command family.
type CommandKind string

const (
	// CommandKindStop stops active transport playback.
	CommandKindStop CommandKind = "stop"
	// CommandKindStrokeWindow applies a physical stroke envelope.
	CommandKindStrokeWindow CommandKind = "stroke_window"
	// CommandKindHSPAdd appends HSP timed points to a stream.
	CommandKindHSPAdd CommandKind = "hsp_add"
	// CommandKindHSPPlay starts or resumes an HSP stream.
	CommandKindHSPPlay CommandKind = "hsp_play"
)

// Transport is the deterministic boundary used by motion code to command Handy
// playback without knowing the concrete dispatch owner.
type Transport interface {
	DiagnosticsProvider

	Stop(context.Context, StopCommand) (CommandResult, error)
	SetStrokeWindow(context.Context, StrokeWindowCommand) (CommandResult, error)
	AddHSP(context.Context, HSPAddCommand) (CommandResult, error)
	PlayHSP(context.Context, HSPPlayCommand) (CommandResult, error)
}

// DiagnosticsProvider exposes a safe transport diagnostics snapshot.
type DiagnosticsProvider interface {
	Diagnostics() TransportDiagnostics
}

// TimedPoint is a single HSP timed point. Position is expressed as 0..100.
type TimedPoint struct {
	PositionPercent int   `json:"position_percent"`
	TimeMillis      int64 `json:"time_ms"`
}

// StopCommand requests transport stop.
type StopCommand struct {
	Reason string `json:"reason"`
}

// StrokeWindowCommand applies the physical stroke envelope at transport level.
type StrokeWindowCommand struct {
	MinPercent int `json:"min_percent"`
	MaxPercent int `json:"max_percent"`
}

// HSPAddCommand appends timed points to an HSP stream.
type HSPAddCommand struct {
	StreamID string       `json:"stream_id"`
	Points   []TimedPoint `json:"points"`
}

// HSPPlayCommand starts or resumes playback of an HSP stream.
type HSPPlayCommand struct {
	StreamID        string `json:"stream_id"`
	StartTimeMillis int64  `json:"start_time_ms"`
}

// Command is the safe serialized transport command shape used for diagnostics.
type Command struct {
	ID           string               `json:"id"`
	Kind         CommandKind          `json:"kind"`
	IssuedAt     string               `json:"issued_at"`
	Stop         *StopCommand         `json:"stop,omitempty"`
	StrokeWindow *StrokeWindowCommand `json:"stroke_window,omitempty"`
	HSPAdd       *HSPAddCommand       `json:"hsp_add,omitempty"`
	HSPPlay      *HSPPlayCommand      `json:"hsp_play,omitempty"`
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
