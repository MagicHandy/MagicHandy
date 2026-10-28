package chat

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestOutputFactsLeaveLocalPromptsAndSchemasUnchanged(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2, MotionModeDynamic, MotionModePattern, MotionModeOff} {
		for _, running := range []bool{false, true} {
			state := MotionContext{MotionMode: mode, Running: running, SpeedPercent: 40, SpeedMinPercent: 10, SpeedMaxPercent: 80}
			caps := Capabilities{Motion: mode != MotionModeOff, MotionMode: mode}
			before := ComposeInteractivePrompt(set, nil, nil, caps, &state, nil).Prompt
			schema := string(InteractiveResponseSchema(nil, caps, &state))
			state.Outputs = &OutputContext{Strokers: 1, Vibrator: &VibrationContext{Independent: true, Ready: true}}
			if after := ComposeInteractivePrompt(set, nil, nil, caps, &state, nil).Prompt; after != before {
				t.Fatalf("local %s prompt changed", mode)
			}
			if string(InteractiveResponseSchema(nil, caps, &state)) != schema {
				t.Fatalf("local %s schema changed", mode)
			}
		}
	}
}

func TestIndependentVibrationValidationAndIsolation(t *testing.T) {
	before := DefaultLayeredScore(40)
	state := MotionContext{MotionMode: MotionModeLayered, Running: true, Layered: &before, Outputs: &OutputContext{Strokers: 1, Vibrator: &VibrationContext{Independent: true, Ready: true}}}
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true}
	svc := Service{Capabilities: &caps}
	raw := `{"action":"none","edits":{},"vibration":{"mode":"wave","min_percent":10,"max_percent":70,"period_seconds":4},"reply":"Independent swell."}`
	response, err := svc.parseLayeredResponse(raw, before, state, config.DefaultSettings().Motion, false)
	if err != nil || response.Motion == nil || response.Motion.Action != MotionActionNone || response.Motion.Layered != nil || response.Motion.Vibration == nil {
		t.Fatalf("isolated output: %+v %v", response, err)
	}
	for _, bad := range []string{
		strings.Replace(raw, `"max_percent":70`, `"max_percent":101`, 1),
		strings.Replace(raw, `"period_seconds":4`, `"period_seconds":0`, 1),
		strings.Replace(raw, `"min_percent":10`, `"min_percent":80`, 1),
		strings.Replace(raw, `"mode":"wave"`, `"mode":"off"`, 1),
		strings.Replace(raw, `"mode":"wave"`, `"mode":"wave","motor":1`, 1),
		strings.Replace(raw, `"min_percent":10`, `"min_percent":10,"min_percent":0`, 1),
	} {
		if _, err := svc.parseLayeredResponse(bad, before, state, config.DefaultSettings().Motion, false); err == nil {
			t.Fatalf("accepted malformed envelope: %s", bad)
		}
	}
	state.Outputs.Vibrator.Stopped = true
	if _, err := svc.parseLayeredResponse(raw, before, state, config.DefaultSettings().Motion, false); err == nil {
		t.Fatal("model armed a zero-intensity device")
	}
	state.Outputs.Vibrator.Stopped = false
	state.Paused = true
	if _, err := svc.parseLayeredResponse(raw, before, state, config.DefaultSettings().Motion, false); err == nil {
		t.Fatal("model changed a paused output")
	}
	caps.Motion = false
	if _, err := svc.parseLayeredResponse(raw, before, state, config.DefaultSettings().Motion, false); err == nil {
		t.Fatal("chat-only request gained output control")
	}
}

func TestGeometryRepairNamesConflictingStructuredFields(t *testing.T) {
	before := DefaultLayeredScore(40)
	_, _, _, err := ParseLayeredReply(`{"edits":{"geometry":"full_band","layers":[{"axis":"center","amount_percent":35}]},"reply":"Full strokes."}`, before, config.DefaultSettings().Motion)
	var conflict *LayeredGeometryConflict
	if !errors.As(err, &conflict) || conflict.Geometry != "full_band" || !strings.Contains(conflict.Expected, "no range/center") || !strings.Contains(conflict.Actual, "center amount_percent 35") {
		t.Fatalf("repair cannot locate conflict: %v", err)
	}
}

func TestUnavailableVibrationStrictSchemaUsesTypedOffOrNull(t *testing.T) {
	state := MotionContext{MotionMode: MotionModeLayered, Outputs: &OutputContext{Vibrator: &VibrationContext{Independent: true, Ready: true, Stopped: true}}}
	caps := Capabilities{Motion: true, AdvancedMotion: true, MotionMode: MotionModeLayered}
	raw, err := llm.OpenAISchema(InteractiveResponseSchema(nil, caps, &state))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	branches := schema["properties"].(map[string]any)["proposal"].(map[string]any)["anyOf"].([]any)
	for _, branch := range branches {
		value := branch.(map[string]any)["properties"].(map[string]any)["vibration"].(map[string]any)
		choices := value["anyOf"].([]any)
		object := choices[1].(map[string]any)
		if object["type"] != "object" || object["const"] != nil || object["additionalProperties"] != false {
			t.Fatalf("unsupported strict output: %s", raw)
		}
		mode := object["properties"].(map[string]any)["mode"].(map[string]any)["enum"].([]any)
		if len(mode) != 1 || mode[0] != "off" {
			t.Fatal("a stopped schema enabled output")
		}
	}
}
