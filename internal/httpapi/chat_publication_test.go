package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/modes"
)

func TestChatHistoryCancellationDoesNotWaitForPublication(t *testing.T) {
	s := newTestServer(t)
	t.Cleanup(s.Close)
	id, err := s.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.chatSpeechMu.Lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.chatSpeechMu.Unlock()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/api/chat/messages?session_id="+id, nil).WithContext(ctx)
	done := make(chan struct{})
	go func() { defer close(done); s.handleChatMessages(httptest.NewRecorder(), r) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled history read is still queued behind publication")
	}
}

func TestCanceledAutopilotWaiterDoesNotPublish(t *testing.T) {
	s := newTestServer(t)
	t.Cleanup(s.Close)
	if err := s.chatSpeechMu.Lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.chatSpeechMu.Unlock()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	done := make(chan modes.Announcement, 1)
	go func() { done <- s.autopilotAnnounce(ctx, "queued synthetic announcement") }()
	select {
	case announcement := <-done:
		if announcement.Published {
			t.Fatal("canceled publication became visible")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled Autopilot still waits for publication")
	}
	var count int
	if err := s.store.Datastore().SQL().QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("canceled draft retained: %d, %v", count, err)
	}
}
