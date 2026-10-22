//go:build liveeval && magichandy_labs

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type paceEvaluationCase struct {
	name, request  string
	speed, ceiling int
	fastEnd        string
	direction      int
}

func paceEvaluationCases() []paceEvaluationCase {
	return []paceEvaluationCase{
		{name: "describe_capped", request: "Describe the current pace in one sentence. Leave the motion unchanged.", speed: 22, ceiling: 22},
		{name: "hammer_capped", request: "Hammer the tip.", speed: 22, ceiling: 22},
		{name: "fast_low_cap", request: "Use quick short strokes near the top.", speed: 5, ceiling: 13, direction: 1},
		{name: "fast_capped", request: "Make short strokes at the tip and make them fast.", speed: 7, ceiling: 22, direction: 1},
		{name: "fast_open", request: "Make short strokes at the tip and make them fast.", speed: 22, ceiling: 83, direction: 1},
		{name: "increase_middle", request: "That is too slow. Make a clear increase in pace, keeping the reach and location.", speed: 34, ceiling: 79, direction: 1},
		{name: "ease_high", request: "Ease the pace back now. Keep the reach and location.", speed: 68, ceiling: 91, direction: -1},
		{name: "tip_fast_capped", request: "Cycle between fast short strokes near the tip and slow short strokes near the base, then repeat.", speed: 12, ceiling: 22, fastEnd: "tip"},
		{name: "tip_fast_open", request: "Cycle between fast short strokes near the tip and slow short strokes near the base, then repeat.", speed: 31, ceiling: 83, fastEnd: "tip"},
		{name: "base_fast_open", request: "Alternate slow short strokes at the tip with quick short strokes at the base, repeating that contrast.", speed: 36, ceiling: 79, fastEnd: "base"},
		{name: "base_quick_paraphrase", request: "Give the lower end a brisk rhythm, visit the upper end gently, and keep cycling between them with short strokes.", speed: 28, ceiling: 71, fastEnd: "base"},
		{name: "even_compound", request: "Alternate short strokes between both ends while keeping the current pace and pace texture unchanged.", speed: 37, ceiling: 83},
	}
}

// This opt-in comparison has no engine or transport. Every accepted score is
// compiled by the shared planner. Failed selections and raw replies are kept;
// reply honesty requires reading those replies, not a word-matching verdict.
func TestHostedPaceLiveComparison(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_EVAL_REPEATS"))
	for _, variant := range []string{"baseline", "candidate"} {
		if selected := os.Getenv("MAGICHANDY_PACE_VARIANTS"); selected != "" && !strings.Contains(","+selected+",", ","+variant+",") {
			continue
		}
		for _, tc := range paceEvaluationCases() {
			for repeat := range max(1, repeats) {
				t.Run(fmt.Sprintf("%s/%s/%d", variant, tc.name, repeat+1), func(t *testing.T) {
					limits := config.DefaultSettings().Motion
					limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, tc.ceiling
					before := DefaultLayeredScore(tc.speed)
					before.MinPercent, before.MaxPercent, before.Seed = 0, 100, 841
					caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true, PreserveConversationText: true}
					state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &before, SpeedPercent: tc.speed,
						SpeedMinPercent: 1, SpeedMaxPercent: tc.ceiling, Envelope: motion.CurrentPlanningEnvelope(limits)}
					service := Service{Provider: paceEvaluationProvider{Provider: provider, variant: variant}, Model: model,
						Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: 1536}
					started, calls := time.Now(), provider.calls
					result, err := service.Complete(t.Context(), Request{Message: tc.request}, nil)
					after := before
					if command := result.Response.Motion; command != nil && command.Layered != nil {
						after = *command.Layered
					}
					pass := err == nil && result.Response.Reply != "" && provider.calls-calls == 1 && !result.Repaired && !result.SemanticFallback
					if tc.direction != 0 {
						pass = pass && (after.SpeedPercent-before.SpeedPercent)*tc.direction > 0
					}
					var metric paceEndMetrics
					if err == nil {
						metric = evaluatePaceEnds(after, limits)
						if tc.fastEnd == "tip" {
							pass = pass && metric.TipCount >= 4 && metric.BaseCount >= 4 && metric.TipRate > metric.BaseRate*1.25
						}
						if tc.fastEnd == "base" {
							pass = pass && metric.TipCount >= 4 && metric.BaseCount >= 4 && metric.BaseRate > metric.TipRate*1.25
						}
						if tc.name == "even_compound" {
							pass = pass && before.SpeedPercent == after.SpeedPercent && before.PaceVariationPercent == after.PaceVariationPercent && samePaceLayer(before, after)
						}
					}
					row := hostedLayeredLiveRow(variant+"/"+tc.name, model, tc.request, result.Raw, before, result.Response.Motion, limits, started, err, pass)
					row["reply"], row["repeat"], row["provider_calls"] = result.Response.Reply, repeat+1, provider.calls-calls
					row["metrics"], row["variant"], row["messages"] = metric, variant, provider.request.Messages
					rows = append(rows, row)
					saveHostedLayeredRows(t, rows)
					t.Logf("intent=%t speed=%d->%d base=%.1f tip=%.1f elapsed_ms=%v err=%v", pass, before.SpeedPercent, after.SpeedPercent, metric.BaseRate, metric.TipRate, row["elapsed_ms"], err)
					if !pass {
						t.Error("intent failed; all responses retained for review")
					}
				})
			}
		}
	}
}

type paceEvaluationProvider struct {
	llm.Provider
	variant string
}

