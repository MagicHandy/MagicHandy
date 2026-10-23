package motion

import (
	"math"
	"sort"
)

// A pace-only Layered edit keeps the stroke destinations. Counted phrases also
// preserve their ordinal stroke when reach changes without changing counts.
// Splice the unfinished
// stroke into the next reversal instead of crossfading two oscillators that can
// oppose one another. The exact quintic must be monotone and fit the existing
// Flow envelope; otherwise the normal transition remains the fallback.
func layeredPaceTransition(previous MotionPlan, active *planTransition, next *MotionPlan, start int64) *planTransition {
	if active != nil || !isLayeredFlow(previous.Target.Flow) || !isLayeredFlow(next.Target.Flow) ||
		(!sameLayeredDestinations(previous, *next) && !samePhraseSlots(previous, *next)) {
		return nil
	}
	knots := next.curve.authoredKnots
	oldClock := previous.PhaseAt(start) * float64(previous.curve.duration)
	endIndex := sort.Search(len(previous.curve.authoredKnots), func(i int) bool { return float64(previous.curve.authoredKnots[i].TimeMillis) > oldClock+1e-6 })
	if endIndex <= 0 || endIndex >= len(knots) {
		return nil
	}
	from := previous.SampleAt(start).PositionPercent
	to := next.focus.apply(knots[endIndex].PositionPercent)
	left, right := float64(knots[endIndex-1].TimeMillis), float64(knots[endIndex].TimeMillis)
	up := knots[endIndex].PositionPercent > knots[endIndex-1].PositionPercent
	for range 32 {
		middle := (left + right) / 2
		if (next.focus.apply(next.curve.sampleFloat(middle)) < from) == up {
			left = middle
		} else {
			right = middle
		}
	}
	clock := (left + right) / 2
	v, a := layeredPlanDerivatives(previous, start)
	phase := float64(knots[endIndex].TimeMillis) / float64(next.curve.duration)
	endAcceleration := next.curve.accelerationFloat(float64(knots[endIndex].TimeMillis)) * next.focus.gain() *
		math.Pow(float64(next.curve.duration)/float64(next.PeriodMillis), 2)
	preferred := max(int64(1), int64(math.Round((float64(knots[endIndex].TimeMillis)-clock)*float64(next.PeriodMillis)/float64(next.curve.duration))))
	budget := legBudget{velocity: referenceTravelRateForSpeed(100, next.timingModel), acceleration: flowAccelerationBudget, jerk: flowJerkBudget}
	preferred, boundedBudget := phraseReachSpliceParameters(previous, *next, endIndex, from, v, preferred, budget)
	if bridge := fitLayeredSplice(previous, next, start, start, from, to, v, a, endAcceleration, preferred, phase, boundedBudget); bridge != nil {
		return bridge
	}
	// Near a turn the inherited momentum may not permit a monotone splice to
	// that destination. Finish the already moving stroke, then splice the next
	// stroke from its actual state; never blend opposed oscillators to force it.
	return deferredLayeredSplice(previous, next, start, oldClock, endIndex, budget)
}

func deferredLayeredSplice(previous MotionPlan, next *MotionPlan, start int64, oldClock float64, endIndex int, budget legBudget) *planTransition {
	knots, oldKnots := next.curve.authoredKnots, previous.curve.authoredKnots
	oldFactor := float64(previous.PeriodMillis) / float64(previous.curve.duration)
	strokeStart := start + max(int64(1), int64(math.Ceil((float64(oldKnots[endIndex].TimeMillis)-oldClock)*oldFactor)))
	attempts := 1
	if samePhraseSlots(previous, *next) && !sameLayeredDestinations(previous, *next) {
		// A severe narrowing can need the old pair to finish: a single long
		// quintic cannot always join the short turn's acceleration at low speed.
		// The next shared turnaround offers a feasible join without a speed burst.
		attempts = 2
	}
	for range attempts {
		previousIndex := endIndex % (len(knots) - 1)
		endIndex = previousIndex + 1
		from := previous.SampleAt(strokeStart).PositionPercent
		v, a := layeredPlanDerivatives(previous, strokeStart)
		to := next.focus.apply(knots[endIndex].PositionPercent)
		phase := float64(knots[endIndex].TimeMillis) / float64(next.curve.duration)
		endAcceleration := next.curve.accelerationFloat(float64(knots[endIndex].TimeMillis)) * next.focus.gain() * math.Pow(float64(next.curve.duration)/float64(next.PeriodMillis), 2)
		preferred := max(int64(1), int64(math.Round(float64(knots[endIndex].TimeMillis-knots[endIndex-1].TimeMillis)*float64(next.PeriodMillis)/float64(next.curve.duration))))
		preferred, boundedBudget := phraseReachSpliceParameters(previous, *next, endIndex, from, v, preferred, budget)
		if bridge := fitLayeredSplice(previous, next, start, strokeStart, from, to, v, a, endAcceleration, preferred, phase, boundedBudget); bridge != nil {
			return bridge
		}
		strokeStart += max(int64(1), int64(math.Ceil(float64(oldKnots[endIndex].TimeMillis-oldKnots[previousIndex].TimeMillis)*oldFactor)))
	}
	return nil
}

