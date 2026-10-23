package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func localContinuousRepairFixture(t *testing.T, provider llm.Provider, mode string) (*Server, *transport.Fake) {
	t.Helper()
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.MotionGenerationMode = mode
		// The hosted switch does not control the established local correction
		// policy, including its newly covered continuous modes.
		disabled := false
		settings.LLM.HostedResponseRepairEnabled = &disabled
		return settings
	})
	return server, fake
}

func TestLocalContinuousRepairStreamsProgressAndCommitsOnlyReplacement(t *testing.T) {
	for _, mode := range []string{config.LLMMotionModeLayered, config.LLMMotionModeCreativeV2} {
		t.Run(mode, func(t *testing.T) {
			good := `{"action":"start","edits":{"controls":{"speed_percent":30},"geometry":"full_slider"},"reply":"Full strokes at 30 percent."}`
			if mode == config.LLMMotionModeCreativeV2 {
				good = `{"action":"start","edits":[{"range":{"min_percent":0,"max_percent":100}},{"focus":{"position_percent":50,"width_percent":30,"mix_percent":0,"roam_percent":0}},{"speed_percent":30}],"reply":"Full strokes at 30 percent."}`
			}
			bad := `{"unknown":true,"reply":"Rejected draft must not be committed."}`
			provider := &scriptedLLMProvider{responses: []string{bad, good}}
			server, _ := localContinuousRepairFixture(t, provider, mode)
			stream := postChatStream(t, server, `{"message":"Start full-length motion at 30 percent."}`)
			if provider.callCount() != 2 || !strings.Contains(stream, `"state":"repairing_response"`) || !strings.Contains(stream, `"repaired":true`) ||
				strings.Count(stream, "event: message\n") != 1 || !strings.Contains(stream, `"applied":true`) || strings.Contains(stream, `"available":true`) {
				t.Fatalf("local correction failed normal SSE/dispatch: %s", stream)
			}
			assertSuccessfulLocalRepairHistory(t, server, bad, good)
			callMotion(t, server, http.MethodPost, "/api/motion/stop", `{}`)
		})
	}
}

func assertSuccessfulLocalRepairHistory(t *testing.T, server *Server, bad, good string) {
	t.Helper()
	session, _ := server.chatLog.ActiveSessionIDContext(t.Context())
	messages, err := server.chatLog.RecentSession(session, 10)
	if err != nil || len(messages) != 2 || messages[0].Role != chat.MessageRoleUser || messages[1].Role != chat.MessageRoleAssistant ||
		messages[1].Content != "Full strokes at 30 percent." || !messages[1].Diagnostics.Repaired || messages[1].Diagnostics.ProviderCalls != 2 {
		t.Fatalf("rejected draft or duplicate turn committed: %+v %v", messages, err)
	}
	failures, err := server.chatLog.Failures(t.Context(), session)
	if err != nil || len(failures) != 1 {
		t.Fatalf("missing original local rejection: %+v %v", failures, err)
	}
	evidence, err := server.chatLog.Failure(t.Context(), session, failures[0].ID, true)
	if err != nil || evidence.Details.Raw != bad || evidence.Details.RepairRaw != good || evidence.Details.InitialError == "" || evidence.Details.Schema == "" {
		t.Fatalf("local repair evidence incomplete: %+v %v", evidence, err)
	}
}

type delayedLocalRepairProvider struct {
	llm.Provider
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (p *delayedLocalRepairProvider) StreamChat(_ context.Context, _ llm.ChatRequest, _ func(string) error) (string, error) {
	if p.calls.Add(1) == 1 {
		return `{"unknown":true,"reply":"Rejected draft."}`, nil
	}
	close(p.started)
	<-p.release // Deliberately ignore cancellation to test the acceptance gate.
	return `{"action":"start","edits":{"controls":{"speed_percent":30}},"reply":"Late start."}`, nil
}

func (p *delayedLocalRepairProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Provider: "local-test", Available: true}
}

func TestLocalContinuousRepairStopAndClientCancellationKeepEvidenceWithoutMotion(t *testing.T) {
	for _, stop := range []bool{true, false} {
		name := "client cancellation"
		if stop {
			name = "Stop"
		}
		t.Run(name, func(t *testing.T) {
			provider := &delayedLocalRepairProvider{started: make(chan struct{}), release: make(chan struct{})}
			server, fake := localContinuousRepairFixture(t, provider, config.LLMMotionModeLayered)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			request := withController(httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(`{"message":"Start motion at 30 percent."}`)).WithContext(ctx))
			request.Header.Set(stopSequenceHeader, "0")
			recorder, done := httptest.NewRecorder(), make(chan struct{})
			go func() {
				server.Handler().ServeHTTP(recorder, request)
				close(done)
			}()
			select {
			case <-provider.started:
			case <-time.After(2 * time.Second):
				close(provider.release)
				t.Fatal("local repair did not start")
			}
			if stop {
				callMotion(t, server, http.MethodPost, "/api/motion/stop", `{}`)
			} else {
				cancel()
			}
			close(provider.release)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("canceled local repair did not finish")
			}
			if provider.calls.Load() != 2 || strings.Contains(recorder.Body.String(), "event: message\n") {
				t.Fatalf("late completion escaped cancellation: %s", recorder.Body.String())
			}
			assertCanceledLocalRepairState(t, server, fake)
		})
	}
}

func assertCanceledLocalRepairState(t *testing.T, server *Server, fake *transport.Fake) {
	t.Helper()
	for _, command := range fake.Commands() {
		if command.Kind != transport.CommandKindStop {
			t.Fatalf("canceled repair issued motion: %+v", fake.Commands())
		}
	}
	session, _ := server.chatLog.ActiveSessionIDContext(t.Context())
	messages, err := server.chatLog.RecentSession(session, 10)
	if err != nil || len(messages) != 1 || messages[0].Role != chat.MessageRoleUser {
		t.Fatalf("canceled assistant was persisted: %+v %v", messages, err)
	}
	failures, err := server.chatLog.Failures(t.Context(), session)
	if err != nil || len(failures) != 1 {
		t.Fatalf("canceled repair evidence missing: %+v %v", failures, err)
	}
	evidence, err := server.chatLog.Failure(t.Context(), session, failures[0].ID, true)
	if err != nil || !strings.Contains(evidence.Details.Raw, "Rejected draft") || !strings.Contains(evidence.Details.RepairRaw, "Late start") || evidence.Details.InitialError == "" {
		t.Fatalf("canceled repair details incomplete: %+v %v", evidence, err)
	}
}

func TestLocalRepairCannotAddMotionWhenOffOrVideoOwned(t *testing.T) {
	for _, owner := range []string{"", "script", "off"} {
		provider := &scriptedLLMProvider{responses: []string{`{"unknown":true}`, `{"reply":"Text only.","motion":{"action":"start","speed_percent":30}}`}}
		mode := config.LLMMotionModeCreativeV2
		if owner == "" {
			mode = config.LLMMotionModeOff
		}
		server, fake := localContinuousRepairFixture(t, provider, mode)
		body := `{"message":"Explain the current settings without moving.","motion_owner":"` + owner + `"}`
		stream := postChatStream(t, server, body)
		if provider.callCount() != 2 || !strings.Contains(stream, `"reply":"Text only."`) || strings.Contains(stream, "event: motion") || len(fake.Commands()) != 0 {
			t.Fatalf("chat-only repair acquired motion: owner=%s %s", owner, stream)
		}
	}
}
