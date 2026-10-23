//go:build liveeval

package chat

import (
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// This comparison deliberately retains unrepresentable requests as failures.
// A valid response or an honest explanation is not proof of requested motion.
// Only semantic scores are compiled; no transport or playback engine exists.
func TestHostedCompoundMotionComparison(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	repeats := 2
	if value, err := strconv.Atoi(os.Getenv("MAGICHANDY_COMPOUND_REPEATS")); err == nil && value > 0 {
		repeats = value
	}
	cases := []struct {
		name, message string
		check         func(motion.FlowSpec, motion.FlowSpec) bool
	}{
		{"counted_full_base", "Repeat five full-range strokes, then five shorter strokes near the lower end. Keep that exact order and count, with smooth flowing turns.", func(_, b motion.FlowSpec) bool {
			return b.Phrase != nil && len(b.Phrase.Strokes) == 2 && b.Phrase.Strokes[0].Count == 5 && b.Phrase.Strokes[0].BottomPercent == 0 && b.Phrase.Strokes[0].TopPercent == 100 && b.Phrase.Strokes[1].Count == 5 && b.Phrase.Strokes[1].BottomPercent == 0 && b.Phrase.Strokes[1].TopPercent < 50
		}},
		{"counted_upper_full", "Repeat two short upper-end strokes followed by one full-range sweep. Keep that exact order and count. Make it flow smoothly.", func(_, b motion.FlowSpec) bool {
			return b.Phrase != nil && len(b.Phrase.Strokes) == 2 && b.Phrase.Strokes[0].Count == 2 && b.Phrase.Strokes[0].BottomPercent > 50 && b.Phrase.Strokes[0].TopPercent == 100 && b.Phrase.Strokes[1].Count == 1 && b.Phrase.Strokes[1].BottomPercent == 0 && b.Phrase.Strokes[1].TopPercent == 100
		}},
		{"regional_timing", "Mix slower full-range sweeps with quicker short strokes at the upper end, returning to the full sweeps regularly. Do not increase the overall speed.", func(a, b motion.FlowSpec) bool {
			if b.Phrase == nil || b.SpeedPercent != a.SpeedPercent {
				return false
			}
			full, upper := 0, 0
			for _, g := range b.Phrase.Strokes {
				if g.BottomPercent == 0 && g.TopPercent == 100 {
					full = g.RatePercent
				}
				if g.BottomPercent > 50 {
					upper = g.RatePercent
				}
			}
			return full > 0 && upper > full
		}},
		{"free_variation", "Vary the stroke lengths and working location more, with flowing motion. Keep the same overall speed.", func(a, b motion.FlowSpec) bool {
			if b.SpeedPercent != a.SpeedPercent {
				return false
			}
			if b.Gesture != nil {
				return b.Gesture.VariationPercent > a.Gesture.VariationPercent || b.Gesture.FocusRoamPercent > a.Gesture.FocusRoamPercent
			}
			return b.RangeFloorPercent < a.RangeFloorPercent || b.RangeCeilingPercent != a.RangeCeilingPercent || b.AnchorPercent != a.AnchorPercent || b.Seed != a.Seed
		}},
		{"pace_only", "Increase the speed by five percentage points. Keep all reach and pattern details unchanged.", func(a, b motion.FlowSpec) bool {
			if b.SpeedPercent != a.SpeedPercent+5 {
				return false
			}
			b.SpeedPercent = a.SpeedPercent
			return reflect.DeepEqual(a, b)
		}},
		{"gentle_syncopation", "Add gentle syncopation: let the timing breathe between slower stretches and brief quicker flurries. Keep the overall pace unchanged.", func(a, b motion.FlowSpec) bool {
			if b.SpeedPercent != a.SpeedPercent {
				return false
			}
			if b.Gesture != nil {
				return b.Gesture.VariationPercent > a.Gesture.VariationPercent
			}
			return b.PaceVariationPercent > a.PaceVariationPercent || pacePeriod(&b) != pacePeriod(&a) || b.Phrase != nil
		}},
	}
	var rows []map[string]any
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for repeat := range repeats {
		for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
			for _, tc := range cases {
				t.Run(string(mode)+"/"+tc.name+"/"+strconv.Itoa(repeat), func(t *testing.T) {
					limits := config.DefaultSettings().Motion
					limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
					before := DefaultLayeredScore(35)
					if mode == MotionModeCreativeV2 {
						before = FreshCreativeV2Score(35)
					}
					before.Seed = 915
					state := MotionContext{Running: true, MotionMode: mode, Layered: &before, SpeedPercent: 35, SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits)}
					caps := Capabilities{Motion: true, MotionMode: mode, HostedMotion: true, PreserveConversationText: true}
					svc := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: 512}
					started, calls := time.Now(), provider.calls
					result, err := svc.Complete(t.Context(), Request{Message: tc.message}, nil)
					command := result.Response.Motion
					pass := err == nil && command != nil && command.Layered != nil && tc.check(before, *command.Layered)
					row := hostedLayeredLiveRow(tc.name, model, tc.message, result.Raw, before, command, limits, started, err, pass)
					row["method"], row["repeat"], row["reply"], row["provider_calls"] = mode, repeat, result.Response.Reply, provider.calls-calls
					if command == nil || command.Layered == nil {
						delete(row, "after")
					}
					rows = append(rows, row)
					saveHostedLayeredRows(t, rows)
					t.Logf("intent=%t calls=%d elapsed_ms=%v error=%v", pass, provider.calls-calls, row["elapsed_ms"], err)
				})
			}
		}
	}
}
