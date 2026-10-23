package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestHostedLayeredOperationsPreserveDomainSemantics(t *testing.T) {
	limits := config.DefaultSettings().Motion
	for _, tc := range []struct{ name, hosted, domain string }{
		{"absolute zero", `{"controls":{"anchor_percent":{"op":"set","value":0},"pace_variation_percent":{"op":"set","value":0}}}`, `{"controls":{"anchor_percent":0,"pace_variation_percent":0}}`},
		{"relative pace", `{"controls":{"speed_percent":{"op":"adjust","value":-5}}}`, `{"change_by":{"speed_percent":-5}}`},
		{"mixed controls", `{"controls":{"anchor_percent":{"op":"set","value":0},"speed_percent":{"op":"adjust","value":-5},"variation_mode":{"op":"set","value":"waves"}}}`, `{"controls":{"anchor_percent":0,"variation_mode":"waves"},"change_by":{"speed_percent":-5}}`},
		{"relative zero", `{"controls":{"speed_percent":{"op":"adjust","value":0}}}`, `{"change_by":{"speed_percent":0}}`},
		{"absolute period", `{"layers":[{"axis":"pace","period_cycles":{"op":"set","value":8}}]}`, `{"layers":[{"axis":"pace","period_cycles":8}]}`},
		{"relative period", `{"layers":[{"axis":"pace","period_cycles":{"op":"adjust","value":4}}]}`, `{"layers":[{"axis":"pace","period_change_cycles":4}]}`},
		{"geometry and pace", `{"controls":{"speed_percent":{"op":"set","value":20}},"geometry":"alternate_ends"}`, `{"controls":{"speed_percent":20},"geometry":"alternate_ends"}`},
		{"empty layers", `{"layers":[],"controls":{}}`, `{"layers":[],"controls":{}}`},
		{"hold", `{}`, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := DefaultLayeredScore(25)
			before := motion.CloneFlowSpec(&current)
			wrap := func(edits string) string { return `{"edits":` + edits + `,"reply":"Test."}` }
			wantReply, want, wantChanged, err := ParseLayeredReply(wrap(tc.domain), current, limits)
			if err != nil {
				t.Fatal(err)
			}
			gotReply, got, changed, err := parseHostedLayeredReply(wrap(tc.hosted), current, limits)
			if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(changed, wantChanged) || !reflect.DeepEqual(gotReply, wantReply) || !reflect.DeepEqual(current, *before) {
				t.Fatalf("operation changed domain semantics: err=%v got=%+v want=%+v", err, got, want)
			}
		})
	}
}

func TestHostedLayeredRejectsAmbiguousAndInvalidOperationsAtomically(t *testing.T) {
	for _, edits := range []string{
		// Reduced from the reported ChatGPT conflict; no user text is retained.
		`{"controls":{"min_percent":0,"anchor_percent":0},"change_by":{"min_percent":0,"anchor_percent":-10}}`,
		`{"controls":{"anchor_percent":{"op":"set","value":0}},"change_by":{"anchor_percent":-10}}`,
		`{"controls":null}`,
		`{"controls":{"speed_percent":24}}`,
		`{"controls":{"speed_percent":null}}`,
		`{"controls":{"speed_percent":{"set":24,"adjust":-5}}}`,
		`{"controls":{"speed_percent":{"op":"multiply","value":2}}}`,
		`{"controls":{"speed_percent":{"op":"set"}}}`,
		`{"controls":{"speed_percent":{"op":"set","value":null}}}`,
		`{"controls":{"speed_percent":{"op":"set","value":101}}}`,
		`{"controls":{"speed_percent":{"op":"adjust","value":1.5}}}`,
		`{"controls":{"speed_percent":{"op":"adjust","value":-101}}}`,
		`{"controls":{"variation_mode":{"op":"adjust","value":1}}}`,
		`{"controls":{"unknown":{"op":"set","value":1}}}`,
		`{"controls":{"speed_percent":{"op":"set","value":20,"extra":5}}}`,
		`{"controls":{"speed_percent":{"op":"set","op":"adjust","value":1}}}`,
		`{"controls":{"speed_percent":{"op":"set","value":20},"speed_percent":{"op":"adjust","value":-5}}}`,
		`{"layers":null}`,
		`{"layers":[{"axis":"pace","period_cycles":8}]}`,
		`{"layers":[{"axis":"pace","period_cycles":{"op":"set","value":8},"period_change_cycles":4}]}`,
		`{"layers":[{"axis":"pace","period_cycles":{"op":"set","value":0}}]}`,
		`{"layers":[{"axis":"range","period_cycles":{"op":"adjust","value":4}}]}`,
	} {
		current := DefaultLayeredScore(25)
		before := motion.CloneFlowSpec(&current)
		_, got, changed, err := parseHostedLayeredReply(`{"edits":`+edits+`,"reply":"Test."}`, current, config.DefaultSettings().Motion)
		if err == nil || !reflect.DeepEqual(got, *before) || !reflect.DeepEqual(current, *before) || len(changed) != 0 {
			t.Fatalf("invalid operation escaped or mutated current score: %s, %v", edits, err)
		}
	}
}

