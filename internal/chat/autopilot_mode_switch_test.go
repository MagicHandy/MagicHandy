package chat

import (
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestAutopilotMotionFactsUseActiveScoreDuringModeSwitch(t *testing.T) {
	for _, active := range []struct {
		mode  MotionMode
		field string
		score motion.FlowSpec
	}{
		{MotionModeLayered, `"layers"`, DefaultLayeredScore(37)},
		{MotionModeCreativeV2, `"roam_percent"`, FreshCreativeV2Score(37)},
	} {
		for _, selected := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
			t.Run(string(active.mode)+"_to_"+string(selected), func(t *testing.T) {
				message := AutopilotMotionMessage(AutopilotContext{
					MotionMode: selected, CurrentFlow: &active.score, CurrentSpeed: 37,
				})
				if !strings.Contains(message, "Current continuous score ("+string(active.mode)+")") || !strings.Contains(message, active.field) {
					t.Fatalf("active motion was relabeled as the selected grammar: %s", message)
				}
				if active.mode != selected && !strings.Contains(message, "selected mode's current_score") {
					t.Fatalf("mode handoff does not distinguish edit state from active motion: %s", message)
				}
			})
		}
	}
}

func TestCreativeV2ScoreContextDoesNotInventMissingGesture(t *testing.T) {
	if context := creativeV2ScoreContext(DefaultLayeredScore(37)); context != nil {
		t.Fatalf("another motion grammar became a Creative v2 score: %+v", context)
	}
}
