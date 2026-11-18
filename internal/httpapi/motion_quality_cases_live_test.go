//go:build liveeval && magichandy_labs

package httpapi

import (
	"fmt"
	"math"
	"reflect"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Bounds are checked on a fresh full-loop compilation of the accepted engine
// target. Partial edits are checked against the actual preceding target, even
// when an earlier turn failed. This avoids awarding a plausible reply credit.
type qualityTurn struct {
	id, request, action string
	check               func(before, after *motion.FlowSpec, trajectory *motion.Review) bool
}

func qualityBounds(r *motion.Review, lo, hi int) bool {
	return r != nil && r.Error == "" && math.Abs(r.Summary.PositionMinPercent-float64(lo)) <= 2 &&
		math.Abs(r.Summary.PositionMaxPercent-float64(hi)) <= 2
}

func qualitySpeedDelta(delta int) func(*motion.FlowSpec, *motion.FlowSpec, *motion.Review) bool {
	return func(before, after *motion.FlowSpec, _ *motion.Review) bool {
		if before == nil || after == nil {
			return false
		}
		want := motion.CloneFlowSpec(before)
		want.SpeedPercent += delta
		return reflect.DeepEqual(want, after)
	}
}

func qualityCreativeCases(mirror bool) []qualityTurn {
	end, other, width, pace, decay, count := "tip", "base", 18, 36, 70, 3
	lo, hi, anchor := 82, 100, 0
	if mirror {
		end, other, width, pace, decay, count = "base", "tip", 22, 42, 65, 2
		lo, hi, anchor = 0, 22, 100
	}
	start := fmt.Sprintf("Start moving. Only short strokes at the %s, %d points wide, right up to that end of the slider. Set speed to %d percent and travel faster toward the %s than on the return.", end, width, pace, end)
	mix := fmt.Sprintf("Now mix full-length strokes across the entire slider with short rebounds at the %s. Use a local width of 40 points, exactly %d rebounds retaining %d percent each. Keep my speed.", other, count, decay)
	return []qualityTurn{
		{"local_only", start, "start", func(_, a *motion.FlowSpec, r *motion.Review) bool {
			return a != nil && a.Gesture != nil && a.SpeedPercent == pace && a.Gesture.FasterDirection == end && a.Gesture.ContrastPercent > 0 && qualityBounds(r, lo, hi)
		}},
		{"pace_only", "Raise only the overall speed by seven percentage points. Keep the reach and timing character exactly as they are.", "update", qualitySpeedDelta(7)},
		{"mix", mix, "update", func(b, a *motion.FlowSpec, r *motion.Review) bool {
			if a == nil || b == nil || a.Gesture == nil || r == nil {
				return false
			}
			g := a.Gesture
			return a.SpeedPercent == b.SpeedPercent && a.MinPercent == 0 && a.MaxPercent == 100 && g.FocusPercent == anchor && g.FocusWidthPercent == 40 && g.FocusMixPercent > 0 && g.FocusMixPercent < 100 && g.ReboundCount == count && g.ReboundDecayPercent == decay && r.Summary.PositionMaxPercent-r.Summary.PositionMinPercent >= 92
		}},
		{"remove_only", "Remove the rebounds. Keep every other part of what is actually playing.", "update", qualityReboundsRemoved},
		{"hold", "Keep exactly this same repeating motion. No new variation or other changes.", "none", nil},
		{"evolve", "Give me a fresh variation of the current pattern without changing any of its settings.", "update", qualitySeedChanged},
		{"stop", "Stop.", "stop", nil},
	}
}

func qualityLayeredCases(mirror bool) []qualityTurn {
	lo, hi, speed, delta := 14, 82, 34, 6
	if mirror {
		lo, hi, speed, delta = 22, 76, 41, 8
	}
	return []qualityTurn{
		{"full_band", fmt.Sprintf("Start with smooth, full strokes inside %d to %d only, reaching both ends of that band every time. Set the overall speed to %d percent.", lo, hi, speed), "start", func(_, a *motion.FlowSpec, r *motion.Review) bool {
			return a != nil && a.SpeedPercent == speed && qualityBounds(r, lo, hi) && math.Abs(r.Summary.MeanStrokePercent-float64(hi-lo)) < 3
		}},
		{"pace_only", fmt.Sprintf("Increase only the speed by %d percentage points. Preserve everything else in that pattern.", delta), "update", qualitySpeedDelta(delta)},
		{"alternate", "Alternate short strokes at the lower end with short strokes at the upper end of the current band. Keep the overall pace and outer limits.", "update", func(b, a *motion.FlowSpec, r *motion.Review) bool {
			if b == nil || a == nil || r == nil || b.SpeedPercent != a.SpeedPercent || b.MinPercent != a.MinPercent || b.MaxPercent != a.MaxPercent {
				return false
			}
			// Count regional strokes even when the transition from one end to
			// the other inflates the overall mean excursion.
			return qualityRegionalStrokes(r, b.MinPercent, b.MaxPercent)
		}},
		{"replace", "Replace the short strokes with full sweeps over the entire slider, from zero to one hundred. Keep the speed.", "update", func(b, a *motion.FlowSpec, r *motion.Review) bool {
			return b != nil && a != nil && a.SpeedPercent == b.SpeedPercent && qualityBounds(r, 0, 100) && r.Summary.MeanStrokePercent >= 96
		}},
		{"hold", "Just tell me what is playing. Leave it exactly the same.", "none", nil},
		{"stop", "Stop.", "stop", nil},
	}
}

func qualityRegionalStrokes(r *motion.Review, lo, hi int) bool {
	low, high, direction := false, false, 0
	if len(r.Samples) < 3 {
		return false
	}
	lastTurn := r.Samples[0][1]
	mid := float64(lo+hi) / 2
	for i := 1; i < len(r.Samples); i++ {
		last, current := r.Samples[i-1][1], r.Samples[i][1]
		if math.Abs(current-last) < 1e-8 {
			continue
		}
		next := 1
		if current < last {
			next = -1
		}
		if direction != 0 && next != direction {
			if math.Abs(lastTurn-last) <= float64(hi-lo)*.55 {
				low = low || math.Max(lastTurn, last) < mid
				high = high || math.Min(lastTurn, last) > mid
			}
			lastTurn = last
		}
		direction = next
	}
	return low && high
}

func qualityCue(mirror bool) (string, int, int, int) {
	if mirror {
		return "Orchard", 19, 73, 37
	}
	return "Harbor", 24, 66, 31
}

func qualityRecallCases(mirror bool) ([]qualityTurn, string) {
	cue, lo, hi, speed := qualityCue(mirror)
	correction := fmt.Sprintf("Correction to %s: its full sweeps now go from %d to %d at speed %d percent. Replace the old definition. Acknowledge only, without starting yet.", cue, lo, hi, speed)
	return []qualityTurn{
		{"future", fmt.Sprintf("For later, let %s mean smooth full sweeps from 10 to 85 at speed 44 percent. Remember the name and just acknowledge it for now. Leave motion idle.", cue), "none", nil},
		{"correction", correction, "none", nil},
		{"other", "Start a different pattern now: full strokes between 5 and 95 at speed 46 percent.", "start", func(_, a *motion.FlowSpec, r *motion.Review) bool {
			return a != nil && a.SpeedPercent == 46 && qualityBounds(r, 5, 95)
		}},
		{"recall", fmt.Sprintf("Switch to %s now, using the corrected definition. If you cannot recall its exact settings, ask me and keep the current motion unchanged.", cue), "update", func(_, a *motion.FlowSpec, r *motion.Review) bool {
			return a != nil && a.SpeedPercent == speed && qualityBounds(r, lo, hi) && math.Abs(r.Summary.MeanStrokePercent-float64(hi-lo)) < 3
		}},
		{"follow_up", "Just seven percentage points slower than what is playing now. Leave the shape, endpoints and rhythm details alone.", "update", qualitySpeedDelta(-7)},
		{"stop", "Stop.", "stop", nil},
	}, correction
}

func qualityReboundsRemoved(b, a *motion.FlowSpec, _ *motion.Review) bool {
	if b == nil || b.Gesture == nil || a == nil {
		return false
	}
	want := motion.CloneFlowSpec(b)
	want.Gesture.ReboundCount = 0
	return reflect.DeepEqual(want, a)
}

func qualitySeedChanged(b, a *motion.FlowSpec, _ *motion.Review) bool {
	if b == nil || a == nil || a.Seed == b.Seed {
		return false
	}
	want := motion.CloneFlowSpec(b)
	want.Seed = a.Seed
	return reflect.DeepEqual(want, a)
}
