package modes

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
)

func TestCanceledModeStartPreservesActiveMode(t *testing.T) {
	manager := newTestManager(t, &fakeEngine{}, &fakeClock{now: time.Now()}, diagnostics.NewTraceRing(16))
	if _, err := manager.Start(context.Background(), ModeChat); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err := manager.Start(ctx, ModeFreestyle)
	if !errors.Is(err, context.Canceled) || status.Mode != ModeChat {
		t.Fatalf("canceled start = (%+v, %v), want existing chat mode and cancellation", status, err)
	}
}

func TestModeStartCanceledWhileWaitingForControl(t *testing.T) {
	manager := newTestManager(t, &fakeEngine{}, &fakeClock{now: time.Now()}, diagnostics.NewTraceRing(16))
	if _, err := manager.Start(context.Background(), ModeChat); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	previousIntent := manager.userPauseID
	manager.mu.Unlock()
	manager.userControlMu.Lock()
	releaseControl := sync.OnceFunc(manager.userControlMu.Unlock)
	defer releaseControl()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := manager.Start(ctx, ModeFreestyle)
		done <- err
	}()
	// Wait for admission registration, before the start can take the control gate.
	waitFor(t, time.Second, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		return manager.userPauseID != previousIntent
	})
	cancel()
	releaseControl()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("queued start error = %v, want cancellation", err)
	}
	if status := manager.Status(); status.Mode != ModeChat {
		t.Fatalf("canceled queued start replaced the active mode: %+v", status)
	}
}

func TestModeStartCanceledWhileDrainingPreviousLoop(t *testing.T) {
	manager := newTestManager(t, &fakeEngine{}, &fakeClock{now: time.Now()}, diagnostics.NewTraceRing(16))
	stopping := make(chan struct{})
	drained := make(chan struct{})
	releaseDrain := sync.OnceFunc(func() { close(drained) })
	defer releaseDrain()
	// Model a previous loop that needs time to finish cancellation.
	manager.mode = ModeChat
	manager.cancel = func() { close(stopping) }
	manager.done = drained
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := manager.Start(ctx, ModeFreestyle)
		done <- err
	}()
	select {
	case <-stopping:
	case <-time.After(time.Second):
		t.Fatal("replacement did not cancel the previous loop")
	}
	cancel()
	releaseDrain()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("start after drain error = %v, want cancellation", err)
	}
	if status := manager.Status(); status.Mode != "" {
		t.Fatalf("canceled replacement launched a new mode: %+v", status)
	}
}

func TestAcceptedModeOutlivesCompletedRequest(t *testing.T) {
	manager := newTestManager(t, &fakeEngine{}, &fakeClock{now: time.Now()}, diagnostics.NewTraceRing(16))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := manager.Start(ctx, ModeChat); err != nil {
		t.Fatal(err)
	}
	cancel()
	if status := manager.Status(); status.Mode != ModeChat {
		t.Fatalf("accepted mode ended with its HTTP request: %+v", status)
	}
}
