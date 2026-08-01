package voice

import "testing"

func TestCancelPendingPreservesCompletedRequests(t *testing.T) {
	manager := NewManager()
	t.Cleanup(manager.Shutdown)
	queued := &PendingRequest{ID: "tts-queued", Role: RoleTTS, state: RequestStateQueued}
	active := &PendingRequest{ID: "tts-active", Role: RoleTTS, state: RequestStateActive}
	done := &PendingRequest{ID: "tts-done", Role: RoleTTS, state: RequestStateDone, audio: []byte("kept")}
	manager.Track(queued)
	manager.Track(active)
	manager.Track(done)

	canceled := manager.CancelPending(RoleTTS)
	if len(canceled) != 2 {
		t.Fatalf("canceled requests = %d, want 2", len(canceled))
	}
	if queued.Snapshot().State != RequestStateCanceled || active.Snapshot().State != RequestStateCanceled {
		t.Fatalf("pending states = %q, %q", queued.Snapshot().State, active.Snapshot().State)
	}
	done.mu.Lock()
	retainedAudio := string(done.audio)
	done.mu.Unlock()
	if done.Snapshot().State != RequestStateDone || retainedAudio != "kept" {
		t.Fatalf("completed request was modified: %+v", done.Snapshot())
	}
}
