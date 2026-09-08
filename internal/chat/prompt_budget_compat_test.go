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
