package motion

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestEnginePauseStopsDeviceAndFreezesState(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(128)
	engine := newTestEngine(t, fake, traces, 5*time.Millisecond)

	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(25 * time.Millisecond)

	state, err := engine.Pause(context.Background(), "test_pause")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if state.Running || !state.Paused {
		t.Fatalf("state = %+v, want paused and not running", state)
	}
	if countCommands(fake.Commands(), transport.CommandKindStop) != 1 {
		t.Fatalf("commands = %+v, want one transport stop on pause", fake.Commands())
	}

	// No more dispatch after pause: the loop is dead.
	before := len(fake.Commands())
	time.Sleep(25 * time.Millisecond)
	if got := len(fake.Commands()); got != before {
		t.Fatalf("commands after pause grew from %d to %d", before, got)
	}

	// Pause is idempotent.
	if _, err := engine.Pause(context.Background(), "again"); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
}

func TestEngineResumePreservesTargetAndPhase(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(128)
	engine := newTestEngine(t, fake, traces, time.Hour)
	defer func() {
		_, _ = engine.Stop(context.Background(), "cleanup")
	}()

	target := testTarget()
	if _, err := engine.Start(context.Background(), target, config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}

	paused, err := engine.Pause(context.Background(), "test_pause")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	frozenPhase := paused.Phase

	resumed, err := engine.Resume(context.Background(), "test_resume")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !resumed.Running || resumed.Paused {
		t.Fatalf("state = %+v, want running after resume", resumed)
	}
	if resumed.Target.PatternID != target.PatternID {
		t.Fatalf("resumed target = %+v, want original pattern %q", resumed.Target, target.PatternID)
	}

	// The resumed plan continues from the frozen phase: its phase at stream
	// start equals the pause-time phase (modulo the samples already buffered).
	engine.mu.Lock()
	resumedStartPhase := engine.plan.PhaseOffset
	phasePreserved := engine.plan.PhasePreserved
	engine.mu.Unlock()
	delta := resumedStartPhase - frozenPhase
	if delta < 0 {
		delta = -delta
	}
	// frozenPhase came from the pre-pause snapshot's buffered tail; allow the
	// gap between estimated playback and buffered tail.
	if !phasePreserved {
		t.Fatal("resumed plan did not mark phase preserved")
	}
	if delta > 1.0 {
		t.Fatalf("resume phase delta = %v (start %v, frozen %v)", delta, resumedStartPhase, frozenPhase)
	}

	// A fresh play command went out for the new stream.
	if countCommands(fake.Commands(), transport.CommandKindPointsPlay) != 2 {
		t.Fatalf("commands = %+v, want a second HSP play after resume", fake.Commands())
	}
	assertTraceAnnotation(t, traces.Rows(), "test_resume", "phase_preserved=true")
}

func TestEngineStopClearsPausedStateAndRunClock(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(128)
	engine := newTestEngine(t, fake, traces, time.Hour)

	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := engine.Pause(context.Background(), "test_pause"); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	state, err := engine.Stop(context.Background(), "test_stop")
	if err != nil {
		t.Fatalf("Stop from paused: %v", err)
	}
	if state.Running || state.Paused {
		t.Fatalf("state = %+v, want fully stopped", state)
	}
	if state.RunningMillis != 0 {
		t.Fatalf("running_ms after stop = %d, want 0", state.RunningMillis)
	}
	if _, err := engine.Resume(context.Background(), "after_stop"); err == nil {
		t.Fatal("Resume succeeded after Stop; paused state should be cleared")
	}
	// Stop from paused still sends the unconditional safety stop.
	if countCommands(fake.Commands(), transport.CommandKindStop) != 2 {
		t.Fatalf("commands = %+v, want pause stop plus explicit stop", fake.Commands())
	}
}

func TestEngineRunClockAccumulatesAcrossPause(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(8)
	engine := newTestEngine(t, fake, traces, time.Hour)
	defer func() {
		_, _ = engine.Stop(context.Background(), "cleanup")
	}()

	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	paused, err := engine.Pause(context.Background(), "clock_pause")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.RunningMillis <= 0 {
		t.Fatalf("running_ms while paused = %d, want > 0", paused.RunningMillis)
	}
	frozen := paused.RunningMillis
	time.Sleep(20 * time.Millisecond)
	if got := engine.Snapshot().RunningMillis; got != frozen {
		t.Fatalf("running_ms advanced while paused: %d -> %d", frozen, got)
	}

	if _, err := engine.Resume(context.Background(), "clock_resume"); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if got := engine.Snapshot().RunningMillis; got <= frozen {
		t.Fatalf("running_ms did not accumulate after resume: %d <= %d", got, frozen)
	}
}
