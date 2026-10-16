package chat

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestBuiltinMotionPromptsFitMinimumManagedContext(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	for _, mode := range []MotionMode{MotionModeDynamic, MotionModeLayered, MotionModeCreativeV2} {
		t.Run(string(mode), func(t *testing.T) {
			caps := FullCapabilities()
			caps.MotionMode, caps.Voice = mode, VoiceExplicit
			state := &MotionContext{MotionMode: mode}
			prompt := ComposePrompt(set, nil, nil, caps, state, nil)
			schema := PatternResponseSchema(nil, caps, state)
			if mode == MotionModeLayered {
				schema = LayeredResponseSchema(config.DefaultSettings().Motion, true)
			}
			if mode == MotionModeCreativeV2 {
				schema = CreativeV2ResponseSchema(config.DefaultSettings().Motion, true)
			}
			_, budget, err := llm.BudgetChatRequest(llm.ChatRequest{MaxTokens: 256, JSONSchema: schema, Messages: []llm.Message{{Role: "system", Content: prompt.Prompt}, {Role: "user", Content: "Hello."}}}, 16384)
			if err != nil {
				t.Fatalf("builtin %s prompt (%d bytes) is unusable at minimum supported context: %v", mode, prompt.Bytes, err)
			}
			t.Logf("system=%d bytes; request=%d bytes; limit=%d bytes", prompt.Bytes, budget.InputBytes, budget.LimitBytes)
		})
	}
}

func TestInteractiveMotionPromptsFitMinimumManagedContext(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, running := range []bool{false, true} {
			caps := FullCapabilities()
			caps.MotionMode, caps.Voice = mode, VoiceExplicit
			state := &MotionContext{MotionMode: mode, Running: running}
			prompt := ComposeInteractivePrompt(set, nil, nil, caps, state, nil)
			schema := InteractiveResponseSchema(nil, caps, state)
			_, budget, err := llm.BudgetChatRequest(llm.ChatRequest{MaxTokens: 256, JSONSchema: schema, Messages: []llm.Message{{Role: "system", Content: prompt.Prompt}, {Role: "user", Content: "Start smooth full-range strokes from 0 to 100 at speed 20 percent, with even timing in both directions."}}}, 16384)
			if err != nil {
				t.Errorf("live %s running=%t prompt exceeds the minimum context: %v", mode, running, err)
			}
			t.Logf("%s running=%t request=%d bytes; limit=%d bytes", mode, running, budget.InputBytes, budget.LimitBytes)
		}
	}
}
