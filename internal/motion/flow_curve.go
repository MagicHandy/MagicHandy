package motion

import (
	"errors"
	"math"
)

// compileFlowCurve samples an analytic carrier in cycle space, preserving its
// velocity and acceleration in quintic Hermite intervals. Simpson integration
// and inverse clock correction prevent integer-millisecond knot rounding from
// becoming unintended acceleration/jerk impulses.
func compileFlowCurve(spec FlowSpec, handyModel string) (Curve, error) {
	const samplesPerCycle = 32
	cycles := spec.cycleCount()
	count := int(cycles) * samplesPerCycle
	clock := func(u float64) float64 { _, dt := spec.signal(u, handyModel); return dt }
	integral := func(a, b float64) float64 { return (b - a) * (clock(a) + 4*clock((a+b)/2) + clock(b)) / 6 }
	times := make([]float64, count+1)
	for index := 1; index <= count; index++ {
		times[index] = times[index-1] + integral(float64(index-1)/samplesPerCycle, float64(index)/samplesPerCycle)
	}
	duration := int64(math.Ceil(times[count]))
	clockScale := float64(duration) / times[count]
	points := make([]CurvePoint, count+1)
	velocities := make([]float64, count+1)
	accelerations := make([]float64, count+1)
	for index := 0; index <= count; index++ {
		origin := float64(index) / samplesPerCycle
		at := int64(math.Round(times[index] * clockScale))
		u := origin
		for range 3 {
			u += (float64(at)/clockScale - times[index] - integral(origin, u)) / clock(u)
		}
		x, dt := spec.signal(u, handyModel)
		const probe = 0.0001
		before, dtBefore := spec.signal(u-probe, handyModel)
		after, dtAfter := spec.signal(u+probe, handyModel)
		xPrime := (after - before) / (2 * probe)
		xSecond := (after - 2*x + before) / (probe * probe)
		dtPrime := (dtAfter - dtBefore) / (2 * probe)
		points[index] = CurvePoint{TimeMillis: at, PositionPercent: x}
		velocities[index] = xPrime / (dt * clockScale)
		accelerations[index] = (xSecond*dt - xPrime*dtPrime) / (dt * dt * dt * clockScale * clockScale)
	}
	points[count].PositionPercent = points[0].PositionPercent
	velocities[count], accelerations[count] = velocities[0], accelerations[0]
	if err := validateCurvePoints(points, duration); err != nil {
		return Curve{}, err
	}
	curve := Curve{points: points, slopes: velocities, accelerations: accelerations,
		quintics: buildQuinticSegments(points, velocities, accelerations), duration: duration, loop: true}
	curve.authoredKnots = flowTurningPoints(curve)
	curve.minPosition, curve.maxPosition = curvePointBounds(curve.authoredKnots)
	// Reject invalid numeric states before the plan can scale or sample them.
	for _, segment := range curve.quintics {
		for _, coefficient := range segment.coefficients {
			if math.IsNaN(coefficient) || math.IsInf(coefficient, 0) {
				return Curve{}, errors.New("flow produced a non-finite kinematic state")
			}
		}
	}
	return curve, nil
}

func flowTurningPoints(curve Curve) []CurvePoint {
	points := []CurvePoint{curve.points[0]}
	for index := 1; index < len(curve.points)-1; index++ {
		before, point, after := curve.points[index-1], curve.points[index], curve.points[index+1]
		if (point.PositionPercent-before.PositionPercent)*(after.PositionPercent-point.PositionPercent) >= 0 {
			continue
		}
		left, right := float64(before.TimeMillis), float64(after.TimeMillis)
		for range 35 {
			middle := (left + right) / 2
			if curve.velocityFloat(left)*curve.velocityFloat(middle) <= 0 {
				right = middle
			} else {
				left = middle
			}
		}
		at := int64(math.Round((left + right) / 2))
		if at > points[len(points)-1].TimeMillis && at < curve.duration {
			points = append(points, CurvePoint{TimeMillis: at, PositionPercent: curve.sampleFloat(float64(at))})
		}
	}
	return append(points, curve.points[len(curve.points)-1])
}
