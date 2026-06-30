package transport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const fakeTransportName = "fake_handy"

// FakeOption configures the deterministic fake transport.
type FakeOption func(*Fake)

// Fake is an in-memory Handy simulator that records commands and diagnostics.
type Fake struct {
	mu            sync.Mutex
	clock         func() time.Time
	latency       time.Duration
	nextID        int
	commands      []Command
	results       []CommandResult
	playbackState string
	lastError     string
}

// NewFake returns a fake transport with deterministic command recording.
func NewFake(options ...FakeOption) *Fake {
	fake := &Fake{
		clock:         func() time.Time { return time.Now().UTC() },
		nextID:        1,
		playbackState: "idle",
	}
	for _, option := range options {
		option(fake)
	}
	return fake
}

// WithClock sets the fake transport clock.
func WithClock(clock func() time.Time) FakeOption {
	return func(fake *Fake) {
		if clock != nil {
			fake.clock = clock
		}
	}
}

// WithLatency sets the fake transport latency reported in command results.
func WithLatency(latency time.Duration) FakeOption {
	return func(fake *Fake) {
		fake.latency = latency
	}
}

// Stop records a transport stop command.
func (f *Fake) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	recorded := Command{
		Kind: CommandKindStop,
		Stop: &command,
	}
	return f.record(ctx, recorded, "idle")
}

// SetStrokeWindow records a stroke-window command.
func (f *Fake) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	recorded := Command{
		Kind:         CommandKindStrokeWindow,
		StrokeWindow: &command,
	}
	return f.record(ctx, recorded, f.currentPlaybackState())
}

// AddHSP records an HSP add command.
func (f *Fake) AddHSP(ctx context.Context, command HSPAddCommand) (CommandResult, error) {
	recorded := Command{
		Kind:   CommandKindHSPAdd,
		HSPAdd: cloneHSPAdd(command),
	}
	return f.record(ctx, recorded, "buffered")
}

// PlayHSP records an HSP play command.
func (f *Fake) PlayHSP(ctx context.Context, command HSPPlayCommand) (CommandResult, error) {
	recorded := Command{
		Kind:    CommandKindHSPPlay,
		HSPPlay: &command,
	}
	return f.record(ctx, recorded, "playing")
}

// Commands returns the recorded safe command snapshots.
func (f *Fake) Commands() []Command {
	f.mu.Lock()
	defer f.mu.Unlock()

	return cloneCommands(f.commands)
}

// Results returns the recorded command results.
func (f *Fake) Results() []CommandResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	results := make([]CommandResult, len(f.results))
	copy(results, f.results)
	return results
}

// Diagnostics returns the current fake transport diagnostics.
func (f *Fake) Diagnostics() TransportDiagnostics {
	f.mu.Lock()
	defer f.mu.Unlock()

	var lastCommand *Command
	if len(f.commands) > 0 {
		command := SafeCommand(f.commands[len(f.commands)-1])
		lastCommand = &command
	}

	var lastResult *CommandResult
	var lastLatency int64
	if len(f.results) > 0 {
		result := SafeCommandResult(f.results[len(f.results)-1])
		lastLatency = result.LatencyMillis
		lastResult = &result
	}

	return TransportDiagnostics{
		Name:              fakeTransportName,
		Connected:         true,
		PlaybackState:     f.playbackState,
		CommandCount:      len(f.commands),
		LastLatencyMillis: lastLatency,
		LastCommand:       lastCommand,
		LastResult:        lastResult,
		LastError:         f.lastError,
	}
}

func (f *Fake) record(ctx context.Context, command Command, nextPlaybackState string) (CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return f.recordFailure(command.Kind, err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	command.ID = fmt.Sprintf("fake-%06d", f.nextID)
	command.IssuedAt = f.clock().Format(time.RFC3339Nano)
	f.nextID++
	f.commands = append(f.commands, cloneCommand(command))
	f.playbackState = nextPlaybackState
	f.lastError = ""

	result := CommandResult{
		CommandID:     command.ID,
		Kind:          command.Kind,
		Transport:     fakeTransportName,
		OK:            true,
		Status:        "recorded",
		LatencyMillis: f.latency.Milliseconds(),
		CompletedAt:   f.clock().Add(f.latency).Format(time.RFC3339Nano),
	}
	f.results = append(f.results, result)
	return result, nil
}

func (f *Fake) recordFailure(kind CommandKind, err error) (CommandResult, error) {
	if err == nil {
		err = errors.New("transport command failed")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.lastError = err.Error()
	result := CommandResult{
		Kind:        kind,
		Transport:   fakeTransportName,
		OK:          false,
		Status:      "failed",
		Error:       err.Error(),
		CompletedAt: f.clock().Format(time.RFC3339Nano),
	}
	f.results = append(f.results, result)
	return result, err
}

func (f *Fake) currentPlaybackState() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playbackState
}

func cloneHSPAdd(command HSPAddCommand) *HSPAddCommand {
	clone := command
	clone.Points = make([]TimedPoint, len(command.Points))
	copy(clone.Points, command.Points)
	return &clone
}

func cloneCommands(commands []Command) []Command {
	clones := make([]Command, len(commands))
	for index, command := range commands {
		clones[index] = cloneCommand(command)
	}
	return clones
}

func cloneCommand(command Command) Command {
	if command.Stop != nil {
		stop := *command.Stop
		command.Stop = &stop
	}
	if command.StrokeWindow != nil {
		window := *command.StrokeWindow
		command.StrokeWindow = &window
	}
	if command.HSPAdd != nil {
		command.HSPAdd = cloneHSPAdd(*command.HSPAdd)
	}
	if command.HSPPlay != nil {
		play := *command.HSPPlay
		command.HSPPlay = &play
	}
	return command
}
