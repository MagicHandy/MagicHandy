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
	historyLimit  int
	commandCount  int
	resultCount   int
	nextID        int
	commands      []Command
	results       []CommandResult
	playbackState string
	lastError     string
	tempo         bool
}

// NewFake returns a fake transport with deterministic command recording.
func NewFake(options ...FakeOption) *Fake {
	fake := &Fake{
		clock:         time.Now,
		nextID:        1,
		playbackState: "idle",
	}
	for _, option := range options {
		option(fake)
	}
	return fake
}

// NewSimulator creates a runtime fake that retains the latest 64 commands and
// results. Lifetime counters also cover the commands evicted from either ring.
func NewSimulator() *Fake {
	return NewFake(WithHistoryLimit(64))
}

// WithHistoryLimit retains the most recent commands and results in bounded rings.
// NewFake without this option keeps the complete history for deterministic tests.
func WithHistoryLimit(limit int) FakeOption {
	return func(fake *Fake) { fake.historyLimit = max(1, limit) }
}

// WithClock sets the fake transport clock.
func WithClock(clock func() time.Time) FakeOption {
	return func(fake *Fake) {
		if clock != nil {
			fake.clock = clock
		}
	}
}

// WithTempo makes the fake a tempo owner, like the VacuGlide (ADR 0071).
func WithTempo() FakeOption {
	return func(fake *Fake) { fake.tempo = true }
}

// TakesTempo reports whether the fake plays the tempo channel.
func (f *Fake) TakesTempo() bool { return f.tempo }

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

// AppendPoints records a timed-point append command.
func (f *Fake) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	recorded := Command{
		Kind:      CommandKindPointsAdd,
		PointsAdd: &command,
	}
	return f.record(ctx, recorded, "buffered")
}

// Play records a timed-point play command.
func (f *Fake) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	recorded := Command{
		Kind:       CommandKindPointsPlay,
		PointsPlay: &command,
	}
	return f.record(ctx, recorded, "playing")
}

// Commands returns the recorded safe command snapshots.
func (f *Fake) Commands() []Command {
	f.mu.Lock()
	defer f.mu.Unlock()

	return cloneCommands(orderedFakeHistory(f.commands, f.commandCount))
}

// Results returns the recorded command results.
func (f *Fake) Results() []CommandResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	results := make([]CommandResult, len(f.results))
	copy(results, orderedFakeHistory(f.results, f.resultCount))
	return results
}

// Diagnostics returns the current fake transport diagnostics.
func (f *Fake) Diagnostics() TransportDiagnostics {
	f.mu.Lock()
	defer f.mu.Unlock()

	var lastCommand *Command
	if len(f.commands) > 0 {
		command := SafeCommand(f.commands[(f.commandCount-1)%len(f.commands)])
		lastCommand = &command
	}

	var lastResult *CommandResult
	var lastLatency int64
	if len(f.results) > 0 {
		result := SafeCommandResult(f.results[(f.resultCount-1)%len(f.results)])
		lastLatency = result.LatencyMillis
		lastResult = &result
	}

	return TransportDiagnostics{
		Name:              fakeTransportName,
		Connected:         true,
		PlaybackState:     f.playbackState,
		CommandCount:      f.commandCount,
		LastLatencyMillis: lastLatency,
		LastCommand:       lastCommand,
		LastResult:        lastResult,
		LastError:         redactedError(f.lastError),
	}
}

func (f *Fake) record(ctx context.Context, command Command, nextPlaybackState string) (CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return f.recordFailure(command.Kind, err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	command.ID = fmt.Sprintf("fake-%06d", f.nextID)
	command.IssuedAt = f.clock().UTC().Format(time.RFC3339Nano)
	f.nextID++
	f.commands = retainFakeHistory(f.commands, f.commandCount, f.historyLimit, cloneCommand(command))
	f.commandCount++
	f.playbackState = nextPlaybackState
	f.lastError = ""

	result := CommandResult{
		CommandID:     command.ID,
		Kind:          command.Kind,
		Transport:     fakeTransportName,
		OK:            true,
		Status:        "recorded",
		LatencyMillis: f.latency.Milliseconds(),
		CompletedAt:   f.clock().Add(f.latency).UTC().Format(time.RFC3339Nano),
	}
	f.results = retainFakeHistory(f.results, f.resultCount, f.historyLimit, result)
	f.resultCount++
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
		CompletedAt: f.clock().UTC().Format(time.RFC3339Nano),
	}
	f.results = retainFakeHistory(f.results, f.resultCount, f.historyLimit, result)
	f.resultCount++
	return result, err
}

func (f *Fake) currentPlaybackState() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playbackState
}

func retainFakeHistory[T any](history []T, count, limit int, value T) []T {
	if limit == 0 || len(history) < limit {
		return append(history, value)
	}
	history[count%limit] = value
	return history
}

func orderedFakeHistory[T any](history []T, count int) []T {
	if count <= len(history) {
		return history
	}
	start := count % len(history)
	ordered := make([]T, 0, len(history))
	ordered = append(ordered, history[start:]...)
	return append(ordered, history[:start]...)
}

func cloneAppendPoints(command AppendPointsCommand) *AppendPointsCommand {
	clone := command
	if command.Air != nil {
		air := *command.Air
		clone.Air = &air
	}
	if command.Tempo != nil {
		frame := *command.Tempo
		frame.Points = append([]TimedLevel(nil), frame.Points...)
		clone.Tempo = &frame
	}
	if command.Vibration != nil {
		frame := *command.Vibration
		frame.Points = append([]TimedLevel(nil), frame.Points...)
		clone.Vibration = &frame
	}
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
	if command.HSPSetup != nil {
		setup := *command.HSPSetup
		command.HSPSetup = &setup
	}
	if command.PointsAdd != nil {
		command.PointsAdd = cloneAppendPoints(*command.PointsAdd)
	}
	if command.PointsPlay != nil {
		play := *command.PointsPlay
		command.PointsPlay = &play
	}
	return command
}

func redactedError(value string) string {
	if value == "" {
		return ""
	}
	return "redacted"
}
