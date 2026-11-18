package chat

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
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
	score := DefaultLayeredScore(56)
	creativeScore := FreshCreativeV2Score(56)
	sectioned := score
	sectioned.Steps = []motion.FlowStep{{MinPercent: 8, MaxPercent: 88, SpeedPercent: 56, Cycles: 3}}
	for _, tc := range []struct {
		name  string
		caps  Capabilities
		state MotionContext
		id    string
		want  bool
	}{
		{"running", base, MotionContext{Running: true, Layered: &score}, DefaultPromptSetID, true},
		{"live chat during autopilot", base, MotionContext{Running: true, Layered: &score, Autopilot: true}, DefaultPromptSetID, true},
		{"stopped", base, MotionContext{Layered: &score}, DefaultPromptSetID, false},
		{"paused", base, MotionContext{Paused: true, Layered: &score}, DefaultPromptSetID, false},
		{"other output", base, MotionContext{Running: true, Layered: &score, Outputs: &OutputContext{}}, DefaultPromptSetID, false},
		{"local", local, MotionContext{Running: true, Layered: &score}, DefaultPromptSetID, false},
		{"creative mode", creative, MotionContext{Running: true, Layered: &creativeScore}, DefaultPromptSetID, false},
		{"localized", base, MotionContext{Running: true, Layered: &score}, PromptSetIDJapanese, false},
		{"playing library pattern", base, MotionContext{Running: true, PatternID: "library"}, DefaultPromptSetID, false},
		{"playing Creative after mode switch", base, MotionContext{Running: true, Layered: &creativeScore}, DefaultPromptSetID, false},
		{"playing ordered sections", base, MotionContext{Running: true, Layered: &sectioned}, DefaultPromptSetID, false},
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
			provider := &layeredTestProvider{raw: `{"action":"update","edits":{"composition":"speed_only","speed_percent":{"op":"adjust","value":-7}},"reply":"Slowing down.","stay_unchanged":false}`}
			service := Service{Provider: provider, Prompt: set, Capabilities: &tc.caps, MotionContext: &tc.state, SingleAttempt: true}
			result, err := service.Complete(t.Context(), Request{Message: "Just a little slower."}, nil)
			if (err == nil) != tc.want || string(provider.request.JSONSchema) != string(schema) {
				t.Fatalf("live parser or schema disagrees with inspector: %v", err)
			}
			if tc.want && (result.Response.Motion == nil || result.Response.Motion.Layered == nil || result.Response.Motion.Layered.SpeedPercent != 49) {
				t.Fatal("requested pace was not applied to the active score")
			}
		})
	}
}

func TestHostedPaceEditPreservesTheCompletePlayingScore(t *testing.T) {
	base := DefaultLayeredScore(56)
	base.Seed, base.MinPercent, base.MaxPercent = 479, 5, 95
	base.RangeCeilingPercent = 70
	base.DirectionalPace = &motion.DirectionalPace{UpRatePercent: 55, DownRatePercent: 100}
	counted := base
	counted.Layers = nil
	full := motion.PhraseStroke{BottomPercent: 5, TopPercent: 95, Count: 1, RatePercent: 100}
	tip := motion.PhraseStroke{BottomPercent: 65, TopPercent: 95, Count: 3, RatePercent: 60}
	counted.Phrase = &motion.FlowPhrase{Strokes: []motion.PhraseStroke{full, tip}}
	regional := counted
	regional.Phrase = &motion.FlowPhrase{Regions: []motion.PhraseRegion{
		{PhraseStroke: full, Cycle: "up_down"}, {PhraseStroke: tip, Cycle: "down_up"},
	}}
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true}
	for name, current := range map[string]motion.FlowSpec{"layered": base, "counted": counted, "regional": regional} {
		t.Run(name, func(t *testing.T) {
			state := MotionContext{Running: true, Layered: &current, SpeedMinPercent: 20, SpeedMaxPercent: 80}
			for _, tc := range []struct {
				operation string
				speed     int
			}{{`{"op":"adjust","value":-7}`, 49}, {`{"op":"set","value":80}`, 80}} {
				provider := &layeredTestProvider{raw: `{"action":"update","edits":{"composition":"speed_only","speed_percent":` + tc.operation + `},"reply":"Changing only the pace."}`}
				svc := Service{Capabilities: &caps, MotionContext: &state, Provider: provider, SingleAttempt: true}
				got, err := svc.Complete(t.Context(), Request{Message: "Adjust only the overall speed."}, nil)
				want := *motion.CloneFlowSpec(&current)
				want.SpeedPercent = tc.speed
				if err != nil || got.Response.Motion == nil || !reflect.DeepEqual(got.Response.Motion.Layered, &want) {
					t.Fatalf("pace edit %s changed the score incorrectly: %v", tc.operation, err)
				}
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
	state.Autopilot = true
	for _, hold := range []bool{false, true} {
		state.StandingHold = hold
		if !strings.Contains(hostedInteractivePaceGuide(state), `"stay_unchanged":`+strconv.FormatBool(hold)) {
			t.Fatal("the ceiling example lost the existing Autopilot hold setting")
		}
	}
}
