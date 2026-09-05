package motion

import (
	"errors"
	"math"
)

// Adjacent strokes share one acceleration at their reversal. Fit that joint
// curve, not isolated rest-to-rest legs, under the unchanged runtime envelope.
func fitGestureCurves(spec FlowSpec, legs []gestureLeg, handyModel string) ([]Curve, error) {
	durations := make([]int64, len(legs))
	rate := referenceTravelRateForSpeed(spec.SpeedPercent, handyModel)
	ceiling := referenceTravelRateForSpeed(100, handyModel)
	for i, leg := range legs {
		floor := gestureTimeScale(gestureLegCurve(leg, 1000), ceiling)
		if spec.Gesture.FasterDirection != "even" && (leg.to > leg.from) != (spec.Gesture.FasterDirection == "tip") {
			contrast := 0.65 * float64(spec.Gesture.ContrastPercent) / 100
			floor *= (1 + contrast) / (1 - contrast)
		}
		seconds := math.Max(math.Abs(leg.to-leg.from)/rate*leg.weight, floor)
		durations[i] = int64(math.Ceil(seconds * 1000 * 1.002))
	}
	curves := make([]Curve, len(legs))
	for range 16 {
		turns := gestureTurnAccelerations(legs, durations)
		changed := false
		for i, leg := range legs {
			curves[i] = gestureTurnCurve(leg, durations[i], turns[i], turns[(i+1)%len(legs)])
			if scale := gestureTimeScale(curves[i], ceiling); scale > 1 {
				durations[i] = int64(math.Ceil(float64(durations[i]) * scale * 1.002))
				changed = true
			}
		}
		if !changed {
			return curves, nil
		}
	}
	return nil, errors.New("creative v2 joint timing did not converge inside motion limits")
}

func gestureTimeScale(curve Curve, ceiling float64) float64 {
	floor := math.Max(float64(runtimeMinimumReversalGapMillis)/float64(curve.duration), curve.maximumVelocityPerMillis()*1000/ceiling)
	floor = math.Max(floor, math.Sqrt(curve.maximumAccelerationPerMillis2()*1e6/runtimeMaxAccelerationPercentPerSecond2))
	return math.Max(floor, math.Cbrt(curve.maximumJerkPerMillis3()*1e9/runtimeMaxJerkPercentPerSecond3))
}

func gestureTurnAccelerations(legs []gestureLeg, durations []int64) []float64 {
	turns := make([]float64, len(legs))
	for i, leg := range legs {
		previous := (i + len(legs) - 1) % len(legs)
		_, _, before := gestureProgress(1, legs[previous].inertia)
		_, _, after := gestureProgress(0, leg.inertia)
		a := before * (legs[previous].to - legs[previous].from) / float64(durations[previous]*durations[previous])
		b := after * (leg.to - leg.from) / float64(durations[i]*durations[i])
		turns[i] = math.Copysign(math.Min(math.Abs(a), math.Abs(b)), b)
	}
	return turns
}
