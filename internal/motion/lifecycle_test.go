package motion

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// blockingPlayTransport blocks in Play until released, so a test can hold
// Start inside its transport-setup window (after running is published, before
// the loop launches) and interleave a concurrent Stop.
type blockingPlayTransport struct {
	*transport.Fake
	entered chan struct{}
	release chan struct{}
	once    int32
}

type delayedPlayTransport struct {
	*transport.Fake
	delay     time.Duration
	startedAt time.Time
}

func (d *delayedPlayTransport) Play(ctx context.Context, command transport.PlayCommand) (transport.CommandResult, error) {
	timer := time.NewTimer(d.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return transport.CommandResult{}, ctx.Err()
	case <-timer.C:
		result, err := d.Fake.Play(ctx, command)
		d.startedAt = time.Now()
		return result, err
	}
}

func (d *delayedPlayTransport) PlaybackStartTime() time.Time { return d.startedAt }

func TestEnginePlaybackClockStartsAfterTransportPlay(t *testing.T) {
	commandTransport := &delayedPlayTransport{Fake: transport.NewFake(), delay: 40 * time.Millisecond}
	engine := newTestEngine(t, commandTransport, diagnostics.NewTraceRing(32), time.Hour)
	started := time.Now()
	state, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = engine.Stop(context.Background(), "clock_test_cleanup") }()
	if time.Since(started) < 35*time.Millisecond {
		t.Fatal("transport Play did not exercise the startup delay")
	}
	if state.RunningMillis > 15 {
		t.Fatalf("running clock advanced %dms during transport setup", state.RunningMillis)
	}
}

func (b *blockingPlayTransport) Play(ctx context.Context, command transport.PlayCommand) (transport.CommandResult, error) {
	if atomic.CompareAndSwapInt32(&b.once, 0, 1) {
		close(b.entered)
		<-b.release
	}
	return b.Fake.Play(ctx, command)
}

// TestConcurrentStopDuringStartupDoesNotPanic reproduces the startup race: a
// Stop that arrives while Start is still in transport setup must find a real
// cancel handle, not a nil one. Before the fix this panicked on a nil
// context.CancelFunc.
func TestConcurrentStopDuringStartupDoesNotPanic(t *testing.T) {
	fake := transport.NewFake()
	blocking := &blockingPlayTransport{
		Fake:    fake,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	engine := newTestEngine(t, blocking, diagnostics.NewTraceRing(64), time.Hour)

	startDone := make(chan struct{})
	go func() {
		defer close(startDone)
		_, _ = engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion)
	}()

	// Wait until Start is blocked inside Play: running is published but the
	// loop goroutine has not launched yet.
	select {
	case <-blocking.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("Start never reached Play")
	}
	if state := engine.Snapshot(); !state.Running || !state.Starting {
		t.Fatalf("startup state = %+v, want running setup distinguished from playback", state)
	}
	if _, err := engine.ApplyTarget(context.Background(), testTarget(), "startup_retarget"); err == nil {
		t.Fatal("retarget was admitted before startup Play completed")
	}

	// Stop cancels the run immediately but waits for the in-flight transport
	// command to drain before it sends the final wire Stop.
	type stopOutcome struct {
		state ActiveMotionState
		err   error
	}
	stopDone := make(chan stopOutcome, 1)
	go func() {
		state, err := engine.Stop(context.Background(), "concurrent_stop")
		stopDone <- stopOutcome{state: state, err: err}
	}()
	waitForEngineState(t, engine, time.Second, func(state ActiveMotionState) bool { return !state.Running })
	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err == nil {
		t.Fatal("new Start was admitted while Stop was waiting for the wire barrier")
	}
	select {
	case outcome := <-stopDone:
		t.Fatalf("Stop returned before the blocked Play drained: %+v", outcome)
	case <-time.After(30 * time.Millisecond):
	}

	close(blocking.release)
	select {
	case outcome := <-stopDone:
		if outcome.err != nil {
			t.Fatalf("Stop during startup: %v", outcome.err)
		}
		if outcome.state.Running {
			t.Fatalf("state = %+v, want stopped", outcome.state)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return after blocked Play drained")
	}
	select {
	case <-startDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after release")
	}

	// The loop never launched (startLoop saw running=false), so no goroutine
	// keeps dispatching; goleak in TestMain confirms none leaked.
	if engine.Snapshot().Running {
		t.Fatal("engine still running after concurrent stop")
	}
	if countCommands(fake.Commands(), transport.CommandKindStop) == 0 {
		t.Fatal("no transport stop recorded for the concurrent stop")
	}

	// Motion must not restart: the play() that was in-flight when Stop landed
	// runs on the loop context Stop cancelled, so it aborts instead of sending
	// an HSP play after the stop. A play recorded here means startup work
	// restarted motion the user just stopped.
	if plays := countCommands(fake.Commands(), transport.CommandKindPointsPlay); plays != 0 {
		t.Fatalf("HSP play commands = %d after concurrent stop, want 0 (motion must not restart)", plays)
	}
}

func TestStopInvalidatesPreviouslyAdmittedStart(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(32), time.Hour)
	admission := engine.AdmissionGeneration()
	if _, err := engine.Stop(context.Background(), "admission_test"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := engine.StartAtGeneration(context.Background(), testTarget(), config.DefaultSettings().Motion, admission); err == nil {
		t.Fatal("a Start admitted before Stop was accepted after Stop")
	}
	for _, command := range fake.Commands() {
		if command.Kind != transport.CommandKindStop {
			t.Fatalf("invalidated Start issued command after Stop: %+v", command)
		}
	}
}

// recoveryTransport reports unhealthy playback on demand so the dispatch loop
// triggers recovery from inside the loop goroutine.
type recoveryTransport struct {
	*transport.Fake
	unhealthy atomic.Bool
}

func (r *recoveryTransport) Diagnostics() transport.TransportDiagnostics {
	diag := r.Fake.Diagnostics()
	if r.unhealthy.Load() {
		diag.PlaybackState = "starved"
	}
	return diag
}

// TestLoopTriggeredRecoveryDoesNotDeadlock covers the recovery path when the
// dispatch loop itself detects unhealthy playback. The recovery stop must not
// wait on the loop goroutine's own done channel; if it did, the loop goroutine
// would wait on itself forever.
func TestLoopTriggeredRecoveryDoesNotDeadlock(t *testing.T) {
	fake := transport.NewFake()
	recovery := &recoveryTransport{Fake: fake}
	engine := newTestEngine(t, recovery, diagnostics.NewTraceRing(128), 5*time.Millisecond)

	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Flip playback unhealthy so the next loop tick drives recovery from the
	// loop goroutine.
	recovery.unhealthy.Store(true)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !engine.Snapshot().Running && countCommands(fake.Commands(), transport.CommandKindStop) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("loop-triggered recovery did not stop the engine (deadlock?); commands=%+v", fake.Commands())
}
