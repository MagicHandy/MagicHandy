package motion

import (
	"errors"
	"math"
)

// gestureProgress warps a minimum-jerk travel primitive toward a later velocity
// crest. Its clock derivative stays positive, so inertia cannot introduce a
// reversal. Position, velocity and acceleration agree at every junction.
func gestureProgress(u, inertia float64) (position, velocity, acceleration float64) {
	k := 0.65 * inertia
	x := u - k*math.Sin(math.Pi*u)/math.Pi
	dx, ddx := 1-k*math.Cos(math.Pi*u), k*math.Pi*math.Sin(math.Pi*u)
	f := x * x * x * (10 + x*(-15+6*x))
	df := 30 * x * x * (1 - x) * (1 - x)
	ddf := 60 * x * (1 - x) * (1 - 2*x)
	return f, df * dx, ddf*dx*dx + df*ddx
}

func gestureLegCurve(leg gestureLeg, duration int64) Curve {
	const intervals = 8
	points := make([]CurvePoint, intervals+1)
	velocities, accelerations := make([]float64, intervals+1), make([]float64, intervals+1)
	for index := range points {
		at := int64(math.Round(float64(index) * float64(duration) / intervals))
		x, v, a := gestureProgress(float64(at)/float64(duration), leg.inertia)
		distance := leg.to - leg.from
		points[index] = CurvePoint{at, leg.from + distance*x}
		velocities[index] = distance * v / float64(duration)
		accelerations[index] = distance * a / float64(duration*duration)
	}
	points[0].PositionPercent, points[intervals].PositionPercent = leg.from, leg.to
	velocities[0], velocities[intervals], accelerations[0], accelerations[intervals] = 0, 0, 0, 0
	return Curve{points: points, slopes: velocities, accelerations: accelerations,
		quintics: buildQuinticSegments(points, velocities, accelerations), duration: duration}
}

// compileGestureCurve fits each stroke locally before constructing immutable
// content. A short rebound therefore does not dictate the speed of every broad
// stroke. The ordinary prepared plan and runtime sanitizer still enforce the
// exact global envelope, live limits, startup, retargeting and Stop.
func compileGestureCurve(spec FlowSpec, handyModel string) (Curve, error) {
	legs := gestureLegs(spec)
	result := Curve{loop: true}
	rate := referenceTravelRateForSpeed(spec.SpeedPercent, handyModel)
	ceiling := referenceTravelRateForSpeed(100, handyModel)
	for _, leg := range legs {
		unit := gestureLegCurve(leg, 1000)
		seconds := math.Abs(leg.to-leg.from) / rate * leg.weight
		floor := math.Max(float64(runtimeMinimumReversalGapMillis)/1000, unit.maximumVelocityPerMillis()*1000/ceiling)
		floor = math.Max(floor, math.Sqrt(unit.maximumAccelerationPerMillis2()*1e6/runtimeMaxAccelerationPercentPerSecond2))
		floor = math.Max(floor, math.Cbrt(unit.maximumJerkPerMillis3()*1e9/runtimeMaxJerkPercentPerSecond3))
		// If the fast direction reaches its physical floor, keep the slow leg
		// proportionately longer. Independently clamping both legs would silently
		// erase the requested directional contrast at ordinary/high speeds.
		g := spec.Gesture
		if g.FasterDirection != "even" && (leg.to > leg.from) != (g.FasterDirection == "tip") {
			contrast := 0.65 * float64(g.ContrastPercent) / 100
			floor *= (1 + contrast) / (1 - contrast)
		}
		seconds = math.Max(seconds, floor)
		curve := gestureLegCurve(leg, int64(math.Ceil(seconds*1000*1.002)))
		start := 0
		if len(result.points) > 0 {
			start = 1
		}
		result.authoredKnots = append(result.authoredKnots, CurvePoint{result.duration, leg.from})
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
		return Curve{}, errors.New("creative v2 produced an invalid stroke count")
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
				return Curve{}, errors.New("creative v2 produced a non-finite curve")
			}
		}
		c := segment.coefficients
		sign := math.Copysign(1, segment.position(1)-segment.position(0))
		candidates := append([]float64{0, 1}, cubicRootsInUnitInterval(20*c[5], 12*c[4], 6*c[3], 2*c[2])...)
		for _, u := range candidates {
			if sign*segment.velocity(u) < -1e-9 {
				return Curve{}, errors.New("creative v2 interpolation introduced an unintended reversal")
			}
		}
	}
	return result, nil
}
