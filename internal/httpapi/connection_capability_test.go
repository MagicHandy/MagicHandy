package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestLocalMotionContractsForInklingAndDefaultModels(t *testing.T) {
	const local = "http://localhost:8080"
	for _, tc := range []struct {
		provider, model, base, level, output string
		guidance, objectRoot                 bool
	}{
		{"llama_cpp", "gemma-4-e4b", local, "simple", "", true, false},
		{"llama_cpp", "gemma-4-12b", local, "intermediate", "", true, false},
		{"ollama", "gemma-4-12b", local, "intermediate", "", true, false},
		{"chatgpt", "gpt-6.1-terra", "", "advanced", "auto", false, false},
		{"llama_cpp", "inkling-2-mini", local, "intermediate", "", true, true},
		{"compatible", "inkling-2-mini", local + "/v1", "intermediate", "prompt", true, true},
		{"compatible", "thinkingmachines/Inkling-2-Mini", "http://[::1]:8080/v1", "intermediate", "prompt", true, true},
		{"compatible", "inkling-2-mini", local + "/v1", "intermediate", "auto", false, true},
		{"compatible", "inkling-2-mini", local + "/v1", "advanced", "prompt", false, true},
		{"compatible", "inkling-2-mini", "https://models.example/v1", "intermediate", "prompt", false, false},
		{"compatible", "other-model", local + "/v1", "intermediate", "prompt", false, false},
	} {
		t.Run(tc.provider+"/"+tc.model+"/"+tc.output, func(t *testing.T) {
			settings := config.DefaultSettings().LLM
			settings.Provider, settings.Model = tc.provider, tc.model
			settings.LocalCapabilityLevel, settings.LlamaCPPBaseURL = tc.level, tc.base
			if tc.output != "" {
				settings = settings.WithConnection(config.ModelConnection{ID: "review", Provider: tc.provider,
					Model: tc.model, BaseURL: tc.base, CapabilityLevel: tc.level, OutputMode: tc.output})
			}
			for _, mode := range []string{config.LLMMotionModeLayered, config.LLMMotionModeCreativeV2} {
				settings.MotionGenerationMode = mode
				caps := chatCapabilities(settings, nil)
				if caps.LocalInference != tc.guidance {
					t.Errorf("%s local guidance = %t, want %t", mode, caps.LocalInference, tc.guidance)
				}
				var schema struct {
					Type  string            `json:"type"`
					OneOf []json.RawMessage `json:"oneOf"`
				}
				raw := chat.InteractiveResponseSchema(nil, caps, nil)
				wantType := ""
				if tc.objectRoot {
					wantType = "object"
				}
				if err := json.Unmarshal(raw, &schema); err != nil || len(schema.OneOf) < 2 || schema.Type != wantType {
					t.Errorf("%s action union: root %q, branches %d, want type %q (error %v)", mode, schema.Type, len(schema.OneOf), wantType, err)
				}
			}
		})
	}
}

func TestGemmaKeepsTheLastFittingHistoryMessage(t *testing.T) {
	const window = 32768
	const cue = "For later, my catalog word is lapwing."
	for _, level := range []string{config.ModelCapabilitySimple, config.ModelCapabilityIntermediate} {
		settings := config.DefaultSettings().LLM
		settings.Provider, settings.LocalCapabilityLevel = config.LLMProviderLlamaCPP, level
		settings.MotionGenerationMode = config.LLMMotionModeLayered
		schema := chat.InteractiveResponseSchema(nil, chatCapabilities(settings, nil), nil)
		// Give the original action union seven bytes of slack. An extra
		// top-level type costs sixteen and would remove the whole cue.
		var union struct {
			OneOf []json.RawMessage `json:"oneOf"`
		}
		if err := json.Unmarshal(schema, &union); err != nil || len(union.OneOf) == 0 {
			t.Fatalf("%s: missing action union: %v", level, err)
		}
		bare, err := json.Marshal(union)
		if err != nil {
			t.Fatal(err)
		}
		req := llm.ChatRequest{MaxTokens: config.MinimumLocalOutput(level), JSONSchema: bare, Messages: []llm.Message{
			{Role: "user", Content: cue},
			{Role: "user", Content: "What was my catalog word?"},
		}}
		_, original, err := llm.BudgetChatRequest(req, window)
		if err != nil {
			t.Fatal(err)
		}
		req.Messages[1].Content += strings.Repeat("x", original.LimitBytes-original.InputBytes-7)
		req.JSONSchema = schema
		admitted, budget, err := llm.BudgetChatRequest(req, window)
		if err != nil || budget.HistoryMessagesDropped != 0 || len(admitted.Messages) != 2 || admitted.Messages[0].Content != cue {
			t.Errorf("%s: lost the oldest fitting cue, budget %+v, error %v", level, budget, err)
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
