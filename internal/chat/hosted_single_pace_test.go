package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestHostedSinglePaceUsesExistingControlValidation(t *testing.T) {
	limits := config.DefaultSettings().Motion
	before := DefaultLayeredScore(56)
	before.Seed = 479
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true}
	state := MotionContext{MotionMode: MotionModeLayered, Running: true, Layered: &before, SpeedMinPercent: limits.SpeedMinPercent, SpeedMaxPercent: limits.SpeedMaxPercent}
	schema := InteractiveResponseSchema(nil, caps, &state)
	wire := `{"proposal":{"action":"update","edits":{"composition":"speed_only","speed_percent":{"op":"adjust","value":-7}},"reply":"Slowing down."}}`
	raw, err := llm.DomainOutput(wire, schema)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeLayeredOperations(raw, false); err == nil {
		t.Fatal("accepted a shortcut outside its advertised contract")
	}
	normalized, err := normalizeLayeredOperations(raw, true)
	if err != nil {
		t.Fatal(err)
	}
	_, after, _, err := ParseLayeredReply(normalized, before, limits)
	want := before
	want.SpeedPercent -= 7
	if err != nil || !reflect.DeepEqual(after, want) {
		t.Fatalf("the speed edit changed the score incorrectly: %+v, %v", after, err)
	}
	for _, edits := range []string{
		`{"composition":"speed_only"}`,
		`{"composition":"speed_only","speed_percent":12}`,
		`{"composition":"speed_only","speed_percent":{"op":"adjust","value":"fast"}}`,
		`{"composition":"speed_only","speed_percent":{"op":"set","value":39},"evolve":true}`,
		`{"composition":"speed_only","speed_percent":{"op":"set","value":39},"layers":{}}`,
		`{"composition":"speed_only","speed_percent":{"op":"set","value":39},"recall":1}`,
	} {
		raw := `{"action":"update","edits":` + edits + `,"reply":"OK"}`
		normalized, err := normalizeLayeredOperations(raw, true)
		if err == nil {
			_, _, _, err = ParseLayeredReply(normalized, before, limits)
		}
		if err == nil {
			t.Errorf("accepted malformed speed-only operation %s", edits)
		}
	}
}

func TestHostedSinglePaceSchemaMatchesInteractivePrompt(t *testing.T) {
	base := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true}
	local, creative := base, base
	local.LocalInference, creative.MotionMode = true, MotionModeCreativeV2
	for _, tc := range []struct {
		name  string
		caps  Capabilities
		state MotionContext
		id    string
		want  bool
	}{
		{"running", base, MotionContext{Running: true}, DefaultPromptSetID, true},
		{"live chat during autopilot", base, MotionContext{Running: true, Autopilot: true}, DefaultPromptSetID, true},
		{"stopped", base, MotionContext{}, DefaultPromptSetID, false},
		{"paused", base, MotionContext{Paused: true}, DefaultPromptSetID, false},
		{"other output", base, MotionContext{Running: true, Outputs: &OutputContext{}}, DefaultPromptSetID, false},
		{"local", local, MotionContext{Running: true}, DefaultPromptSetID, false},
		{"creative", creative, MotionContext{Running: true}, DefaultPromptSetID, false},
		{"localized", base, MotionContext{Running: true}, PromptSetIDJapanese, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.state.MotionMode = tc.caps.MotionMode
			set, _ := BuiltinPromptSetByID(tc.id)
			prompt := ComposeInteractivePrompt(set, nil, nil, tc.caps, &tc.state, nil)
			schema := InteractiveResponseSchema(nil, tc.caps, &tc.state, tc.id)
			strict, err := llm.OpenAISchema(schema)
			if err != nil || !json.Valid(strict) || strings.Contains(string(strict), `"speed_only"`) != tc.want ||
				strings.Contains(prompt.Prompt, hostedSinglePaceGuide) != tc.want {
				t.Fatalf("speed-only guidance/schema availability mismatch: %v", err)
			}
		})
	}
}

func TestHostedPaceLimitUsesTheCurrentSavedCeiling(t *testing.T) {
	score := DefaultLayeredScore(56)
	state := MotionContext{Running: true, Layered: &score, SpeedMaxPercent: 57}
	if strings.Contains(hostedInteractivePaceGuide(state), hostedMaximumPaceGuide) {
		t.Fatal("claimed the ceiling before reaching it")
	}
	state.SpeedMaxPercent = 56
	if !strings.Contains(hostedInteractivePaceGuide(state), hostedMaximumPaceGuide) {
		t.Fatal("missed a configured ceiling other than 100")
	}
}