func TestHostedLayeredServiceKeepsRecallTimingAndAuthority(t *testing.T) {
	score := DefaultLayeredScore(25)
	prior := DefaultLayeredScore(20)
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true, PreserveConversationText: true}
	state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &score, SpeedMinPercent: 10, SpeedMaxPercent: 50, EarlierScores: []EarlierScore{{Flow: prior}}}
	provider := &layeredTestProvider{raw: `{"action":"update","edits":{"recall":1,"controls":{"speed_percent":{"op":"adjust","value":-5}}},"reply":"Earlier score, slower."}`}
	service := Service{Provider: provider, Capabilities: &caps, MotionContext: &state}
	result, err := service.Complete(t.Context(), Request{Message: "Test recall."}, nil)
	if err != nil || result.Response.Motion == nil || result.Response.Motion.Layered.SpeedPercent != 15 || provider.calls != 1 || result.Raw != provider.raw || result.Repaired || result.SemanticFallback {
		t.Fatalf("recall/one-call contract changed: %+v %v", result, err)
	}
	for _, tc := range []struct {
		running, paused bool
		action          string
		accept          bool
	}{
		{false, false, "start", true}, {false, false, "update", false}, {true, true, "update", false}, {true, false, "start", false},
	} {
		state.Running, state.Paused = tc.running, tc.paused
		provider.raw = `{"action":"` + tc.action + `","edits":{"controls":{"speed_percent":{"op":"set","value":20}}},"reply":"Test."}`
		result, err = service.Complete(t.Context(), Request{Message: "Test authority."}, nil)
		if (err == nil) != tc.accept || (result.Response.Motion != nil) != tc.accept {
			t.Fatalf("authority changed: %+v %v", tc, err)
		}
	}
	state.Running, state.Paused = true, false
	caps.AutoTiming = true
	provider.raw = `{"edits":{"controls":{"speed_percent":{"op":"adjust","value":-5}}},"reply":"Slower stretch.","next_check_seconds":24}`
	autopilot := AutopilotService{Provider: provider, Capabilities: caps, MotionContext: &state}
	decision, err := autopilot.Complete(t.Context(), AutopilotKindMotion, Request{Message: "Plan one stretch."})
	if err != nil || decision.NextCheckSeconds != 24 || decision.Motion == nil || decision.Motion.Layered.SpeedPercent != 20 {
		t.Fatalf("timed planner: %+v %v", decision, err)
	}
	// Metadata extraction must not hide duplicate fields before validation.
	provider.raw = `{"edits":{},"edits":{"controls":{"speed_percent":{"op":"set","value":20}}},"reply":"Test.","next_check_seconds":24}`
	if _, err = autopilot.Complete(t.Context(), AutopilotKindMotion, Request{Message: "Test duplicate."}); err == nil {
		t.Fatal("duplicate edits hidden by timing extraction")
	}
}

func TestHostedLayeredPromptInspectionAndExamplesAgree(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	for _, state := range []MotionContext{{}, {Running: true}, {Running: true, Paused: true}, {Running: true, Autopilot: true}, {Running: true, Autopilot: true, StandingHold: true}} {
		score := DefaultLayeredScore(25)
		state.Layered = &score
		state.MotionMode = MotionModeLayered
		state.SpeedMinPercent = 10
		state.SpeedMaxPercent = 50
		caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true}
		provider := &layeredTestProvider{raw: `{"action":"none","edits":{},"reply":"Test.","stay_unchanged":false}`}
		service := Service{Provider: provider, Prompt: set, Capabilities: &caps, MotionContext: &state}
		if _, err := service.Complete(t.Context(), Request{Message: "Question."}, nil); err != nil {
			t.Fatal(err)
		}
		inspected := ComposeInteractivePrompt(set, nil, nil, caps, &state, nil)
		if inspected.Prompt != provider.request.Messages[0].Content || string(InteractiveResponseSchema(nil, caps, &state)) != string(provider.request.JSONSchema) {
			t.Fatal("inspector differs from hosted request")
		}
		for _, section := range inspected.Sections {
			if section.ID == "response_contract" {
				checkLiveExamples(t, section.Text, service, state)
				if strings.Contains(section.Text, `"change_by":`) || !strings.Contains(section.Text, `"op":"set"`) {
					t.Fatal("hosted examples use the legacy dialect")
				}
				if hostedLayeredContract(section.Text) != section.Text {
					t.Fatal("hosted contract translation is not idempotent")
				}
			}
		}
	}
	if contractInstructions(Capabilities{Motion: true, MotionMode: MotionModeLayered}) != layeredContract {
		t.Fatal("local contract changed")
	}
}

func TestHostedLayeredSchemaMakesOperationsExclusive(t *testing.T) {
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(layeredResponseSchema(config.DefaultSettings().Motion, false, true), &schema); err != nil {
		t.Fatal(err)
	}
	var edits struct {
		Properties map[string]json.RawMessage `json:"properties"`
		OneOf      []json.RawMessage          `json:"oneOf"`
	}
	_ = json.Unmarshal(schema.Properties["edits"], &edits)
	if len(edits.OneOf) != 5 {
		t.Fatal("expected hold, manual, disjoint geometry and counted phrase branches")
	}
	_ = json.Unmarshal(edits.OneOf[1], &edits)
	if _, found := edits.Properties["change_by"]; found {
		t.Fatal("conflicting legacy group is still offered")
	}
	var controls struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	_ = json.Unmarshal(edits.Properties["controls"], &controls)
	for _, name := range layeredControlNames {
		var operation struct {
			OneOf []struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"oneOf"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		_ = json.Unmarshal(controls.Properties[name], &operation)
		if name == "variation_mode" {
			if string(operation.Properties["op"]) != `{"const":"set"}` || len(operation.OneOf) != 0 {
				t.Fatal("named control permits relative edits")
			}
		} else if len(operation.OneOf) != 2 || string(operation.OneOf[0].Properties["op"]) != `{"const":"set"}` || string(operation.OneOf[1].Properties["op"]) != `{"const":"adjust"}` {
			t.Fatalf("%s lacks disjoint operation branches", name)
		}
	}
}
