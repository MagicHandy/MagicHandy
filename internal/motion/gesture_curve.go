package motion

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// gestureProgress warps a rounded oscillator half-cycle toward a later velocity
// crest. Unlike a rest-to-rest primitive, its turn has nonzero acceleration:
// it starts returning without settling at every destination.
func gestureProgress(u, inertia float64) (position, velocity, acceleration float64) {
	return legProgress(u, inertia, 0)
}

// legProgress is one stroke's normalized travel. Softness blends the rounded
// half-cycle toward a septic stroke whose ends have no velocity, acceleration
// or jerk, the same blend the historical continuous carrier used for its soft
// turns. Inertia time-warps either shape toward a later velocity crest.
func legProgress(u, inertia, softness float64) (position, velocity, acceleration float64) {
	k := 0.65 * inertia
	x := u - k*math.Sin(math.Pi*u)/math.Pi
	dx, ddx := 1-k*math.Cos(math.Pi*u), k*math.Pi*math.Sin(math.Pi*u)
	f := (1 - math.Cos(math.Pi*x)) / 2
	df := math.Pi * math.Sin(math.Pi*x) / 2
	ddf := math.Pi * math.Pi * math.Cos(math.Pi*x) / 2
	if softness > 0 {
		rest := 1 - x
		septic := x * x * x * x * (35 + x*(-84+x*(70-20*x)))
		dSeptic := 140 * x * x * x * rest * rest * rest
		ddSeptic := 420 * x * x * rest * rest * (1 - 2*x)
		f += softness * (septic - f)
		df += softness * (dSeptic - df)
		ddf += softness * (ddSeptic - ddf)
	}
	return f, df * dx, ddf*dx*dx + df*ddx
}

func gestureLegCurve(leg gestureLeg, duration int64) Curve {
	_, _, start := strokeProgress(leg, 0)
	_, _, end := strokeProgress(leg, 1)
	gain := (leg.to - leg.from) / float64(duration*duration)
	return gestureTurnCurve(leg, duration, start*gain, end*gain)
}

func gestureTurnCurve(leg gestureLeg, duration int64, startAcceleration, endAcceleration float64) Curve {
	const intervals = 8
	times := make([]int64, intervals+1)
	for index := range times {
		times[index] = int64(math.Round(float64(index) * float64(duration) / intervals))
	}
	rise, fall := cruiseRamps(leg)
	if leg.cruise > 0 {
		// Include the ramp/body joins so interpolation preserves the cruise.
		riseAt, fallAt := int64(math.Round(rise*float64(duration))), int64(math.Round((1-fall)*float64(duration)))
		times = append(times, riseAt, fallAt)
		slices.Sort(times)
		times = slices.Compact(times)
		rise, fall = float64(riseAt)/float64(duration), 1-float64(fallAt)/float64(duration)
	}
	points := make([]CurvePoint, len(times))
	velocities, accelerations := make([]float64, len(times)), make([]float64, len(times))
	distance := leg.to - leg.from
	_, _, nativeStart := legProgress(0, leg.inertia, leg.softness)
	_, _, nativeEnd := legProgress(1, leg.inertia, leg.softness)
	deltaStart := startAcceleration*float64(duration*duration) - distance*nativeStart
	deltaEnd := endAcceleration*float64(duration*duration) - distance*nativeEnd
	for index := range points {
		at := times[index]
		u := float64(at) / float64(duration)
		x, v, a := legProgress(u, leg.inertia, leg.softness)
		cx, cv, ca := gestureAccelerationBlend(u, deltaStart, deltaEnd)
		x, v, a = distance*x+cx, distance*v+cv, distance*a+ca
		if leg.cruise > 0 && distance != 0 {
			gain := float64(duration*duration) / distance
			qx, qv, qa := jointCruiseProgress(u, rise, fall, startAcceleration*gain, endAcceleration*gain)
			x += leg.cruise * (distance*qx - x)
			v += leg.cruise * (distance*qv - v)
			a += leg.cruise * (distance*qa - a)
		}
		points[index] = CurvePoint{at, leg.from + x}
		velocities[index] = v / float64(duration)
		accelerations[index] = a / float64(duration*duration)
	}
	last := len(points) - 1
	points[0].PositionPercent, points[last].PositionPercent = leg.from, leg.to
	velocities[0], velocities[last] = 0, 0
	accelerations[0], accelerations[last] = startAcceleration, endAcceleration
	return Curve{points: points, slopes: velocities, accelerations: accelerations,
		quintics: buildQuinticSegments(points, velocities, accelerations), duration: duration}
}

