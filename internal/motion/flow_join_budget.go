package motion

import "math"

// Connecting two different states needs headroom above steady playback. The
// evaluated 1x envelope delayed fast reductions; 2x preserved those joins while
// keeping gentle entries below the fast global envelope.
const layeredJoinHeadroom = 2.0

// Low-speed joins retain the established gentle envelope. Faster plans can
// carry their existing acceleration/jerk into a join, but choosing a slow
// pattern does not grant every handoff the maximum fast-pattern envelope.
func layeredJoinBudget(previous, next MotionPlan) legBudget {
	acceleration, jerk := 2400., 24000.
	for _, plan := range []MotionPlan{previous, next} {
		factor := float64(plan.curve.duration) / float64(plan.PeriodMillis)
		acceleration = math.Max(acceleration, layeredJoinHeadroom*plan.curve.maximumAccelerationPerMillis2()*plan.focus.gain()*factor*factor*1e6)
		jerk = math.Max(jerk, layeredJoinHeadroom*plan.curve.maximumJerkPerMillis3()*plan.focus.gain()*factor*factor*factor*1e9)
	}
	return legBudget{velocity: referenceTravelRateForSpeed(100, next.timingModel),
		acceleration: math.Min(acceleration, flowAccelerationBudget), jerk: math.Min(jerk, flowJerkBudget)}
}
