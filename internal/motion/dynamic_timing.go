package motion

import (
	"errors"
	"math"
)

const maximumDynamicTimingFitPasses = 32

// retimeDynamicContent fits each Creative interval independently at the
// requested calibrated travel rate. The former planner applied one global
// slowdown chosen by the phrase's single worst interval. One short or strongly
// varied stroke could therefore collapse every selected speed onto the same
// period. Local fitting preserves authored rhythm where feasible and lengthens
// only intervals that need acceleration, jerk, or reversal headroom.
//
// The result is still one deterministic C2 curve in the shared engine. The
// ordinary exact runtime envelope remains a final fail-safe after focus.
func retimeDynamicContent(
	content resolvedContent,
	speedPercent int,
	handyModel string,
) (resolvedContent, error) {
	if !content.loop || content.reversalProfile != curveReversalC2Flow ||
		len(content.points) < 3 || content.duration <= 0 {
		return content, nil
	}

	travel := totalCurveTravel(content.points)
	requestedRate := referenceTravelRateForSpeed(speedPercent, handyModel)
	if travel <= 0 || requestedRate <= 0 {
		return content, nil
	}
	desiredDuration := travel * 1000 / requestedRate
	globalScale := desiredDuration / float64(content.duration)
	durations := make([]int64, len(content.points)-1)
	for index := range durations {
		authored := content.points[index+1].TimeMillis - content.points[index].TimeMillis
		durations[index] = max(int64(1), int64(math.Round(float64(authored)*globalScale)))
	}
	fitDynamicReversalGaps(content.points, durations)

	for range maximumDynamicTimingFitPasses {
		points, duration := dynamicPointsWithDurations(content.points, durations)
		curve, err := newCurveWithReversalProfile(
			points, duration, true, false, content.maximumPointLimit(),
			neutralPlaybackScale(), curveReversalC2Flow,
		)
		if err != nil {
			return content, err
		}
		changed := false
		for index, segment := range curve.quintics {
			velocity := segment.maximumVelocity() * 1000
			acceleration := segment.maximumAcceleration() * 1e6
			jerk := segment.maximumJerk() * 1e9
			factor := 1.0
			if velocity > requestedRate {
				factor = math.Max(factor, velocity/requestedRate)
			}
			if acceleration > runtimeMaxAccelerationPercentPerSecond2 {
				factor = math.Max(factor,
					math.Sqrt(acceleration/runtimeMaxAccelerationPercentPerSecond2))
			}
			if jerk > runtimeMaxJerkPercentPerSecond3 {
				factor = math.Max(factor,
					math.Cbrt(jerk/runtimeMaxJerkPercentPerSecond3))
			}
			if factor <= 1.001 {
				continue
			}
			next := int64(math.Ceil(float64(durations[index]) * factor * 1.002))
			if next <= durations[index] {
				next = durations[index] + 1
			}
			durations[index] = next
			changed = true
		}
		if fitDynamicReversalGaps(content.points, durations) {
			changed = true
		}
		if changed {
			continue
		}
		content.points = points
		content.duration = duration
		content.timingResolved = true
		return content, nil
	}
	return content, errors.New("creative timing did not converge inside the runtime envelope")
}

func (c resolvedContent) maximumPointLimit() int {
	if c.maximumPoints > 0 {
		return c.maximumPoints
	}
	return maximumCurvePoints
}

func dynamicPointsWithDurations(points []CurvePoint, durations []int64) ([]CurvePoint, int64) {
	result := make([]CurvePoint, len(points))
	elapsed := int64(0)
	for index, point := range points {
		result[index] = CurvePoint{TimeMillis: elapsed, PositionPercent: point.PositionPercent}
		if index < len(durations) {
			elapsed += durations[index]
		}
	}
	return result, elapsed
}

// fitDynamicReversalGaps gives every pair of true extrema the runtime minimum
// separation without treating pass-through anchors as reversals.
func fitDynamicReversalGaps(points []CurvePoint, durations []int64) bool {
	if len(points) < 3 || len(durations) != len(points)-1 {
		return false
	}
	knotCount := len(points) - 1 // final loop point duplicates knot zero
	reversals := make([]int, 0, knotCount)
	for index := range knotCount {
		previous := (index + knotCount - 1) % knotCount
		incoming := points[index].PositionPercent - points[previous].PositionPercent
		outgoing := points[index+1].PositionPercent - points[index].PositionPercent
		if incoming*outgoing < 0 {
			reversals = append(reversals, index)
		}
	}
	if len(reversals) < 2 {
		return false
	}
	changed := false
	for index, start := range reversals {
		end := reversals[(index+1)%len(reversals)]
		indices := dynamicCyclicIntervalIndices(start, end, knotCount)
		gap := int64(0)
		for _, interval := range indices {
			gap += durations[interval]
		}
		if gap >= runtimeMinimumReversalGapMillis || gap <= 0 {
			continue
		}
		scale := float64(runtimeMinimumReversalGapMillis) / float64(gap)
		for _, interval := range indices {
			durations[interval] = max(
				durations[interval]+1,
				int64(math.Ceil(float64(durations[interval])*scale)),
			)
		}
		changed = true
	}
	return changed
}

func dynamicCyclicIntervalIndices(start, end, count int) []int {
	result := make([]int, 0, count)
	for current := start; current != end; current = (current + 1) % count {
		result = append(result, current)
		if len(result) > count {
			break
		}
	}
	return result
}