// Distribute the shared-turn correction over the entire stroke. Overwriting
// only the endpoint would concentrate its acceleration change into one short
// interpolation interval, creating an artificial braking shoulder.
func gestureAccelerationBlend(u, start, end float64) (position, velocity, acceleration float64) {
	u2, u3, u4, u5 := u*u, u*u*u, u*u*u*u, u*u*u*u*u
	return start*0.5*(u2-3*u3+3*u4-u5) + end*0.5*(u3-2*u4+u5),
		start*(u-4.5*u2+6*u3-2.5*u4) + end*(1.5*u2-4*u3+2.5*u4),
		start*(1-9*u+18*u2-10*u3) + end*(3*u-12*u2+10*u3)
}

// compileGestureCurve fits each stroke locally before constructing immutable
// content. A short rebound therefore does not dictate the speed of every broad
// stroke. The ordinary prepared plan and runtime sanitizer still enforce the
// exact global envelope, live limits, startup, retargeting and Stop.
func compileGestureCurve(spec FlowSpec, handyModel string) (Curve, error) {
	legs := gestureLegs(spec)
	curves, err := fitGestureCurves(spec, legs, handyModel)
	if err != nil {
		return Curve{}, err
	}
	best, err := assembleLegCurve(legs, curves, "creative v2")
	if err != nil {
		return Curve{}, err
	}
	// Use the established rounded profile unless its physical fit has added
	// time. Breathing, contrast and deliberate rests are requested rhythm,
	// not unwanted slowdowns to optimize away.
	rate := referenceTravelRateForSpeed(spec.SpeedPercent, handyModel)
	requested := 0.0
	for _, leg := range legs {
		if leg.rest > 0 {
			requested += leg.rest * 1000
		} else {
			requested += math.Ceil(math.Abs(leg.to-leg.from) / rate * leg.weight * 1000 * 1.002)
		}
	}
	if float64(best.duration) <= requested*1.005 {
		return best, nil
	}
	budget := legBudget{velocity: referenceTravelRateForSpeed(100, handyModel),
		acceleration: runtimeMaxAccelerationPercentPerSecond2, jerk: runtimeMaxJerkPercentPerSecond3}
	for _, amount := range []float64{0.25, 0.5, 0.75, 1} {
		candidateLegs := slices.Clone(legs)
		if !fitGestureCruise(candidateLegs, amount, budget) {
			continue
		}
		candidateCurves, fitErr := fitGestureCurves(spec, candidateLegs, handyModel)
		if fitErr != nil {
			continue
		}
		candidate, fitErr := assembleLegCurve(candidateLegs, candidateCurves, "creative v2")
		if fitErr == nil && float64(candidate.duration) < float64(best.duration)*0.995 {
			best = candidate
		}
		if float64(best.duration) <= requested*1.005 {
			break
		}
	}
	return best, nil
}

// assembleLegCurve joins fitted strokes into one looping curve and verifies the
// actual interpolant: every stroke must stay monotonic, so the only reversals
// are the authored turns between strokes.
func assembleLegCurve(legs []gestureLeg, curves []Curve, name string) (Curve, error) {
	return assembleLegs(legs, curves, name, true)
}

// assembleLegs is assembleLegCurve for looping or finite content.
func assembleLegs(legs []gestureLeg, curves []Curve, name string, loop bool) (Curve, error) {
	result := Curve{loop: loop}
	for legIndex, curve := range curves {
		start := 0
		if len(result.points) > 0 {
			start = 1
		}
		result.authoredKnots = append(result.authoredKnots, CurvePoint{result.duration, legs[legIndex].from})
		for index := start; index < len(curve.points); index++ {
			point := curve.points[index]
			point.TimeMillis += result.duration
			result.points = append(result.points, point)
			result.slopes = append(result.slopes, curve.slopes[index])
			result.accelerations = append(result.accelerations, curve.accelerations[index])
		}
		result.duration += curve.duration
	}
	if len(result.points) < 3 || len(result.points) > maximumCurvePoints {
		return Curve{}, fmt.Errorf("%s produced an invalid stroke count", name)
	}
	result.authoredKnots = append(result.authoredKnots, result.points[len(result.points)-1])
	result.quintics = buildQuinticSegments(result.points, result.slopes, result.accelerations)
	result.minPosition, result.maxPosition = curvePointBounds(result.authoredKnots)
	if err := validateCurvePoints(result.points, result.duration); err != nil {
		return Curve{}, err
	}
	// Analytic travel is monotonic; verify the actual interpolant before playback.
	for _, segment := range result.quintics {
		for _, coefficient := range segment.coefficients {
			if math.IsNaN(coefficient) || math.IsInf(coefficient, 0) {
				return Curve{}, fmt.Errorf("%s produced a non-finite curve", name)
			}
		}
		c := segment.coefficients
		sign := math.Copysign(1, segment.position(1)-segment.position(0))
		candidates := append([]float64{0, 1}, cubicRootsInUnitInterval(20*c[5], 12*c[4], 6*c[3], 2*c[2])...)
		for _, u := range candidates {
			if sign*segment.velocity(u) < -1e-9 {
				return Curve{}, errors.New(name + " interpolation introduced an unintended reversal")
			}
		}
	}
	return result, nil
}
