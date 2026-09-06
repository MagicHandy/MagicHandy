package chat

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestContinuousActionEnforcesStateWithoutClassifyingUserWords(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		empty, edit, excessive := `{}`, `{"change_by":{"speed_percent":-10}}`, `{"controls":{"speed_percent":99}}`
		if mode == MotionModeCreativeV2 {
			empty, edit, excessive = `[]`, `[{"speed_percent":30}]`, `[{"speed_percent":99}]`
		}
		for _, tc := range []struct {
			name, action, message                 string
			running, paused, edits, reject, moves bool
			outsideLimits                         bool
		}{
			{name: "hold", action: `"none"`, running: true},
			{name: "compound request", action: `"update"`, running: true, edits: true, moves: true, message: "Cut the rate by a quarter without changing the reach, then explain why it feels different."},
			{name: "translated request", action: `"update"`, running: true, edits: true, moves: true, message: "Reduce la velocidad un cuarto y conserva todo lo demás."},
			{name: "explicit start", action: `"start"`, moves: true, message: "Begin at the current settings."},
			{name: "stopped hold", action: `"none"`},
			{name: "paused hold", action: `"none"`, running: true, paused: true},
			{name: "contradictory hold", action: `"none"`, running: true, edits: true, reject: true},
			{name: "update cannot start", action: `"update"`, edits: true, reject: true},
			{name: "start cannot update", action: `"start"`, running: true, reject: true},
			{name: "paused start", action: `"start"`, paused: true, reject: true},
			{name: "paused update", action: `"update"`, running: true, paused: true, edits: true, reject: true},
			{name: "paused edits", action: `"none"`, paused: true, edits: true, reject: true},
			{name: "missing action", running: true, reject: true},
			{name: "unknown action", action: `"resume"`, running: true, reject: true},
			{name: "null action", action: `null`, running: true, reject: true},
			{name: "saved speed limit", action: `"update"`, running: true, outsideLimits: true, reject: true},
		} {
			t.Run(string(mode)+"/"+tc.name, func(t *testing.T) {
				score := DefaultLayeredScore(40)
				if mode == MotionModeCreativeV2 {
					score = FreshCreativeV2Score(40)
				}
				fragment := empty
				if tc.edits {
					fragment = edit
				}
				if tc.outsideLimits {
					fragment = excessive
				}
				action := ""
				if tc.action != "" {
					action = `"action":` + tc.action + `,`
				}
				raw := fmt.Sprintf(`{%s"edits":%s,"reply":"A bounded response."}`, action, fragment)
				provider := &layeredTestProvider{raw: raw}
				service := Service{Provider: provider, Capabilities: &Capabilities{Motion: true, MotionMode: mode}, MotionContext: &MotionContext{Running: tc.running, Paused: tc.paused, Layered: &score, SpeedMinPercent: 10, SpeedMaxPercent: 50}}
				message := tc.message
				if message == "" {
					message = "Keep the current settings."
				}
				result, err := service.Complete(t.Context(), Request{Message: message}, nil)
				if (err != nil) != tc.reject || (result.Response.Motion != nil) != tc.moves || result.Repaired || result.SemanticFallback || provider.calls != 1 {
					t.Fatalf("unexpected decision: %+v; %v", result, err)
				}
				if tc.moves && tc.edits && result.Response.Motion.Layered.SpeedPercent != 30 {
					t.Fatal("requested relative change was rewritten")
				}
			})
		}
	}
}

func TestContinuousDecisionSchemaUsesOnlyBackendState(t *testing.T) {
	limits := config.DefaultSettings().Motion
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, state := range []MotionContext{{}, {Running: true}, {Paused: true}, {Running: true, Paused: true}} {
			state.MotionMode = mode
			schema := LayeredResponseSchema(limits, false)
			if mode == MotionModeCreativeV2 {
				schema = CreativeV2ResponseSchema(limits, false)
			}
			var decoded struct {
				OneOf []struct {
					Properties map[string]json.RawMessage `json:"properties"`
					Required   []string                   `json:"required"`
				} `json:"oneOf"`
			}
			if err := json.Unmarshal(continuousActionSchema(schema, state), &decoded); err != nil {
				t.Fatal(err)
			}
			want := []string{"none"}
			if !state.Paused {
				if state.Running {
					want = append(want, "update")
				} else {
					want = append(want, "start")
				}
			}
			got := []string{}
			for _, branch := range decoded.OneOf {
				var action struct {
					Const string `json:"const"`
				}
				if err := json.Unmarshal(branch.Properties["action"], &action); err != nil {
					t.Fatal(err)
				}
				got = append(got, action.Const)
				if !strings.Contains(strings.Join(branch.Required, ","), "action") {
					t.Fatal("action is not required")
				}
				if action.Const == "none" && !strings.Contains(string(branch.Properties["edits"]), `"const"`) {
					t.Fatal("hold grammar permitted edits")
				}
			}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("incorrect backend state grammar: %v", got)
			}
		}
	}
}
