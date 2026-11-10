package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestAdvancedContinuousStopHasNoMotionPayload(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, paused := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/paused=%t", mode, paused), func(t *testing.T) {
				score, empty := DefaultLayeredScore(40), `{}`
				if mode == MotionModeCreativeV2 {
					score, empty = FreshCreativeV2Score(40), `[]`
				}
				caps := Capabilities{Motion: true, AdvancedMotion: true, MotionMode: mode}
				state := MotionContext{Running: true, Paused: paused, MotionMode: mode, Layered: &score,
					Outputs: &OutputContext{Strokers: 1, Vibrator: &VibrationContext{Independent: true, Ready: true}}}
				service := Service{Capabilities: &caps}
				raw := fmt.Sprintf(`{"action":"stop","edits":%s,"vibration":null,"reply":"Playback is stopped."}`, empty)
				response, err := service.parseLayeredResponse(raw, score, state, config.DefaultSettings().Motion, false)
				if err != nil || response.Motion == nil || response.Motion.Action != MotionActionStop || response.Motion.Layered != nil || response.Motion.Vibration != nil {
					t.Fatalf("stop must be a bare stop command: %+v %v", response, err)
				}
				service.TrustedMotionInput = true
				if _, err := service.parseLayeredResponse(raw, score, state, config.DefaultSettings().Motion, false); err == nil {
					t.Fatal("live Stop capability leaked into autonomous decisions")
				}
				service.TrustedMotionInput = false
				caps.AdvancedMotion = false
				if _, err := service.parseLayeredResponse(raw, score, state, config.DefaultSettings().Motion, false); err == nil {
					t.Fatal("Simple contract unexpectedly accepted the Advanced action")
				}
			})
		}
	}
}

func TestAdvancedContinuousStopCannotCarryOutputChanges(t *testing.T) {
	score := DefaultLayeredScore(40)
	caps := Capabilities{Motion: true, AdvancedMotion: true, MotionMode: MotionModeLayered}
	state := MotionContext{Running: true, Layered: &score,
		Outputs: &OutputContext{Strokers: 1, Vibrator: &VibrationContext{Independent: true, Ready: true}}}
	service := Service{Capabilities: &caps}
	for _, raw := range []string{
		`{"action":"stop","edits":{"evolve":true},"reply":"Stopped."}`,
		`{"action":"stop","edits":{},"vibration":{"mode":"steady","min_percent":50,"max_percent":50,"period_seconds":0},"reply":"Stopped."}`,
	} {
		if _, err := service.parseLayeredResponse(raw, score, state, config.DefaultSettings().Motion, false); err == nil {
			t.Fatal("Stop accepted a simultaneous output change")
		}
	}
}

func TestAdvancedContinuousStopSchemaRequiresEmptyEditsAndNullVibration(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		caps := Capabilities{Motion: true, AdvancedMotion: true, MotionMode: mode}
		state := MotionContext{MotionMode: mode, Running: true,
			Outputs: &OutputContext{Strokers: 1, Vibrator: &VibrationContext{Independent: true, Ready: true}}}
		var root struct {
			OneOf []struct{ Properties map[string]json.RawMessage }
		}
		if err := json.Unmarshal(InteractiveResponseSchema(nil, caps, &state), &root); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, branch := range root.OneOf {
			if !strings.Contains(string(branch.Properties["action"]), `"stop"`) {
				continue
			}
			found = true
			if !strings.Contains(string(branch.Properties["edits"]), `"const"`) || string(branch.Properties["vibration"]) != `{"type":"null"}` {
				t.Fatal("Stop grammar permits output edits")
			}
		}
		if !found {
			t.Fatalf("%s lacks Stop branch", mode)
		}
	}
}
