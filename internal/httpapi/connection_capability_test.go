package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestLocalMotionPromptsForInklingConnections(t *testing.T) {
	for _, tc := range []struct {
		model, base, level, output string
		want                       bool
	}{
		{"inkling-2-mini", "http://127.0.0.1:8080/v1", "intermediate", "prompt", true},
		{"thinkingmachines/Inkling-2-Mini", "http://[::1]:8080/v1", "intermediate", "prompt", true},
		{"inkling-2-mini", "https://models.example/v1", "intermediate", "prompt", false},
		{"other-model", "http://localhost:8080/v1", "intermediate", "prompt", false},
		{"inkling-2-mini", "http://localhost:8080/v1", "advanced", "prompt", false},
		{"inkling-2-mini", "http://localhost:8080/v1", "intermediate", "auto", false},
	} {
		connection := config.ModelConnection{ID: "candidate", Provider: config.LLMProviderCompatible,
			Model: tc.model, BaseURL: tc.base, CapabilityLevel: tc.level, OutputMode: tc.output}
		settings := config.DefaultSettings().LLM.WithConnection(connection)
		if got := chatCapabilities(settings, nil).LocalInference; got != tc.want {
			t.Errorf("model %q at %q, %s/%s: local prompts = %t, want %t", tc.model, tc.base, tc.level, tc.output, got, tc.want)
		}
	}
}

func lmStudioChat(level string) config.LLMSettings {
	settings := config.DefaultSettings().LLM
	settings.MotionGenerationMode = config.LLMMotionModeLayered
	settings.Connections = []config.ModelConnection{
		config.ModelConnection{ID: "lm-studio", Name: "LM Studio", Provider: config.LLMProviderCompatible, BaseURL: "http://127.0.0.1:1234/v1", Model: "qwen3-14b", NoAuthentication: true, CapabilityLevel: level}.Normalize(),
		config.ModelConnection{ID: "review", Name: "Reviewer", Provider: config.LLMProviderOpenAI, Model: "gpt-6-sol"}.Normalize(),
	}
	settings.ConversationConnectionID = "lm-studio"
	settings.MotionReviewer = config.MotionReviewerSettings{Enabled: true, ConnectionID: "review"}
	return settings
}

func TestSimpleConnectionUsesTheCompactContractAndBudget(t *testing.T) {
	chat := lmStudioChat(config.ModelCapabilitySimple).ConversationSettings()
	capabilities := chatCapabilities(chat, nil)
	if capabilities.AdvancedMotion || capabilities.IntermediateMotion || capabilities.PreserveConversationText {
		t.Fatalf("Simple connection received a richer contract: %+v", capabilities)
	}
	if budget := chatPromptBudget(chat); budget.ContextSize != 16384 {
		t.Fatalf("Simple connection budget = %+v", budget)
	}
	advanced := lmStudioChat("").ConversationSettings()
	if !chatCapabilities(advanced, nil).AdvancedMotion || chatPromptBudget(advanced).ContextSize != config.MinimumExternalContext {
		t.Fatal("a connection saved without a level lost its Advanced contract")
	}
}

func TestSecondOpinionReviewsModelsBelowAdvancedOnAnyTransport(t *testing.T) {
	server := &Server{}
	for _, tc := range []struct {
		name     string
		settings config.LLMSettings
		want     bool
	}{
		{"simple connection", lmStudioChat(config.ModelCapabilitySimple).ConversationSettings(), true},
		{"intermediate connection", lmStudioChat(config.ModelCapabilityIntermediate).ConversationSettings(), true},
		{"advanced connection", lmStudioChat(config.ModelCapabilityAdvanced).ConversationSettings(), false},
		{"connection without a level", lmStudioChat("").ConversationSettings(), false},
	} {
		if got := server.reviewModelChoice(tc.settings); got != tc.want {
			t.Errorf("%s: review = %v, want %v", tc.name, got, tc.want)
		}
	}
	local := lmStudioChat("")
	local.ConversationConnectionID = ""
	if !server.reviewModelChoice(local.ConversationSettings()) {
		t.Error("the Simple local model is no longer reviewed")
	}
	local.LocalCapabilityLevel, local.LlamaCPPContextSize, local.MaxOutputTokens = config.ModelCapabilityAdvanced, config.MinimumExternalContext, 1024
	if server.reviewModelChoice(local.ConversationSettings()) {
		t.Error("an Advanced local model is reviewed")
	}
}

func TestMotionCardSavesPatternLibraryOptionsWithoutChangingTheMode(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	put := func(body string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPut, "/api/settings/llm-motion-mode", strings.NewReader(body))))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
	}
	put(`{"mode":"pattern"}`)
	put(`{"mode":"pattern","area_focus":false,"experimental_patterns":true}`)
	saved, _ := server.store.Snapshot()
	capabilities := saved.LLM.Capabilities()
	if saved.LLM.MotionGenerationMode != config.LLMMotionModePattern || capabilities.AreaFocus || !capabilities.ExperimentalPatterns || !capabilities.Patterns {
		t.Fatalf("saved = %s %+v", saved.LLM.MotionGenerationMode, capabilities)
	}
	put(`{"mode":"pattern"}`)
	kept, _ := server.store.Snapshot()
	if kept.LLM.Capabilities() != capabilities {
		t.Fatal("a request without options changed them")
	}
}
