package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesContextBudgetAndUsage(t *testing.T) {
	for _, plan := range []bool{false, true} {
		t.Run(fmt.Sprint(plan), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Input []Message `json:"input"`
					Store bool      `json:"store"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body.Input) != 4 || len(body.Input[1].Content) != 72000 || body.Input[0].Role != "developer" || body.Input[3].Content != "current" || body.Store {
					t.Error("long prompt changed or stored")
				}
				_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"reply\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":20000,\"output_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":19000}}}}\n\n")
			}))
			defer server.Close()
			p, err := NewOpenAIResponsesProvider(OpenAIOptions{BaseURL: server.URL, Model: "model", PlanUsage: plan, ContextSize: 131072, Token: func(context.Context) (string, error) { return "test", nil }})
			if err != nil {
				t.Fatal(err)
			}
			var budget PromptBudget
			var progress ProviderProgress
			request := ChatRequest{MaxTokens: 1536, HistoryMessagesDropped: 7, Messages: []Message{{Role: "system", Content: "current instructions"}, {Role: "user", Content: strings.Repeat("x", 72000)}, {Role: "assistant", Content: "previous"}, {Role: "user", Content: "current"}}, OnBudget: func(b PromptBudget) { budget = b }, OnProgress: func(p ProviderProgress) {
				if p.PromptTokens > 0 {
					progress = p
				}
			}}
			if _, err := p.StreamChat(t.Context(), request, nil); err != nil {
				t.Fatal(err)
			}
			if budget.InputBytes < 72000 || budget.HistoryMessagesDropped != 7 || progress.PromptTokens != 20000 || progress.CachedPromptTokens != 19000 || progress.GeneratedTokens != 12 {
				t.Fatal(budget, progress)
			}
			_, short, err := BudgetChatRequest(request, 32768)
			if err != nil || short.HistoryMessagesDropped != 8 {
				t.Fatal("standard provider budget widened", short, err)
			}
		})
	}
}

func TestContextCapacityFailureHasActionablePrivateMessage(t *testing.T) {
	err := cloudResponseError(400, "context_length_exceeded")
	if !strings.Contains(err.Error(), "Conversation history") || !err.(*CloudError).HoldMotion() {
		t.Fatal(err)
	}
}
