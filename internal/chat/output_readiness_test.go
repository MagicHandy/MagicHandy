package chat

import (
	"strings"
	"testing"
)

func TestVibrationOnlyReadinessConstrainsSchemaAndUnconstrainedReplies(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, ready := range []bool{true, false} {
			for _, armed := range []bool{true, false} {
				state := MotionContext{MotionMode: mode, Outputs: &OutputContext{Vibrator: &VibrationContext{Ready: ready, Armed: armed}}}
				caps := Capabilities{Motion: true, MotionMode: mode, IntermediateMotion: true}
				schema := InteractiveResponseSchema(nil, caps, &state)
				available := ready && armed
				if strings.Contains(string(schema), `"const":"start"`) != available {
					t.Fatalf("readiness not reflected in schema: %s", schema)
				}
				current := DefaultLayeredScore(43)
				if mode == MotionModeCreativeV2 {
					current = FreshCreativeV2Score(43)
				}
				service := Service{Capabilities: &caps}
				reply := AssistantResponse{continuousAction: MotionActionStart}
				err := service.authorizeLayeredReply(&reply, current, nil, state)
				if (err == nil) != available {
					t.Fatalf("unconstrained start bypassed readiness: ready=%t armed=%t err=%v", ready, armed, err)
				}
				// A silent vibrator does not remove authority over another
				// selected device; dispatch still checks that device's readiness.
				state.Outputs.Strokers = 1
				reply = AssistantResponse{continuousAction: MotionActionStart}
				if err := service.authorizeLayeredReply(&reply, current, nil, state); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
