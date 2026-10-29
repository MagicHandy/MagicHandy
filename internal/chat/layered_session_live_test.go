//go:build liveeval && magichandy_labs

package chat

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Mechanical reproductions of session failures. No hardware or playback
// engine is created. Keep unsuccessful model selections in the atlas too.
func TestHostedLayeredSessionLive(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true}
	base, earlier := DefaultLayeredScore(42), DefaultLayeredScore(36)
	base.MinPercent, base.MaxPercent, base.RangeFloorPercent, base.RangeCeilingPercent = 0, 100, 35, 85
	base.PaceVariationPercent, base.MemoryCycles = 18, 10
	base.Layers = []motion.FlowLayer{{Axis: "center", AmountPercent: 75, PeriodCycles: 10, Shape: "wave"}, {Axis: "pace", AmountPercent: 40, LowRatePercent: 65, PeriodCycles: 12, Shape: "wave"}, {Axis: "range", AmountPercent: 80, PeriodCycles: 8, PhasePercent: 20, Shape: "wave"}}
	earlier.AnchorPercent, earlier.MinPercent, earlier.MaxPercent = 0, 15, 90
	earlier.RangeFloorPercent, earlier.RangeCeilingPercent, earlier.Layers = 75, 75, nil
	maximum := *motion.CloneFlowSpec(&base)
	maximum.SpeedPercent, maximum.PaceVariationPercent = 100, 30
	maximum.Layers[1] = motion.FlowLayer{Axis: "pace", AmountPercent: 85, LowRatePercent: 20, PeriodCycles: 10, Shape: "wave"}
	checks := []struct {
		name, message string
		before        motion.FlowSpec
		check         func(*MotionCommand) bool
	}{
		{"faster_current", "Faster. Keep this pattern and its pace variation otherwise unchanged.", base, func(m *MotionCommand) bool {
			if m == nil || m.Layered == nil || m.Layered.SpeedPercent <= base.SpeedPercent {
				return false
			}
			after := *motion.CloneFlowSpec(m.Layered)
			after.SpeedPercent = base.SpeedPercent
			return reflect.DeepEqual(after, base)
		}},
		{"maximum_without_slowdowns", "Use the fastest available pace throughout, without the periodic slowdowns. Keep the current reach and pattern.", maximum, func(m *MotionCommand) bool {
			if m == nil || m.Layered == nil {
				return false
			}
			s := m.Layered
			return s.SpeedPercent == 100 && s.PaceVariationPercent == 0 && noActivePaceLayer(s) && s.MinPercent == maximum.MinPercent && s.MaxPercent == maximum.MaxPercent && s.RangeFloorPercent == maximum.RangeFloorPercent
		}},
		{"maximum_with_variation", "Keep the maximum speed setting, but make occasional deliberate changes of pace. Keep the current reach and pattern.", maximum, func(m *MotionCommand) bool {
			s := &maximum
			if m != nil {
				s = m.Layered
			}
			return s != nil && s.SpeedPercent == 100 && (s.PaceVariationPercent > 0 || !noActivePaceLayer(s))
		}},
		{"full_and_tip", "Change it up: alternate full-device strokes and shorter strokes at the upper end, with varied reach. Keep the current speed.", base, func(m *MotionCommand) bool {
			return m != nil && m.Layered != nil && m.Layered.MinPercent == 0 && m.Layered.MaxPercent == 100 && m.Layered.RangeCeilingPercent == 100 && m.Layered.RangeFloorPercent < 100 && m.Layered.SpeedPercent == base.SpeedPercent
		}},
		{"disjoint_regions", "Repeat four complete out-and-back cycles between 0 and 30, then two complete out-and-back cycles between 70 and 100. Travel smoothly between the regions, without extra local cycles or widening either region. Keep the base speed.", base, func(m *MotionCommand) bool {
			return sessionPhraseMatches(m, []motion.PhraseStroke{{BottomPercent: 0, TopPercent: 30, Count: 4}, {BottomPercent: 70, TopPercent: 100, Count: 2}}) && m.Layered.SpeedPercent == base.SpeedPercent
		}},
		{"three_regions", "Repeat two complete down-and-up cycles between 35 and 65, three complete down-and-up cycles between 70 and 100, then two complete up-and-down cycles between 0 and 30. Join the regions smoothly; the connecting travel is separate from those local cycles. Keep the base speed.", base, func(m *MotionCommand) bool {
			return sessionPhraseMatches(m, []motion.PhraseStroke{{BottomPercent: 35, TopPercent: 65, Count: 2}, {BottomPercent: 70, TopPercent: 100, Count: 3}, {BottomPercent: 0, TopPercent: 30, Count: 2}})
		}},
		{"recall_with_phrase", "Bring back earlier score 1, raise its speed by 10 points and change its rhythm to three strokes between 15 and 55 followed by one between 15 and 90. Keep its other timing choices.", base, func(m *MotionCommand) bool {
			return sessionPhraseMatches(m, []motion.PhraseStroke{{BottomPercent: 15, TopPercent: 55, Count: 3}, {BottomPercent: 15, TopPercent: 90, Count: 1}}) && m.Layered.SpeedPercent == 46 && m.Layered.PaceVariationPercent == earlier.PaceVariationPercent
		}},
		{"reach_only", "Use full-device strokes. Keep every pace setting and variation unchanged.", base, func(m *MotionCommand) bool {
			return m != nil && m.Layered != nil && m.Layered.MinPercent == 0 && m.Layered.MaxPercent == 100 && m.Layered.RangeFloorPercent == 100 && m.Layered.SpeedPercent == base.SpeedPercent && samePaceLayerValue(base, *m.Layered)
		}},
		{"unchanged", "Keep exactly what is playing now, including its existing variation.", base, func(m *MotionCommand) bool { return m == nil }},
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	repeats := 1
	if n, err := strconv.Atoi(os.Getenv("MAGICHANDY_SESSION_REPEATS")); err == nil {
		repeats = max(1, min(n, 5))
	}
	for round := range repeats {
		for _, tc := range checks {
			t.Run(fmt.Sprintf("%d/%s", round+1, tc.name), func(t *testing.T) {
				state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &tc.before, SpeedMinPercent: 1, SpeedMaxPercent: 100, EarlierScores: []EarlierScore{{Flow: earlier, StartedSecondsAgo: 90, PlayedSeconds: 45}}}
				target, compileErr := motion.FlowTarget(tc.before, limits)
				if compileErr != nil {
					t.Fatal(compileErr)
				}
				plan := motion.NewMotionPlan("inert", target, limits, 0, 0, time.Unix(0, 0))
				state.CommandedSummary = &plan.Perceptual
				service := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off"}
				started := time.Now()
				result, err := service.Complete(t.Context(), Request{Message: tc.message}, nil)
				pass := err == nil && !result.Repaired && !result.SemanticFallback && result.Response.Reply != "" && tc.check(result.Response.Motion)
				row := hostedLayeredLiveRow(fmt.Sprintf("%d-%s", round+1, tc.name), model, tc.message, result.Raw, tc.before, result.Response.Motion, limits, started, err, pass)
				row["requests"], row["repaired"] = provider.request, result.Repaired
				rows = append(rows, row)
				t.Logf("pass=%t elapsed_ms=%v error=%v", pass, row["elapsed_ms"], err)
				if !pass {
					t.Error("intent/first-pass check failed; retained in capture")
				}
			})
		}
	}
}

func noActivePaceLayer(s *motion.FlowSpec) bool {
	for _, l := range s.Layers {
		if l.Axis == "pace" && l.AmountPercent > 0 && l.LowRatePercent != 100 {
			return false
		}
	}
	return true
}

func sessionPhraseMatches(m *MotionCommand, groups []motion.PhraseStroke) bool {
	if m == nil || m.Layered == nil || m.Layered.Phrase == nil {
		return false
	}
	if regions := m.Layered.Phrase.Regions; len(regions) > 0 {
		if len(regions) != len(groups) {
			return false
		}
		for i, r := range regions {
			g := groups[i]
			if r.BottomPercent != g.BottomPercent || r.TopPercent != g.TopPercent || r.Count != g.Count {
				return false
			}
		}
		return true
	}
	if len(m.Layered.Phrase.Strokes) != len(groups) {
		return false
	}
	for i, g := range groups {
		s := m.Layered.Phrase.Strokes[i]
		if s.BottomPercent != g.BottomPercent || s.TopPercent != g.TopPercent || s.Count != g.Count {
			return false
		}
	}
	return true
}
