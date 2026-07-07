package intiface

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const transportName = "intiface"

// TransportOptions configures linear motion mapping for Intiface dispatch.
type TransportOptions struct {
	ReverseDirection bool
	StrokeMinPercent int
	StrokeMaxPercent int
}

// Transport implements transport.Transport by converting HSP timed points into
// Buttplug LinearCmd moves on a 0..1 axis.
type Transport struct {
	mu sync.Mutex

	client      *Client
	options     TransportOptions
	diagnostics transport.TransportDiagnostics
	nextID      int
	lastTimeMS  int64
}

// NewTransport wraps an Intiface client as a motion transport.
func NewTransport(client *Client, options TransportOptions) *Transport {
	if client == nil {
		panic("intiface client is required")
	}
	return &Transport{
		client: client,
		options: TransportOptions{
			ReverseDirection: options.ReverseDirection,
			StrokeMinPercent: options.StrokeMinPercent,
			StrokeMaxPercent: options.StrokeMaxPercent,
		},
		diagnostics: transport.TransportDiagnostics{
			Name:          transportName,
			PlaybackState: "idle",
		},
	}
}

// Diagnostics returns a safe transport diagnostics snapshot.
func (t *Transport) Diagnostics() transport.TransportDiagnostics {
	t.mu.Lock()
	defer t.mu.Unlock()
	return cloneDiagnostics(t.diagnostics, t.client.Connected())
}

// Stop sends StopAllDevices and marks playback idle.
func (t *Transport) Stop(ctx context.Context, command transport.StopCommand) (transport.CommandResult, error) {
	recorded := transport.Command{
		Kind: transport.CommandKindStop,
		Stop: &command,
	}
	result, err := t.record(ctx, recorded, func(ctx context.Context) error {
		return t.client.StopAllDevices(ctx)
	})
	if err == nil {
		t.setPlaybackState("idle")
	}
	return result, err
}

// SetStrokeWindow stores the semantic stroke envelope used for linear mapping.
func (t *Transport) SetStrokeWindow(ctx context.Context, command transport.StrokeWindowCommand) (transport.CommandResult, error) {
	recorded := transport.Command{
		Kind:         transport.CommandKindStrokeWindow,
		StrokeWindow: &command,
	}
	result, err := t.record(ctx, recorded, func(_ context.Context) error {
		t.mu.Lock()
		t.options.StrokeMinPercent = command.MinPercent
		t.options.StrokeMaxPercent = command.MaxPercent
		t.mu.Unlock()
		return nil
	})
	return result, err
}

// AddHSP converts timed points into queued linear moves.
func (t *Transport) AddHSP(ctx context.Context, command transport.HSPAddCommand) (transport.CommandResult, error) {
	recorded := transport.Command{
		Kind:   transport.CommandKindHSPAdd,
		HSPAdd: cloneHSPAdd(command),
	}
	result, err := t.record(ctx, recorded, func(ctx context.Context) error {
		if len(command.Points) == 0 {
			return nil
		}
		if err := t.client.EnsureConnected(ctx); err != nil {
			return err
		}
		t.mu.Lock()
		options := t.options
		lastTime := t.lastTimeMS
		t.mu.Unlock()

		for index, point := range command.Points {
			durationMS := int64(125)
			if index+1 < len(command.Points) {
				delta := command.Points[index+1].TimeMillis - point.TimeMillis
				if delta > 0 {
					durationMS = delta
				}
			} else if lastTime >= 0 && point.TimeMillis > lastTime {
				delta := point.TimeMillis - lastTime
				if delta > 0 {
					durationMS = delta
				}
			}
			position := mapPosition(point.PositionPercent, options)
			if err := t.client.MoveTo(ctx, position, int(durationMS)); err != nil {
				return err
			}
			lastTime = point.TimeMillis
		}

		t.mu.Lock()
		t.lastTimeMS = lastTime
		t.mu.Unlock()
		t.setPlaybackState("playing")
		return nil
	})
	return result, err
}

// PlayHSP marks playback active; linear moves are dispatched on AddHSP.
func (t *Transport) PlayHSP(ctx context.Context, command transport.HSPPlayCommand) (transport.CommandResult, error) {
	recorded := transport.Command{
		Kind:    transport.CommandKindHSPPlay,
		HSPPlay: &command,
	}
	result, err := t.record(ctx, recorded, func(_ context.Context) error {
		t.setPlaybackState("playing")
		return nil
	})
	return result, err
}

func (t *Transport) record(
	ctx context.Context,
	command transport.Command,
	action func(context.Context) error,
) (transport.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return t.recordFailure(command.Kind, err), err
	}
	start := time.Now()
	t.mu.Lock()
	t.nextID++
	command.ID = fmt.Sprintf("intiface-%06d", t.nextID)
	command.IssuedAt = start.UTC().Format(time.RFC3339Nano)
	t.mu.Unlock()

	err := action(ctx)
	latency := time.Since(start).Milliseconds()
	result := transport.CommandResult{
		CommandID:     command.ID,
		Kind:          command.Kind,
		Transport:     transportName,
		OK:            err == nil,
		Status:        statusFromError(err),
		LatencyMillis: latency,
		CompletedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err != nil {
		result.Error = err.Error()
	}

	t.mu.Lock()
	t.diagnostics.CommandCount++
	t.diagnostics.LastLatencyMillis = latency
	safeCommand := transport.SafeCommand(command)
	t.diagnostics.LastCommand = &safeCommand
	safeResult := transport.SafeCommandResult(result)
	t.diagnostics.LastResult = &safeResult
	if err != nil {
		t.diagnostics.LastError = err.Error()
	} else {
		t.diagnostics.LastError = ""
	}
	t.mu.Unlock()
	return result, err
}

func (t *Transport) recordFailure(kind transport.CommandKind, err error) transport.CommandResult {
	if err == nil {
		err = errors.New("transport command failed")
	}
	return transport.CommandResult{
		Kind:        kind,
		Transport:   transportName,
		OK:          false,
		Status:      "failed",
		Error:       err.Error(),
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (t *Transport) setPlaybackState(state string) {
	t.mu.Lock()
	t.diagnostics.PlaybackState = state
	t.mu.Unlock()
}

func mapPosition(percent int, options TransportOptions) float64 {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if options.ReverseDirection {
		percent = 100 - percent
	}
	strokeMin := options.StrokeMinPercent
	strokeMax := options.StrokeMaxPercent
	if strokeMax < strokeMin {
		strokeMin, strokeMax = strokeMax, strokeMin
	}
	span := float64(strokeMax - strokeMin)
	mapped := float64(strokeMin) + span*(float64(percent)/100.0)
	return mapped / 100.0
}

func statusFromError(err error) string {
	if err == nil {
		return "ok"
	}
	return "failed"
}

func cloneHSPAdd(command transport.HSPAddCommand) *transport.HSPAddCommand {
	clone := command
	clone.Points = make([]transport.TimedPoint, len(command.Points))
	copy(clone.Points, command.Points)
	return &clone
}

func cloneDiagnostics(diagnostics transport.TransportDiagnostics, connected bool) transport.TransportDiagnostics {
	clone := diagnostics
	clone.Connected = connected
	if clone.LastCommand != nil {
		command := transport.SafeCommand(*clone.LastCommand)
		clone.LastCommand = &command
	}
	if clone.LastResult != nil {
		result := transport.SafeCommandResult(*clone.LastResult)
		clone.LastResult = &result
	}
	if clone.LastError != "" {
		clone.LastError = "redacted"
	}
	return clone
}
