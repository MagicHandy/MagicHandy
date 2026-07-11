package patterns

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	defaultSimplifyError = 1.5
	maximumRawPoints     = 4096
	previewSampleTarget  = 240
)

// PreviewPattern simplifies authored points, closes/floor-stretches the loop,
// and samples it with the same backend PCHIP implementation used for playback.
func PreviewPattern(input PatternInput) (PreviewResult, error) {
	originalCount := len(input.Points)
	points, err := prepareRawPoints(input.Points, input.CycleMillis)
	if err != nil {
		return PreviewResult{}, err
	}
	tolerance := input.SimplifyError
	if tolerance <= 0 {
		tolerance = defaultSimplifyError
	}
	// Leave one slot for loop closure when the drawn endpoints differ.
	points, err = simplifyToLimit(points, tolerance, maxPatternPoints-1)
	if err != nil {
		return PreviewResult{}, err
	}
	definition, err := motion.NormalizePatternDefinition(motion.PatternDefinition{
		ID:          "preview",
		Name:        "Preview",
		Description: input.Description,
		Kind:        input.Kind,
		CycleMillis: input.CycleMillis,
		Points:      points,
		Tags:        input.Tags,
	})
	if err != nil {
		return PreviewResult{}, err
	}
	curve, err := motion.NewCurve(definition.Points, definition.CycleMillis, true)
	if err != nil {
		return PreviewResult{}, err
	}
	interval := max(int64(25), definition.CycleMillis/previewSampleTarget)
	return PreviewResult{
		Points:          definition.Points,
		Samples:         curve.Preview(interval),
		CycleMillis:     definition.CycleMillis,
		OriginalCount:   originalCount,
		SimplifiedCount: len(definition.Points),
	}, nil
}

// AuditionDefinition applies a temporary backend-owned feel filter. It never
// mutates the saved pattern and still uses the shared PCHIP sampler.
func AuditionDefinition(definition motion.PatternDefinition, feel string) motion.PatternDefinition {
	feel = strings.ToLower(strings.TrimSpace(feel))
	result := definition
	result.Points = append([]motion.CurvePoint(nil), definition.Points...)
	switch feel {
	case "smooth":
		for index := 1; index < len(result.Points)-1; index++ {
			previous := definition.Points[index-1].PositionPercent
			current := definition.Points[index].PositionPercent
			next := definition.Points[index+1].PositionPercent
			result.Points[index].PositionPercent = (previous + 2*current + next) / 4
		}
		result.CycleMillis = int64(math.Round(float64(result.CycleMillis) * 1.15))
	case "crisp":
		for index := 1; index < len(result.Points)-1; index++ {
			position := 50 + (definition.Points[index].PositionPercent-50)*1.1
			result.Points[index].PositionPercent = math.Max(0, math.Min(100, position))
		}
	}
	normalized, err := motion.NormalizePatternDefinition(result)
	if err != nil {
		return definition
	}
	return normalized
}

func prepareRawPoints(raw []motion.CurvePoint, cycleMillis int64) ([]motion.CurvePoint, error) {
	if len(raw) < 2 || len(raw) > maximumRawPoints {
		return nil, fmt.Errorf("authoring requires 2..%d points", maximumRawPoints)
	}
	if cycleMillis < 0 || cycleMillis > maxContentDuration {
		return nil, errors.New("motion content duration must be between zero and 24 hours")
	}
	points := append([]motion.CurvePoint(nil), raw...)
	for index, point := range points {
		if point.TimeMillis < 0 || point.TimeMillis > maxContentDuration {
			return nil, fmt.Errorf("authoring point %d has invalid time", index)
		}
		if math.IsNaN(point.PositionPercent) || math.IsInf(point.PositionPercent, 0) {
			return nil, fmt.Errorf("authoring point %d has invalid position", index)
		}
	}
	slices.SortStableFunc(points, comparePointTime)
	points = deduplicatePointTimes(points)
	if len(points) < 2 {
		return nil, errors.New("authoring points need distinct times")
	}
	start := points[0].TimeMillis
	duration := points[len(points)-1].TimeMillis - start
	if duration <= 0 {
		return nil, errors.New("authoring duration must be positive")
	}
	if cycleMillis <= 0 {
		cycleMillis = duration
	}
	for index := range points {
		relative := points[index].TimeMillis - start
		points[index].TimeMillis = int64(math.Round(float64(relative) * float64(cycleMillis) / float64(duration)))
		points[index].PositionPercent = math.Max(0, math.Min(100, points[index].PositionPercent))
		if index > 0 && points[index].TimeMillis <= points[index-1].TimeMillis {
			points[index].TimeMillis = points[index-1].TimeMillis + 1
		}
	}
	points[len(points)-1].TimeMillis = cycleMillis
	return points, nil
}

