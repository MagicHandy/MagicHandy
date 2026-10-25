package chat

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestLocalResumeCueKeepsMessagesAndOnlyAppliesToEligibleTurns(t *testing.T) {
	messages := []llm.Message{{Role: "system", Content: "Full contract"}, {Role: "assistant", Content: "Earlier playback was running."}, {Role: "user", Content: "Resume with something different."}}
	for _, tc := range []struct {
		name  string
		caps  Capabilities
		state MotionContext
		want  bool
	}{
		{"intermediate stopped", Capabilities{IntermediateMotion: true}, MotionContext{}, true},
		{"simple unchanged", Capabilities{}, MotionContext{}, false},
		{"advanced unchanged", Capabilities{AdvancedMotion: true, IntermediateMotion: true}, MotionContext{}, false},
		{"active unchanged", Capabilities{IntermediateMotion: true}, MotionContext{Running: true}, false},
		{"paused unchanged", Capabilities{IntermediateMotion: true}, MotionContext{Paused: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, permit := range []bool{false, true} {
				state := tc.state
				state.Outputs = &OutputContext{Vibrator: &VibrationContext{Ready: true, ResumeOnMotionRequest: permit}}
				got := withLocalManualResumeState(messages, tc.caps, state)
				if tc.want && permit {
					if len(got) != len(messages)+1 || got[len(got)-2].Role != "system" || !reflect.DeepEqual(got[:len(messages)-1], messages[:len(messages)-1]) || got[len(got)-1] != messages[len(messages)-1] {
						t.Fatal("state cue changed speech or was not placed beside the request")
					}
				} else if !reflect.DeepEqual(got, messages) {
					t.Fatal("unrelated model request changed")
				}
			}
		})
	}
}

func TestManualResumeOutputGuideAndIntermediateGuard(t *testing.T) {
	v := &VibrationContext{Ready: true, Independent: true}
	state := MotionContext{Outputs: &OutputContext{Vibrator: v}}
	if !unavailableVibrationOnly(state) {
		t.Fatal("unarmed device has no start authority")
	}
	v.ResumeOnMotionRequest = true
	if unavailableVibrationOnly(state) {
		t.Fatal("backend manual permission was ignored")
	}
	guide := outputContextInstructions(state.Outputs)
	if strings.Contains(guide, vibrationAvailabilityGuide) || !strings.Contains(guide, "MANUAL RESTART PERMISSION") {
		t.Fatal("manual permission has contradictory guidance")
	}
	v.Ready = false
	if !unavailableVibrationOnly(state) {
		t.Fatal("resume permission overrides connection readiness")
	}
	caps := Capabilities{MotionMode: MotionModeLayered, IntermediateMotion: true}
	if !strings.Contains(continuousOutputGuard(caps), `"edits":[]`) {
		t.Fatal("Intermediate guard contradicts grouped contract")
	}
	caps.AdvancedMotion = true
	if !strings.Contains(continuousOutputGuard(caps), `"edits":{}`) {
		t.Fatal("Advanced guard changed")
	}
}
