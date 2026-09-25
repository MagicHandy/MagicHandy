package motion

import (
	"errors"
	"math"
)

// legBudget is the kinematic envelope a stroke sequence is fitted inside:
// peak velocity in percent per second, acceleration and jerk.
type legBudget struct {
	velocity, acceleration, jerk float64
}

// Adjacent strokes share one acceleration at their reversal. Fit that joint
// curve, not isolated rest-to-rest legs, under the unchanged runtime envelope.
func fitGestureCurves(spec FlowSpec, legs []gestureLeg, handyModel string) ([]Curve, error) {
	durations := make([]int64, len(legs))
	rate := referenceTravelRateForSpeed(spec.SpeedPercent, handyModel)
	budget := legBudget{velocity: referenceTravelRateForSpeed(100, handyModel),
		acceleration: runtimeMaxAccelerationPercentPerSecond2, jerk: runtimeMaxJerkPercentPerSecond3}
	for i, leg := range legs {
		if leg.rest > 0 {
			durations[i] = int64(math.Ceil(leg.rest * 1000))
			continue
		}
		floor := legTimeScale(gestureLegCurve(leg, 1000), budget)
		if spec.Gesture.FasterDirection != "even" && (leg.to > leg.from) != (spec.Gesture.FasterDirection == "tip") {
			contrast := 0.65 * leg.contrast
			floor *= (1 + contrast) / (1 - contrast)
		}
		seconds := math.Max(math.Abs(leg.to-leg.from)/rate*leg.weight, floor)
		durations[i] = int64(math.Ceil(seconds * 1000 * 1.002))
	}
	curves, err := fitLegCurves(legs, durations, budget)
	if err != nil {
		return nil, errors.New("creative v2 joint timing did not converge inside motion limits")
	}
	return curves, nil
}

// fitLegCurves lengthens any stroke whose joint curve exceeds the budget, then
// refits its neighbors' shared turns, until every stroke fits.
func fitLegCurves(legs []gestureLeg, durations []int64, budget legBudget) ([]Curve, error) {
	curves := make([]Curve, len(legs))
	for range 16 {
		turns := gestureTurnAccelerations(legs, durations)
		changed := false
		for i, leg := range legs {
			curves[i] = gestureTurnCurve(leg, durations[i], turns[i], turns[(i+1)%len(legs)])
			if scale := legTimeScale(curves[i], budget); scale > 1 {
				durations[i] = int64(math.Ceil(float64(durations[i]) * scale * 1.002))
				changed = true
			}
		}
		if !changed {
			return curves, nil
		}
	}
	return nil, errors.New("joint stroke timing did not converge inside motion limits")
}

func legTimeScale(curve Curve, budget legBudget) float64 {
	floor := math.Max(float64(runtimeMinimumReversalGapMillis)/float64(curve.duration), curve.maximumVelocityPerMillis()*1000/budget.velocity)
	floor = math.Max(floor, math.Sqrt(curve.maximumAccelerationPerMillis2()*1e6/budget.acceleration))
	return math.Max(floor, math.Cbrt(curve.maximumJerkPerMillis3()*1e9/budget.jerk))
}

func gestureTurnAccelerations(legs []gestureLeg, durations []int64) []float64 {
	turns := make([]float64, len(legs))
	for i, leg := range legs {
		previous := (i + len(legs) - 1) % len(legs)
		_, _, before := legProgress(1, legs[previous].inertia, legs[previous].softness)
		_, _, after := legProgress(0, leg.inertia, leg.softness)
		a := before * (legs[previous].to - legs[previous].from) / float64(durations[previous]*durations[previous])
		b := after * (leg.to - leg.from) / float64(durations[i]*durations[i])
		turns[i] = math.Copysign(math.Min(math.Abs(a), math.Abs(b)), b)
	}
	return turns
}
