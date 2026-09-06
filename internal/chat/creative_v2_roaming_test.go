package chat

import (
	"strings"
	"testing"
)

func TestCreativeV2RoamingDirectionsKeepAuthority(t *testing.T) {
	for _, tc := range []struct {
		message string
		reject  bool
	}{
		{"Let the working location roam freely.", false},
		{"Release that fixed location and let it roam freely.", false},
		{"What does roaming do?", true},
		{"Do not change the working location.", true},
	} {
		score := FreshCreativeV2Score(25)
		score.Gesture.FocusRoamPercent = 0
		action := "update"
		if tc.reject {
			action = "none"
		}
		provider := &layeredTestProvider{raw: `{"action":"` + action + `","edits":[{"focus":{"position_percent":50,"width_percent":25,"mix_percent":40,"roam_percent":100}}],"reply":"The location can roam."}`}
		service := Service{Provider: provider, Capabilities: &Capabilities{Motion: true, MotionMode: MotionModeCreativeV2}, MotionContext: &MotionContext{Running: true, Layered: &score}}
		result, err := service.Complete(t.Context(), Request{Message: tc.message}, nil)
		if (err != nil) != tc.reject || (!tc.reject && result.Response.Motion == nil) {
			t.Fatalf("%s: %+v %v", tc.message, result, err)
		}
	}
}

func TestCreativeV2SpeechDoesNotClaimImpossibleRoaming(t *testing.T) {
	score := FreshCreativeV2Score(25)
	score.MinPercent, score.MaxPercent, score.Gesture.FocusWidthPercent = 85, 95, 10
	score.Gesture.FocusMixPercent = 100
	var facts strings.Builder
	writeAutopilotSpeechFacts(&facts, AutopilotContext{CurrentSpeed: 25, CurrentFlow: &score})
	if strings.Contains(facts.String(), "both endpoints can move") || !strings.Contains(facts.String(), "no remaining space") {
		t.Fatalf("invented movement inside a full-width stroke: %s", facts.String())
	}
}