func simplifyToLimit(points []motion.CurvePoint, tolerance float64, limit int) ([]motion.CurvePoint, error) {
	if tolerance < 0.1 {
		tolerance = 0.1
	}
	for range 12 {
		simplified := simplifyPreservingReversals(points, tolerance)
		if len(simplified) <= limit {
			return simplified, nil
		}
		tolerance *= 1.35
	}
	return nil, fmt.Errorf("curve has more than %d essential reversal points", limit)
}

func simplifyPreservingReversals(points []motion.CurvePoint, tolerance float64) []motion.CurvePoint {
	anchors := reversalAnchors(points)
	result := make([]motion.CurvePoint, 0, len(points))
	for index := 1; index < len(anchors); index++ {
		segment := simplifySegment(points[anchors[index-1]:anchors[index]+1], tolerance)
		if len(result) > 0 && len(segment) > 0 {
			segment = segment[1:]
		}
		result = append(result, segment...)
	}
	return result
}

func reversalAnchors(points []motion.CurvePoint) []int {
	anchors := []int{0}
	previousDirection := 0
	for index := 1; index < len(points); index++ {
		direction := sign(points[index].PositionPercent - points[index-1].PositionPercent)
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

func simplifySegment(points []motion.CurvePoint, tolerance float64) []motion.CurvePoint {
	if len(points) <= 2 {
		return append([]motion.CurvePoint(nil), points...)
	}
	keep := make([]bool, len(points))
	keep[0], keep[len(points)-1] = true, true
	markSimplified(points, 0, len(points)-1, tolerance, keep)
	result := make([]motion.CurvePoint, 0, len(points))
	for index, point := range points {
		if keep[index] {
			result = append(result, point)
		}
	}
	return result
}

func markSimplified(points []motion.CurvePoint, start, end int, tolerance float64, keep []bool) {
	if end-start <= 1 {
		return
	}
	maximumError := 0.0
	maximumIndex := -1
	for index := start + 1; index < end; index++ {
		errorValue := verticalError(points[start], points[end], points[index])
		if errorValue > maximumError {
			maximumError, maximumIndex = errorValue, index
		}
	}
	if maximumIndex < 0 || maximumError <= tolerance {
		return
	}
	keep[maximumIndex] = true
	markSimplified(points, start, maximumIndex, tolerance, keep)
	markSimplified(points, maximumIndex, end, tolerance, keep)
}

func verticalError(left, right, point motion.CurvePoint) float64 {
	width := float64(right.TimeMillis - left.TimeMillis)
	if width <= 0 {
		return math.Abs(point.PositionPercent - right.PositionPercent)
	}
	fraction := float64(point.TimeMillis-left.TimeMillis) / width
	expected := left.PositionPercent + (right.PositionPercent-left.PositionPercent)*fraction
	return math.Abs(point.PositionPercent - expected)
}

func comparePointTime(left, right motion.CurvePoint) int {
	switch {
	case left.TimeMillis < right.TimeMillis:
		return -1
	case left.TimeMillis > right.TimeMillis:
		return 1
	default:
		return 0
	}
}

func deduplicatePointTimes(points []motion.CurvePoint) []motion.CurvePoint {
	result := make([]motion.CurvePoint, 0, len(points))
	for _, point := range points {
		if len(result) > 0 && result[len(result)-1].TimeMillis == point.TimeMillis {
			result[len(result)-1] = point
			continue
		}
		result = append(result, point)
	}
	return result
}

func sign(value float64) int {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	default:
		return 0
	}
}