func (p paceEvaluationProvider) StreamChat(ctx context.Context, request llm.ChatRequest, emit func(string) error) (string, error) {
	request.Messages = slices.Clone(request.Messages)
	if p.variant == "baseline" {
		for i := range request.Messages {
			if request.Messages[i].Role == "system" {
				request.Messages[i].Content = strings.ReplaceAll(request.Messages[i].Content, "\n"+hostedPaceGuide, "")
				request.Messages[i].Content = strings.ReplaceAll(request.Messages[i].Content, "\n"+hostedLayeredCoordinationGuide, "")
				break
			}
		}
		var schema map[string]any
		if err := json.Unmarshal(request.JSONSchema, &schema); err != nil {
			return "", err
		}
		removePaceFloorSchema(schema)
		request.JSONSchema, _ = json.Marshal(schema)
	}
	return p.Provider.StreamChat(ctx, request, emit)
}

func removePaceFloorSchema(value any) {
	switch value := value.(type) {
	case map[string]any:
		if properties, ok := value["properties"].(map[string]any); ok {
			delete(properties, "low_rate_percent")
			delete(properties, "inverted")
		}
		if branches, ok := value["oneOf"].([]any); ok {
			value["oneOf"] = slices.DeleteFunc(branches, func(branch any) bool {
				object, _ := branch.(map[string]any)
				properties, _ := object["properties"].(map[string]any)
				op, _ := properties["op"].(map[string]any)
				return op["const"] == "follow"
			})
		}
		for _, child := range value {
			removePaceFloorSchema(child)
		}
	case []any:
		for _, child := range value {
			removePaceFloorSchema(child)
		}
	}
}

type paceEndMetrics struct {
	BaseCount int     `json:"base_half_strokes"`
	TipCount  int     `json:"tip_half_strokes"`
	BaseRate  float64 `json:"base_mean_travel_per_second"`
	TipRate   float64 `json:"tip_mean_travel_per_second"`
}

// Compare full half-strokes wholly within an end's region. Transition strokes
// crossing the middle are excluded. Rates are commanded estimates, not device
// telemetry, and the threshold is an evaluation criterion only.
func evaluatePaceEnds(score motion.FlowSpec, limits config.MotionSettings) paceEndMetrics {
	plan := motion.NewMotionPlan("pace-evaluation", motion.MotionTarget{Flow: &score, SpeedPercent: score.SpeedPercent}, limits, 0, 0, time.Unix(0, 0))
	var result paceEndMetrics
	last, reversal := plan.SampleAt(0), plan.SampleAt(0)
	direction := 0.0
	var baseTravel, baseTime, tipTravel, tipTime float64
	for at := int64(10); at <= plan.PeriodMillis; at += 10 {
		next := plan.SampleAt(at)
		delta := next.PositionPercent - last.PositionPercent
		if delta*direction < 0 {
			width := math.Abs(last.PositionPercent - reversal.PositionPercent)
			duration := float64(last.TimeMillis-reversal.TimeMillis) / 1000
			if width >= 8 && duration > 0 {
				switch {
				case math.Max(last.PositionPercent, reversal.PositionPercent) <= 40:
					baseTravel, baseTime = baseTravel+width, baseTime+duration
					result.BaseCount++
				case math.Min(last.PositionPercent, reversal.PositionPercent) >= 60:
					tipTravel, tipTime = tipTravel+width, tipTime+duration
					result.TipCount++
				}
			}
			reversal = last
		}
		if delta != 0 {
			direction = delta
		}
		last = next
	}
	if baseTime > 0 {
		result.BaseRate = baseTravel / baseTime
	}
	if tipTime > 0 {
		result.TipRate = tipTravel / tipTime
	}
	return result
}

func samePaceLayer(a, b motion.FlowSpec) bool {
	pace := func(s motion.FlowSpec) motion.FlowLayer {
		for _, layer := range s.Layers {
			if layer.Axis == "pace" {
				return layer
			}
		}
		return motion.FlowLayer{}
	}
	return pace(a) == pace(b)
}

func TestHostedCreativePaceLive(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range paceEvaluationCases() {
		if tc.name != "describe_capped" && tc.name != "fast_capped" && tc.name != "fast_open" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			limits := config.DefaultSettings().Motion
			limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, tc.ceiling
			before := FreshCreativeV2Score(tc.speed)
			before.Seed = 841
			caps := Capabilities{Motion: true, MotionMode: MotionModeCreativeV2, HostedMotion: true, PreserveConversationText: true}
			state := MotionContext{Running: true, MotionMode: MotionModeCreativeV2, Layered: &before, SpeedPercent: tc.speed,
				SpeedMinPercent: 1, SpeedMaxPercent: tc.ceiling, Envelope: motion.CurrentPlanningEnvelope(limits)}
			service := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: 512}
			started, calls := time.Now(), provider.calls
			result, err := service.Complete(t.Context(), Request{Message: tc.request}, nil)
			after := before
			if result.Response.Motion != nil && result.Response.Motion.Layered != nil {
				after = *result.Response.Motion.Layered
			}
			pass := err == nil && provider.calls-calls == 1 && result.Response.Reply != "" && !result.Repaired && !result.SemanticFallback
			if tc.name == "describe_capped" {
				pass = pass && result.Response.Motion == nil
			} else {
				pass = pass && after.SpeedPercent > before.SpeedPercent && after.Gesture.FocusPercent == 100 && after.Gesture.FocusMixPercent == 100
			}
			row := hostedLayeredLiveRow("creative_v2/"+tc.name, model, tc.request, result.Raw, before, result.Response.Motion, limits, started, err, pass)
			row["method"], row["reply"] = "creative_v2", result.Response.Reply
			rows = append(rows, row)
			if !pass {
				t.Errorf("Creative v2 pace intent failed: %v", err)
			}
		})
	}
}
