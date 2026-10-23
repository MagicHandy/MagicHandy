package motion

import "math"

// Changed reach can leave the carriage outside the destination leg's interval.
// Give that longer trip proportionate time and retain the selected plans' pace,
// rather than accepting any bridge below the device's absolute velocity limit.
// Ordinary field motion and pace-only splices keep their existing behavior.
func phraseReachSpliceParameters(previous, next MotionPlan, end int, from, velocity float64, preferred int64, budget legBudget) (int64, legBudget) {
	if !samePhraseSlots(previous, next) || sameLayeredDestinations(previous, next) {
		return preferred, budget
	}
	knots := next.curve.authoredKnots
	left, right := next.focus.apply(knots[end-1].PositionPercent), next.focus.apply(knots[end].PositionPercent)
	if from < min(left, right) || from > max(left, right) {
		fullTime := float64(knots[end].TimeMillis-knots[end-1].TimeMillis) * float64(next.PeriodMillis) / float64(next.curve.duration)
		preferred = max(int64(1), int64(math.Ceil(fullTime*math.Abs(right-from)/math.Abs(right-left))))
	}
	budget.velocity = min(budget.velocity, max(math.Abs(velocity)*1000, layeredLegPeakVelocity(previous, end), layeredLegPeakVelocity(next, end)))
	return preferred, budget
}

func layeredLegPeakVelocity(plan MotionPlan, end int) float64 {
	left, right := plan.curve.authoredKnots[end-1].TimeMillis, plan.curve.authoredKnots[end].TimeMillis
	peak := 0.0
	for i, segment := range plan.curve.quintics {
		if plan.curve.points[i].TimeMillis >= right {
			break
		}
		if plan.curve.points[i+1].TimeMillis > left {
			peak = max(peak, segment.maximumVelocity())
		}
	}
	return peak * 1000 * plan.focus.gain() * float64(plan.curve.duration) / float64(plan.PeriodMillis)
}
