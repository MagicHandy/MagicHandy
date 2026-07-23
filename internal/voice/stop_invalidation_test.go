package voice

import (
	"testing"
	"time"
)

// Emergency Stop invalidates speech through Manager.InvalidateAll. A submission
// that is waiting on its supervisor — which happens whenever a worker start
// holds that lock across a process spawn — must not be able to hold the manager
// lock while it waits, or the physical Stop queues behind a process launch.
func TestInvalidateAllDoesNotWaitOnASubmissionBlockedInItsSupervisor(t *testing.T) {
	manager := NewManager()
	t.Cleanup(manager.Shutdown)
	worker := manager.Worker(RoleTTS)

	// Hold the supervisor lock the way start() does while spawning a process.
	worker.mu.Lock()
	submitting := make(chan struct{})
	submitted := make(chan struct{})
	go func() {
		close(submitting)
		// Blocks inside Supervisor.submit until the lock above is released.
		_, _ = manager.Submit(RoleTTS, Request{Type: RequestSpeak, Text: "reply"})
		close(submitted)
	}()
	<-submitting
	time.Sleep(20 * time.Millisecond)

	invalidated := make(chan []*PendingRequest, 1)
	go func() {
		invalidated <- manager.InvalidateAll(RoleTTS)
	}()
	select {
	case <-invalidated:
	case <-time.After(2 * time.Second):
		worker.mu.Unlock()
		t.Fatal("Stop invalidation waited on a submission blocked in its supervisor")
	}

	worker.mu.Unlock()
	select {
	case <-submitted:
	case <-time.After(2 * time.Second):
		t.Fatal("submission never completed after the supervisor lock was released")
	}
}
