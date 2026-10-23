package chat

import (
	"encoding/json"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestLocalPausedCreativeUsesReplyOnlyWithoutWeakeningValidation(t *testing.T) {
	for _, hosted := range []bool{false, true} {
		caps := Capabilities{Motion: true, MotionMode: MotionModeDynamic, HostedMotion: hosted}
		for _, paused := range []bool{false, true} {
			state := MotionContext{MotionMode: MotionModeDynamic, Paused: paused, SpeedMinPercent: 10, SpeedMaxPercent: 80}
			schema := PatternResponseSchema(nil, caps, &state)
			if (len(schema) > 0) != (!hosted && paused) {
				t.Fatal("state-specific local grammar changed another motion contract")
			}
			if !hosted && paused {
				var root struct{ Properties map[string]json.RawMessage }
				if err := json.Unmarshal(schema, &root); err != nil || len(root.Properties) != 1 || root.Properties["reply"] == nil {
					t.Fatalf("paused grammar is not reply-only: %s", schema)
				}
				provider := &scriptedProvider{responses: []string{hostedStartFixture(MotionModeDynamic, MotionActionStart, 40)}}
				svc := Service{Provider: provider, Capabilities: &caps, MotionContext: &state, SingleAttempt: true}
				result, _ := svc.Complete(t.Context(), Request{Message: "Start moving now."}, nil)
				if result.Response.Motion != nil || len(provider.requests) != 1 || !result.Malformed {
					t.Fatal("an unconstrained provider bypassed the paused-state validator")
				}
			}
		}
	}
}

func TestLocalStateSchemasFitVoiceAndContextBudgets(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	for _, mode := range []MotionMode{MotionModeDynamic, MotionModePattern} {
		for _, voice := range []VoiceLevel{VoiceUtility, VoiceWarm, VoiceIntimate, VoiceExplicit} {
			for _, state := range []MotionContext{{}, {Running: true}, {Paused: true}} {
				state.MotionMode, state.SpeedMinPercent, state.SpeedMaxPercent = mode, 10, 80
				caps := Capabilities{Motion: true, MotionMode: mode, Patterns: mode == MotionModePattern, Voice: voice}
				choices := continuousTestChoices()
				prompt := ComposeInteractivePrompt(set, nil, choices, caps, &state, nil)
				input := llm.ChatRequest{MaxTokens: 256, JSONSchema: InteractiveResponseSchema(choices, caps, &state), Messages: []llm.Message{
					{Role: "system", Content: prompt.Prompt}, {Role: "user", Content: "Only explain the available controls; keep motion unchanged."},
				}}
				for _, size := range []int{16384, 32768} {
					if _, _, err := llm.BudgetChatRequest(input, size); err != nil {
						t.Fatalf("%s voice=%s running=%t paused=%t context=%d: %v", mode, voice, state.Running, state.Paused, size, err)
					}
				}
			}
		}
	}
}
