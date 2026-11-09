package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestChatInterruptionExplainsKnownCausesWithoutClaimingStop(t *testing.T) {
	s := newTestServer(t)
	for _, tc := range []struct {
		name    string
		cause   error
		message string
	}{
		{"unknown", context.Canceled, chatInterruptedMessage},
		{"timeout", context.DeadlineExceeded, chatTimedOutMessage},
		{"settings", errLLMSettingsChanged, chatChangedMessage},
		{"control", errChatControlChanged, chatControlChangedMessage},
		{"sign_in", errChatAccessEnded, chatAccessEndedMessage},
		{"shutdown", errChatServerStopped, chatServerStoppedMessage},
		{"position_reset", errChatPositionReset, chatPositionResetMessage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(tc.cause)
			var events []map[string]any
			emit := func(event string, payload any) error {
				b, _ := json.Marshal(payload)
				var m map[string]any
				_ = json.Unmarshal(b, &m)
				m["event"] = event
				events = append(events, m)
				return nil
			}
			if !s.emitChatCompletionError(ctx, s.stopSequence.Load(), emit, chat.Result{}, tc.cause, chat.MessageDiagnostics{}) {
				t.Fatal("cancellation was accepted")
			}
			if len(events) != 2 || events[0]["message"] != tc.message || events[1]["ok"] != false {
				t.Fatalf("wrong cancellation: %+v", events)
			}
			if clientChatFailure(map[string]string{"message": tc.message}) != tc.message {
				t.Fatal("remote controller lost explanation")
			}
			events = nil
			s.emitChatCancellationAt(ctx, s.stopSequence.Load(), emit, true)
			if events[0]["reply_retained"] != "true" || events[0]["message"] != chatPostReplyInterruptedMessage {
				t.Fatal("saved reply incorrectly discarded")
			}
		})
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errChatControlChanged)
	stop := s.stopSequence.Load()
	s.stopSequence.Add(1)
	if got := s.chatInterruptionMessage(ctx, stop, context.Canceled); got != "Chat canceled by Emergency Stop." {
		t.Fatalf("Stop did not take priority: %s", got)
	}
	if got := s.chatInterruptionMessage(t.Context(), s.stopSequence.Load(), errors.New("model supplied the words context canceled")); got != "" {
		t.Fatal("classified untrusted language as a lifecycle event")
	}
}

func TestPositionResetCancellationIsNotCalledEmergencyStop(t *testing.T) {
	s := newTestServer(t)
	before := s.stopSequence.Load()
	finish, sequence := s.beginGlobalStop("position_reset")
	finish()
	if got := s.chatInterruptionMessage(t.Context(), before, context.Canceled); got != chatPositionResetMessage {
		t.Fatalf("wrong reset explanation: %s", got)
	}
	if s.stopCommandClearedMessage() != "Position reset cleared the command." {
		t.Fatal("remote command mislabeled")
	}
	for _, retained := range []bool{false, true} {
		var message string
		emit := func(event string, payload any) error {
			if event == "error" {
				message = clientChatFailure(payload)
			}
			return nil
		}
		s.emitChatCancellationAt(t.Context(), before, emit, retained)
		want := chatPositionResetMessage
		if retained {
			want = chatPositionResetAfterReply
		}
		if message != want {
			t.Fatalf("wrong shared cancellation: %s", message)
		}
	}
	finish, _ = s.beginGlobalStop("ui_stop")
	finish()
	s.recordPositionResetSequence(sequence - 1)
	if s.lastStopWasPositionReset() || s.stopCommandClearedMessage() != "Emergency Stop cleared the command." {
		t.Fatal("a newer Emergency Stop retained the reset label")
	}
}

