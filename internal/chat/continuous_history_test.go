package chat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestContinuousHistoryRetainsVoiceAndCarriesSpeechOnly(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		raw := `{"action":"none","edits":{},"reply":"No change."}`
		score := DefaultLayeredScore(25)
		if mode == MotionModeCreativeV2 {
			raw = `{"action":"none","edits":[],"reply":"No change."}`
			score = FreshCreativeV2Score(25)
		}
		provider := &layeredTestProvider{raw: raw}
		service := Service{Provider: provider, Capabilities: &Capabilities{Motion: true, MotionMode: mode, Voice: VoiceExplicit}, MotionContext: &MotionContext{Layered: &score}}
		_, err := service.Complete(t.Context(), Request{Message: "What do you remember?", History: []llm.Message{{Role: "assistant", Content: "An earlier reply."}}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		// A replayed {"edits":[]} envelope was a standing example of changing
		// nothing, and misreported turns that had edited the score.
		if history := provider.request.Messages[1].Content; history != "An earlier reply." {
			t.Fatal("history replayed an edit envelope instead of speech", history)
		}
		system := provider.request.Messages[0].Content
		if !strings.Contains(system, finalVoiceCheck(VoiceExplicit)) || !strings.HasSuffix(system, continuousOutputGuard(*service.Capabilities)) {
			t.Fatal("continuous mode lost selected voice or its own guard")
		}
		if provider.request.Temperature != chatTemperature || provider.request.RepeatPenalty != chatRepeatPenalty {
			t.Fatal("chat sampling diverged")
		}
	}
}

func TestPromptHistoryRetainsLongSessionWithinByteBudget(t *testing.T) {
	history := []llm.Message{{Role: "user", Content: "Remember the corrected green notebook."}}
	for range 46 {
		history = append(history, llm.Message{Role: "assistant", Content: "The conversation continues."})
	}
	got := sanitizeHistory(history)
	if len(got) != len(history) || got[0].Content != history[0].Content {
		t.Fatal("a short 24-turn session lost its beginning")
	}
	for range 80 {
		history = append(history, llm.Message{Role: "user", Content: strings.Repeat("x", 2000)})
	}
	got = sanitizeHistory(history)
	bytes := 0
	for _, m := range got {
		bytes += len(m.Content)
	}
	if bytes > maxPromptHistoryBytes || len(got) > PromptHistoryLimit || len(got) == 0 {
		t.Fatal("history was not bounded", bytes, len(got))
	}
}

func TestContinuousHistoryAcceptsStructuredPriorRepliesWithoutReplayingEdits(t *testing.T) {
	history := []llm.Message{{Role: "assistant", Content: `{"edits":[{"speed_percent":99}],"reply":"A prior reply."}`}}
	got := continuousMessages("system", history, "What happened?")
	if got[1].Content != "A prior reply." || !strings.Contains(history[0].Content, "speed_percent") {
		t.Fatal("structured history was replayed, double quoted or mutated", got)
	}
}

func TestRecentUserRequestsKeepTheLatestHumanLinesUnfiltered(t *testing.T) {
	log := openTestLog(t)
	lines := []string{"Keep this exact pattern repeating. No changes from now on."}
	for index := range 12 {
		lines = append(lines, fmt.Sprintf("Question %d about your day.", index))
	}
	lines = append(lines, "harder", "Manten la velocidad actual")
	for _, line := range lines {
		if _, err := log.Append(MessageRoleUser, line, "test"); err != nil {
			t.Fatal(err)
		}
	}
	id, err := log.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	requests, err := log.RecentUserRequests(id)
	if err != nil {
		t.Fatal(err)
	}
	// No word list decides what counts: small talk, slang and other languages
	// all reach the model, newest last, and the model judges what still applies.
	want := []string{"Question 6 about your day.", "Question 7 about your day.", "Question 8 about your day.", "Question 9 about your day.",
		"Question 10 about your day.", "Question 11 about your day.", "harder", "Manten la velocidad actual"}
	if strings.Join(requests, "|") != strings.Join(want, "|") {
		t.Fatal("recent requests", requests)
	}
	if got := SelectRecentUserRequests([]string{"only", " ", ""}); len(got) != 1 || got[0] != "only" {
		t.Fatal("blank lines were counted", got)
	}
}

func TestContinuousAutopilotJudgesHumanRequestsInsteadOfLocking(t *testing.T) {
	for _, requests := range [][]string{nil, {"Keep working the tip at this pace.", "No aumentes la velocidad."}} {
		score := FreshCreativeV2Score(25)
		provider := &layeredTestProvider{raw: `{"edits":[{"focus":{"position_percent":0,"width_percent":45,"mix_percent":60,"roam_percent":0}},{"speed_percent":35}],"reply":"A broader lower-end contrast."}`}
		state := &MotionContext{Running: true, Layered: &score, UserRequests: requests}
		service := AutopilotService{Provider: provider, Temperature: .81, Capabilities: Capabilities{Motion: true, MotionMode: MotionModeCreativeV2}, MotionContext: state}
		result, err := service.Complete(t.Context(), AutopilotKindMotion, Request{Message: "Session so far: 120 seconds."})
		// The host no longer overrules the model with a word-matched lock; the
		// saved limits the parser enforces are the hard boundary.
		if err != nil || result.Motion == nil || result.Motion.Layered.Gesture.FocusPercent != 0 || result.Motion.Layered.SpeedPercent != 35 {
			t.Fatal("autopilot decision was not accepted", requests, err)
		}
		system := provider.request.Messages[0].Content
		last := provider.request.Messages[len(provider.request.Messages)-1].Content
		if !strings.Contains(last, "120 seconds") || !strings.Contains(last, "honor whatever in them still applies") || provider.request.Temperature != .81 {
			t.Fatal("autonomy lost context, policy or sampling")
		}
		for _, request := range requests {
			if !strings.Contains(system, request) {
				t.Fatal("a human request did not reach the model verbatim", request)
			}
		}
	}
}

func TestAutopilotSpeechSeesContinuousMotionWithChatOnlyAuthority(t *testing.T) {
	score := DefaultLayeredScore(35)
	message := AutopilotSpeechMessage(AutopilotContext{MotionMode: MotionModeLayered, CurrentFlow: &score, CurrentSpeed: 35, CurrentPatternID: "flow-private-id"})
	if !strings.Contains(message, "pace") || !strings.Contains(message, "35") || strings.Contains(message, "catalog") || strings.Contains(message, "flow-private-id") {
		t.Fatal(message)
	}
}

func TestSpeechFactsOmitInactiveLocalControlsAndInstantaneousPhase(t *testing.T) {
	score := FreshCreativeV2Score(35)
	score.Gesture.FocusMixPercent = 0
	score.Gesture.FocusPercent = 73
	score.Gesture.FocusWidthPercent = 29
	score.Gesture.ReboundCount = 3
	message := AutopilotSpeechMessage(AutopilotContext{MotionMode: MotionModeCreativeV2, CurrentFlow: &score, CurrentSpeed: 35, SpeedTrend: "rising", SessionTracking: true, SessionSeconds: 70})
	for _, unwanted := range []string{"73", "29", "3 shrinking", "rising", "pace layer"} {
		if strings.Contains(message, unwanted) {
			t.Fatal("inactive or stale speech fact", unwanted)
		}
	}
	if !strings.Contains(message, "Only broad strokes") || !strings.Contains(message, "70 seconds") {
		t.Fatal(message)
	}
}

func TestContinuousSpeechWithMotionAuthorityUsesItsOwnContract(t *testing.T) {
	score := FreshCreativeV2Score(25)
	provider := &layeredTestProvider{raw: `{"edits":[],"reply":"The broad strokes continue."}`}
	service := AutopilotService{Provider: provider, Capabilities: Capabilities{Motion: true, MotionMode: MotionModeCreativeV2}, MotionContext: &MotionContext{Running: true, Layered: &score}}
	_, err := service.Complete(t.Context(), AutopilotKindSpeech, Request{Message: AutopilotSpeechMessage(AutopilotContext{MotionMode: MotionModeCreativeV2, CurrentFlow: &score, CurrentSpeed: 25})})
	if err != nil {
		t.Fatal(err)
	}
	message := provider.request.Messages[len(provider.request.Messages)-1].Content
	if strings.Contains(message, "Set next") || !strings.Contains(message, "edits/reply contract") {
		t.Fatal("mixed speech contracts", message)
	}
}
