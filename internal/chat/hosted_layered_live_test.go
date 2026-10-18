//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/openaiauth"
)

// This opt-in evaluation uses the existing host credential store, never copies
// credentials, and has no transport or playback engine. Retain every response,
// including rejected or semantically wrong choices, for motion-atlas review.
func TestHostedLayeredLiveOperations(t *testing.T) {
	directory, model := os.Getenv("MAGICHANDY_HOSTED_DATA_DIR"), os.Getenv("MAGICHANDY_LIVE_MODEL")
	if directory == "" || model == "" {
		t.Skip("set MAGICHANDY_HOSTED_DATA_DIR and MAGICHANDY_LIVE_MODEL for an existing ChatGPT sign-in")
	}
	manager, err := openaiauth.Open(openaiauth.Options{DataDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	token, err := manager.ActiveTokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	responses, err := llm.NewOpenAIResponsesProvider(llm.OpenAIOptions{Model: model, PlanUsage: true, ReasoningEffort: "none", Token: token, Timeout: 60 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	provider := &hostedLayeredCapture{Provider: responses}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 50
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, LayeredControlOperations: true, PreserveConversationText: true}
	rows := []map[string]any{}
	t.Cleanup(func() {
		if path := os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE"); path != "" {
			encoded, marshalErr := json.MarshalIndent(map[string]any{"turns": rows}, "", "  ")
			if marshalErr != nil {
				t.Error(marshalErr)
				return
			}
			if writeErr := os.WriteFile(path, encoded, 0600); writeErr != nil {
				t.Error(writeErr)
			}
		}
	})
	for _, tc := range []struct {
		name, message string
		check         func(*MotionCommand) bool
	}{
		{"absolute_zero", "Set the anchor to zero. Keep every other control as it is.", func(m *MotionCommand) bool {
			return m != nil && m.Layered.AnchorPercent == 0 && m.Layered.SpeedPercent == 25
		}},
		{"relative_speed", "Reduce the speed by five percentage points. Keep all other controls as they are.", func(m *MotionCommand) bool {
			return m != nil && m.Layered.SpeedPercent == 20 && m.Layered.AnchorPercent == 50
		}},
		{"relative_period", "Make the existing pace layer take four more cycles to develop. Keep its amount and the base speed unchanged.", func(m *MotionCommand) bool {
			return m != nil && m.Layered.SpeedPercent == 25 && pacePeriod(m.Layered) == 26
		}},
		{"alternate_ends", "Alternate short strokes at both ends. Make the stroke widths vary between 15 and 30 percent. Keep the current speed.", func(m *MotionCommand) bool {
			return m != nil && m.Layered.SpeedPercent == 25 && m.Layered.RangeFloorPercent == 15 && m.Layered.RangeCeilingPercent == 30
		}},
		{"explanation", "What does the pace layer do? Please only explain, leave the motion unchanged.", func(m *MotionCommand) bool { return m == nil }},
		{"recall", "Bring back earlier score 1, but reduce its speed by five percentage points. Leave everything else from that score unchanged.", func(m *MotionCommand) bool { return m != nil && m.Layered.SpeedPercent == 15 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, prior := DefaultLayeredScore(25), DefaultLayeredScore(20)
			before.Seed, prior.Seed = 811, 822
			state := MotionContext{Running: true, Autopilot: true, MotionMode: MotionModeLayered, Layered: &before, SpeedMinPercent: 10, SpeedMaxPercent: 50, EarlierScores: []EarlierScore{{Flow: prior, StartedSecondsAgo: 60, PlayedSeconds: 30}}}
			service := Service{Provider: provider, Model: model, ReasoningMode: "off", Capabilities: &caps, MotionContext: &state}
			started := time.Now()
			result, callErr := service.Complete(t.Context(), Request{Message: tc.message}, nil)
			pass := callErr == nil && tc.check(result.Response.Motion) && result.Response.Reply != "" && !result.Repaired && !result.SemanticFallback
			row := hostedLayeredLiveRow(tc.name, model, tc.message, result.Raw, before, result.Response.Motion, limits, started, callErr, pass)
			rows = append(rows, row)
			t.Logf("%s pass=%t elapsed_ms=%v error=%v", tc.name, pass, row["elapsed_ms"], callErr)
			if !pass {
				t.Errorf("intent check failed; see retained evaluation record")
			}
		})
	}
	t.Run("autopilot", func(t *testing.T) {
		before := DefaultLayeredScore(25)
		before.Seed = 833
		wish := "Keep the base speed at 25 percent; gently vary the stroke location."
		ago := 5
		state := MotionContext{Running: true, Autopilot: true, MotionMode: MotionModeLayered, Layered: &before, SpeedMinPercent: 10, SpeedMaxPercent: 50, UserRequests: []string{wish}, UserRequestSecondsAgo: []int{ago}}
		caps.AutoTiming = true
		planner := AutopilotService{Provider: provider, Model: model, Capabilities: caps, MotionContext: &state, ReasoningMode: "off"}
		started := time.Now()
		decision, err := planner.Complete(t.Context(), AutopilotKindMotion, Request{
			History: []llm.Message{{Role: "user", Content: wish}, {Role: "assistant", Content: "Keeping the base speed at 25 percent."}},
			Message: AutopilotMotionMessage(AutopilotContext{MotionMode: MotionModeLayered, CurrentFlow: &before, CurrentSpeed: 25, SpeedMinPercent: 10, SpeedMaxPercent: 50, MotionChangeLevel: 3, AutoTiming: true, LastHumanSecondsAgo: &ago}),
		})
		pass := err == nil && (decision.Motion == nil || decision.Motion.Layered.SpeedPercent == 25) && decision.NextCheckSeconds >= 8 && decision.NextCheckSeconds <= 120
		rows = append(rows, hostedLayeredLiveRow("autopilot", model, wish, provider.raw, before, decision.Motion, limits, started, err, pass))
		if !pass {
			t.Errorf("autopilot intent/timing failed: %+v %v", decision, err)
		}
	})
	t.Run("reviewer", func(t *testing.T) {
		before, prior := DefaultLayeredScore(25), DefaultLayeredScore(30)
		before.Seed, prior.Seed = 833, 833
		reviewer := MotionReviewService{Provider: provider, Model: model, Capabilities: caps}
		started := time.Now()
		review, err := reviewer.Complete(t.Context(), MotionReviewInput{Source: "chat", Request: "Reduce the speed by ten percentage points.", Limits: limits, Before: motion.MotionTarget{Flow: &prior, SpeedPercent: 30}, After: motion.MotionTarget{Flow: &before, SpeedPercent: 25}})
		pass := err == nil && review.Motion != nil && review.Motion.Layered.SpeedPercent == 20
		rows = append(rows, hostedLayeredLiveRow("reviewer", model, "Reduce speed by ten from 30; local picked 25.", provider.raw, before, review.Motion, limits, started, err, pass))
		if !pass {
			t.Errorf("reviewer intent failed: %+v %v", review, err)
		}
	})
}

type hostedLayeredCapture struct {
	llm.Provider
	raw string
}

func (p *hostedLayeredCapture) StreamChat(ctx context.Context, request llm.ChatRequest, emit func(string) error) (string, error) {
	raw, err := p.Provider.StreamChat(ctx, request, emit)
	p.raw = raw
	return raw, err
}

func pacePeriod(score *motion.FlowSpec) int {
	for _, layer := range score.Layers {
		if layer.Axis == "pace" {
			return layer.PeriodCycles
		}
	}
	return 0
}

func hostedLayeredLiveRow(name, model, message, raw string, before motion.FlowSpec, command *MotionCommand, limits config.MotionSettings, started time.Time, err error, pass bool) map[string]any {
	after := before
	if command != nil && command.Layered != nil {
		after = *command.Layered
	}
	row := map[string]any{"method": "layered", "model": model, "recipe_name": name, "expected_recipe": name, "message": message, "raw": raw, "valid": err == nil, "intent_pass": pass, "before": before, "after": after, "limits": limits, "elapsed_ms": time.Since(started).Milliseconds()}
	if err != nil {
		row["error"] = err.Error()
	}
	if command == nil {
		row["motion_unchanged"] = true
	}
	return row
}
