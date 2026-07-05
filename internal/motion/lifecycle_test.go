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

// blockingPlayTransport blocks in PlayHSP until released, so a test can hold
// Start inside its transport-setup window (after running is published, before
// the loop launches) and interleave a concurrent Stop.
type blockingPlayTransport struct {
	*transport.Fake
	entered chan struct{}
	release chan struct{}
	once    int32
}

func (b *blockingPlayTransport) PlayHSP(ctx context.Context, command transport.HSPPlayCommand) (transport.CommandResult, error) {
	if atomic.CompareAndSwapInt32(&b.once, 0, 1) {
		close(b.entered)
		<-b.release
	}
	return b.Fake.PlayHSP(ctx, command)
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

	// Wait until Start is blocked inside PlayHSP: running is published but the
	// loop goroutine has not launched yet.
	select {
	case <-blocking.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("Start never reached PlayHSP")
	}

	// This must not panic on a nil cancel.
	stopped, err := engine.Stop(context.Background(), "concurrent_stop")
	if err != nil {
		t.Fatalf("Stop during startup: %v", err)
	}
	if stopped.Running {
		t.Fatalf("state = %+v, want stopped", stopped)
	}

	close(blocking.release)
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
	if plays := countCommands(fake.Commands(), transport.CommandKindHSPPlay); plays != 0 {
		t.Fatalf("HSP play commands = %d after concurrent stop, want 0 (motion must not restart)", plays)
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
