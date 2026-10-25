package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestIntermediateLayeredEditsMatchSharedDomain(t *testing.T) {
	for _, tc := range []struct{ name, intermediate, legacy string }{
		{"paired reach", `[{"range":{"min_percent":65,"max_percent":100}},{"geometry":"wander"}]`, `{"controls":{"min_percent":65,"max_percent":100},"geometry":"wander"}`},
		{"full slider mixture", `[{"geometry":"full_slider_and_tip"},{"stroke_width":{"min_percent":38,"max_percent":100}}]`, `{"controls":{"min_percent":0,"max_percent":100},"geometry":"full_and_tip","stroke_width":{"min_percent":38,"max_percent":100}}`},
		{"full band mixture", `[{"geometry":"full_band_and_base"}]`, `{"geometry":"full_and_base"}`},
		{"relative pace", `[{"change_by":{"speed_percent":7}}]`, `{"change_by":{"speed_percent":7}}`},
		{"independent layers", `[{"layers":[{"axis":"pace","period_change_cycles":4}]},{"remove_layers":["center"]}]`, `{"layers":[{"axis":"pace","period_change_cycles":4}],"remove_layers":["center"]}`},
		{"hold", `[]`, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrap := func(edits string) string { return `{"edits":` + edits + `,"reply":"Selected motion."}` }
			before := DefaultLayeredScore(43)
			limits := config.DefaultSettings().Motion
			_, want, _, err := ParseLayeredReply(wrap(tc.legacy), before, limits)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := normalizeIntermediateLayeredReply(wrap(tc.intermediate))
			if err != nil {
				t.Fatal(err)
			}
			_, got, _, err := ParseLayeredReply(raw, before, limits)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("domain mismatch: got=%+v want=%+v error=%v", got, want, err)
			}
			if got.SpeedPercent != want.SpeedPercent {
				t.Fatal("geometry changed pace")
			}
		})
	}
}

func TestIntermediateLayeredRejectsAmbiguityWithoutMutation(t *testing.T) {
	for _, edits := range []string{
		`[{"range":{"min_percent":5}}]`, `[{"range":{"min_percent":null,"max_percent":80}}]`,
		`[{"range":{"min_percent":5,"min_percent":8,"max_percent":80}}]`,
		`[{"range":{"min_percent":5,"max_percent":101}}]`,
		`[{"range":{"min_percent":80,"max_percent":5}}]`,
		`[{"range":{"min_percent":5,"max_percent":80}},{"controls":{"min_percent":5}}]`,
		`[{"controls":{"speed_percent":45}},{"controls":{"speed_percent":55}}]`,
		`[{"controls":{"speed_percent":45,"speed_percent":55}}]`,
		`[{"geometry":"full_slider_and_tip"},{"range":{"min_percent":20,"max_percent":100}}]`,
		`[{"geometry":"full_slider_and_tip"},{"change_by":{"min_percent":5}}]`,
		`[{"geometry":"wander","evolve":true}]`, `[{"unknown":true}]`,
		`[{"controls":{"speed_percent":45}},{"change_by":{"speed_percent":5}}]`,
		`null`,
	} {
		current := DefaultLayeredScore(43)
		before := motion.CloneFlowSpec(&current)
		caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: true}
		service := Service{Capabilities: &caps}
		state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &current}
		reply, err := service.parseLayeredResponse(`{"action":"update","edits":`+edits+`,"reply":"Invalid."}`, current, state, config.DefaultSettings().Motion, false)
		if err == nil || reply.Motion != nil || !reflect.DeepEqual(current, *before) {
			t.Fatalf("invalid edits mutated score: %s (%v)", edits, err)
		}
	}
}

func TestIntermediateGrammarDoesNotGrantAdvancedToolsOrStartAuthority(t *testing.T) {
	for _, level := range []string{"simple", "intermediate", "advanced"} {
		caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: level == "intermediate", AdvancedMotion: level == "advanced"}
		for _, state := range []MotionContext{{Running: true}, {Paused: true}, {}} {
			state.MotionMode = MotionModeLayered
			caps.PreserveConversationText = caps.AdvancedMotion
			provider := &scriptedProvider{responses: []string{`{"action":"none","edits":[],"reply":"Only explaining."}`}}
			service := Service{Provider: provider, Capabilities: &caps, MotionContext: &state, SingleAttempt: true}
			result, err := service.Complete(t.Context(), Request{Message: "Explain only; no movement."}, nil)
			if result.Response.Motion != nil {
				t.Fatal("reply-only request acquired motion")
			}
			if level == "intermediate" && err != nil {
				t.Fatalf("intermediate reply failed: %v", err)
			}
			if level == "simple" && state.Running && err == nil {
				t.Fatal("simple accepted intermediate dialect")
			}
			if len(provider.requests) != 1 {
				t.Fatal("test unexpectedly repaired")
			}
			schema := string(provider.requests[0].JSONSchema)
			if level == "intermediate" && (strings.Contains(schema, "vibration") || strings.Contains(schema, "next_check_seconds") || strings.Contains(schema, `"phrase"`)) {
				t.Fatal("intermediate acquired advanced controls")
			}
		}
	}
}

func TestIntermediateFullGuidanceFits32KAcrossVoiceAndState(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	for _, voice := range []VoiceLevel{VoiceUtility, VoiceWarm, VoiceIntimate, VoiceExplicit} {
		for _, state := range []MotionContext{{Running: true}, {Paused: true}, {}} {
			state.MotionMode, state.SpeedMinPercent, state.SpeedMaxPercent = MotionModeLayered, 10, 100
			state.Outputs = &OutputContext{Strokers: 1, Vibrator: &VibrationContext{Ready: true, Armed: true, Follow: "speed", Motors: 2}}
			caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: true, Voice: voice}
			prompt := ComposeInteractivePrompt(set, nil, nil, caps, &state, nil, PromptBudgetSettings{ContextSize: 32768, MaxOutputTokens: 512})
			schema := InteractiveResponseSchema(nil, caps, &state)
			if !json.Valid(schema) {
				t.Fatal("invalid schema")
			}
			request := llm.ChatRequest{MaxTokens: 512, JSONSchema: schema, Messages: []llm.Message{{Role: "system", Content: prompt.Prompt}, {Role: "user", Content: "Describe the pattern."}}}
			if _, _, err := llm.BudgetChatRequest(request, 32768); err != nil {
				t.Fatal(err)
			}
			if state.Running && !strings.Contains(prompt.Prompt, "period_change_cycles") {
				t.Fatal("full guidance lost independent layer timing")
			}
		}
	}
}

func TestIntermediateLayeredAutopilotRetainsCompactGrammar(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`{"edits":{"geometry":"wander"},"reply":"Developing freely."}`}}
	flow := DefaultLayeredScore(43)
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: true}
	service := AutopilotService{Provider: provider, Capabilities: caps, MotionContext: &MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &flow}, MaxTokens: 512}
	result, err := service.Complete(t.Context(), AutopilotKindMotion, Request{Message: AutopilotMotionMessage(AutopilotContext{Intermediate: true, MotionMode: MotionModeLayered, CurrentFlow: &flow})})
	if err != nil || result.Motion == nil || len(provider.requests) != 1 {
		t.Fatalf("autonomous contract: %+v %v", result, err)
	}
	if strings.Contains(string(provider.requests[0].JSONSchema), "full_slider_and_tip") {
		t.Fatal("experimental interactive grammar leaked into autonomous Layered")
	}
	if !caps.IntermediateMotion {
		t.Fatal("autonomous call mutated interactive capabilities")
	}
}
