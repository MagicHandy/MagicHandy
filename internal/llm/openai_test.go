package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesRequiresTerminalSuccessAndRedactsFailures(t *testing.T) {
	cases := []struct{ name, terminal, kind string }{
		{"completed", `{"type":"response.completed","response":{"status":"completed"}}`, ""},
		{"quota_after_text", `{"type":"response.failed","response":{"error":{"code":"subscription_sharing_usage_limit_exceeded","message":"private request"}}}`, "quota"},
		{"refusal", `{"type":"response.refusal.done"}`, "refusal"},
		{"terminal_refusal", `{"type":"response.completed","response":{"status":"completed","output":[{"content":[{"type":"refusal"}]}]}}`, "refusal"},
		{"incomplete", `{"type":"response.incomplete"}`, "incomplete"},
		{"missing_terminal", "", "incomplete"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/responses" || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("wrong endpoint or credential")
				}
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Error("invalid request")
				}
				if body["store"] != false || body["stream"] != true {
					t.Error("plan usage flags missing")
				}
				for _, field := range []string{"temperature", "top_p", "max_output_tokens", "previous_response_id", "metadata"} {
					if _, exists := body[field]; exists {
						t.Errorf("unsupported field %s", field)
					}
				}
				input, _ := body["input"].([]any)
				if len(input) == 0 || input[0].(map[string]any)["role"] != "developer" {
					t.Error("system role was not mapped")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"ready\\\":true}\"}\n\n")
				if item.terminal != "" {
					_, _ = fmt.Fprintf(w, "data: %s\n\n", item.terminal)
				}
			}))
			defer server.Close()
			provider, err := NewOpenAIResponsesProvider(OpenAIOptions{BaseURL: server.URL, Model: "account-model", PlanUsage: true, Token: func(context.Context) (string, error) { return "test-token", nil }})
			if err != nil {
				t.Fatal(err)
			}
			deltas := 0
			raw, err := provider.StreamChat(t.Context(), ChatRequest{Messages: []Message{{Role: "system", Content: "test"}}, Temperature: 1, TopP: 1, MaxTokens: 64}, func(string) error { deltas++; return nil })
			if item.kind == "" {
				if err != nil || raw != `{"ready":true}` || deltas != 1 {
					t.Fatalf("completed = %q, %v, deltas=%d", raw, err, deltas)
				}
				return
			}
			assertFailedResponsesOutcome(t, item.kind, raw, err, deltas)
		})
	}
}

func assertFailedResponsesOutcome(t *testing.T, kind, raw string, err error, deltas int) {
	t.Helper()
	var outcome *CloudError
	if !errors.As(err, &outcome) || outcome.Kind != kind || raw != "" || deltas != 0 {
		t.Fatalf("failed proposal escaped: kind=%s, err=%v, deltas=%d", kind, err, deltas)
	}
	if strings.Contains(err.Error(), "private request") {
		t.Fatal("remote error text leaked")
	}
}

func TestOpenAISchemaRestoresOmissionAndPreservesClear(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"reply":{"type":"string"},"edits":{"type":"object","properties":{"layers":{"type":"array","items":{"type":"string"}},"speed":{"type":"integer"},"explicit_null":{"type":["integer","null"]}},"required":[],"additionalProperties":false}},"required":["reply","edits"],"additionalProperties":false}`)
	strict, err := OpenAISchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(strict), `"required":["explicit_null","layers","speed"]`) {
		t.Fatalf("not all fields required: %s", strict)
	}
	raw, err := DomainOutput(`{"reply":"test","edits":{"layers":[],"speed":null,"explicit_null":null}}`, schema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "speed") || !strings.Contains(raw, `"layers":[]`) || !strings.Contains(raw, `"explicit_null":null`) {
		t.Fatalf("clear/omission mapping wrong: %s", raw)
	}
	requiredNull, err := DomainOutput(`{"reply":null,"edits":{}}`, schema)
	if err != nil || !strings.Contains(requiredNull, `"reply":null`) {
		t.Fatal("required null was silently repaired")
	}
}

func TestDecisionsCannotInventChoiceOrExecuteRefusal(t *testing.T) {
	for _, answer := range []string{`{"type":"choice","name":"motion_candidate","choice":"unknown"}`, `{"type":"refusal","name":"motion_candidate"}`, `{"type":"choice","name":"other","choice":"ready"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/decisions" || r.Header.Get("Authorization") != "Bearer own-api-key" {
				t.Error("wrong billing path")
			}
			_, _ = fmt.Fprintf(w, `{"answers":[%s]}`, answer)
		}))
		selected, err := ChooseOpenAICandidate(t.Context(), OpenAIOptions{BaseURL: server.URL, Token: func(context.Context) (string, error) { return "own-api-key", nil }}, "technical state", []DecisionChoice{{Value: "ready"}, {Value: "keep_current"}})
		server.Close()
		if err == nil || selected != "" || !IsCloudOutcome(err) {
			t.Fatalf("invalid choice escaped: %q %v", selected, err)
		}
	}
}

func TestChatGPTCatalogUsesAccountVisibilityAndOrdering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"models":[{"slug":"second","display_name":"Second","visibility":"list"},{"slug":"hidden","visibility":"hide"},{"slug":"first","display_name":"First","visibility":"list"}]}`)
	}))
	defer server.Close()
	provider, err := NewOpenAIResponsesProvider(OpenAIOptions{BaseURL: server.URL, Model: "first", Token: func(context.Context) (string, error) { return "test", nil }})
	if err != nil {
		t.Fatal(err)
	}
	models, err := provider.Models(t.Context())
	if err != nil || len(models) != 2 || models[0].Slug != "second" || models[1].Slug != "first" {
		t.Fatalf("account catalog mismatch: %v %v", models, err)
	}
}
