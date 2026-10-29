package motion

import "math"

// samplingTolerance keeps the trajectory's interpolation bound separate from
// already rounded/retimed output points. In particular, the immutable append
// tail is a wire anchor, not an exact sample of the mathematical trajectory.
// Differentiating it against the next probe can invent a large acceleration
// and densify even slow motion after every append.
type samplingTolerance struct {
	position     float64
	acceleration float64
}

func (e *Engine) samplingTolerance(position float64) samplingTolerance {
	acceleration := runtimeMaxAccelerationPercentPerSecond2
	planBound := func(plan MotionPlan) float64 {
		bound, _ := targetMotionEnvelope(plan.Target)
		if bound <= runtimeMaxAccelerationPercentPerSecond2 || plan.PeriodMillis <= 0 {
			return runtimeMaxAccelerationPercentPerSecond2
		}
		factor := float64(plan.curve.duration) / float64(plan.PeriodMillis)
		return plan.curve.maximumAccelerationPerMillis2() * plan.focus.gain() * factor * factor * 1e6
	}
	acceleration = math.Max(acceleration, planBound(e.plan))
	for transition := e.transition; transition != nil; transition = transition.previousTransition {
		if transition.endMillis <= e.nextSampleMillis {
			continue
		}
		acceleration = math.Max(acceleration, planBound(transition.previousPlan))
		if transition.stroke != nil {
			acceleration = math.Max(acceleration, transition.stroke.maximumAcceleration()*1e6)
		}
	}
	return samplingTolerance{position: position, acceleration: acceleration}
}

func (t samplingTolerance) probeTolerance(probes []MotionSample, resolution float64) float64 {
	var gap int64
	for i := 1; i < len(probes); i++ {
		gap = max(gap, probes[i].TimeMillis-probes[i-1].TimeMillis)
	}
	acceleration := math.Max(runtimeMaxAccelerationPercentPerSecond2, t.acceleration)
	reserve := acceleration * float64(gap) * float64(gap) / 8e6
	return resolution/2 + math.Max(0, t.position-reserve)
}
