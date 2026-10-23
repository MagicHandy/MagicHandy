package motion

import (
	"errors"
	"math"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// HoldingTarget is deliberately cheap and independent of inference. Keep the
// familiar working region, soften the turns, and reduce pace inside saved limits.
func HoldingTarget(state ActiveMotionState, settings config.MotionSettings) (MotionTarget, error) {
	lo, hi := 0, 100
	if state.Perceptual.PositionMaxPercent > state.Perceptual.PositionMinPercent || state.Perceptual.Pace.RequestedPercent > 0 {
		lo = max(0, int(math.Floor(state.Perceptual.PositionMinPercent)))
		hi = min(100, int(math.Ceil(state.Perceptual.PositionMaxPercent)))
	} else if state.Target.Flow != nil {
		lo, hi = state.Target.Flow.MinPercent, state.Target.Flow.MaxPercent
	}
	if hi-lo < 10 {
		return MotionTarget{}, errors.New("the current motion has no usable holding range")
	}
	width := min(hi-lo, max(20, (hi-lo)/2))
	if state.Target.Flow != nil && state.Target.Flow.Gesture != nil {
		// Keep Creative v2's working region on entry. Changing both its stroke
		// grammar and range while braking can insert a small reversal; a slow,
		// soft sweep provides the counterpoint without that spatial contraction.
		width = hi - lo
	}
	// Use the working-region midpoint. The click's instantaneous position is
	// already stale by the buffered handoff and would make identical presses
	// choose different halves of the same region.
	center := (lo + hi) / 2
	start := max(lo, min(hi-width, center-width/2))
	speed := max(settings.SpeedMinPercent, min(settings.SpeedMaxPercent, min(20, int(math.Round(float64(state.Target.SpeedPercent)*0.4)))))
	flow := DefaultFlowSpec()
	flow.MinPercent, flow.MaxPercent = start, start+width
	flow.RangeFloorPercent, flow.RangeCeilingPercent = width, width
	flow.SpeedPercent, flow.PaceVariationPercent = speed, 0
	flow.TurnSoftnessPercent, flow.AnchorPercent = 85, 50
	target, err := FlowTarget(flow, settings)
	target.Source, target.Label = "edge", "Gentle hold"
	return target, err
}
