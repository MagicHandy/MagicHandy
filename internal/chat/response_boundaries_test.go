package chat

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Every alternate form must produce exactly the canonical command. These are
// structural equivalences, not guesses about the user's words or intent.
func TestResponseBoundariesAcceptEquivalentEdits(t *testing.T) {
	for _, tc := range []struct {
		name, canonical, alternate string
		advanced                   bool
	}{
		{"compound intermediate", `[{"geometry":"tip_anchor"},{"controls":{"speed_percent":35}}]`, `[{"geometry":"tip_anchor","controls":{"speed_percent":35}}]`, false},
		{"compound range", `[{"range":{"min_percent":20,"max_percent":90}},{"geometry":"base_anchor"},{"stroke_width":{"min_percent":15,"max_percent":25}}]`, `[{"range":{"min_percent":20,"max_percent":90},"geometry":"base_anchor","stroke_width":{"min_percent":15,"max_percent":25}}]`, false},
		{"full endpoints", `{"composition":"full_slider"}`, `{"composition":"full_slider","controls":{"min_percent":{"op":"set","value":0},"max_percent":{"op":"set","value":100}}}`, true},
		{"tip anchor", `{"composition":"geometry","geometry":"tip_anchor"}`, `{"composition":"geometry","geometry":"tip_anchor","controls":{"anchor_percent":{"op":"set","value":100}}}`, true},
		{"relative base anchor", `{"composition":"geometry","geometry":"base_anchor"}`, `{"composition":"geometry","geometry":"base_anchor","controls":{"anchor_percent":{"op":"adjust","value":-50}}}`, true},
		{"wander variation", `{"composition":"geometry","geometry":"wander"}`, `{"composition":"geometry","geometry":"wander","controls":{"variation_mode":{"op":"set","value":"drift"}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := DefaultLayeredScore(45)
			current.Seed = 20261028
			before := motion.CloneFlowSpec(&current)
			caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: !tc.advanced, AdvancedMotion: tc.advanced}
			svc := Service{Capabilities: &caps}
			state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &current}
			wrap := func(edits string) string {
				return fmt.Sprintf(`{"action":"update","edits":%s,"reply":"Applied."}`, edits)
			}
			want, err := svc.parseLayeredResponse(wrap(tc.canonical), current, state, config.DefaultSettings().Motion, false)
			if err != nil {
				t.Fatal(err)
			}
			got, err := svc.parseLayeredResponse(wrap(tc.alternate), current, state, config.DefaultSettings().Motion, false)
			if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(current, *before) {
				t.Fatalf("equivalent reply differs: %v\ngot=%+v\nwant=%+v", err, got.Motion, want.Motion)
			}
		})
	}
}

func TestNullVibrationDoesNotRequireOutputAuthority(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
			for _, state := range []MotionContext{{}, {Paused: true}, {Outputs: &OutputContext{Strokers: 1}}} {
				caps := Capabilities{Motion: true, MotionMode: mode, AdvancedMotion: advanced}
				raw := `{"reply":"No vibration change.","vibration":null}`
				got, spec, err := takeVibration(raw, caps, state)
				if err != nil || spec != nil || got != `{"reply":"No vibration change."}` {
					t.Fatalf("null acquired an effect or failed: %s %v %+v", got, err, spec)
				}
				for _, value := range []string{`{}`, `{"mode":"off","min_percent":0,"max_percent":0,"period_seconds":0}`, `{"mode":"steady","min_percent":30,"max_percent":30,"period_seconds":0}`} {
					if _, _, err := takeVibration(`{"reply":"Test.","vibration":`+value+`}`, caps, state); err == nil {
						t.Fatal("unavailable output accepted a non-null edit")
					}
				}
			}
		}
	}
}

func TestEquivalentGeometryResolvesAgainstRecalledScore(t *testing.T) {
	current, prior := DefaultLayeredScore(45), DefaultLayeredScore(25)
	prior.AnchorPercent = 30
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true}
	svc := Service{Capabilities: &caps}
	state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &current, EarlierScores: []EarlierScore{{Flow: prior}}}
	for _, delta := range []int{-30, -50} {
		raw := fmt.Sprintf(`{"action":"update","edits":{"recall":1,"composition":"geometry","geometry":"base_anchor","controls":{"anchor_percent":{"op":"adjust","value":%d}}},"reply":"Earlier pattern at the base."}`, delta)
		response, err := svc.parseLayeredResponse(raw, current, state, config.DefaultSettings().Motion, false)
		if delta == -30 {
			want, wantErr := ApplyLayeredEdit(LayeredEdit{Geometry: "base_anchor"}, prior, config.DefaultSettings().Motion)
			if err != nil || wantErr != nil || response.Motion == nil || !reflect.DeepEqual(*response.Motion.Layered, want) {
				t.Fatalf("recall base changed: %v %v", err, wantErr)
			}
		} else if err == nil || response.Motion != nil {
			t.Fatal("current-score delta was accepted against a different recalled anchor")
		}
	}
}

func TestEquivalentBoundariesKeepAmbiguityAndAuthorityRejections(t *testing.T) {
	for _, tc := range []struct {
		edits    string
		advanced bool
	}{
		{`[{"geometry":"tip_anchor","controls":{"speed_percent":101}}]`, false},
		{`[{"geometry":"tip_anchor","geometry":"base_anchor"}]`, false},
		{`[{"controls":{"speed_percent":35}},{"controls":{"speed_percent":35}}]`, false},
		{`[{"controls":{"speed_percent":35},"unknown":true}]`, false},
		{`{"composition":"full_slider","controls":{"min_percent":{"op":"set","value":10}}}`, true},
		{`{"composition":"geometry","geometry":"tip_anchor","controls":{"anchor_percent":{"op":"set","value":0}}}`, true},
		{`{"composition":"geometry","geometry":"base_anchor","controls":{"anchor_percent":{"op":"adjust","value":-40}}}`, true},
		{`{"composition":"geometry","geometry":"wander","controls":{"variation_mode":{"op":"set","value":"waves"}}}`, true},
		{`{"composition":"full_slider","controls":{"min_percent":{"op":"set","value":0,"value":10}}}`, true},
	} {
		current := DefaultLayeredScore(45)
		before := motion.CloneFlowSpec(&current)
		caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: !tc.advanced, AdvancedMotion: tc.advanced}
		svc := Service{Capabilities: &caps}
		state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &current}
		response, err := svc.parseLayeredResponse(`{"action":"update","edits":`+tc.edits+`,"reply":"Test."}`, current, state, config.DefaultSettings().Motion, false)
		if err == nil || response.Motion != nil || !reflect.DeepEqual(current, *before) {
			t.Fatalf("invalid reply escaped: %s %v", tc.edits, err)
		}
	}
	for _, action := range []string{"none", "update", "start"} {
		current := DefaultLayeredScore(45)
		caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true}
		svc := Service{Capabilities: &caps}
		// Paused authority must be checked after semantic equivalence, too.
		state := MotionContext{Paused: true, MotionMode: MotionModeLayered, Layered: &current}
		raw := fmt.Sprintf(`{"action":%q,"edits":{"composition":"full_slider","controls":{"min_percent":{"op":"set","value":0}}},"reply":"Test."}`, action)
		if response, err := svc.parseLayeredResponse(raw, current, state, config.DefaultSettings().Motion, false); err == nil || response.Motion != nil {
			t.Fatal("equivalence bypassed paused authority")
		}
	}
}
