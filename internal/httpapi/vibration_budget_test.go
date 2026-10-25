package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestIndependentVibrationFitsDefaultExternalContext(t *testing.T) {
	for _, mode := range []string{config.LLMMotionModeLayered, config.LLMMotionModeCreativeV2} {
		t.Run(mode, func(t *testing.T) {
			s := independentVibrationServer(t)
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.MotionGenerationMode = mode
				return settings
			})
			edits := "{}"
			if mode == config.LLMMotionModeCreativeV2 {
				edits = "[]"
			}
			provider := &scriptedLLMProvider{responses: []string{`{"action":"none","edits":` + edits + `,"vibration":null,"reply":"Ready."}`}}
			s.llm.provider = provider
			postChatStream(t, s, `{"message":"Explain independent vibration without changing either device."}`)
			if len(provider.requests) != 1 {
				t.Fatal("request did not reach provider exactly once")
			}
			request := provider.requests[0]
			_, budget, err := llm.BudgetChatRequest(request, config.MinimumExternalContext)
			t.Logf("input=%d schema=%d output=%d remaining=%d", budget.InputBytes, len(request.JSONSchema), request.MaxTokens, budget.LimitBytes-budget.InputBytes)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
