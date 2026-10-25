package chat

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestInteractivePatternGrammarKeepsHoldsInertAndFollowsState(t *testing.T) {
	for _, state := range []*MotionContext{nil, {}, {Running: true}, {Paused: true}, {Running: true, Paused: true}} {
		caps := Capabilities{Motion: true, MotionMode: MotionModePattern, Patterns: true, AdvancedMotion: true}
		var schema struct {
			Properties struct {
				Motion struct {
					OneOf []struct {
						Properties map[string]json.RawMessage
						Required   []string
						Additional bool `json:"additionalProperties"`
					}
				}
			}
		}
		if err := json.Unmarshal(PatternResponseSchema(continuousTestChoices(), caps, state), &schema); err != nil {
			t.Fatal(err)
		}
		want := []string{MotionActionNone, MotionActionStop}
		if state != nil && !state.Paused {
			if state.Running {
				want = append(want, MotionActionTarget)
			} else {
				want = append(want, MotionActionStart)
			}
		}
		got := []string{}
		for _, branch := range schema.Properties.Motion.OneOf {
			var action struct{ Const string }
			if err := json.Unmarshal(branch.Properties["action"], &action); err != nil {
				t.Fatal(err)
			}
			got = append(got, action.Const)
			if branch.Additional || !slices.Contains(branch.Required, "action") {
				t.Fatal("action grammar permits ambiguous fields")
			}
			if (action.Const == MotionActionNone || action.Const == MotionActionStop) && len(branch.Properties) != 1 {
				t.Fatal("an inert decision advertises target fields")
			}
			if action.Const == MotionActionStart && !slices.Contains(branch.Required, "speed_percent") {
				t.Fatal("start does not require an explicit speed")
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("wrong lifecycle choices: got %v want %v", got, want)
		}
		assertLocalPatternStateSchema(t, caps, state)
	}
}

func assertLocalPatternStateSchema(t *testing.T, caps Capabilities, state *MotionContext) {
	t.Helper()
	caps.AdvancedMotion = false
	var local map[string]any
	if err := json.Unmarshal(PatternResponseSchema(continuousTestChoices(), caps, state), &local); err != nil {
		t.Fatal(err)
	}
	motion := local["properties"].(map[string]any)["motion"].(map[string]any)
	if state != nil {
		if motion["oneOf"] == nil || motion["properties"] != nil {
			t.Fatal("local interactive library did not receive state branches")
		}
	} else if motion["oneOf"] != nil || motion["properties"].(map[string]any)["speed_percent"] == nil {
		t.Fatal("state-free local contract changed")
	}
}

func TestHostedPatternStartGrammarDoesNotChangeAutopilot(t *testing.T) {
	for _, state := range []*MotionContext{nil, {}, {Running: true}, {Paused: true}} {
		for _, kind := range []AutopilotKind{AutopilotKindMotion, AutopilotKindSpeech} {
			caps := Capabilities{Motion: true, MotionMode: MotionModePattern, Patterns: true}
			before := autopilotPatternSchema(continuousTestChoices(), caps, state, kind)
			caps.AdvancedMotion = true
			after := autopilotPatternSchema(continuousTestChoices(), caps, state, kind)
			if string(before) != string(after) {
				t.Fatal("hosted interactive start policy changed the autonomous contract")
			}
			var schema struct {
				Properties struct {
					Motion struct {
						Properties struct {
							Action struct{ Enum []string }
						}
					}
				}
			}
			if err := json.Unmarshal(after, &schema); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(schema.Properties.Motion.Properties.Action.Enum, []string{MotionActionNone, MotionActionTarget}) {
				t.Fatal("Autopilot gained interactive start/stop authority")
			}
		}
	}
}
