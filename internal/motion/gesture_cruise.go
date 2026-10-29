package motion

import "math"

// fitGestureCruise uses more of a broad stroke's body when its rounded crest
// would consume the device velocity ceiling. Short strokes, rests and
// unconstrained speeds retain the original profile. Keep a candidate only when it reduces
// the exact kinematic floor; a cruise shoulder must never make a jerk-limited
// stroke slower. This is still fitted with its neighbors before playback.
func fitGestureCruise(legs []gestureLeg, amount float64, budget legBudget) bool {
	changed := false
	for i, leg := range legs {
		span := math.Abs(leg.to - leg.from)
		if leg.rest > 0 || span <= 20 {
			continue
		}
		original := gestureLegCurve(leg, 1000)
		geometry := clampFloat((span-20)/55, 0, 1)
		geometry *= geometry * (3 - 2*geometry)
		leg.cruise = amount * geometry
		if leg.cruise > 0 && legTimeScale(gestureLegCurve(leg, 1000), budget.forLeg(leg)) < legTimeScale(original, budget) {
			legs[i] = leg
			changed = true
		}
	}
	return changed
}

func strokeProgress(leg gestureLeg, u float64) (position, velocity, acceleration float64) {
	position, velocity, acceleration = legProgress(u, leg.inertia, leg.softness)
	if leg.cruise == 0 {
		return
	}
	// Wider strokes can carry a longer constant-velocity body without needing
	// sharper turns. Inertia shifts that body later by lengthening acceleration
	// and shortening braking, rather than warping its cruise velocity upward.
	rise, fall := cruiseRamps(leg)
	x, v, a := cruiseProgress(u, rise, fall)
	position += leg.cruise * (x - position)
	velocity += leg.cruise * (v - velocity)
	acceleration += leg.cruise * (a - acceleration)
	return
}

func cruiseRamps(leg gestureLeg) (rise, fall float64) {
	ramp := 0.38 - 0.001*math.Abs(leg.to-leg.from)
	if leg.cruiseRamp > 0 {
		ramp *= leg.cruiseRamp
	}
	return ramp * (1 + 0.65*leg.inertia), ramp * (1 - 0.65*leg.inertia)
}

// Cubic velocity ramps meet a constant-velocity body with zero acceleration.
// Their nonzero endpoint acceleration preserves an immediate rounded reversal.
func cruiseProgress(u, rise, fall float64) (position, velocity, acceleration float64) {
	return jointCruiseProgress(u, rise, fall, flowingCruiseAcceleration, -flowingCruiseAcceleration)
}

// Adjust only the rounded ramps to the neighbors' shared acceleration. Applying
// an endpoint correction across the whole stroke would add a new velocity hump
// to the cruise body and reintroduce the artificial peak-speed ceiling.
func jointCruiseProgress(u, rise, fall, start, end float64) (position, velocity, acceleration float64) {
	crest := (1 - start*rise*rise/12 + end*fall*fall/12) / (1 - (rise+fall)/2)
	if u < rise {
		return cruiseRamp(u, rise, crest, start)
	}
	if u > 1-fall {
		x, v, a := cruiseRamp(1-u, fall, crest, -end)
		return 1 - x, v, -a
	}
	return crest*(u-rise/2) + start*rise*rise/12, crest, 0
}

func cruiseRamp(at, duration, crest, acceleration float64) (position, velocity, rate float64) {
	u := at / duration
	u2, u3, u4 := u*u, u*u*u, u*u*u*u
	a := acceleration * duration
	return duration * (crest*(u3-u4/2) + a*(u2/2-2*u3/3+u4/4)),
		crest*(3*u2-2*u3) + a*(u-2*u2+u3),
		(crest*(6*u-6*u2) + a*(1-4*u+3*u2)) / duration
}
