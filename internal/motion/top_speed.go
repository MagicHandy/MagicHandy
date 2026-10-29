package motion

// Top-speed reversals (ADR 0064). The speed scale defines 100% as mean travel
// equal to the device's peak velocity, which no stroke with turnarounds can
// average. Under the fixed envelope, generated strokes reached that peak with
// rounded turns and stopped getting faster near 79% effective, so requests
// from about 85% up all played alike. Above the knee the turn envelope widens
// and a cruise stroke may shorten its ramps, so each higher request turns
// more sharply and travels faster. At and below the knee nothing changes.
const (
	topSpeedKneePercent = 75
	// Multipliers of the software turn-quality envelope at 100%. These are
	// not manufacturer acceleration/jerk ratings. Physical acceptance is open.
	topSpeedAccelerationScale = 3.0
	topSpeedJerkScale         = 8.0
	// The shortest cruise ramp, as a share of the ordinary ramp, at 100%.
	topSpeedMinimumCruiseRamp = 0.35
	// A sharper-turning cruise stroke holds the velocity ceiling for most of
	// its length. Headroom keeps the quantized wire timing room to round.
	steepCruiseVelocityShare = 0.985
	// The widened envelope never turns a stroke faster than this, so immediate
	// transports keep their commands (an Intiface Handy needs 110 ms).
	topSpeedMinimumLegMillis = 120
)

// topSpeedShare is 0 at and below the knee and rises smoothly to 1 at 100%.
func topSpeedShare(speedPercent int) float64 {
	t := clampFloat(float64(speedPercent-topSpeedKneePercent)/float64(100-topSpeedKneePercent), 0, 1)
	return t * t * (3 - 2*t)
}

// topSpeedEnvelope returns the played acceleration and jerk ceilings for a
// requested speed: the runtime envelope at and below the knee.
func topSpeedEnvelope(speedPercent int) (acceleration, jerk float64) {
	share := topSpeedShare(speedPercent)
	return runtimeMaxAccelerationPercentPerSecond2 * (1 + (topSpeedAccelerationScale-1)*share),
		runtimeMaxJerkPercentPerSecond3 * (1 + (topSpeedJerkScale-1)*share)
}

// Only prepared generators opt into sharper top-speed turns. Legacy patterns
// and imported content retain their own base envelope, including in diagnostics.
func targetMotionEnvelope(target MotionTarget) (acceleration, jerk float64) {
	if target.prepared == nil {
		return runtimeMaxAccelerationPercentPerSecond2, runtimeMaxJerkPercentPerSecond3
	}
	acceleration, jerk = topSpeedEnvelope(target.SpeedPercent)
	return min(acceleration, target.prepared.acceleration), min(jerk, target.prepared.jerk)
}

// topSpeedBudget is the leg budget generated strokes are fitted inside at a
// requested speed, including how short a cruise stroke's ramps may become.
func topSpeedBudget(speedPercent int, handyModel string) legBudget {
	acceleration, jerk := topSpeedEnvelope(speedPercent)
	return legBudget{velocity: referenceTravelRateForSpeed(100, handyModel),
		acceleration: acceleration, jerk: jerk,
		cruiseRampFloor:  1 - (1-topSpeedMinimumCruiseRamp)*topSpeedShare(speedPercent),
		widenedLegMillis: topSpeedMinimumLegMillis}
}

// forLeg is the budget for one stroke: a stroke with shortened cruise ramps
// keeps a little velocity headroom below the device ceiling.
func (b legBudget) forLeg(leg gestureLeg) legBudget {
	if leg.cruise > 0 && leg.cruiseRamp > 0 && leg.cruiseRamp < 1 {
		b.velocity *= steepCruiseVelocityShare
	}
	return b
}

// cruiseRampCandidates are the ramp shares a velocity-limited cruise stroke
// may try under budget, ordinary first. Only a strictly faster valid fit is
// kept, so a steeper ramp is never chosen when it gains nothing.
func cruiseRampCandidates(budget legBudget) []float64 {
	candidates := []float64{1}
	if budget.cruiseRampFloor <= 0 {
		return candidates
	}
	// Nested five-percent ramp steps avoid the large shape jumps of a four
	// item menu. Every newly allowed profile remains available at higher
	// speeds; a moving single candidate can discard yesterday's better fit.
	for step := 19; step >= 7; step-- {
		ramp := float64(step) / 20
		if ramp >= budget.cruiseRampFloor-1e-9 {
			candidates = append(candidates, ramp)
		}
	}
	return candidates
}

// peakSpeedPercent is the fastest speed the score requests, across its steps.
func (s FlowSpec) peakSpeedPercent() int {
	peak := s.SpeedPercent
	for _, step := range s.Steps {
		peak = max(peak, step.SpeedPercent)
	}
	return peak
}
