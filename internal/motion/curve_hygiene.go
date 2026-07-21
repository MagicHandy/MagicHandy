package motion

import "math"

// MinimumPatternReversalProminence is the smallest adjacent swing treated as
// an intentional loop reversal. Smaller extrema are positional chatter.
const MinimumPatternReversalProminence = 2.0

const (
	maximumPatternChatterFlankMillis = int64(250)
	minimumPatternChatterCycleParts  = int64(25)
)

// StabilizePatternReversals removes rapid, insignificant extrema while
// preserving monotonic detail, endpoints, dwell timing, and slow subtle motion.
func StabilizePatternReversals(points []CurvePoint, minimumProminence float64) []CurvePoint {
	result := append([]CurvePoint(nil), points...)
	if minimumProminence <= 0 {
		return result
	}
	for len(result) > 2 {
		anchors := curveReversalAnchors(result)
		removed := false
		for index := 1; index < len(anchors)-1; index++ {
			left := result[anchors[index-1]].PositionPercent
			current := result[anchors[index]].PositionPercent
			right := result[anchors[index+1]].PositionPercent
			prominence := math.Min(math.Abs(current-left), math.Abs(current-right))
			leftMillis := result[anchors[index]].TimeMillis - result[anchors[index-1]].TimeMillis
			rightMillis := result[anchors[index+1]].TimeMillis - result[anchors[index]].TimeMillis
			if prominence > minimumProminence ||
				min(leftMillis, rightMillis) > patternChatterFlankMillis(result) {
				continue
			}
			pointIndex := anchors[index]
			result = append(result[:pointIndex], result[pointIndex+1:]...)
			removed = true
			break
		}
		if !removed {
			break
		}
	}
	return result
}

func patternChatterFlankMillis(points []CurvePoint) int64 {
	duration := points[len(points)-1].TimeMillis - points[0].TimeMillis
	return max(int64(1), min(maximumPatternChatterFlankMillis, duration/minimumPatternChatterCycleParts))
}

func curveReversalAnchors(points []CurvePoint) []int {
	anchors := []int{0}
	previousDirection := 0
	for index := 1; index < len(points); index++ {
		direction := curveDirection(points[index].PositionPercent - points[index-1].PositionPercent)
		if direction == 0 {
			continue
		}
		if previousDirection != 0 && direction != previousDirection {
			anchors = append(anchors, index-1)
		}
		previousDirection = direction
	}
	last := len(points) - 1
	if anchors[len(anchors)-1] != last {
		anchors = append(anchors, last)
	}
	return anchors
}

func curveDirection(value float64) int {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	default:
		return 0
	}
}
