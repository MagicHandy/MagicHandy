package chat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func levelState(control bool) MotionContext {
	before := DefaultLayeredScore(40)
	return MotionContext{MotionMode: MotionModeLayered, Running: true, Layered: &before, SpeedPercent: 40, SpeedMinPercent: 10, SpeedMaxPercent: 80,
		Outputs: &OutputContext{Strokers: 1, Vibrator: &VibrationContext{LevelControl: control, LevelPercent: 50, Ready: true, Follow: "speed", Motors: 1}}}
}

func TestLocalTiersGetOneNullableVibrationLevel(t *testing.T) {
	for _, tier := range []string{"simple", "intermediate"} {
		t.Run(tier, func(t *testing.T) {
			caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, SimpleMotion: tier == "simple", IntermediateMotion: tier == "intermediate"}
			state := levelState(true)
			if schema := string(InteractiveResponseSchema(nil, caps, &state)); !strings.Contains(schema, `"vibration_level":{"anyOf":[{"type":"null"},{"maximum":100,"minimum":0,"type":"integer"}]}`) {
				t.Fatalf("missing level schema: %s", schema)
			}
			edits := `{}`
			if tier == "intermediate" {
				edits = `[]`
			}
			svc := Service{Capabilities: &caps}
			raw := `{"action":"none","edits":` + edits + `,"vibration_level":80,"reply":"Stronger."}`
			response, err := svc.parseLayeredResponse(raw, *state.Layered, state, config.DefaultSettings().Motion, false)
			if err != nil || response.Motion == nil || response.Motion.VibrationLevel == nil || *response.Motion.VibrationLevel != 80 || response.Motion.Layered != nil {
				t.Fatalf("level-only reply: %+v %v", response.Motion, err)
			}
			response, err = svc.parseLayeredResponse(strings.Replace(raw, "80", "null", 1), *state.Layered, state, config.DefaultSettings().Motion, false)
			if err != nil || (response.Motion != nil && response.Motion.VibrationLevel != nil) {
				t.Fatalf("null must keep the level: %+v %v", response.Motion, err)
			}
			if _, err := svc.parseLayeredResponse(strings.Replace(raw, "80", "120", 1), *state.Layered, state, config.DefaultSettings().Motion, false); err == nil {
				t.Fatal("accepted a level above 100")
			}
			// Stop's suspension: the schema allows only null and a raise is dropped.
			state.Outputs.Vibrator.Stopped = true
			if schema := string(InteractiveResponseSchema(nil, caps, &state)); !strings.Contains(schema, `"vibration_level":{"type":"null"}`) {
				t.Fatalf("stopped schema still offers a level: %s", schema)
			}
			response, err = svc.parseLayeredResponse(raw, *state.Layered, state, config.DefaultSettings().Motion, false)
			if err != nil || (response.Motion != nil && response.Motion.VibrationLevel != nil) {
				t.Fatalf("a stopped vibrator accepted a raise: %+v %v", response.Motion, err)
			}
		})
	}
}

func TestAdvancedAndManualControlHaveNoVibrationLevel(t *testing.T) {
	advanced := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true}
	state := levelState(true)
	if schema := string(InteractiveResponseSchema(nil, advanced, &state)); strings.Contains(schema, "vibration_level") {
		t.Fatal("Advanced received the local level")
	}
	intermediate := Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: true}
	state = levelState(false)
	if schema := string(InteractiveResponseSchema(nil, intermediate, &state)); strings.Contains(schema, "vibration_level") {
		t.Fatal("model control off still offers a level")
	}
	svc := Service{Capabilities: &intermediate}
	response, err := svc.parseLayeredResponse(`{"action":"none","edits":[],"vibration_level":90,"reply":"Stronger."}`, *state.Layered, state, config.DefaultSettings().Motion, false)
	if err != nil || (response.Motion != nil && response.Motion.VibrationLevel != nil) {
		t.Fatalf("an unavailable level must be dropped, not applied: %+v %v", response.Motion, err)
	}
}

func TestPausedPlaybackDoesNotAcceptALocalVibrationLevel(t *testing.T) {
	state := levelState(true)
	state.Running, state.Paused = false, true
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: true}
	if schema := string(InteractiveResponseSchema(nil, caps, &state)); !strings.Contains(schema, `"vibration_level":{"type":"null"}`) {
		t.Fatal("paused schema still offers a level change")
	}
	svc := Service{Capabilities: &caps}
	if _, err := svc.parseLayeredResponse(`{"action":"none","edits":[],"vibration_level":90,"reply":"Stronger."}`, *state.Layered, state, config.DefaultSettings().Motion, false); err == nil {
		t.Fatal("a local model changed vibration while paused")
	}
}

