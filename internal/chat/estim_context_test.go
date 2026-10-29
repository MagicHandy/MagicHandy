package chat

import (
	"fmt"
	"strings"
	"testing"
)

func coyoteState() MotionContext {
	before := DefaultLayeredScore(40)
	return MotionContext{MotionMode: MotionModeLayered, Running: true, Layered: &before, SpeedPercent: 40, SpeedMinPercent: 10, SpeedMaxPercent: 80,
		Outputs: &OutputContext{Strokers: 1, Coyote: true, Estim: &EstimContext{Follows: "stroke", Connected: true, Silent: true}}}
}

// Every tier learns that a Coyote follows the movement and that its strength
// is the user's alone; none gains a control (ADR 0063).
func TestCoyoteFactsReachEveryTierWithoutAControl(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	for _, tc := range []struct {
		name  string
		caps  Capabilities
		guide string
	}{
		{"simple", Capabilities{Motion: true, MotionMode: MotionModeLayered, SimpleMotion: true}, "E-STIM: A Coyote e-stim box (e-stim, stim or shocks) follows the movement."},
		{"intermediate", Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: true}, "E-STIM: estim_follows_motion"},
		{"advanced", Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true}, "E-STIM: estim_follows_motion"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := coyoteState()
			prompt := ComposeInteractivePrompt(set, nil, nil, tc.caps, &state, nil).Prompt
			if !strings.Contains(prompt, tc.guide) || strings.Contains(prompt, "VIBRATION LEVEL") || strings.Contains(prompt, "INDEPENDENT VIBRATION") || strings.Contains(prompt, "VIBRATOR:") {
				t.Fatal("Coyote facts missing, or vibrator text without a vibrator")
			}
			if tc.name == "simple" {
				if !strings.Contains(prompt, "It is silent now: both channels are at zero") || strings.Contains(prompt, "Active playback outputs") {
					t.Fatal("Simple must get the compact facts, including silence at zero")
				}
			} else if !strings.Contains(prompt, `"silent_until_user_raises_strength":true`) {
				t.Fatal("the backend facts must reach the model")
			}
			if schema := string(InteractiveResponseSchema(nil, tc.caps, &state)); strings.Contains(schema, "vibration") || strings.Contains(schema, "estim") {
				t.Fatalf("the Coyote must add no control: %s", schema)
			}
			state.Outputs = nil
			if strings.Contains(ComposeInteractivePrompt(set, nil, nil, tc.caps, &state, nil).Prompt, "E-STIM") {
				t.Fatal("without a Coyote the prompt is unchanged")
			}
		})
	}
}

func TestSimpleCoyoteFactsReportDisconnection(t *testing.T) {
	estim := EstimContext{Follows: "speed"}
	if got := simpleEstimInstructions(estim, "{}"); !strings.Contains(got, "It is not connected now") {
		t.Fatalf("disconnected: %s", got)
	}
	estim.Connected = true
	if got := simpleEstimInstructions(estim, "{}"); got != fmt.Sprintf(simpleEstimGuide, "{}") || !strings.Contains(got, `"edits":{}`) {
		t.Fatalf("a felt Coyote needs no extra fact: %s", got)
	}
}
