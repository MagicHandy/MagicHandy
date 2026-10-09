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
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func writeRefusalFixture(w http.ResponseWriter) {
	_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"refusal\":\"private refusal detail\"}}]}\n\ndata: [DONE]\n\n")
}

func configureLocalRetry(t *testing.T, s *Server, endpoint string, enabled bool) {
	t.Helper()
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.RetryRefusalLocally = enabled
		settings.LLM.Provider, settings.LLM.LlamaCPPMode = config.LLMProviderLlamaCPP, config.LlamaCPPModeExternal
		settings.LLM.LlamaCPPBaseURL, settings.LLM.Model = endpoint, "saved-local-model"
		return settings
	})
}

func localRetryEndpoint(t *testing.T, outcome string, calls *atomic.Int32, captured chan []llm.Message) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("hosted credentials reached local retry")
		}
		var request struct {
			Model    string        `json:"model"`
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "saved-local-model" {
			t.Errorf("wrong local model: %s", request.Model)
		}
		captured <- request.Messages
		switch outcome {
		case "continuous":
			writeHostedFixtureReply(w, `{"action":"none","edits":[],"reply":"Local completion."}`)
		case "invalid":
			writeHostedFixtureReply(w, "not valid JSON")
		case "unavailable":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			writeHostedFixtureReply(w, `{"reply":"Local completion."}`)
		}
	}))
}

func TestChatRefusalRetryIsOptInTypedAndBounded(t *testing.T) {
	for _, tc := range []struct {
		name, primary, local    string
		enabled, retry, success bool
	}{
		{"off", "refusal", "valid", false, false, false},
		{"typed refusal", "refusal", "valid", true, true, true},
		{"ordinary refusal wording", "prose", "valid", true, false, true},
		{"invalid hosted output", "invalid", "valid", true, false, false},
		{"provider unavailable", "unavailable", "valid", true, false, false},
		{"no local repair loop", "refusal", "invalid", true, true, false},
		{"local unavailable", "refusal", "unavailable", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hostedCalls, localCalls atomic.Int32
			captured := make(chan []llm.Message, 2)
			local := localRetryEndpoint(t, tc.local, &localCalls, captured)
			defer local.Close()
			s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				hostedCalls.Add(1)
				switch tc.primary {
				case "refusal":
					writeRefusalFixture(w)
				case "invalid":
					writeHostedFixtureReply(w, "not valid JSON")
				case "unavailable":
					w.WriteHeader(http.StatusServiceUnavailable)
				case "prose":
					writeHostedFixtureReply(w, `{"reply":"I cannot help with that request."}`)
				}
			})
			defer s.Close()
			configureLocalRetry(t, s, local.URL, tc.enabled)
			const user = "Technical adult-language test: orgasm is a vocabulary token. Keep motion off."
			body, _ := json.Marshal(map[string]string{"message": user, "motion_owner": "off"})
			stream := postChatStream(t, s, string(body))
			wantLocal := int32(0)
			if tc.retry {
				wantLocal = 1
			}
			if hostedCalls.Load() != 1 || localCalls.Load() != wantLocal || strings.Contains(stream, "event: motion") || len(fake.Commands()) != 0 {
				t.Fatalf("wrong calls/motion: hosted=%d local=%d stream=%s", hostedCalls.Load(), localCalls.Load(), stream)
			}
			if strings.Contains(stream, `"state":"retrying_local"`) != tc.retry || strings.Contains(stream, `"ok":true`) != tc.success || strings.Contains(stream, "private refusal detail") {
				t.Fatalf("wrong visible outcome: %s", stream)
			}
			if tc.retry {
				assertOriginalRetryTurn(t, <-captured, user)
			}
			if tc.retry && tc.success {
				assertRetryProvenance(t, s, stream)
			}
		})
	}
}

