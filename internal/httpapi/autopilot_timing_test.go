package httpapi

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
)

// Auto timing reaches a planner only on a hosted route with the switch on;
// local planning and speech keep their sampled windows.
func TestAutoMotionTimingRequiresAHostedPlanner(t *testing.T) {
	settings := config.DefaultSettings()
	settings.Autopilot.AutoMotionTiming = true
	if autoMotionTiming(chat.AutopilotKindMotion, settings) {
		t.Fatal("local planning gained auto timing")
	}
	settings.LLM = settings.LLM.WithConnection(config.ModelConnection{ID: "hosted", Name: "Hosted",
		Provider: config.LLMProviderCompatible, BaseURL: "https://hosted.test/v1", Model: "model"})
	if !autoMotionTiming(chat.AutopilotKindMotion, settings) {
		t.Fatal("hosted planning lost auto timing")
	}
	if autoMotionTiming(chat.AutopilotKindSpeech, settings) {
		t.Fatal("speech gained auto timing")
	}
	settings.Autopilot.AutoMotionTiming = false
	if autoMotionTiming(chat.AutopilotKindMotion, settings) {
		t.Fatal("auto timing applied while switched off")
	}
}

// A hosted Creative v2 planner's horizon reaches the scheduler's decision, and
// the request asks for it only while the switch is on.
func TestHostedCreativeV2PlannerChoosesItsHorizon(t *testing.T) {
	bodies := make(chan string, 4)
	var reply atomic.Value
	reply.Store(`{"edits":[{"evolve":true}],"next_check_seconds":150,"reply":"Letting it build."}`)
	s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- string(body)
		writeHostedFixtureReply(w, reply.Load().(string))
	})
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Motion.SpeedMinPercent, settings.Motion.SpeedMaxPercent = 10, 80
		settings.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		settings.Autopilot.AutoMotionTiming = true
		return settings
	})
	flow := chat.FreshCreativeV2Score(30)
	input := modes.DecisionInput{CurrentFlow: &flow, CurrentSpeed: 30, SpeedMinPercent: 10, SpeedMaxPercent: 80,
		MotionChangeLevel: 4, RecentCheckSeconds: []int{60, 60}}
	decision, err := s.autopilotDecide(t.Context(), input)
	if err != nil || decision.Hold || decision.Segment.Flow == nil || decision.NextCheck != 150*time.Second {
		t.Fatalf("hosted auto-timed decision = %+v, %v", decision, err)
	}
	body := <-bodies
	for _, want := range []string{"next_check_seconds", "Timing is yours", "oldest to newest: 60, 60"} {
		if !strings.Contains(body, want) {
			t.Fatalf("hosted request lacks %q:\n%s", want, body)
		}
	}

	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Autopilot.AutoMotionTiming = false
		return settings
	})
	reply.Store(`{"edits":[{"evolve":true}],"reply":"Fresh."}`)
	decision, err = s.autopilotDecide(t.Context(), input)
	if err != nil || decision.NextCheck != 0 {
		t.Fatalf("switched-off decision = %+v, %v", decision, err)
	}
	if body := <-bodies; strings.Contains(body, "next_check_seconds") || strings.Contains(body, "Timing is yours") {
		t.Fatal("a switched-off hosted planner was asked for its horizon")
	}
	if len(fake.Commands()) != 0 {
		t.Fatal("planning dispatched motion")
	}
}

