package motion

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func resetTestPosition(position float64) transport.MotionStartupState {
	return transport.MotionStartupState{PositionAbsolute: position, PositionWithinStrokePercent: position,
		StrokeMaxPercent: 100, StrokeMaxAbsolute: 100}
}

func failedPositionEngine(t *testing.T, owner transport.Transport) (*Engine, string) {
	t.Helper()
	e := newTestEngine(t, owner, diagnostics.NewTraceRing(128), time.Hour)
	e.startupWait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	state, err := e.Start(t.Context(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, config.DefaultSettings().Motion)
	if !errors.Is(err, ErrStartupArrival) || state.StartupRecovery == nil || state.Running {
		t.Fatalf("missing recoverable stopped failure: state=%+v err=%v", state.StartupRecovery, err)
	}
	return e, state.StartupRecovery.ID
}

func TestResetStartupPositionOnlyStopsAndReads(t *testing.T) {
	owner := &startupStateTransport{Fake: transport.NewFake(), states: []transport.MotionStartupState{resetTestPosition(50)}}
	e, id := failedPositionEngine(t, owner)
	before, reads := len(owner.Commands()), owner.ReadCount()
	state, err := e.ResetStartupPosition(t.Context(), id)
	if err != nil || state.Running || state.Paused || state.StartupRecovery != nil || state.LastError != "" {
		t.Fatalf("reset = %+v, %v", state, err)
	}
	commands := owner.Commands()[before:]
	if len(commands) != 1 || commands[0].Kind != transport.CommandKindStop || owner.ReadCount() <= reads {
		t.Fatalf("reset must only Stop and read: commands=%+v reads=%d", commands, owner.ReadCount()-reads)
	}
	if _, err := e.ResetStartupPosition(t.Context(), id); !errors.Is(err, ErrStartupRecoveryUnavailable) {
		t.Fatalf("old acknowledgement reused: %v", err)
	}
	// Reset is not an override: the next start must still verify arrival.
	state, err = e.Start(t.Context(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, config.DefaultSettings().Motion)
	if !errors.Is(err, ErrStartupArrival) || state.StartupRecovery == nil || state.StartupRecovery.ID == id {
		t.Fatalf("reset bypassed arrival or reused the old failure: %+v %v", state.StartupRecovery, err)
	}
	before = len(owner.Commands())
	if _, err := e.ResetStartupPosition(t.Context(), id); !errors.Is(err, ErrStartupRecoveryUnavailable) || len(owner.Commands()) != before {
		t.Fatalf("stale reset touched a new failure: %v", err)
	}
	owner.mu.Lock()
	owner.states = []transport.MotionStartupState{resetTestPosition(0)}
	owner.mu.Unlock()
	if _, err := e.ResetStartupPosition(t.Context(), state.StartupRecovery.ID); err != nil {
		t.Fatal(err)
	}
	state, err = e.Start(t.Context(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, config.DefaultSettings().Motion)
	if err != nil || !state.Running || state.StartupRecovery != nil {
		t.Fatalf("explicit start after successful recheck = %v, %v", state.Running, err)
	}
	_, _ = e.Stop(t.Context(), "test_done")
}

func TestResetStartupPositionRetainsFaultForBadReadings(t *testing.T) {
	for _, name := range []string{"moving", "nonfinite", "outside_travel", "invalid_calibration"} {
		t.Run(name, func(t *testing.T) {
			owner := &startupStateTransport{Fake: transport.NewFake(), states: []transport.MotionStartupState{resetTestPosition(50)}}
			e, id := failedPositionEngine(t, owner)
			bad := resetTestPosition(50)
			switch name {
			case "moving":
				bad.SpeedAbsolute = 20
			case "nonfinite":
				bad.PositionAbsolute = math.NaN()
			case "outside_travel":
				bad.PositionAbsolute = 120
			case "invalid_calibration":
				bad.StrokeMaxAbsolute = 0
			}
			owner.mu.Lock()
			owner.states = []transport.MotionStartupState{bad}
			owner.mu.Unlock()
			before := len(owner.Commands())
			state, err := e.ResetStartupPosition(t.Context(), id)
			if err == nil || state.StartupRecovery == nil || state.StartupRecovery.Resetting || state.LastError == "" || state.Running {
				t.Fatalf("bad recheck acknowledged: %+v %v", state, err)
			}
			commands := owner.Commands()[before:]
			if len(commands) != 1 || commands[0].Kind != transport.CommandKindStop {
				t.Fatalf("recheck generated motion: %+v", commands)
			}
		})
	}
}

type resetBlockingTransport struct {
	*startupStateTransport
	gateMu      sync.Mutex
	readEntered chan struct{}
	stopFailure bool
}

func (b *resetBlockingTransport) ReadMotionStartupState(ctx context.Context) (transport.MotionStartupState, transport.MotionStartupStateResults, error) {
	b.gateMu.Lock()
	entered := b.readEntered
	b.gateMu.Unlock()
	if entered != nil {
		close(entered)
		<-ctx.Done()
		return transport.MotionStartupState{}, transport.MotionStartupStateResults{}, ctx.Err()
	}
	return b.startupStateTransport.ReadMotionStartupState(ctx)
}

func (b *resetBlockingTransport) Stop(ctx context.Context, command transport.StopCommand) (transport.CommandResult, error) {
	b.gateMu.Lock()
	fail := b.stopFailure
	b.gateMu.Unlock()
	if fail {
		return transport.CommandResult{Kind: transport.CommandKindStop}, errors.New("Stop failed")
	}
	return b.startupStateTransport.Stop(ctx, command)
}

func TestStopCancelsPositionResetAndPreventsStaleAcknowledgement(t *testing.T) {
	owner := &resetBlockingTransport{startupStateTransport: &startupStateTransport{
		Fake: transport.NewFake(), states: []transport.MotionStartupState{resetTestPosition(50)},
	}}
	e, id := failedPositionEngine(t, owner)
	entered := make(chan struct{})
	owner.gateMu.Lock()
	owner.readEntered = entered
	owner.gateMu.Unlock()
	done := make(chan error, 1)
	go func() { _, err := e.ResetStartupPosition(t.Context(), id); done <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reset did not start reading")
	}
	if !e.Snapshot().StartupRecovery.Resetting {
		t.Fatal("reset status was not published")
	}
	if _, err := e.ResetStartupPosition(t.Context(), id); !errors.Is(err, ErrStartupRecoveryUnavailable) {
		t.Fatalf("duplicate reset admitted: %v", err)
	}
	if _, err := e.Start(t.Context(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, config.DefaultSettings().Motion); err == nil {
		t.Fatal("start crossed the position reset")
	}
	if _, err := e.Stop(t.Context(), "user_stop"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("reset error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the read")
	}
	state := e.Snapshot()
	if state.Running || state.StartupRecovery == nil || state.StartupRecovery.Resetting {
		t.Fatalf("canceled reset cleared the failure: %+v", state.StartupRecovery)
	}
	commands := owner.Commands()
	if commands[len(commands)-1].Stop.Reason != "user_stop" {
		t.Fatal("Stop was not the final command")
	}
}

func TestPositionResetRequiresConfirmedStop(t *testing.T) {
	owner := &resetBlockingTransport{startupStateTransport: &startupStateTransport{
		Fake: transport.NewFake(), states: []transport.MotionStartupState{resetTestPosition(50)},
	}}
	e, id := failedPositionEngine(t, owner)
	owner.gateMu.Lock()
	owner.stopFailure = true
	owner.gateMu.Unlock()
	reads := owner.ReadCount()
	state, err := e.ResetStartupPosition(t.Context(), id)
	if err == nil || state.StartupRecovery == nil || owner.ReadCount() != reads {
		t.Fatalf("unconfirmed Stop allowed recheck: state=%+v err=%v", state.StartupRecovery, err)
	}
}

func TestPositionResetRechecksEveryLinkedSlider(t *testing.T) {
	first := &startupStateTransport{Fake: transport.NewFake(), states: []transport.MotionStartupState{resetTestPosition(50)}}
	second := &startupStateTransport{Fake: transport.NewFake(), states: []transport.MotionStartupState{resetTestPosition(40)}}
	owner := transport.NewLinkedSession([]transport.LinkedParticipant{
		{ID: "first", Label: "First Handy", Transport: first},
		{ID: "second", Label: "Second Handy", Transport: second},
	}, false)
	e, id := failedPositionEngine(t, owner)
	beforeFirst, beforeSecond := len(first.Commands()), len(second.Commands())
	readsFirst, readsSecond := first.ReadCount(), second.ReadCount()
	state, err := e.ResetStartupPosition(t.Context(), id)
	if err != nil || state.StartupRecovery != nil || state.Running {
		t.Fatalf("linked reset: %v", err)
	}
	if first.ReadCount() <= readsFirst || second.ReadCount() <= readsSecond {
		t.Fatal("a linked slider was not rechecked")
	}
	for _, commands := range [][]transport.Command{first.Commands()[beforeFirst:], second.Commands()[beforeSecond:]} {
		if len(commands) != 1 || commands[0].Kind != transport.CommandKindStop {
			t.Fatalf("linked recheck moved a device: %+v", commands)
		}
	}
}

func TestPositionResetCannotCrossAnEarlierStop(t *testing.T) {
	owner := &startupStateTransport{Fake: transport.NewFake(), states: []transport.MotionStartupState{resetTestPosition(50)}}
	e, id := failedPositionEngine(t, owner)
	admission := e.AdmissionGeneration()
	if _, err := e.Stop(t.Context(), "newer_stop"); err != nil {
		t.Fatal(err)
	}
	before := len(owner.Commands())
	state, err := e.ResetStartupPositionAtGeneration(t.Context(), id, admission)
	if !errors.Is(err, ErrStartupRecoveryUnavailable) || state.StartupRecovery == nil || len(owner.Commands()) != before {
		t.Fatalf("reset crossed Stop: %v, %+v", err, state.StartupRecovery)
	}
}

func TestPositionRecoveryDistinguishesInvalidReadingsAndMovingDevice(t *testing.T) {
	for _, kind := range []string{"invalid_telemetry", "moving_after_stop"} {
		state := resetTestPosition(50)
		if kind == "invalid_telemetry" {
			state.StrokeMaxAbsolute = 0
		} else {
			state.SpeedAbsolute = 20
		}
		owner := &startupStateTransport{Fake: transport.NewFake(), states: []transport.MotionStartupState{state}}
		e := newTestEngine(t, owner, diagnostics.NewTraceRing(32), time.Hour)
		e.startupWait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
		got, err := e.Start(t.Context(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, config.DefaultSettings().Motion)
		if err == nil || got.StartupRecovery == nil || got.StartupRecovery.Kind != kind {
			t.Fatalf("wanted %s: %+v %v", kind, got.StartupRecovery, err)
		}
	}
}
