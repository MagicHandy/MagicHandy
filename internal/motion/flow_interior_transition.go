package motion

import "math"

// A large pace increase can make a single quintic join to the next reversal
// impossible: the destination's high turn acceleration cannot be reconciled
// with the old slow motion within the same velocity ceiling. Join an interior
// point of that same destination stroke instead, matching all three kinematic
// values. The remaining stroke then follows the normal plan to its authored
// turn. No turn/count is skipped and no opposed oscillator blend is needed.
func fitLayeredInteriorSplice(previous MotionPlan, next *MotionPlan, start, strokeStart int64, endIndex int, budget legBudget) *planTransition {
	knots := next.curve.authoredKnots
	from := previous.SampleAt(strokeStart).PositionPercent
	v, a := layeredPlanDerivatives(previous, strokeStart)
	left, right := knots[endIndex-1], knots[endIndex]
	direction := right.PositionPercent - left.PositionPercent
	factor := float64(next.curve.duration) / float64(next.PeriodMillis)
	// Prefer the middle where acceleration is lowest, then approach each end.
	for _, fraction := range []float64{.5, .375, .625, .25, .75, .125, .875} {
		clock := float64(left.TimeMillis) + fraction*float64(right.TimeMillis-left.TimeMillis)
		to := next.focus.apply(next.curve.sampleFloat(clock))
		if (to-from)*direction <= 0 {
			continue
		}
		endV := next.curve.velocityFloat(clock) * next.focus.gain() * factor
		endA := next.curve.accelerationFloat(clock) * next.focus.gain() * factor * factor
		mean := math.Abs(next.focus.apply(right.PositionPercent)-next.focus.apply(left.PositionPercent)) * factor / float64(right.TimeMillis-left.TimeMillis)
		preferred := max(int64(1), int64(math.Ceil(math.Abs(to-from)/mean)))
		phase := clock / float64(next.curve.duration)
		if bridge := fitLayeredSpliceState(previous, next, start, strokeStart, from, to, v, a, endV, endA, preferred, phase, budget, true); bridge != nil {
			return bridge
		}
	}
	return nil
}
