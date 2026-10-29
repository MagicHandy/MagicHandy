package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestChatFailureReportsActualResponsesReasoning(t *testing.T) {
	for _, provider := range []string{config.LLMProviderChatGPT, config.LLMProviderOpenAI, config.LLMProviderOpenRouter} {
		for _, effort := range []string{"none", "low", ""} {
			t.Run(provider+"/"+effort, func(t *testing.T) {
				settings := config.LLMSettings{ReasoningMode: "off"}.WithConnection(config.ModelConnection{
					ID: "fixture", Name: "Fixture", Provider: provider, Model: "fixture-model", ReasoningEffort: effort,
				})
				result := chat.Result{Input: &llm.ChatRequest{ReasoningMode: "off"}}
				item := (&Server{}).chatFailureEvidence(result, nil, chat.MessageDiagnostics{}, settings)
				want := effort
				if want == "" {
					want = "default"
				}
				if item.ReasoningMode != want {
					t.Fatalf("reported %q, want actual Responses effort %q", item.ReasoningMode, want)
				}
			})
		}
	}
	item := (&Server{}).chatFailureEvidence(chat.Result{Input: &llm.ChatRequest{ReasoningMode: "off"}}, nil,
		chat.MessageDiagnostics{}, config.LLMSettings{ReasoningMode: "auto"})
	if item.ReasoningMode != "off" {
		t.Fatalf("local request reasoning changed: %q", item.ReasoningMode)
	}
}
