package motion

// The base Layered envelope, which the top-speed envelope widens only above
// its knee (ADR 0064).
const flowAccelerationBudget = runtimeMaxAccelerationPercentPerSecond2
const flowJerkBudget = runtimeMaxJerkPercentPerSecond3

// envelopeAcceleration and envelopeJerk are what a plan may reach at its
// requested speed: the runtime envelope at and below the top-speed knee.
func envelopeAcceleration(plan MotionPlan) float64 {
	acceleration, _ := topSpeedEnvelope(plan.Target.SpeedPercent)
	return acceleration
}

func envelopeJerk(plan MotionPlan) float64 {
	_, jerk := topSpeedEnvelope(plan.Target.SpeedPercent)
	return jerk
}

// joinEnvelope bounds a bridge between two plans: the faster plan's envelope.
func joinEnvelope(previous, next MotionPlan) (acceleration, jerk float64) {
	return topSpeedEnvelope(max(previous.Target.SpeedPercent, next.Target.SpeedPercent))
}
