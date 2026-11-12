//go:build liveeval

package chat

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// TestFinishTargetLive asks whether an Advanced planner paces toward a finish
// that the person only said in chat, compared with the Session buildup bar and
// with both. It walks one session's Autopilot decisions through production
// prompts and parsers, feeding each choice back as the current motion. A
// chosen tempo is fed back at the level it has reached by the next decision,
// not the level it is heading to. Every response is retained; nothing reaches
// a transport.
func TestFinishTargetLive(t *testing.T) {
	if os.Getenv("MAGICHANDY_FINISH_EVAL") != "1" {
		t.Skip("opt-in finish target evaluation")
	}
	provider, model, hosted := naturalLiveProvider(t)
	if !hosted {
		t.Skip("the finish target evaluation is for Advanced hosted planners")
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 100
	const sessionMinutes = 8
	elapsed := []int{30, 120, 240, 360, 450, 480, 510}
	verbal := []llm.Message{
		{Role: "user", Content: fmt.Sprintf("Let's take about %d minutes tonight. Start slow, build up gradually, and finish strong right at the end.", sessionMinutes)},
		{Role: "assistant", Content: "Sounds good: starting slow and building the whole way to a strong finish."},
	}
	neutral := []llm.Message{
		{Role: "user", Content: "Let's get started."},
		{Role: "assistant", Content: "Starting now."},
	}
	devices := []string{"tempo", "positions"}
	if only := strings.TrimSpace(os.Getenv("MAGICHANDY_FINISH_DEVICE")); only != "" {
		devices = []string{only}
	}
	conditions := []string{"verbal", "buildup", "both"}
	if only := strings.TrimSpace(os.Getenv("MAGICHANDY_FINISH_CONDITION")); only != "" {
		conditions = []string{only}
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, device := range devices {
		for _, condition := range conditions {
			history := neutral
			if condition != "buildup" {
				history = verbal
			}
			arc := condition != "verbal"
			flow := chatDefaultFlow(20)
			tempo := motion.TempoSpec{Mode: motion.TempoModeSteady, LevelPercent: 20}
			sample, sampleErr := motion.TempoReviewSampler(tempo, 0, 20)
			if sampleErr != nil {
				t.Fatal(sampleErr)
			}
			final, previousSecond := 0.0, 0
			var trail []string
			for index, seconds := range elapsed {
				level := float64(flow.SpeedPercent)
				if device == "tempo" {
					level = sample(int64(seconds) * 1000)
				}
				context := AutopilotContext{Hosted: true, Style: "balanced", SegmentIndex: index, SpeedMinPercent: limits.SpeedMinPercent,
					SpeedMaxPercent: limits.SpeedMaxPercent, MotionMode: MotionModeLayered, MotionChangeLevel: 4, MotionMinSeconds: 20, MotionMaxSeconds: 60,
					SessionTracking: true, SessionSeconds: seconds, CurrentSpeed: int(math.Round(level)), ArcEnabled: arc}
				if arc {
					context.ArcPercent = min(100, seconds*100/(sessionMinutes*60))
					context.ArcMinutes = sessionMinutes
				}
				caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true, AutoTiming: true, AutoTimingFloor: 10}
				context.AutoTiming, context.AutoTimingFloor = true, 10
				var state MotionContext
				if device == "tempo" {
					context.TempoOnly = true
					current := tempo.Clone()
					state = MotionContext{Running: true, Outputs: &OutputContext{Strokers: 1, PaceOnly: 1, Tempo: &TempoContext{Leads: true, Ready: true, Running: true,
						Current: &current, Level: level}}}
				} else {
					context.CurrentFlow = flow
					state = naturalState(flow, MotionModeLayered, limits)
				}
				state.UserRequests, state.UserRequestSecondsAgo = []string{history[0].Content}, []int{seconds}
				if seconds == sessionMinutes*60 {
					final = level
				}
				var samples [][2]float64
				for at := previousSecond; at < seconds; at++ {
					value := float64(flow.SpeedPercent)
					if device == "tempo" {
						value = sample(int64(at) * 1000)
					}
					samples = append(samples, [2]float64{float64(at), value})
				}
				previousSecond = seconds
				request := Request{Message: AutopilotMotionMessage(context), History: history}
				before, started := len(provider.raws), time.Now()
				response, err := (AutopilotService{Provider: provider, Model: model, Capabilities: caps, MotionContext: &state,
					ReasoningMode: "off", MaxTokens: naturalMaxTokens(), PromptBudget: naturalPromptBudget()}).Complete(t.Context(), AutopilotKindMotion, request)
				choice := "hold"
				if err == nil && response.Motion != nil {
					switch {
					case response.Motion.Tempo != nil:
						tempo = response.Motion.Tempo.Clone()
						sample, sampleErr = motion.TempoReviewSampler(tempo, int64(seconds)*1000, level)
						if sampleErr != nil {
							t.Fatal(sampleErr)
						}
						choice = describeTempo(tempo)
					case response.Motion.Layered != nil:
						flow = motion.CloneFlowSpec(response.Motion.Layered)
						choice = fmt.Sprintf("speed %d", flow.SpeedPercent)
					}
				}
				if err != nil {
					choice = "error: " + err.Error()
					if len(choice) > 160 {
						choice = choice[:160]
					}
				}
				trail = append(trail, fmt.Sprintf("%ds@%.0f→%s", seconds, level, choice))
				row := map[string]any{"device": device, "condition": condition, "elapsed_s": seconds, "model": model, "level_at_decision": level,
					"choice": choice, "intent": response.Intent, "next": response.Next, "elapsed_ms": time.Since(started).Milliseconds(),
					"raw_attempts": append([]string(nil), provider.raws[before:]...), "prompt": request.Message, "samples": samples, "next_check_seconds": response.NextCheckSeconds}
				if response.Motion != nil {
					row["command"] = response.Motion
				}
				if err != nil {
					row["error"] = err.Error()
					t.Errorf("%s/%s at %ds: %v", device, condition, seconds, err)
				}
				rows = append(rows, row)
				saveHostedLayeredRows(t, rows)
			}
			t.Logf("%s/%s: %s; at %d min %.0f", device, condition, strings.Join(trail, ", "), sessionMinutes, final)
		}
	}
}

func chatDefaultFlow(speed int) *motion.FlowSpec {
	flow := DefaultLayeredScore(speed)
	return &flow
}

func describeTempo(spec motion.TempoSpec) string {
	switch spec.Mode {
	case motion.TempoModeRamp:
		return fmt.Sprintf("ramp %d/%.0fs", spec.LevelPercent, spec.PeriodSeconds)
	case motion.TempoModeSteady:
		return fmt.Sprintf("steady %d", spec.LevelPercent)
	case motion.TempoModeWave, motion.TempoModePulse:
		return fmt.Sprintf("%s %d-%d", spec.Mode, spec.MinPercent, spec.MaxPercent)
	case motion.TempoModeSequence:
		parts := make([]string, 0, len(spec.Sections))
		for _, section := range spec.Sections {
			parts = append(parts, describeTempo(motion.TempoSpec{Mode: section.Mode, LevelPercent: section.LevelPercent,
				MinPercent: section.MinPercent, MaxPercent: section.MaxPercent, PeriodSeconds: section.PeriodSeconds})+fmt.Sprintf(" for %.0fs", section.Seconds))
		}
		return "seq[" + strings.Join(parts, "; ") + "]"
	}
	return spec.Mode
}
