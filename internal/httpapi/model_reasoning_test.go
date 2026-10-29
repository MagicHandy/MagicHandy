package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
)

func TestReasoningChangeInvalidatesProviderAndGenerationProof(t *testing.T) {
	connection := config.ModelConnection{ID: "chatgpt", Name: "ChatGPT", Provider: config.LLMProviderChatGPT, Model: "account-model"}.Normalize()
	settings := config.DefaultSettings().LLM.WithConnection(connection)
	before := hostedProviderKey(settings)
	proof := connectionTestKey(connection, 7)
	connection.ReasoningEffort = "low"
	if hostedProviderKey(settings.WithConnection(connection)) == before || connectionTestKey(connection, 7) == proof {
		t.Fatal("reasoning change reused a provider or generation proof")
	}
}

func TestOpenRouterEnableChangeInvalidatesProviderAndGenerationProof(t *testing.T) {
	connection := config.ModelConnection{ID: "router", Name: "Router", Provider: config.LLMProviderOpenRouter, Model: "fixture"}.Normalize()
	settings := config.DefaultSettings().LLM.WithConnection(connection)
	before, proof := hostedProviderKey(settings), connectionTestKey(connection, 7)
	off := false
	connection.ReasoningEnabled = &off
	if hostedProviderKey(settings.WithConnection(connection)) == before || connectionTestKey(connection, 7) == proof {
		t.Fatal("explicit Off reused default reasoning readiness")
	}
}

func TestOpenRouterAutopilotKeepsReasoningFailureActionable(t *testing.T) {
	s, fake, _ := hostedFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("wrong provider") })
	connection := config.ModelConnection{ID: "router", Name: "Router", Provider: config.LLMProviderOpenRouter, Model: "fixture", OutputMode: "prompt", ReasoningEffort: "high"}.Normalize()
	if err := s.cloudPlanning.auth.SetConnectionKey(t.Context(), connection, "fixture-key"); err != nil {
		t.Fatal(err)
	}
	generationCalls := 0
	s.cloudPlanning.client = &http.Client{Transport: modelRoutingTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/models" {
			_, _ = fmt.Fprint(w, `{"data":[{"id":"fixture","supported_parameters":["reasoning","max_tokens"],"reasoning":{"supported_efforts":["low"]}}]}`)
		} else {
			generationCalls++
			w.WriteHeader(http.StatusInternalServerError)
		}
		return w.Result(), nil
	})}
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.Connections = []config.ModelConnection{connection}
		settings.LLM.ConversationConnectionID = connection.ID
		settings.LLM.MotionGenerationMode = config.LLMMotionModePattern
		return settings
	})
	_, err := s.autopilotDecide(t.Context(), modes.DecisionInput{})
	var outcome *llm.CloudError
	if !errors.As(err, &outcome) || outcome.Kind != "reasoning" || !outcome.HoldMotion() || !strings.Contains(err.Error(), "reasoning") {
		t.Fatalf("unsupported reasoning lost its actionable explanation: %v", err)
	}
	if generationCalls != 0 || len(fake.Commands()) != 0 {
		t.Fatal("an unsupported reasoning setting generated a reply or dispatched motion")
	}
}

func TestOpenRouterReadinessAllowsReasoningBeforeAnswer(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name, effort string
		enabled      *bool
		budget       int
	}{
		{name: "provider default", budget: 2048},
		{name: "enabled", enabled: &yes, budget: 2048},
		{name: "low", effort: "low", budget: 2048},
		{name: "disabled", enabled: &no, budget: 64},
		{name: "none", effort: "none", budget: 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, fake, _ := hostedFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("wrong provider") })
			connection := config.ModelConnection{ID: "router", Name: "Router", Provider: config.LLMProviderOpenRouter, Model: "fixture", OutputMode: "prompt", ReasoningEffort: tc.effort, ReasoningEnabled: tc.enabled}.Normalize()
			if err := s.cloudPlanning.auth.SetConnectionKey(t.Context(), connection, "fixture-key"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			s.cloudPlanning.client = &http.Client{Transport: modelRoutingTransport(func(r *http.Request) (*http.Response, error) {
				w := httptest.NewRecorder()
				if r.Method == http.MethodGet {
					_, _ = fmt.Fprint(w, `{"data":[{"id":"fixture","supported_parameters":["reasoning","max_tokens"],"reasoning":{"supported_efforts":["none","low"]}}]}`)
					return w.Result(), nil
				}
				calls++
				var request struct {
					MaxTokens int             `json:"max_tokens"`
					Reasoning json.RawMessage `json:"reasoning"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				wantBudget := tc.budget
				if calls == 2 && wantBudget == 64 {
					// Off keeps each caller's ordinary answer budget.
					wantBudget = config.DefaultSettings().LLM.ChatMaxOutputTokens(config.LLMReplyLengthShort)
				}
				if request.MaxTokens != wantBudget {
					t.Errorf("call %d budget = %d, want %d", calls, request.MaxTokens, wantBudget)
				}
				if tc.name == "provider default" && request.Reasoning != nil {
					t.Error("test overrode provider default reasoning")
				}
				if tc.budget == 2048 && request.MaxTokens < 2048 {
					// A model using its output allowance on hidden reasoning.
					_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n")
				} else if calls == 1 {
					writeHostedFixtureReply(w, `{"ready":true}`)
				} else {
					writeHostedFixtureReply(w, `{"reply":"The chat is ready."}`)
				}
				return w.Result(), nil
			})}
			body, _ := json.Marshal(map[string]any{"connection": connection})
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, withController(httptest.NewRequest(http.MethodPost, "/api/llm/connections/test", strings.NewReader(string(body)))))
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ready":true`) || calls != 1 || len(fake.Commands()) != 0 {
				t.Fatalf("readiness = %d %s; calls %d", w.Code, w.Body.String(), calls)
			}
			checkReasoningChatAfterReadiness(t, s, connection)
			if calls != 2 || len(fake.Commands()) != 0 {
				t.Fatalf("real turn retried or dispatched motion: calls=%d commands=%v", calls, fake.Commands())
			}
		})
	}
}

func checkReasoningChatAfterReadiness(t *testing.T, s *Server, connection config.ModelConnection) {
	t.Helper()
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.Connections = []config.ModelConnection{connection}
		settings.LLM.ConversationConnectionID = connection.ID
		settings.LLM.MaxOutputTokens = config.DefaultSettings().LLM.MaxOutputTokens
		settings.LLM.ReplyLength = config.LLMReplyLengthShort
		return settings
	})
	stream := postChatStream(t, s, `{"message":"Please confirm the chat works. Text only."}`)
	if !strings.Contains(stream, `"reply":"The chat is ready."`) || strings.Contains(stream, "event: malformed") || strings.Contains(stream, "event: motion") {
		t.Fatalf("Ready did not predict a successful real turn: %s", stream)
	}
}
