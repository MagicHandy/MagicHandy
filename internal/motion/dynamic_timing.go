package motion

import (
	"errors"
	"math"
)

const (
	maximumDynamicTimingFitPasses          = 48
	maximumDynamicTimingRedistributePasses = 32
)

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
	requestedMeanRate := referenceTravelRateForSpeed(speedPercent, handyModel)
	devicePeakRate := referenceTravelRateForSpeed(100, handyModel)
	if travel <= 0 || requestedMeanRate <= 0 || devicePeakRate <= 0 {
		return content, nil
	}
	// The selected percentage describes effective travel over time, not the
	// instantaneous crest of an eased stroke. Reversals may exceed that mean,
	// but never the selected device profile's absolute velocity ceiling.
	desiredDuration := travel * 1000 / requestedMeanRate
	authoredDurations := make([]int64, len(content.points)-1)
	for index := range authoredDurations {
		authoredDurations[index] = content.points[index+1].TimeMillis - content.points[index].TimeMillis
	}
	targetDuration := max(int64(1), int64(math.Ceil(desiredDuration)))
	durations, err := resolveDynamicDurations(
		content, authoredDurations, targetDuration, devicePeakRate,
	)
	if err != nil {
		return content, err
	}
	points, duration := dynamicPointsWithDurations(content.points, durations)
	content.points = points
	content.duration = duration
	content.timingResolved = true
	return content, nil
}

func resolveDynamicDurations(
	content resolvedContent,
	authoredDurations []int64,
	targetDuration int64,
	devicePeakRate float64,
) ([]int64, error) {
	physicalFloorSeed := make([]int64, len(authoredDurations))
	for index := range physicalFloorSeed {
		physicalFloorSeed[index] = 1
	}
	physicalFloors, err := fitDynamicDurationsToEnvelope(
		content, physicalFloorSeed, devicePeakRate,
	)
	if err != nil {
		return nil, err
	}
	var durations []int64
	for range maximumDynamicTimingRedistributePasses {
		if sumDynamicDurations(physicalFloors) >= targetDuration {
			durations, err = fitDynamicDurationsToEnvelope(
				content, physicalFloors, devicePeakRate,
			)
			if err != nil {
				return nil, err
			}
			break
		}
		durations = distributeDynamicDurations(
			physicalFloors, authoredDurations, targetDuration,
		)
		candidate := append([]int64(nil), durations...)
		durations, err = fitDynamicDurationsToEnvelope(
			content, durations, devicePeakRate,
		)
		if err != nil {
			return nil, err
		}
		if sumDynamicDurations(durations) <= targetDuration {
			break
		}
		changed := false
		for index := range physicalFloors {
			if durations[index] > candidate[index] && durations[index] > physicalFloors[index] {
				physicalFloors[index] = durations[index]
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	if len(durations) == 0 {
		durations, err = fitDynamicDurationsToEnvelope(content, physicalFloors, devicePeakRate)
		if err != nil {
			return nil, err
		}
	}
	return durations, nil
}

func fitDynamicDurationsToEnvelope(
	content resolvedContent,
	durations []int64,
	devicePeakRate float64,
) ([]int64, error) {
	durations = append([]int64(nil), durations...)
	fitDynamicReversalGaps(content.points, durations)
	for range maximumDynamicTimingFitPasses {
		points, duration := dynamicPointsWithDurations(content.points, durations)
		runtimeScale := neutralPlaybackScale()
		runtimeScale.maxAccelerationPercentSecond2 = runtimeMaxAccelerationPercentPerSecond2
		curve, err := newCurveWithReversalProfile(
			points, duration, true, false, content.maximumPointLimit(),
			runtimeScale, curveReversalC2Flow,
		)
		if err != nil {
			return nil, err
		}
		changed := false
		for index, segment := range curve.quintics {
			velocity := segment.maximumVelocity() * 1000
			acceleration := segment.maximumAcceleration() * 1e6
			jerk := segment.maximumJerk() * 1e9
			factor := 1.0
			if velocity > devicePeakRate {
				factor = math.Max(factor, velocity/devicePeakRate)
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
		if !changed {
			return durations, nil
		}
	}
	return nil, errors.New("creative timing did not converge inside the runtime envelope")
}

func sumDynamicDurations(durations []int64) int64 {
	total := int64(0)
	for _, duration := range durations {
		total += duration
	}
	return total
}

func distributeDynamicDurations(floors, authored []int64, target int64) []int64 {
	result := append([]int64(nil), floors...)
	if len(result) == 0 || sumDynamicDurations(result) >= target {
		return result
	}
	sumAtScale := func(scale float64) int64 {
		total := int64(0)
		for index, duration := range authored {
			total += max(floors[index], int64(math.Floor(float64(duration)*scale)))
		}
		return total
	}
	lower, upper := 0.0, 1.0
	for sumAtScale(upper) < target {
		upper *= 2
	}
	for range 60 {
		middle := (lower + upper) / 2
		if sumAtScale(middle) <= target {
			lower = middle
		} else {
			upper = middle
		}
	}
	total := int64(0)
	for index, duration := range authored {
		result[index] = max(floors[index], int64(math.Floor(float64(duration)*lower)))
		total += result[index]
	}
	for index := 0; total < target; index = (index + 1) % len(result) {
		result[index]++
		total++
	}
	return result
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