func TestInterruptedHostedReplyRetainsTechnicalEvidenceWithoutApplyingDraft(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	s, fake, _ := hostedFixture(t, func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	done := make(chan string, 1)
	go func() { done <- postChatStream(t, s, `{"message":"Text-only greeting."}`) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request not started")
	}
	s.hostedRequests.invalidate()
	var stream string
	select {
	case stream = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not finish")
	}
	if !strings.Contains(stream, chatChangedMessage) || strings.Contains(stream, "context canceled") || strings.Contains(stream, "event: motion") || len(fake.Commands()) != 0 {
		t.Fatalf("wrong interruption: %s", stream)
	}
	session, _ := s.chatLog.ActiveSessionIDContext(t.Context())
	failures, err := s.chatLog.Failures(t.Context(), session)
	if err != nil || len(failures) != 1 || failures[0].Kind != "interrupted" || !strings.Contains(failures[0].Error, "model or motion settings changed") {
		t.Fatalf("missing technical evidence: %+v %v", failures, err)
	}
	items, err := s.chatLog.RecentSessionContext(t.Context(), session, 10)
	if err != nil || len(items) != 1 || items[0].Role != chat.MessageRoleUser {
		t.Fatalf("canceled draft persisted: %+v %v", items, err)
	}
}

// interruptHostedReply starts a hosted reply, runs interrupt while the provider
// request is in flight, and returns the stream and the one recorded failure.
func interruptHostedReply(t *testing.T, interrupt func(*Server)) (string, chat.RequestFailure, *transport.Fake) {
	t.Helper()
	started := make(chan struct{})
	release := make(chan struct{})
	s, fake, _ := hostedFixture(t, func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	done := make(chan string, 1)
	go func() { done <- postChatStream(t, s, `{"message":"Text-only greeting."}`) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request not started")
	}
	interrupt(s)
	var stream string
	select {
	case stream = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not finish")
	}
	session, _ := s.chatLog.ActiveSessionIDContext(t.Context())
	failures, err := s.chatLog.Failures(t.Context(), session)
	if err != nil || len(failures) != 1 || failures[0].Kind != "interrupted" {
		t.Fatalf("missing technical evidence: %+v %v", failures, err)
	}
	return stream, failures[0], fake
}

// Edge cancels a reply in flight so it cannot override the gentle hold. The
// reply says so and records Edge, not a settings change or Emergency Stop.
func TestEdgeInterruptsHostedReplyWithItsOwnExplanation(t *testing.T) {
	stream, failure, fake := interruptHostedReply(t, func(s *Server) {
		// No motion runs here, so the hold itself is refused after the
		// cancellation; only the interrupted reply is under test.
		_, _ = s.activateEdge(t.Context())
	})
	if !strings.Contains(stream, chatEdgeMessage) || strings.Contains(stream, chatChangedMessage) ||
		strings.Contains(stream, "Emergency Stop") || strings.Contains(stream, "event: motion") || len(fake.Commands()) != 0 {
		t.Fatalf("wrong interruption: %s", stream)
	}
	if !strings.HasPrefix(failure.Error, "edge") {
		t.Fatalf("recorded cause = %q", failure.Error)
	}
}

// The quick speed and stroke controls save motion settings, which retires a
// hosted reply in flight. Its message and record both name a settings change.
func TestQuickMotionChangeExplainsTheRetiredHostedReply(t *testing.T) {
	stream, failure, _ := interruptHostedReply(t, func(s *Server) {
		request := withController(httptest.NewRequest(http.MethodPost, "/api/motion/quick", strings.NewReader(`{"speed_max_percent":70}`)))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		s.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Errorf("quick control status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	})
	if !strings.Contains(stream, chatChangedMessage) || strings.Contains(stream, chatEdgeMessage) {
		t.Fatalf("wrong interruption: %s", stream)
	}
	if !strings.HasPrefix(failure.Error, "model or motion settings changed") {
		t.Fatalf("recorded cause = %q", failure.Error)
	}
}

func TestEdgeCancellationNamesEdgeBeforeAndAfterTheReply(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errChatEdge)
	stop := s.stopSequence.Load()
	if got := s.chatInterruptionMessage(ctx, stop, context.Canceled); got != chatEdgeMessage {
		t.Fatalf("Edge mislabeled: %s", got)
	}
	if outcome := s.chatRepairCancellationOutcome(ctx, stop, context.Canceled); outcome != "canceled" {
		t.Fatalf("Edge is a cancellation, not stale repair input: %s", outcome)
	}
	for _, retained := range []bool{false, true} {
		var message string
		emit := func(event string, payload any) error {
			if event == "error" {
				message = clientChatFailure(payload)
			}
			return nil
		}
		s.emitChatCancellationAt(ctx, stop, emit, retained)
		want := chatEdgeMessage
		if retained {
			want = chatEdgeAfterReply
		}
		if message != want {
			t.Fatalf("wrong shared Edge cancellation: %s", message)
		}
	}
	s.stopSequence.Add(1)
	if got := s.chatInterruptionMessage(ctx, stop, context.Canceled); got != "Chat canceled by Emergency Stop." {
		t.Fatalf("Stop did not take priority over Edge: %s", got)
	}
}

func TestRetainedFailurePreservesTypedCauseButNotPrivateText(t *testing.T) {
	e := retainedChatFailure{message: "redacted", cause: fmt.Errorf("private endpoint: %w", context.DeadlineExceeded)}
	if e.Error() != "redacted" || !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("redacted error lost its type or revealed its cause")
	}
	s := newTestServer(t)
	if s.chatInterruptionMessage(t.Context(), s.stopSequence.Load(), e) != chatTimedOutMessage {
		t.Fatal("wrapped timeout not explained")
	}
	r := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	view, err := s.clientChatEvent(r, "error", map[string]string{"message": chatPostReplyInterruptedMessage, "reply_retained": "true", "private": "hidden"})
	b, _ := json.Marshal(view)
	if err != nil || !strings.Contains(string(b), `"reply_retained":"true"`) || strings.Contains(string(b), "hidden") {
		t.Fatalf("remote reply retention: %s %v", b, err)
	}
}

func TestInterruptedProviderErrorRetainsCauseAndCannotResurrectDeletedChat(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(fmt.Sprintf("deleted_%t", deleted), func(t *testing.T) {
			s := newTestServer(t)
			session, err := s.chatLog.ActiveSessionID()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(errChatControlChanged)
			if deleted {
				if _, err := s.chatLog.SaveSession(session); err != nil {
					t.Fatal(err)
				}
				if _, err := s.chatLog.CreateSession(false); err != nil {
					t.Fatal(err)
				}
				if err := s.chatLog.DeleteSession(session); err != nil {
					t.Fatal(err)
				}
			}
			_, failure := s.retainChatFailure(ctx, s.stopSequence.Load(), session, chat.Result{}, errors.New("provider stream ended"), chat.MessageDiagnostics{})
			if !errors.Is(failure, errChatControlChanged) || s.chatInterruptionMessage(t.Context(), s.stopSequence.Load(), failure) != chatControlChangedMessage {
				t.Fatalf("provider error lost cancellation classification: %v", failure)
			}
			items, err := s.chatLog.Failures(t.Context(), session)
			if err != nil {
				t.Fatal(err)
			}
			if deleted {
				if len(items) != 0 {
					t.Fatal("detached diagnostics resurrected deleted session content")
				}
			} else if len(items) != 1 || items[0].Kind != "interrupted" {
				t.Fatalf("diagnostics disagree with user explanation: %+v", items)
			}
		})
	}
}

func TestSpecificCancellationCausePrecedesStaleRepair(t *testing.T) {
	s := newTestServer(t)
	candidate := &chatRepairCandidate{}
	candidate.active.Store(true)
	ctx, cancel := context.WithCancelCause(context.WithValue(t.Context(), chatRepairContextKey{}, candidate))
	cancel(errChatAccessEnded)
	if got := s.chatInterruptionMessage(ctx, s.stopSequence.Load(), context.Canceled); got != chatAccessEndedMessage {
		t.Fatalf("known sign-out mislabeled as stale repair: %s", got)
	}
	ctx, cancel = context.WithCancelCause(context.WithValue(t.Context(), chatRepairContextKey{}, candidate))
	cancel(context.Canceled)
	if got := s.chatInterruptionMessage(ctx, s.stopSequence.Load(), context.Canceled); got != chatInterruptedMessage {
		t.Fatalf("unknown cancellation invented a repair-input change: %s", got)
	}
	ctx, cancel = context.WithCancelCause(t.Context())
	cancel(errLLMSettingsChanged)
	if outcome := s.chatRepairCancellationOutcome(ctx, s.stopSequence.Load(), context.Canceled); outcome != "stale" {
		t.Fatalf("settings cancellation outcome: %s", outcome)
	}
}
