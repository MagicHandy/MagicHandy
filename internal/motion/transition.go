package motion

import (
	"math"
	"sort"
)

const (
	retargetTransitionMillis  = int64(750)
	transitionPositionEpsilon = 0.05
)

// planTransition blends the previously scheduled path into the current plan.
// Keeping the previous transition makes repeated retargets continuous instead
// of snapping back to the previous plan while an earlier blend is still live.
type planTransition struct {
	previousPlan       MotionPlan
	previousTransition *planTransition
	startMillis        int64
	endMillis          int64
}

func newPlanTransition(previous MotionPlan, transition *planTransition, startMillis int64) *planTransition {
	return &planTransition{
		previousPlan:       previous,
		previousTransition: transition,
		startMillis:        startMillis,
		endMillis:          startMillis + retargetTransitionMillis,
	}
}

func sampleMotionPath(plan MotionPlan, transition *planTransition, streamMillis int64) MotionSample {
	next := plan.SampleAt(streamMillis)
	if transition == nil || streamMillis >= transition.endMillis {
		return next
	}
	previous := sampleMotionPath(transition.previousPlan, transition.previousTransition, streamMillis)
	if streamMillis < transition.startMillis {
		return previous
	}

	progress := float64(streamMillis-transition.startMillis) /
		float64(transition.endMillis-transition.startMillis)
	weight := smootherStep(math.Max(0, math.Min(1, progress)))
	next.PositionPercent = previous.PositionPercent +
		(next.PositionPercent-previous.PositionPercent)*weight
	return next
}

func smootherStep(value float64) float64 {
	return value * value * value * (value*(value*6-15) + 10)
}

func transitionRequired(previous MotionPlan, transition *planTransition, next MotionPlan, handoffMillis int64) bool {
	// A phase-preserving tempo update already meets the old path at handoff when
	// its authored shape and spatial projection are unchanged. Crossfading the
	// two differently paced trajectories after that point can make them oppose
	// each other, creating an artificial reversal or near-hold on every speed
	// adjustment. Change pace directly at the continuous handoff in this narrow
	// case; spatial/content changes and an in-progress transition still use the
	// full bounded bridge below.
	if transition == nil && samePhaseGeometry(previous, next) {
		before := previous.SampleAt(handoffMillis).PositionPercent
		after := next.SampleAt(handoffMillis).PositionPercent
		beforeDirection := previous.DirectionAt(handoffMillis)
		afterDirection := next.DirectionAt(handoffMillis)
		if beforeDirection == afterDirection && math.Abs(before-after) <= transitionPositionEpsilon {
			return false
		}
	}

	times := motionPathKnotTimes(previous, transition, handoffMillis, handoffMillis+retargetTransitionMillis+1)
	times = append(times, next.knotTimesBetween(handoffMillis, handoffMillis+retargetTransitionMillis+1)...)
	for at := handoffMillis; at <= handoffMillis+retargetTransitionMillis; at += bufferedProbeIntervalMillis {
		times = append(times, at)
	}
	sort.Slice(times, func(left, right int) bool { return times[left] < times[right] })
	for _, at := range uniqueMillis(times) {
		before := sampleMotionPath(previous, transition, at).PositionPercent
		after := next.SampleAt(at).PositionPercent
		if math.Abs(before-after) > transitionPositionEpsilon {
			return true
		}
	}
	return false
}

func samePhaseGeometry(previous, next MotionPlan) bool {
	if !next.PhasePreserved || !previous.Loop || !next.Loop ||
		previous.PatternID != next.PatternID || previous.ProgramID != next.ProgramID ||
		previous.MediaID != next.MediaID || previous.curve.duration != next.curve.duration ||
		previous.curve.loop != next.curve.loop || previous.curve.linear != next.curve.linear ||
		len(previous.curve.authoredKnots) != len(next.curve.authoredKnots) ||
		!sameFocusProjection(previous.focus, next.focus) {
		return false
	}
	for index := range previous.curve.authoredKnots {
		if previous.curve.authoredKnots[index] != next.curve.authoredKnots[index] {
			return false
		}
	}
	return true
}

func sameFocusProjection(left, right focusProjection) bool {
	if left.sourceMin != right.sourceMin || left.sourceSpan != right.sourceSpan ||
		left.targetMin != right.targetMin || left.targetSpan != right.targetSpan {
		return false
	}
	if left.anchor == nil || right.anchor == nil {
		return left.anchor == nil && right.anchor == nil
	}
	return *left.anchor == *right.anchor
}

func motionPathDirection(plan MotionPlan, transition *planTransition, streamMillis int64) int {
	before := sampleMotionPath(plan, transition, streamMillis-bufferedProbeIntervalMillis).PositionPercent
	after := sampleMotionPath(plan, transition, streamMillis+bufferedProbeIntervalMillis).PositionPercent
	return curveDirection(after - before)
}

func motionPathKnotTimes(plan MotionPlan, transition *planTransition, startMillis, endMillis int64) []int64 {
	times := plan.knotTimesBetween(startMillis, endMillis)
	if transition != nil && startMillis < transition.endMillis {
		times = append(times, motionPathKnotTimes(
			transition.previousPlan,
			transition.previousTransition,
			startMillis,
			min(endMillis, transition.endMillis),
		)...)
	}
	return times
}

func pruneTransitionHistory(transition *planTransition, streamMillis int64) *planTransition {
	if transition == nil || streamMillis >= transition.endMillis {
		return nil
	}
	transition.previousTransition = pruneTransitionHistory(transition.previousTransition, streamMillis)
	return transition
}

func transitionOverlaps(transition *planTransition, startMillis, endMillis int64) bool {
	if transition == nil {
		return false
	}
	if startMillis < transition.endMillis && endMillis > transition.startMillis {
		return true
	}
	return transitionOverlaps(transition.previousTransition, startMillis, endMillis)
}

func transitionBoundaryTimes(transition *planTransition, startMillis, endMillis int64) []int64 {
	if transition == nil {
		return nil
	}
	times := transitionBoundaryTimes(transition.previousTransition, startMillis, endMillis)
	for _, at := range []int64{transition.startMillis, transition.endMillis} {
		if at >= startMillis && at < endMillis {
			times = append(times, at)
		}
	}
	return times
}
