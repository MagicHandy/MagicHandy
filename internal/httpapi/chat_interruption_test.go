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
	if err != nil || len(failures) != 1 || failures[0].Kind != "interrupted" || !strings.Contains(failures[0].Error, "model settings changed") {
		t.Fatalf("missing technical evidence: %+v %v", failures, err)
	}
	items, err := s.chatLog.RecentSessionContext(t.Context(), session, 10)
	if err != nil || len(items) != 1 || items[0].Role != chat.MessageRoleUser {
		t.Fatalf("canceled draft persisted: %+v %v", items, err)
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
