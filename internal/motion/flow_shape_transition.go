package motion

import (
	"math"
	"sort"
)

// layeredTurnTransition joins a changed stroke grammar at one real turn.
// Blending two independently moving oscillators can cancel their velocities
// mid-stroke. Here the old motion instead continues into a monotone, bounded
// stroke with matching position, velocity and acceleration at both ends.
// Counts inside the new phrase are unchanged; only the entry stroke is a join.
func layeredTurnTransition(previous MotionPlan, active *planTransition, next *MotionPlan, start int64) *planTransition {
	if bridge := layeredPaceTransition(previous, active, next, start); bridge != nil {
		return bridge
	}
	if active != nil || !layeredStrokePlan(previous) || !layeredStrokePlan(*next) || !sameFocusProjection(previous.focus, next.focus) {
		return nil
	}
	// The existing splice owns ordinal preservation for same-slot edits. If
	// that fit failed, do not silently enter another stroke of the same phrase.
	if samePhraseSlots(previous, *next) || sameLayeredDestinations(previous, *next) {
		return nil
	}
	// The destination phase was selected by the existing nearby-stroke search.
	// Keep that phrase neighborhood instead of starting every edit at slot zero.
	clock := next.PhaseAt(start) * float64(next.curve.duration)
	knots := next.curve.authoredKnots
	index := max(0, sort.Search(len(knots), func(i int) bool { return float64(knots[i].TimeMillis) > clock })-1)
	if bridge := joinContinuousTurn(previous, next, start, start, index); bridge != nil {
		return bridge
	}
	// Inherited momentum near a turn can make an immediate join infeasible.
	// Finish that authored leg before joining; never insert an arbitrary hold.
	oldClock := previous.PhaseAt(start) * float64(previous.curve.duration)
	end := sort.Search(len(previous.curve.authoredKnots), func(i int) bool { return float64(previous.curve.authoredKnots[i].TimeMillis) > oldClock+1e-6 })
	if end <= 0 || end >= len(previous.curve.authoredKnots) {
		return nil
	}
	strokeStart := start + max(int64(1), int64(math.Ceil((float64(previous.curve.authoredKnots[end].TimeMillis)-oldClock)*float64(previous.PeriodMillis)/float64(previous.curve.duration))))
	return joinContinuousTurn(previous, next, start, strokeStart, index)
}

func layeredStrokePlan(plan MotionPlan) bool {
	flow := plan.Target.Flow
	return plan.Loop && flow != nil && isLayeredFlow(flow) && len(plan.curve.authoredKnots) > 2
}

func joinContinuousTurn(previous MotionPlan, next *MotionPlan, start, strokeStart int64, index int) *planTransition {
	from := previous.SampleAt(strokeStart).PositionPercent
	v, a := layeredPlanDerivatives(previous, strokeStart)
	knots := next.curve.authoredKnots
	count := len(knots) - 1
	oldClock := previous.PhaseAt(strokeStart) * float64(previous.curve.duration)
	oldEnd := min(len(previous.curve.authoredKnots)-1, max(1, sort.Search(len(previous.curve.authoredKnots), func(i int) bool { return float64(previous.curve.authoredKnots[i].TimeMillis) > oldClock+1e-6 })))
	oldKnots := previous.curve.authoredKnots
	oldDirection := oldKnots[oldEnd].PositionPercent - oldKnots[oldEnd-1].PositionPercent
	// Search the selected slot first, then nearby slots. Direction, position,
	// speed and exact derivative bounds determine feasibility, not request text.
	for _, offset := range []int{0, 1, -1, 2, -2, 3, -3, 4, -4} {
		slot := (index + offset%count + count) % count
		left, right := next.focus.apply(knots[slot].PositionPercent), next.focus.apply(knots[slot+1].PositionPercent)
		if math.Abs(right-left) < 1 || (right-left)*(right-from) <= 0 || (right-left)*v < -1e-9 || (right-left)*oldDirection <= 0 {
			continue
		}
		fullTime := float64(knots[slot+1].TimeMillis-knots[slot].TimeMillis) * float64(next.PeriodMillis) / float64(next.curve.duration)
		preferred := max(int64(1), int64(math.Ceil(fullTime*math.Abs(right-from)/math.Abs(right-left))))
		budget := legBudget{velocity: min(referenceTravelRateForSpeed(100, next.timingModel), max(math.Abs(v)*1000, layeredLegPeakVelocity(previous, oldEnd), layeredLegPeakVelocity(*next, slot+1))), acceleration: flowAccelerationBudget, jerk: flowJerkBudget}
		phase := float64(knots[slot+1].TimeMillis) / float64(next.curve.duration)
		endAcceleration := next.curve.accelerationFloat(float64(knots[slot+1].TimeMillis)) * next.focus.gain() * math.Pow(float64(next.curve.duration)/float64(next.PeriodMillis), 2)
		if bridge := fitLayeredSpliceShape(previous, next, start, strokeStart, from, right, v, a, endAcceleration, preferred, phase, budget, true); bridge != nil {
			return bridge
		}
	}
	return nil
}

// Monotonic position alone permits braking almost to rest halfway through a
// stroke, then accelerating again. Such a shoulder feels like a catch-up even
// without a reversal. A connecting leg may have a speed crest, not an interior
// speed minimum; finish the old turn instead when its momentum requires one.
func singleCrestJoin(segment quinticSegment, distance float64) bool {
	c := segment.coefficients
	for _, u := range cubicRootsInUnitInterval(20*c[5], 12*c[4], 6*c[3], 2*c[2]) {
		if u > 1e-6 && u < 1-1e-6 && math.Copysign(1, distance)*(6*c[3]+24*c[4]*u+60*c[5]*u*u) > 1e-8 {
			return false
		}
	}
	return true
}