// The switch never changes what a local planner is asked.
func TestLocalPlannerIgnoresAutoTiming(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{`{"edits":[{"evolve":true}],"reply":"Fresh."}`}}
	s := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(s.Close)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Motion.SpeedMinPercent, settings.Motion.SpeedMaxPercent = 10, 80
		settings.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		settings.Autopilot.AutoMotionTiming = true
		return settings
	})
	flow := chat.FreshCreativeV2Score(30)
	decision, err := s.autopilotDecide(t.Context(), modes.DecisionInput{CurrentFlow: &flow, CurrentSpeed: 30,
		SpeedMinPercent: 10, SpeedMaxPercent: 80, RecentCheckSeconds: []int{60}})
	if err != nil || decision.NextCheck != 0 {
		t.Fatalf("local decision = %+v, %v", decision, err)
	}
	provider.mu.Lock()
	requests := append([]llm.ChatRequest(nil), provider.requests...)
	provider.mu.Unlock()
	if len(requests) != 1 || strings.Contains(string(requests[0].JSONSchema), "next_check_seconds") {
		t.Fatalf("local schema = %s", requests[0].JSONSchema)
	}
	for _, message := range requests[0].Messages {
		if strings.Contains(message.Content, "next_check_seconds") {
			t.Fatal("a local planner was asked for its horizon")
		}
	}
}

// Decisions are prepared ahead only for external planners the person has not
// switched it off for; a local model keeps its single slot free for chat.
func TestPreparedDecisionsRequireAnExternalPlanner(t *testing.T) {
	settings := config.DefaultSettings()
	if prepareAutopilotDecisions(settings) {
		t.Fatal("a local planner prepares decisions")
	}
	settings.LLM.Connections = []config.ModelConnection{{ID: "hosted", Name: "Hosted", Provider: config.LLMProviderCompatible,
		BaseURL: "https://hosted.test/v1", Model: "model"}}
	settings.LLM.MotionPlanner = config.MotionPlannerSettings{Provider: "connection", ConnectionID: "hosted", ContextPolicy: config.ContextTechnical}
	if !prepareAutopilotDecisions(settings) {
		t.Fatal("a hosted planner does not prepare decisions")
	}
	settings.Autopilot.PrepareNextDecision = false
	if prepareAutopilotDecisions(settings) {
		t.Fatal("prepared decisions ignore the switch")
	}
	settings.Autopilot.PrepareNextDecision = true
	settings.LLM.MotionGenerationMode = config.LLMMotionModeOff
	if prepareAutopilotDecisions(settings) {
		t.Fatal("motion off prepares decisions")
	}
	settings.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
	settings.LLM.MotionPlanner = config.MotionPlannerSettings{Provider: config.MotionPlannerDecisions, ContextPolicy: config.ContextTechnical}
	if !prepareAutopilotDecisions(settings) {
		t.Fatal("the Decisions planner does not prepare decisions")
	}
}

// A prepared decision reads the person's lines as old as they will be when it
// applies, not when it was asked.
func TestPreparedDecisionMeasuresChatAgesAtItsBoundary(t *testing.T) {
	bodies := make(chan string, 2)
	s, _, _ := hostedFixture(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- string(body)
		writeHostedFixtureReply(w, `{"edits":[{"evolve":true}],"reply":"Fresh."}`)
	})
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Motion.SpeedMinPercent, settings.Motion.SpeedMaxPercent = 10, 80
		settings.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		return settings
	})
	if _, err := s.chatLog.Append(chat.MessageRoleUser, "Slower, please.", "client"); err != nil {
		t.Fatal(err)
	}
	flow := chat.FreshCreativeV2Score(30)
	for _, lookahead := range []time.Duration{0, 75 * time.Second} {
		input := modes.DecisionInput{CurrentFlow: &flow, CurrentSpeed: 30, SpeedMinPercent: 10, SpeedMaxPercent: 80, Lookahead: lookahead}
		if _, err := s.autopilotDecide(t.Context(), input); err != nil {
			t.Fatalf("decide with lookahead %s: %v", lookahead, err)
		}
		body := <-bodies
		want := []string{`seconds_ago\":0`, `seconds_ago\":1`}
		if lookahead > 0 {
			want = []string{`seconds_ago\":75`, `seconds_ago\":76`}
		}
		if !strings.Contains(body, want[0]) && !strings.Contains(body, want[1]) {
			t.Fatalf("lookahead %s: request lacks %s", lookahead, want[0])
		}
	}
}
