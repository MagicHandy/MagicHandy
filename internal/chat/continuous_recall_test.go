package chat

import (
	"strings"
	"testing"
)

// A person who praises or asks for something that played earlier can have it
// back exactly; other edits in the same reply apply on top.
func TestRecallBringsBackAnEarlierScoreExactly(t *testing.T) {
	playing := FreshCreativeV2Score(35)
	earlier := FreshCreativeV2Score(45)
	earlier.Gesture.FocusPercent, earlier.Gesture.InertiaPercent = 90, 70
	cases := []struct {
		name, raw string
		speed     int
		reject    bool
	}{
		{name: "recall", raw: `{"action":"update","edits":[{"recall":1}],"reply":"Back to that."}`, speed: 45},
		{name: "recall and slow down", raw: `{"action":"update","edits":[{"recall":1},{"speed_percent":30}],"reply":"That again, slower."}`, speed: 30},
		{name: "no such score", raw: `{"action":"update","edits":[{"recall":2}],"reply":"Back to that."}`, reject: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &layeredTestProvider{raw: tc.raw}
			state := &MotionContext{Running: true, Layered: &playing, SpeedMinPercent: 15, SpeedMaxPercent: 54,
				EarlierScores: []EarlierScore{{Flow: earlier, StartedSecondsAgo: 180, PlayedSeconds: 60}}}
			service := Service{Provider: provider, Capabilities: &Capabilities{Motion: true, MotionMode: MotionModeCreativeV2}, MotionContext: state}
			result, err := service.Complete(t.Context(), Request{Message: "Do that thing from earlier again."}, nil)
			if tc.reject {
				if err == nil {
					t.Fatal("an unknown earlier score was recalled")
				}
				return
			}
			if err != nil || result.Response.Motion == nil {
				t.Fatalf("recall failed: %v", err)
			}
			got := *result.Response.Motion.Layered
			if got.Seed != earlier.Seed || got.Gesture.FocusPercent != 90 || got.Gesture.InertiaPercent != 70 || got.SpeedPercent != tc.speed {
				t.Fatalf("recalled %+v, want the earlier score at speed %d", got, tc.speed)
			}
			system := provider.request.Messages[0].Content
			if !strings.Contains(system, `"earlier_scores":[{"id":1,"started_seconds_ago":180,"played_seconds":60`) ||
				!strings.Contains(system, earlierScoresGuide) || !strings.Contains(string(provider.request.JSONSchema), `"recall"`) {
				t.Fatal("the earlier score or its recall edit was not offered")
			}
		})
	}
}

func TestRecallIsLiveChatOnlyAndNeedsAPlayableScore(t *testing.T) {
	playing := FreshLayeredScore(35)
	earlier := FreshLayeredScore(40)
	earlier.AnchorPercent = 100
	state := &MotionContext{Running: true, Layered: &playing, SpeedMinPercent: 15, SpeedMaxPercent: 54,
		EarlierScores: []EarlierScore{{Flow: earlier, StartedSecondsAgo: 90, PlayedSeconds: 45}}}
	provider := &layeredTestProvider{raw: `{"action":"update","edits":{"recall":1},"reply":"Back to that."}`}
	chat := Service{Provider: provider, Capabilities: &Capabilities{Motion: true, MotionMode: MotionModeLayered}, MotionContext: state}
	result, err := chat.Complete(t.Context(), Request{Message: "Go back to what you were doing before."}, nil)
	if err != nil || result.Response.Motion == nil || result.Response.Motion.Layered.AnchorPercent != 100 || result.Response.Motion.Layered.Seed != earlier.Seed {
		t.Fatalf("layered recall failed: %+v %v", result.Response.Motion, err)
	}

	// Planning re-read old requests and recalled again after chat had answered,
	// so it is never offered earlier scores.
	provider.raw = `{"edits":{},"reply":"Holding."}`
	planner := AutopilotService{Provider: provider, Capabilities: Capabilities{Motion: true, MotionMode: MotionModeLayered}, MotionContext: state}
	if _, err := planner.Complete(t.Context(), AutopilotKindMotion, Request{Message: "Plan."}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(provider.request.JSONSchema), `"recall"`) || strings.Contains(provider.request.Messages[0].Content, "earlier_scores") {
		t.Fatal("a planning turn was offered recall")
	}

	// A Creative v2 score cannot be recalled into Layered.
	state.EarlierScores = []EarlierScore{{Flow: FreshCreativeV2Score(40)}}
	provider.raw = `{"action":"none","edits":{},"reply":"Mm."}`
	if _, err := chat.Complete(t.Context(), Request{Message: "Mm."}, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(provider.request.JSONSchema), `"recall"`) || strings.Contains(provider.request.Messages[0].Content, "earlier_scores") {
		t.Fatal("recall was offered without a playable earlier score")
	}
}

// Each earlier score names the person's lines said while it played, so "the
// way you were doing it before that" matches by what was said.
func TestEarlierScoresNameWhatThePersonSaidWhileTheyPlayed(t *testing.T) {
	playing := FreshLayeredScore(35)
	earlier := FreshLayeredScore(40)
	earlier.AnchorPercent = 100
	state := &MotionContext{Running: true, Layered: &playing, SpeedMinPercent: 15, SpeedMaxPercent: 54,
		UserRequests:          []string{"slower", "try something completely different", "mm, the way you were doing it before that was perfect"},
		UserRequestSecondsAgo: []int{150, 42, 0},
		EarlierScores:         []EarlierScore{{Flow: earlier, StartedSecondsAgo: 120, PlayedSeconds: 78}}}
	provider := &layeredTestProvider{raw: `{"action":"update","edits":{"recall":1},"reply":"Back to that."}`}
	chat := Service{Provider: provider, Capabilities: &Capabilities{Motion: true, MotionMode: MotionModeLayered}, MotionContext: state}
	if _, err := chat.Complete(t.Context(), Request{Message: state.UserRequests[2]}, nil); err != nil {
		t.Fatal(err)
	}
	if want := `"played_seconds":78,"said_while_playing":["try something completely different"],"score"`; !strings.Contains(provider.request.Messages[0].Content, want) {
		t.Fatalf("earlier score does not name the line said while it played:\n%s", provider.request.Messages[0].Content)
	}
}
