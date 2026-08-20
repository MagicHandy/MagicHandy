package motion

import (
	"cmp"
	"math"
	"slices"
)

// Stored patterns use a short velocity ramp to preserve their authored stroke
// body while still reaching zero velocity at a true reversal. Its length is an
// acceleration budget rather than a constant: a fixed duration makes a slow or
// narrowed stroke spend the same absolute time crawling through its turn as a
// fast full-range one. Creative deliberately selects whole-leg PCHIP easing
// instead, because natural braking matters more there than retaining a catalog
// waveform's constant-velocity body.
const (
	// These are played-time limits. Pattern authoring deliberately stays inside
	// the quieter catalogMaxAcceleration/catalogMinReversalGap envelope, while
	// runtime playback may use the wider calibrated device envelope when the
	// selected speed asks for it.
	runtimeMaxAccelerationPercentPerSecond2       = 7500.0
	runtimeMinimumReversalGapMillis               = int64(100)
	maximumPatternReversalBlendMillis       int64 = 75
	minimumPatternReversalBlendMillis       int64 = 4
	// Two thirds of the selected curve ceiling leaves the rest of the curve its
	// own headroom. playbackScale chooses the catalog ceiling for authoring and
	// the wider runtime ceiling for an engine plan.
	reversalAccelerationShare = 2.0 / 3.0
)

// curveReversalProfile selects how a generated curve approaches a true turn.
// Stored patterns retain their short, acceleration-budgeted ramps so authored
// rhythm is not silently rewritten. Creative content uses the natural PCHIP
// profile: the whole leg participates in braking and acceleration, which avoids
// a long constant-velocity run ending in a perceptibly abrupt direction swap.
type curveReversalProfile uint8

const (
	curveReversalBoundedRamp curveReversalProfile = iota
	curveReversalWholeLeg
)

// playbackScale describes how authored curve coordinates become played motion:
// one authored millisecond lasts timeFactor played milliseconds, and one
// authored percent covers amplitudeFactor played percent. Both compress the
// reversal budget, so both belong in the ramp calculation.
type playbackScale struct {
	timeFactor                    float64
	amplitudeFactor               float64
	maxAccelerationPercentSecond2 float64
}

func neutralPlaybackScale() playbackScale {
	return playbackScale{
		timeFactor: 1, amplitudeFactor: 1,
		maxAccelerationPercentSecond2: catalogMaxAcceleration,
	}
}

func (s playbackScale) normalized() playbackScale {
	if !(s.timeFactor > 0) || math.IsInf(s.timeFactor, 0) {
		s.timeFactor = 1
	}
	if !(s.amplitudeFactor > 0) || math.IsInf(s.amplitudeFactor, 0) {
		s.amplitudeFactor = 1
	}
	if !(s.maxAccelerationPercentSecond2 > 0) || math.IsInf(s.maxAccelerationPercentSecond2, 0) {
		s.maxAccelerationPercentSecond2 = catalogMaxAcceleration
	}
	return s
}

// reversalBlendMillis returns the shortest ramp, in authored curve
// milliseconds, that still respects the acceleration budget once playback
// scaling is applied.
//
// A ramp of b on each side leaves d-b of cruise, so the cruise velocity is
// delta/(d-b) and the ramp acceleration is that over b. Solving
// b*(d-b) = delta/a for the smaller root gives the shortest ramp that stays
// inside the budget; a leg too fast to satisfy it at all ramps throughout.
func (s playbackScale) reversalBlendMillis(deltaPercent float64, durationMillis int64) int64 {
	if durationMillis <= 0 {
		return minimumPatternReversalBlendMillis
	}
	s = s.normalized()
	duration := float64(durationMillis)
	maximumAccelerationPerMillis2 := s.maxAccelerationPercentSecond2 * reversalAccelerationShare / 1e6
	budget := math.Abs(deltaPercent) * s.amplitudeFactor /
		(maximumAccelerationPerMillis2 * s.timeFactor * s.timeFactor)
	blend := duration / 2
	if discriminant := duration*duration - 4*budget; discriminant > 0 {
		blend = (duration - math.Sqrt(discriminant)) / 2
	}
	// The curve stores authored milliseconds, but the reversal envelope is a
	// played-time contract. Keeping a fixed authored cap made faster playback
	// shorten the physical ramp exactly when it needed more room, while very
	// slow playback could stretch a four-millisecond guide into a visible hold.
	minimumAuthored := max(int64(1), int64(math.Ceil(
		float64(minimumPatternReversalBlendMillis)/s.timeFactor,
	)))
	maximumAuthored := max(minimumAuthored, int64(math.Ceil(
		float64(maximumPatternReversalBlendMillis)/s.timeFactor,
	)))
	return min(
		maximumAuthored,
		max(minimumAuthored, int64(math.Ceil(blend))),
	)
}

func withBoundedLoopReversalGuides(points []CurvePoint, scale playbackScale) ([]CurvePoint, map[int64]float64) {
	if len(points) < 3 {
		return points, nil
	}
	result := append([]CurvePoint(nil), points...)
	guideSlopes := make(map[int64]float64)
	directions := make([]int, len(points)-1)
	for index := range directions {
		directions[index] = curveDirection(points[index+1].PositionPercent - points[index].PositionPercent)
	}
	for index, direction := range directions {
		if direction == 0 {
			continue
		}
		previousDirection := directions[(index+len(directions)-1)%len(directions)]
		nextDirection := directions[(index+1)%len(directions)]
		leftReversal := previousDirection != 0 && previousDirection != direction
		rightReversal := nextDirection != 0 && nextDirection != direction
		if !leftReversal && !rightReversal {
			continue
		}

		left := points[index]
		right := points[index+1]
		duration := right.TimeMillis - left.TimeMillis
		if duration < 3 {
			continue
		}
		budget := scale.reversalBlendMillis(right.PositionPercent-left.PositionPercent, duration)
		leftBlend := int64(0)
		rightBlend := int64(0)
		if leftReversal {
			leftBlend = min(budget, duration/3)
		}
		if rightReversal {
			rightBlend = min(budget, duration/3)
		}
		travelMillis := float64(duration) - float64(leftBlend+rightBlend)/2
		if travelMillis <= 0 {
			continue
		}
		velocity := (right.PositionPercent - left.PositionPercent) / travelMillis
		if leftBlend > 0 {
			at := left.TimeMillis + leftBlend
			result = append(result, CurvePoint{
				TimeMillis:      at,
				PositionPercent: left.PositionPercent + velocity*float64(leftBlend)/2,
			})
			guideSlopes[at] = velocity
		}
		if rightBlend > 0 {
			at := right.TimeMillis - rightBlend
			result = append(result, CurvePoint{
				TimeMillis:      at,
				PositionPercent: right.PositionPercent - velocity*float64(rightBlend)/2,
			})
			guideSlopes[at] = velocity
		}
	}
	slices.SortStableFunc(result, func(left, right CurvePoint) int {
		return cmp.Compare(left.TimeMillis, right.TimeMillis)
	})
	return deduplicateTimes(result), guideSlopes
}
