package chat

import (
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func richAutopilotInput() (AutopilotContext, MotionContext) {
	score := DefaultLayeredScore(35)
	score.MinPercent, score.MaxPercent, score.RangeCeilingPercent = 0, 100, 100
	score.Phrase = &motion.FlowPhrase{Strokes: []motion.PhraseStroke{
		{BottomPercent: 65, TopPercent: 100, Count: 2, RatePercent: 100},
		{BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: 100},
	}}
	score.Layers = []motion.FlowLayer{{Axis: "pace", AmountPercent: 25, PeriodCycles: 7}}
	age := 70
	prompt := AutopilotContext{Hosted: true, MotionMode: MotionModeLayered, CurrentFlow: &score, CurrentSpeed: 35,
		SpeedMinPercent: 10, SpeedMaxPercent: 80, MotionChangeLevel: 8, MotionMinSeconds: 20, MotionMaxSeconds: 60,
		AutoTiming: true, SessionTracking: true, SessionSeconds: 480, SecondsAtCurrentSpeed: 90,
		ArcEnabled: true, ArcPercent: 40, CommandedMeanTravel: 60, CommandedPeakSpeed: 140,
		CommandedPositionMin: 0, CommandedPositionMax: 100, MeanStrokeLength: 45,
		SecondsAtCurrentPhrase: 180, DecisionsAtCurrentPhrase: 5, ConsecutiveHolds: 3,
		LastHumanSecondsAgo: &age, RecentSpeeds: []SpeedStep{{SpeedPercent: 25, SecondsAgo: 150}, {SpeedPercent: 35, SecondsAgo: 90}},
		RecentPositionBands: []PositionBand{{Minimum: 0, Maximum: 100}, {Minimum: 20, Maximum: 100}},
		RecentCheckSeconds:  []int{21, 38, 26}}
	state := MotionContext{Running: true, Autopilot: true, MotionMode: MotionModeLayered, SpeedPercent: 35,
		Layered: &score, SpeedMinPercent: 10, SpeedMaxPercent: 80,
		UserRequests: []string{"Develop the pace and pattern yourself."}, UserRequestSecondsAgo: []int{70}}
	return prompt, state
}

func TestAutopilotGeneratedContextExceedsTypedLimitAndReachesProvider(t *testing.T) {
	prompt, state := richAutopilotInput()
	message := AutopilotMotionMessage(prompt)
	if len(message) <= maxUserMessageBytes {
		t.Fatalf("fixture must reproduce rich production context: %d bytes", len(message))
	}
	for _, advanced := range []bool{true, false} {
		t.Run(map[bool]string{true: "hosted", false: "local"}[advanced], func(t *testing.T) {
			provider := &scriptedProvider{responses: []string{`{"reply":"Continuing this phrase.","edits":{},"next_check_seconds":31}`}}
			caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, IntermediateMotion: !advanced, AdvancedMotion: advanced, PreserveConversationText: advanced, AutoTiming: true}
			service := AutopilotService{Provider: provider, Capabilities: caps, MotionContext: &state, MaxTokens: 512, DisableIntermediatePlanning: true}
			_, err := service.Complete(t.Context(), AutopilotKindMotion, Request{Message: message})
			if err != nil {
				t.Fatal(err)
			}
			if len(provider.requests) != 1 {
				t.Fatalf("provider calls = %d", len(provider.requests))
			}
			request := provider.requests[0]
			if !strings.Contains(request.Messages[len(request.Messages)-1].Content, message) {
				t.Fatal("generated context was truncated")
			}
			budget := config.MinimumLocalContext(config.ModelCapabilityIntermediate)
			if advanced {
				budget = config.MinimumExternalContext
			}
			if _, _, err := llm.BudgetChatRequest(request, budget); err != nil {
				t.Fatalf("context budget should admit this request: %v", err)
			}
			if _, _, err := llm.BudgetChatRequest(request, 256); err == nil {
				t.Fatal("whole-request budget was bypassed")
			}
		})
	}
	if _, err := ValidateUserMessage(message); err == nil {
		t.Fatal("interactive user message limit was weakened")
	}
}

func TestAutopilotRejectsEmptyGeneratedContext(t *testing.T) {
	provider := &scriptedProvider{}
	_, err := (AutopilotService{Provider: provider}).Complete(t.Context(), AutopilotKindMotion, Request{Message: "  "})
	if err == nil || len(provider.requests) != 0 {
		t.Fatal("empty context reached provider")
	}
}
