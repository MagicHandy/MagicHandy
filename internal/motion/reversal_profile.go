package motion

import (
	"cmp"
	"slices"
)

// A short velocity ramp preserves a true zero-velocity reversal without
// spreading endpoint easing across the whole stroke body.
const maximumPatternReversalBlendMillis int64 = 75

func withBoundedLoopReversalGuides(points []CurvePoint) ([]CurvePoint, map[int64]float64) {
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
		leftBlend := int64(0)
		rightBlend := int64(0)
		if leftReversal {
			leftBlend = min(maximumPatternReversalBlendMillis, duration/3)
		}
		if rightReversal {
			rightBlend = min(maximumPatternReversalBlendMillis, duration/3)
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
