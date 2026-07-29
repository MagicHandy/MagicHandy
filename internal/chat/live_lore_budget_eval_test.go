//go:build liveeval

package chat

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// TestLivePersonaLoreBudgetScorecard measures the cost of persona lore through
// the real Service path without constructing a motion engine or transport.
// "Dispatch-ready latency" is the time until Service returns the validated
// semantic command, which is the earliest the HTTP layer can dispatch motion.
func TestLivePersonaLoreBudgetScorecard(t *testing.T) {
	model := liveEvalModel(t)
	provider, err := llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{
		BaseURL: liveEvalLlamaURL,
		Model:   model,
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	prompt, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	patterns := []PatternChoice{
		{ID: "steady_wave", Name: "Steady wave", Description: "Smooth full-range motion.", Weight: 1},
		{ID: "slow_squeeze", Name: "Slow squeeze", Description: "Gradual pressure-focused motion.", Weight: 1},
		{ID: "playful_tease", Name: "Playful tease", Description: "Variable shallow teasing.", Weight: 1},
		{ID: "deep_roll", Name: "Deep roll", Description: "Long rolling strokes with deep emphasis.", Weight: 1},
	}
	capabilities := FullCapabilities()
	capabilities.Voice = VoiceExplicit
	capabilities.MoodTracking = true

	cases := []struct {
		name       string
		message    string
		context    MotionContext
		wantAction string
		bandMin    int
		bandMax    int
	}{
		{
			name:       "slow start",
			message:    "Start slow and keep it gentle.",
			context:    MotionContext{SpeedMinPercent: 20, SpeedMaxPercent: 80},
			wantAction: MotionActionStart,
			bandMin:    20,
			bandMax:    39,
		},
		{
			name:       "moderate target",
			message:    "Change to a moderate steady pace.",
			context:    MotionContext{Running: true, PatternID: "playful_tease", SpeedPercent: 30, Area: AreaZoneFull, SpeedMinPercent: 20, SpeedMaxPercent: 80},
			wantAction: MotionActionTarget,
			bandMin:    40,
			bandMax:    59,
		},
		{
			name:       "fast target",
			message:    "Go fast now.",
			context:    MotionContext{Running: true, PatternID: "steady_wave", SpeedPercent: 45, Area: AreaZoneFull, SpeedMinPercent: 20, SpeedMaxPercent: 80},
			wantAction: MotionActionTarget,
			bandMin:    60,
			bandMax:    80,
		},
		{
			name:       "stop",
			message:    "Stop now.",
			context:    MotionContext{Running: true, PatternID: "deep_roll", SpeedPercent: 50, Area: AreaZoneFull, SpeedMinPercent: 20, SpeedMaxPercent: 80},
			wantAction: MotionActionStop,
		},
	}

	for _, budget := range []int{0, 500, 1000, 2000} {
		t.Run(loreBudgetName(budget), func(t *testing.T) {
			var durations []time.Duration
			firstPassValid := 0
			repairs := 0
			speedViolations := 0
			actionErrors := 0
			completed := 0
			for _, testCase := range cases {
				conversation := ConversationContext{
					PersonaName:        "Rowan",
					PersonaDescription: "An attentive, energetic adult partner.",
					PersonaLore:        syntheticLoreBudget(budget),
					UserAnatomy:        "penis",
					CurrentMood:        MoodTeasing,
				}
				service := Service{
					Provider:            provider,
					Prompt:              prompt,
					Model:               model,
					MaxTokens:           256,
					ReasoningMode:       "off",
					Patterns:            patterns,
					MotionContext:       &testCase.context,
					ConversationContext: &conversation,
					Capabilities:        &capabilities,
				}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				started := time.Now()
				result, completeErr := service.Complete(ctx, Request{Message: testCase.message}, nil)
				duration := time.Since(started)
				cancel()
				if completeErr != nil {
					t.Errorf("%s: %v", testCase.name, completeErr)
					continue
				}
				completed++
				durations = append(durations, duration)
				if !result.InitialMalformed {
					firstPassValid++
				}
				if result.Repaired {
					repairs++
				}
				if result.Response.Motion == nil || result.Response.Motion.Action != testCase.wantAction {
					actionErrors++
				}
				if loreEvalSpeedViolation(result.Response.Motion, testCase.bandMin, testCase.bandMax) {
					speedViolations++
				}
				t.Logf(
					"model=%s budget=%d case=%s latency=%s first_pass_valid=%t repaired=%t semantic_fallback=%t motion=%+v",
					model,
					budget,
					testCase.name,
					duration.Round(time.Millisecond),
					!result.InitialMalformed,
					result.Repaired,
					result.SemanticFallback,
					result.Response.Motion,
				)
			}
			if completed != len(cases) {
				t.Fatalf("completed %d/%d lore-budget cases", completed, len(cases))
			}
			t.Logf(
				"SCORECARD model=%s lore_chars=%d first_pass_valid=%.0f%% repair_rate=%.0f%% p50_dispatch_ready_ms=%d speed_band_violations=%d action_errors=%d",
				model,
				budget,
				percent(firstPassValid, completed),
				percent(repairs, completed),
				p50Duration(durations).Milliseconds(),
				speedViolations,
				actionErrors,
			)
			if speedViolations != 0 {
				t.Errorf("budget %d produced %d final speed-band violations", budget, speedViolations)
			}
		})
	}
}

func syntheticLoreBudget(characters int) []string {
	const seed = "Rowan remembers a quiet conversation, a blue velvet room, favorite music, and a promise to keep replies grounded in the present. "
	if characters <= 0 {
		return nil
	}
	entries := make([]string, 0, (characters+maxPersonaLoreEntryRunes-1)/maxPersonaLoreEntryRunes)
	remaining := characters
	for remaining > 0 && len(entries) < maxPersonaLoreEntries {
		size := remaining
		if size > maxPersonaLoreEntryRunes {
			size = maxPersonaLoreEntryRunes
		}
		var builder strings.Builder
		for builder.Len() < size {
			builder.WriteString(seed)
		}
		runes := []rune(builder.String())
		entries = append(entries, string(runes[:size]))
		remaining -= size
	}
	return entries
}

func loreEvalSpeedViolation(command *MotionCommand, minimum, maximum int) bool {
	if command == nil || minimum == 0 || maximum == 0 {
		return false
	}
	var value int
	switch {
	case command.SpeedPercent != nil:
		value = *command.SpeedPercent
	case command.Intensity != nil:
		value = *command.Intensity
	default:
		return false
	}
	return value < minimum || value > maximum
}

func loreBudgetName(budget int) string {
	if budget == 0 {
		return "no_lore"
	}
	return "lore_" + strconv.Itoa(budget) + "_chars"
}

func percent(value, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}

func p50Duration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	return sorted[(len(sorted)-1)/2]
}
