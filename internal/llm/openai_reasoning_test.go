package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponsesReasoningIsConnectionScopedAndKeepsPlanRestrictions(t *testing.T) {
	for _, effort := range []string{"", "low", "medium"} {
		t.Run(effort, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if effort == "" {
					if _, ok := body["reasoning"]; ok {
						t.Error("provider default was overwritten")
					}
				} else if body["reasoning"].(map[string]any)["effort"] != effort {
					t.Error("wrong reasoning effort")
				}
				for _, key := range []string{"temperature", "max_output_tokens", "service_tier"} {
					if _, ok := body[key]; ok {
						t.Errorf("unexpected plan parameter %s", key)
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ready\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
			}))
			defer server.Close()
			provider, err := NewOpenAIResponsesProvider(OpenAIOptions{BaseURL: server.URL, Model: "account-model", PlanUsage: true, ReasoningEffort: effort, Token: func(context.Context) (string, error) { return "test-token", nil }})
			if err != nil {
				t.Fatal(err)
			}
			// The local reasoning switch must not override the hosted connection.
			if _, err = provider.StreamChat(t.Context(), ChatRequest{Messages: []Message{{Role: "user", Content: "test"}}, ReasoningMode: "off", MaxTokens: 100, Temperature: 1}, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChatGPTCatalogRetainsAccountReasoningLevels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"models":[{"slug":"account-model","display_name":"Account model","visibility":"list","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"}]}]}`)
	}))
	defer server.Close()
	p, err := NewOpenAIResponsesProvider(OpenAIOptions{BaseURL: server.URL, Model: "account-model", PlanUsage: true, Token: func(context.Context) (string, error) { return "test", nil }})
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.Models(t.Context())
	if err != nil || len(models) != 1 || models[0].DefaultReasoningLevel != "medium" || len(models[0].SupportedReasoningLevels) != 2 || models[0].SupportedReasoningLevels[0].Effort != "low" {
		t.Fatalf("catalog = %+v, %v", models, err)
	}
}