func TestStopRetiresBothRefusalRetryStages(t *testing.T) {
	for _, stage := range []string{"hosted", "local"} {
		t.Run(stage, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			canceled := make(chan struct{})
			var localCalls atomic.Int32
			local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				localCalls.Add(1)
				if stage == "local" {
					close(entered)
					select {
					case <-r.Context().Done():
						close(canceled)
						return
					case <-release:
					}
				}
				writeHostedFixtureReply(w, `{"reply":"Late local completion."}`)
			}))
			defer local.Close()
			s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if stage == "hosted" {
					close(entered)
					select {
					case <-r.Context().Done():
						close(canceled)
						return
					case <-release:
					}
				}
				writeRefusalFixture(w)
			})
			defer s.Close()
			configureLocalRetry(t, s, local.URL, true)
			finished := make(chan string, 1)
			go func() {
				response := httptest.NewRecorder()
				request := withController(httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(`{"message":"Acknowledge, with motion off."}`)))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(stopSequenceHeader, "0")
				s.Handler().ServeHTTP(response, request)
				finished <- response.Body.String()
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				close(release)
				t.Fatal("request did not enter provider")
			}
			stopped := httptest.NewRecorder()
			s.Handler().ServeHTTP(stopped, withController(httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`))))
			if stopped.Code != http.StatusOK {
				close(release)
				t.Fatalf("Stop failed: %s", stopped.Body.String())
			}
			select {
			case <-canceled:
			case <-time.After(5 * time.Second):
				close(release)
				t.Fatal("Stop did not cancel in-flight provider work")
			}
			close(release)
			select {
			case stream := <-finished:
				if strings.Contains(stream, "event: message") || strings.Contains(stream, "event: motion") || (stage == "hosted" && localCalls.Load() != 0) {
					t.Fatalf("Stop admitted late retry: %s", stream)
				}
				assertNoRetryMotion(t, fake)
			case <-time.After(5 * time.Second):
				t.Fatal("stopped retry did not finish")
			}
		})
	}
}

func assertOriginalRetryTurn(t *testing.T, messages []llm.Message, user string) {
	t.Helper()
	count := 0
	for _, message := range messages {
		if message.Role == "user" && message.Content == user {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("original user turn was duplicated/lost: %d", count)
	}
}

func assertNoRetryMotion(t *testing.T, fake *transport.Fake) {
	t.Helper()
	for _, command := range fake.Commands() {
		if command.Kind == transport.CommandKindPointsAdd || command.Kind == transport.CommandKindPointsPlay {
			t.Fatal("Stop admitted motion")
		}
	}
}

func TestRefusalRetryRejectsStopBeforeLocalAdmission(t *testing.T) {
	var localCalls atomic.Int32
	local := localRetryEndpoint(t, "valid", &localCalls, make(chan []llm.Message, 1))
	defer local.Close()
	s, _, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) { writeRefusalFixture(w) })
	defer s.Close()
	configureLocalRetry(t, s, local.URL, true)
	saved, _ := s.store.Snapshot()
	settings := saved.LLM.LocalSettings()
	settings.RequestRole = "refusal_fallback"
	ctx := context.WithValue(s.guardHostedChat(t.Context()), refusalRetryStopKey{}, s.stopSequence.Load())
	provider, err := s.prepareLLMProvider(settings)
	if err != nil {
		t.Fatal(err)
	}
	// Isolate the between-lanes epoch check, even without a canceled parent or
	// a changed runtime generation to help it reject the stale attempt.
	s.stopSequence.Add(1)
	_, err = provider.StreamChat(ctx, llm.ChatRequest{}, nil)
	if !errors.Is(err, context.Canceled) || localCalls.Load() != 0 {
		t.Fatalf("stale local attempt ran: %v, calls=%d", err, localCalls.Load())
	}
}

func TestRefusalRetryRebuildsLocalMotionPrompt(t *testing.T) {
	var localCalls atomic.Int32
	localMessages, hostedMessages := make(chan []llm.Message, 1), make(chan []llm.Message, 1)
	local := localRetryEndpoint(t, "continuous", &localCalls, localMessages)
	defer local.Close()
	s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		hostedMessages <- request.Messages
		writeRefusalFixture(w)
	})
	defer s.Close()
	configureLocalRetry(t, s, local.URL, true)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		return settings
	})
	stream := postChatStream(t, s, `{"message":"Acknowledge this software check with text only. Do not move."}`)
	if !strings.Contains(stream, `"reply":"Local completion."`) || localCalls.Load() != 1 {
		t.Fatalf("local contract failed: %s", stream)
	}
	guide := strings.TrimSpace(strings.TrimSuffix(chat.HostedLLMLabPrompts()["creative_v2"], chat.LLMLabPrompts()["creative_v2"]))
	remote, retry := <-hostedMessages, <-localMessages
	if guide == "" || !strings.Contains(remote[0].Content, guide) || strings.Contains(retry[0].Content, guide) {
		t.Fatal("retry carried the hosted motion guide instead of the local contract")
	}
	assertNoRetryMotion(t, fake)
}

func assertRetryProvenance(t *testing.T, s *Server, stream string) {
	t.Helper()

	if !strings.Contains(stream, `"provider":"llama_cpp"`) || !strings.Contains(stream, `"provider_calls":2`) || !strings.Contains(stream, `"refusal_fallback_from":"compatible"`) || strings.Contains(stream, `"repaired":true`) {
		t.Fatalf("incorrect provenance: %s", stream)
	}
	session, _ := s.chatLog.ActiveSessionID()
	history, err := s.chatLog.RecentSessionContext(t.Context(), session, 10)
	if err != nil || len(history) != 2 || history[0].Role != chat.MessageRoleUser || history[1].Diagnostics.RefusalFallbackFrom != "compatible" || history[1].Diagnostics.ProviderCalls != 2 {
		t.Fatalf("retry history duplicated or missing provenance: %+v %v", history, err)
	}
}