func TestVibrationLevelGuidesFitEachTier(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	simple := Capabilities{Motion: true, MotionMode: MotionModeLayered, SimpleMotion: true}
	state := levelState(true)
	prompt := ComposeInteractivePrompt(set, nil, nil, simple, &state, nil).Prompt
	if !strings.Contains(prompt, "VIBRATOR: Vibration follows the movement.") || !strings.Contains(prompt, "it is 50 now") || strings.Contains(prompt, "Active playback outputs") {
		t.Fatal("Simple must get only the compact level guide")
	}
	state = levelState(false)
	if prompt := ComposeInteractivePrompt(set, nil, nil, simple, &state, nil).Prompt; strings.Contains(prompt, "VIBRATOR:") {
		t.Fatal("Simple without model control gained vibrator text")
	}
	intermediate := Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: true}
	state = levelState(true)
	prompt = ComposeInteractivePrompt(set, nil, nil, intermediate, &state, nil).Prompt
	if !strings.Contains(prompt, "VIBRATION LEVEL:") || !strings.Contains(prompt, `"level_percent_of_max":50`) {
		t.Fatal("Intermediate lost its level guide or facts")
	}
	// The model owns the strength here, so the fixed-strength lines must go.
	if strings.Contains(prompt, "Never claim to change its strength") || strings.Contains(prompt, "use no change and explain the limitation") ||
		!strings.Contains(prompt, "vibration from idle means starting movement") {
		t.Fatal("Intermediate level guide kept the fixed-strength contract")
	}
	state = levelState(false)
	prompt = ComposeInteractivePrompt(set, nil, nil, intermediate, &state, nil).Prompt
	if strings.Contains(prompt, "VIBRATION LEVEL:") || !strings.Contains(prompt, "Never claim to change its strength") {
		t.Fatal("Intermediate without model control must keep the fixed-strength guide alone")
	}
}

func TestAutopilotKeepsTheLiveChatLevel(t *testing.T) {
	state := levelState(true)
	copied := withoutLiveVibrationLevel(&state)
	if copied.Outputs.Vibrator.LevelControl || !state.Outputs.Vibrator.LevelControl || copied.Outputs.Vibrator.LevelPercent != 50 {
		t.Fatalf("copy %+v original %+v", copied.Outputs.Vibrator, state.Outputs.Vibrator)
	}
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: true}
	svc := Service{Capabilities: &caps, TrustedMotionInput: true}
	_, schema := svc.continuousPrompt(PromptSet{}, caps, *copied, config.DefaultSettings().Motion, false)
	if strings.Contains(string(schema), "vibration_level") {
		t.Fatal("Autopilot schema offers the live-chat level")
	}
}

func TestEchoedStrokeWidthIsNotAMotionChange(t *testing.T) {
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, SimpleMotion: true}
	state := levelState(true)
	svc := Service{Capabilities: &caps}
	// E4B copies the displayed widest stroke (the band's width) with its level.
	raw := `{"action":"update","edits":{"layers":[{"axis":"center","amount_percent":35,"period_cycles":14,"phase_percent":17,"shape":"drift"},{"axis":"pace","amount_percent":20,"period_cycles":22,"phase_percent":53,"shape":"drift"}],"stroke_width":{"max_percent":90,"min_percent":20}},"reply":"Stronger.","vibration_level":75}`
	response, err := svc.parseLayeredResponse(raw, *state.Layered, state, config.DefaultSettings().Motion, false)
	if err != nil || response.Motion == nil || response.Motion.Layered != nil || response.Motion.VibrationLevel == nil || *response.Motion.VibrationLevel != 75 {
		t.Fatalf("an echoed score retargeted motion: %+v %v", response.Motion, err)
	}
	// A real narrowing is still a change.
	narrowed := strings.Replace(raw, `"max_percent":90`, `"max_percent":60`, 1)
	response, err = svc.parseLayeredResponse(narrowed, *state.Layered, state, config.DefaultSettings().Motion, false)
	if err != nil || response.Motion == nil || response.Motion.Layered == nil || response.Motion.Layered.RangeCeilingPercent != 60 {
		t.Fatalf("a narrower stroke was dropped: %+v %v", response.Motion, err)
	}
}

func TestSimpleCreativeV2HasNoVibrationLevel(t *testing.T) {
	caps := Capabilities{Motion: true, MotionMode: MotionModeCreativeV2, SimpleMotion: true}
	state := levelState(true)
	state.MotionMode = MotionModeCreativeV2
	if schema := string(InteractiveResponseSchema(nil, caps, &state)); strings.Contains(schema, "vibration_level") {
		t.Fatal("Simple Creative v2 was offered the level")
	}
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	if prompt := ComposeInteractivePrompt(set, nil, nil, caps, &state, nil).Prompt; strings.Contains(prompt, "vibration_level") {
		t.Fatal("Simple Creative v2 prompt mentions the level")
	}
	caps.SimpleMotion, caps.IntermediateMotion = false, true
	if schema := string(InteractiveResponseSchema(nil, caps, &state)); !strings.Contains(schema, "vibration_level") {
		t.Fatal("Intermediate Creative v2 lost the level")
	}
}

func TestStartingMovementRequiresTheModelToChooseALevel(t *testing.T) {
	for _, tc := range []struct {
		name  string
		caps  Capabilities
		field string
	}{
		{"intermediate", Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: true}, "vibration_level"},
		{"advanced", Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true}, "vibration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := levelState(true)
			state.Running = false
			state.Outputs.Vibrator.Independent = tc.caps.AdvancedMotion
			state.Outputs.Vibrator.LevelControl = !tc.caps.AdvancedMotion
			var schema struct {
				OneOf []struct {
					Properties map[string]map[string]any `json:"properties"`
				} `json:"oneOf"`
			}
			if err := json.Unmarshal(InteractiveResponseSchema(nil, tc.caps, &state), &schema); err != nil || len(schema.OneOf) != 2 {
				t.Fatalf("branches: %v", err)
			}
			for _, branch := range schema.OneOf {
				field := branch.Properties[tc.field]
				nullable := field["anyOf"] != nil || field["type"] == "null"
				if start := branch.Properties["action"]["const"] == MotionActionStart; start == nullable {
					t.Fatalf("action %v: %s nullable=%t", branch.Properties["action"]["const"], tc.field, nullable)
				}
			}
		})
	}
}
