package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
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
				if request.MaxTokens != tc.budget {
					t.Errorf("readiness budget = %d, want %d", request.MaxTokens, tc.budget)
				}
				if tc.name == "provider default" && request.Reasoning != nil {
					t.Error("test overrode provider default reasoning")
				}
				writeHostedFixtureReply(w, `{"ready":true}`)
				return w.Result(), nil
			})}
			body, _ := json.Marshal(map[string]any{"connection": connection})
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, withController(httptest.NewRequest(http.MethodPost, "/api/llm/connections/test", strings.NewReader(string(body)))))
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ready":true`) || calls != 1 || len(fake.Commands()) != 0 {
				t.Fatalf("readiness = %d %s; calls %d", w.Code, w.Body.String(), calls)
			}
		})
	}
}
