package chat

import (
	"strings"
	"testing"
)

// Hosted Layered planning is told to keep moving toward a named destination
// and sees the scores it just played. Intermediate and Simple planning, and
// hosted Creative v2 planning, keep their validated prompts.
func TestHostedAutopilotPlanningAddsDirectionAndRecentScores(t *testing.T) {
	current, earlier := DefaultLayeredScore(40), DefaultLayeredScore(40)
	earlier.AnchorPercent = 100
	recent := []EarlierScore{{Flow: earlier, StartedSecondsAgo: 14, PlayedSeconds: 12}}
	base := AutopilotContext{MotionMode: MotionModeLayered, CurrentFlow: &current, CurrentSpeed: 40,
		SpeedMinPercent: 10, SpeedMaxPercent: 80, RecentScores: recent}

	hosted := base
	hosted.Hosted = true
	message := AutopilotMotionMessage(hosted)
	if !strings.Contains(message, hostedAutopilotDirectionGuide) ||
		!strings.Contains(message, `Scores played just before the current one, newest first: [{"started_seconds_ago":14,"played_seconds":12,"score":{"controls":{"anchor_percent":100`) {
		t.Fatalf("hosted planning prompt lacks direction or recent scores:\n%s", message)
	}
	creative := hosted
	creative.MotionMode = MotionModeCreativeV2
	for name, local := range map[string]AutopilotContext{"intermediate": base, "simple": base, "hosted_creative_v2": creative} {
		if name == "intermediate" {
			local.Intermediate = true
		}
		message := AutopilotMotionMessage(local)
		if strings.Contains(message, hostedAutopilotDirectionGuide) || strings.Contains(message, "Scores played just before") {
			t.Fatalf("%s planning prompt changed:\n%s", name, message)
		}
	}
	// Recall IDs and the person's words stay out of planning turns.
	if strings.Contains(message, `"id":`) || strings.Contains(message, "said_while_playing") {
		t.Fatalf("planning prompt offers recall:\n%s", message)
	}
}

// Hosted live chat encodes the immediate part of a request that also names a
// later cue, and counts stroke-by-stroke progressions. Local prompts do not
// change.
func TestHostedLiveChatGuidesCuedRequestsAndProgressions(t *testing.T) {
	for _, advanced := range []bool{true, false} {
		provider := &layeredTestProvider{raw: `{"action":"none","edits":{},"reply":"Ready."}`}
		score := DefaultLayeredScore(20)
		state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &score, SpeedMinPercent: 1, SpeedMaxPercent: 100}
		service := Service{Provider: provider, MotionContext: &state,
			Capabilities: &Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: advanced, PreserveConversationText: advanced}}
		if _, err := service.Complete(t.Context(), Request{Message: "Build it up, and slow right down when I say so."}, nil); err != nil {
			t.Fatal(err)
		}
		system := provider.request.Messages[0].Content
		if strings.Contains(system, hostedLiveIntentGuide) != advanced ||
			strings.Contains(system, "An ordered progression in which each stroke or turn changes") != advanced {
			t.Fatalf("advanced=%t live chat guidance mismatch:\n%s", advanced, system)
		}
	}
}