func samePhraseSlots(previous, next MotionPlan) bool {
	if !previous.Loop || !next.Loop || !sameFocusProjection(previous.focus, next.focus) || previous.Target.Flow == nil || next.Target.Flow == nil {
		return false
	}
	a, b := previous.Target.Flow.Phrase, next.Target.Flow.Phrase
	if a == nil || b == nil || len(a.Strokes) != len(b.Strokes) || len(previous.curve.authoredKnots) != len(next.curve.authoredKnots) {
		return false
	}
	for i := range a.Strokes {
		if a.Strokes[i].Count != b.Strokes[i].Count {
			return false
		}
	}
	return true
}

func fitLayeredSplice(previous MotionPlan, next *MotionPlan, start, strokeStart int64, from, to, v, a, endAcceleration float64, preferred int64, phase float64, budget legBudget) *planTransition {
	return fitLayeredSpliceShape(previous, next, start, strokeStart, from, to, v, a, endAcceleration, preferred, phase, budget, false)
}

func fitLayeredSpliceShape(previous MotionPlan, next *MotionPlan, start, strokeStart int64, from, to, v, a, endAcceleration float64, preferred int64, phase float64, budget legBudget, singleCrest bool) *planTransition {
	if (to-from)*v < -1e-9 || math.Abs(to-from) < 1 {
		return nil
	}
	// Search near the requested remaining stroke duration in both directions.
	// A long stroke can overshoot with its inherited velocity just as a short
	// one can exceed jerk; feasibility is not monotone in duration.
	for step := range 25 {
		for _, direction := range []float64{1, -1} {
			if step == 0 && direction < 0 {
				continue
			}
			duration := max(int64(1), int64(math.Round(float64(preferred)*math.Pow(1.08, direction*float64(step)))))
			segment := newQuinticSegment(CurvePoint{TimeMillis: 0, PositionPercent: from}, CurvePoint{TimeMillis: duration, PositionPercent: to}, v, 0, a, endAcceleration)
			if !monotoneLayeredBridge(segment, to-from, budget) {
				continue
			}
			if singleCrest && !singleCrestJoin(segment, to-from) {
				continue
			}
			bridge := newPlanTransition(previous, nil, start)
			bridge.endMillis, bridge.strokeStartMillis, bridge.stroke = strokeStart+duration, strokeStart, &segment
			next.HandoffMillis, next.PhaseOffset = bridge.endMillis, normalizePhase(phase)
			next.PhasePreserved = false
			return bridge
		}
	}
	return nil
}

func sameLayeredDestinations(previous, next MotionPlan) bool {
	if !previous.Loop || !next.Loop || !sameFocusProjection(previous.focus, next.focus) ||
		len(previous.curve.authoredKnots) != len(next.curve.authoredKnots) {
		return false
	}
	for i, knot := range previous.curve.authoredKnots {
		if math.Abs(knot.PositionPercent-next.curve.authoredKnots[i].PositionPercent) > 1e-9 {
			return false
		}
	}
	return true
}

func layeredPlanDerivatives(plan MotionPlan, at int64) (velocity, acceleration float64) {
	clock := plan.PhaseAt(at) * float64(plan.curve.duration)
	factor := float64(plan.curve.duration) / float64(plan.PeriodMillis)
	return plan.curve.velocityFloat(clock) * plan.focus.gain() * factor,
		plan.curve.accelerationFloat(clock) * plan.focus.gain() * factor * factor
}

func monotoneLayeredBridge(segment quinticSegment, distance float64, budget legBudget) bool {
	if segment.duration <= 0 {
		return false
	}
	for _, coefficient := range segment.coefficients {
		if math.IsNaN(coefficient) || math.IsInf(coefficient, 0) {
			return false
		}
	}
	if segment.maximumVelocity()*1000 > budget.velocity || segment.maximumAcceleration()*1e6 > budget.acceleration ||
		segment.maximumJerk()*1e9 > budget.jerk {
		return false
	}
	c := segment.coefficients
	for _, u := range append([]float64{0, 1}, cubicRootsInUnitInterval(20*c[5], 12*c[4], 6*c[3], 2*c[2])...) {
		if math.Copysign(1, distance)*segment.velocity(u) < -1e-9 {
			return false
		}
	}
	return true
}
