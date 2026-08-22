package motion

import "math"

// flowingReversalAcceleration is the normalized endpoint acceleration of a
// half-cosine leg. Keeping each shared reversal acceleration at or below this
// value preserves monotonic travel while giving a repeating stroke a rounded
// turn instead of a rest-to-rest minimum-jerk pause.
const flowingReversalAcceleration = math.Pi * math.Pi / 2

// quinticSegment stores one wall-time quintic Hermite interval. Coefficients
// are expressed in normalized interval time u; duration converts derivatives
// back to authored milliseconds.
type quinticSegment struct {
	coefficients [6]float64
	duration     float64
}

func newQuinticSegment(
	left, right CurvePoint,
	leftVelocity, rightVelocity,
	leftAcceleration, rightAcceleration float64,
) quinticSegment {
	duration := float64(right.TimeMillis - left.TimeMillis)
	displacement := right.PositionPercent - left.PositionPercent
	leftVelocity *= duration
	rightVelocity *= duration
	leftAcceleration *= duration * duration
	rightAcceleration *= duration * duration
	return quinticSegment{
		duration: duration,
		coefficients: [6]float64{
			left.PositionPercent,
			leftVelocity,
			leftAcceleration / 2,
			10*displacement - 6*leftVelocity - 4*rightVelocity -
				1.5*leftAcceleration + 0.5*rightAcceleration,
			-15*displacement + 8*leftVelocity + 7*rightVelocity +
				1.5*leftAcceleration - rightAcceleration,
			6*displacement - 3*leftVelocity - 3*rightVelocity -
				0.5*leftAcceleration + 0.5*rightAcceleration,
		},
	}
}

func (s quinticSegment) position(u float64) float64 {
	u = clampFloat(u, 0, 1)
	c := s.coefficients
	return c[0] + u*(c[1]+u*(c[2]+u*(c[3]+u*(c[4]+u*c[5]))))
}

func (s quinticSegment) velocity(u float64) float64 {
	if s.duration <= 0 {
		return 0
	}
	u = clampFloat(u, 0, 1)
	c := s.coefficients
	return (c[1] + u*(2*c[2]+u*(3*c[3]+u*(4*c[4]+u*5*c[5])))) / s.duration
}

func (s quinticSegment) acceleration(u float64) float64 {
	if s.duration <= 0 {
		return 0
	}
	u = clampFloat(u, 0, 1)
	c := s.coefficients
	return (2*c[2] + u*(6*c[3]+u*(12*c[4]+u*20*c[5]))) /
		(s.duration * s.duration)
}

func (s quinticSegment) jerk(u float64) float64 {
	if s.duration <= 0 {
		return 0
	}
	u = clampFloat(u, 0, 1)
	c := s.coefficients
	return (6*c[3] + u*(24*c[4]+u*60*c[5])) /
		(s.duration * s.duration * s.duration)
}

func (s quinticSegment) maximumAcceleration() float64 {
	candidates := []float64{0, 1}
	c := s.coefficients
	for _, root := range quadraticRoots(60*c[5], 24*c[4], 6*c[3]) {
		if root > 0 && root < 1 {
			candidates = append(candidates, root)
		}
	}
	maximum := 0.0
	for _, u := range candidates {
		maximum = math.Max(maximum, math.Abs(s.acceleration(u)))
	}
	return maximum
}

func (s quinticSegment) maximumJerk() float64 {
	candidates := []float64{0, 1}
	c := s.coefficients
	if math.Abs(c[5]) > 1e-12 {
		if vertex := -c[4] / (5 * c[5]); vertex > 0 && vertex < 1 {
			candidates = append(candidates, vertex)
		}
	}
	maximum := 0.0
	for _, u := range candidates {
		maximum = math.Max(maximum, math.Abs(s.jerk(u)))
	}
	return maximum
}

func quadraticRoots(a, b, c float64) []float64 {
	if math.Abs(a) <= 1e-12 {
		if math.Abs(b) <= 1e-12 {
			return nil
		}
		return []float64{-c / b}
	}
	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		return nil
	}
	root := math.Sqrt(discriminant)
	return []float64{(-b - root) / (2 * a), (-b + root) / (2 * a)}
}

// flowingQuinticStates derives one velocity and acceleration state per knot.
// PCHIP's duration-aware tangent remains useful at pass-through anchors, but
// is capped to twice either neighboring secant so a zero-acceleration quintic
// cannot reverse inside the interval. True reversals share a single
// cosine-like acceleration selected from the quieter adjacent leg. This makes
// the loop C2 even when neighboring stroke lengths and timings differ.
func flowingQuinticStates(points []CurvePoint, loop bool) ([]float64, []float64) {
	slopes := monotoneSlopes(points, loop)
	accelerations := make([]float64, len(points))
	if !loop || len(points) < 3 {
		return slopes, accelerations
	}

	knotCount := len(points) - 1 // final loop knot duplicates knot zero
	for index := range knotCount {
		previousSegment := (index + knotCount - 1) % knotCount
		nextSegment := index
		previousDuration := float64(points[previousSegment+1].TimeMillis - points[previousSegment].TimeMillis)
		nextDuration := float64(points[nextSegment+1].TimeMillis - points[nextSegment].TimeMillis)
		previousDelta := points[previousSegment+1].PositionPercent - points[previousSegment].PositionPercent
		nextDelta := points[nextSegment+1].PositionPercent - points[nextSegment].PositionPercent
		if previousDuration <= 0 || nextDuration <= 0 || previousDelta*nextDelta <= 0 {
			slopes[index] = 0
			if previousDelta*nextDelta < 0 {
				previousNatural := flowingReversalAcceleration * math.Abs(previousDelta) /
					(previousDuration * previousDuration)
				nextNatural := flowingReversalAcceleration * math.Abs(nextDelta) /
					(nextDuration * nextDuration)
				accelerations[index] = math.Copysign(
					math.Min(previousNatural, nextNatural), nextDelta,
				)
			}
			continue
		}

		// A shared pass-through velocity must satisfy the monotone-quintic
		// bound in both adjacent intervals.
		limit := 2 * math.Min(
			math.Abs(previousDelta)/previousDuration,
			math.Abs(nextDelta)/nextDuration,
		)
		slopes[index] = math.Copysign(math.Min(math.Abs(slopes[index]), limit), nextDelta)
	}
	slopes[len(slopes)-1] = slopes[0]
	accelerations[len(accelerations)-1] = accelerations[0]
	return slopes, accelerations
}

func buildQuinticSegments(
	points []CurvePoint,
	slopes, accelerations []float64,
) []quinticSegment {
	segments := make([]quinticSegment, len(points)-1)
	for index := range segments {
		segments[index] = newQuinticSegment(
			points[index], points[index+1],
			slopes[index], slopes[index+1],
			accelerations[index], accelerations[index+1],
		)
	}
	return segments
}
