package motion

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

const (
	// RoutineCycleFloorMillis is the hardware-derived minimum catalog cycle.
	RoutineCycleFloorMillis int64 = 6600
	// PatternKindRoutine identifies a normal repeating pattern.
	PatternKindRoutine = "routine"
	// PatternKindBurst identifies a deliberately short shape exempt from the
	// routine cycle floor.
	PatternKindBurst = "burst"

	minimumBurstCycleMillis int64 = 500
	maximumCurvePoints            = 4096
	catalogSampleMillis     int64 = 25
	catalogMinReversalGap   int64 = 450
	catalogMaxAcceleration        = 3000.0

	// TagExperimental marks catalog content that is fully playable in the
	// library but exposed to the model only behind the user's
	// experimental-patterns capability gate.
	TagExperimental = "experimental"
)

// CurvePoint is one relative 0..100 motion sample at wall-clock time.
type CurvePoint struct {
	TimeMillis      int64   `json:"time_ms"`
	PositionPercent float64 `json:"position_percent"`
}

// PatternDefinition is reusable loop content. Positions are always relative
// and are projected into the configured stroke window only by the transport.
type PatternDefinition struct {
	ID          PatternID    `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Kind        string       `json:"kind"`
	CycleMillis int64        `json:"cycle_ms"`
	Points      []CurvePoint `json:"points"`
	Tags        []string     `json:"tags,omitempty"`
}

// ProgramDefinition is finite, non-looping motion content.
type ProgramDefinition struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	DurationMillis int64        `json:"duration_ms"`
	Points         []CurvePoint `json:"points"`
}

// CurveMetrics exposes generator budget measurements for tests and diagnostics.
type CurveMetrics struct {
	MaxAccelerationPercentPerSecond2 float64 `json:"max_acceleration_percent_per_second2"`
	MinReversalGapMillis             int64   `json:"min_reversal_gap_ms"`
}

// Curve is a validated time-parameterized monotone cubic sampler.
type Curve struct {
	points   []CurvePoint
	slopes   []float64
	duration int64
	loop     bool
}

var builtinPatternCatalog = []PatternDefinition{
	generateStrokePattern(),
	generatePulsePattern(),
	generateTeasePattern(),
	generateWavesPattern(),
	generateClimbPattern(),
	generateFlutterPattern(),
}

// NewCurve validates points and builds PCHIP-style wall-time derivatives.
func NewCurve(points []CurvePoint, durationMillis int64, loop bool) (Curve, error) {
	if len(points) < 2 {
		return Curve{}, errors.New("a motion curve requires at least two points")
	}
	if len(points) > maximumCurvePoints {
		return Curve{}, fmt.Errorf("a motion curve supports at most %d points", maximumCurvePoints)
	}
	copyPoints := append([]CurvePoint(nil), points...)
	if err := validateCurvePoints(copyPoints, durationMillis); err != nil {
		return Curve{}, err
	}
	return Curve{
		points:   copyPoints,
		slopes:   monotoneSlopes(copyPoints, loop),
		duration: durationMillis,
		loop:     loop,
	}, nil
}

// Sample returns the shape-preserving interpolated relative position.
func (c Curve) Sample(timeMillis int64) float64 {
	return c.sampleFloat(float64(c.normalizeTime(timeMillis)))
}

// Velocity returns the wall-clock derivative in percent per second.
func (c Curve) Velocity(timeMillis int64) float64 {
	return c.velocityFloat(float64(c.normalizeTime(timeMillis))) * 1000
}

// Preview returns backend-sampled points including the final endpoint.
func (c Curve) Preview(intervalMillis int64) []CurvePoint {
	if intervalMillis < 10 {
		intervalMillis = 10
	}
	points := make([]CurvePoint, 0, c.duration/int64(intervalMillis)+2)
	for at := int64(0); at < c.duration; at += intervalMillis {
		points = append(points, CurvePoint{TimeMillis: at, PositionPercent: c.Sample(at)})
	}
	points = append(points, CurvePoint{TimeMillis: c.duration, PositionPercent: c.Sample(c.duration)})
	return points
}

// BuiltinPatternDefinitions returns the small parametrically generated catalog.
func BuiltinPatternDefinitions() []PatternDefinition {
	definitions := make([]PatternDefinition, len(builtinPatternCatalog))
	for index, definition := range builtinPatternCatalog {
		definitions[index] = clonePatternDefinition(definition)
	}
	return definitions
}

// BuiltinPatternDefinition resolves one generated built-in pattern.
func BuiltinPatternDefinition(id PatternID) (PatternDefinition, bool) {
	for _, definition := range builtinPatternCatalog {
		if definition.ID == id {
			return clonePatternDefinition(definition), true
		}
	}
	return PatternDefinition{}, false
}

// NormalizePatternDefinition validates, closes, and floor-stretches loop data.
func NormalizePatternDefinition(definition PatternDefinition) (PatternDefinition, error) {
	definition.ID = PatternID(strings.TrimSpace(string(definition.ID)))
	definition.Name = strings.TrimSpace(definition.Name)
	definition.Description = strings.TrimSpace(definition.Description)
	definition.Kind = strings.ToLower(strings.TrimSpace(definition.Kind))
	if definition.ID == "" || definition.Name == "" {
		return PatternDefinition{}, errors.New("pattern id and name are required")
	}
	if definition.Kind == "" {
		definition.Kind = PatternKindRoutine
	}
	if definition.Kind != PatternKindRoutine && definition.Kind != PatternKindBurst {
		return PatternDefinition{}, fmt.Errorf("unknown pattern kind %q", definition.Kind)
	}

	points, duration, err := normalizePoints(definition.Points, definition.CycleMillis, true)
	if err != nil {
		return PatternDefinition{}, err
	}
	minimum := RoutineCycleFloorMillis
	if definition.Kind == PatternKindBurst {
		minimum = minimumBurstCycleMillis
	}
	if duration < minimum {
		points = scalePointTimes(points, duration, minimum)
		duration = minimum
	}
	definition.CycleMillis = duration
	definition.Points = points
	definition.Tags = normalizeTags(definition.Tags)
	if _, err := NewCurve(points, duration, true); err != nil {
		return PatternDefinition{}, err
	}
	return definition, nil
}

// NormalizeProgramDefinition validates finite program content without looping it.
func NormalizeProgramDefinition(definition ProgramDefinition) (ProgramDefinition, error) {
	definition.ID = strings.TrimSpace(definition.ID)
	definition.Name = strings.TrimSpace(definition.Name)
	if definition.ID == "" || definition.Name == "" {
		return ProgramDefinition{}, errors.New("program id and name are required")
	}
	points, duration, err := normalizePoints(definition.Points, definition.DurationMillis, false)
	if err != nil {
		return ProgramDefinition{}, err
	}
	definition.DurationMillis = duration
	definition.Points = points
	if _, err := NewCurve(points, duration, false); err != nil {
		return ProgramDefinition{}, err
	}
	return definition, nil
}

// MeasureCurve reports the wall-time acceleration and reversal spacing.
func MeasureCurve(points []CurvePoint, durationMillis int64, loop bool) (CurveMetrics, error) {
	curve, err := NewCurve(points, durationMillis, loop)
	if err != nil {
		return CurveMetrics{}, err
	}
	metrics := CurveMetrics{MinReversalGapMillis: reversalGap(points)}
	previousVelocity := curve.Velocity(0)
	for at := catalogSampleMillis; at <= durationMillis; at += catalogSampleMillis {
		velocity := curve.Velocity(at)
		acceleration := math.Abs(velocity-previousVelocity) * 1000 / float64(catalogSampleMillis)
		metrics.MaxAccelerationPercentPerSecond2 = math.Max(metrics.MaxAccelerationPercentPerSecond2, acceleration)
		previousVelocity = velocity
	}
	return metrics, nil
}

func (c Curve) normalizeTime(timeMillis int64) int64 {
	if c.duration <= 0 {
		return 0
	}
	if c.loop {
		timeMillis %= c.duration
		if timeMillis < 0 {
			timeMillis += c.duration
		}
		return timeMillis
	}
	if timeMillis < 0 {
		return 0
	}
	if timeMillis > c.duration {
		return c.duration
	}
	return timeMillis
}

func (c Curve) sampleFloat(at float64) float64 {
	left, right := c.interval(at)
	if left == right {
		return c.points[left].PositionPercent
	}
	h := float64(c.points[right].TimeMillis - c.points[left].TimeMillis)
	u := (at - float64(c.points[left].TimeMillis)) / h
	y0, y1 := c.points[left].PositionPercent, c.points[right].PositionPercent
	m0, m1 := c.slopes[left], c.slopes[right]
	h00 := 2*u*u*u - 3*u*u + 1
	h10 := u*u*u - 2*u*u + u
	h01 := -2*u*u*u + 3*u*u
	h11 := u*u*u - u*u
	return clampFloat(h00*y0+h10*h*m0+h01*y1+h11*h*m1, 0, 100)
}

func (c Curve) velocityFloat(at float64) float64 {
	left, right := c.interval(at)
	if left == right {
		return c.slopes[left]
	}
	h := float64(c.points[right].TimeMillis - c.points[left].TimeMillis)
	u := (at - float64(c.points[left].TimeMillis)) / h
	y0, y1 := c.points[left].PositionPercent, c.points[right].PositionPercent
	m0, m1 := c.slopes[left], c.slopes[right]
	return ((6*u*u-6*u)*y0+(-6*u*u+6*u)*y1)/h + (3*u*u-4*u+1)*m0 + (3*u*u-2*u)*m1
}

func (c Curve) interval(at float64) (int, int) {
	index := sort.Search(len(c.points), func(index int) bool {
		return float64(c.points[index].TimeMillis) >= at
	})
	if index <= 0 {
		return 0, 0
	}
	if index >= len(c.points) {
		last := len(c.points) - 1
		return last, last
	}
	if float64(c.points[index].TimeMillis) == at {
		return index, index
	}
	return index - 1, index
}

func monotoneSlopes(points []CurvePoint, loop bool) []float64 {
	count := len(points)
	h := make([]float64, count-1)
	delta := make([]float64, count-1)
	for index := range count - 1 {
		h[index] = float64(points[index+1].TimeMillis - points[index].TimeMillis)
		delta[index] = (points[index+1].PositionPercent - points[index].PositionPercent) / h[index]
	}
	slopes := make([]float64, count)
	for index := 1; index < count-1; index++ {
		if delta[index-1]*delta[index] <= 0 {
			continue
		}
		w1 := 2*h[index] + h[index-1]
		w2 := h[index] + 2*h[index-1]
		slopes[index] = (w1 + w2) / (w1/delta[index-1] + w2/delta[index])
	}
	if !loop && count > 2 {
		slopes[0] = endpointSlope(h[0], h[1], delta[0], delta[1])
		slopes[count-1] = endpointSlope(h[count-2], h[count-3], delta[count-2], delta[count-3])
	}
	return slopes
}

func endpointSlope(here, next, deltaHere, deltaNext float64) float64 {
	slope := ((2*here+next)*deltaHere - here*deltaNext) / (here + next)
	if slope*deltaHere <= 0 {
		return 0
	}
	if deltaHere*deltaNext < 0 && math.Abs(slope) > math.Abs(3*deltaHere) {
		return 3 * deltaHere
	}
	return slope
}

func validateCurvePoints(points []CurvePoint, durationMillis int64) error {
	if durationMillis <= 0 {
		return errors.New("curve duration must be positive")
	}
	if points[0].TimeMillis != 0 || points[len(points)-1].TimeMillis != durationMillis {
		return errors.New("curve points must span exactly from zero to duration")
	}
	for index, point := range points {
		if point.PositionPercent < 0 || point.PositionPercent > 100 || math.IsNaN(point.PositionPercent) || math.IsInf(point.PositionPercent, 0) {
			return fmt.Errorf("curve point %d position must be between 0 and 100", index)
		}
		if index > 0 && point.TimeMillis <= points[index-1].TimeMillis {
			return errors.New("curve point times must increase strictly")
		}
	}
	return nil
}

func normalizePoints(raw []CurvePoint, requestedDuration int64, closeLoop bool) ([]CurvePoint, int64, error) {
	if len(raw) < 2 || len(raw) > maximumCurvePoints {
		return nil, 0, fmt.Errorf("motion content requires 2..%d points", maximumCurvePoints)
	}
	points := append([]CurvePoint(nil), raw...)
	slices.SortStableFunc(points, func(left, right CurvePoint) int {
		return cmp.Compare(left.TimeMillis, right.TimeMillis)
	})
	points = deduplicateTimes(points)
	if len(points) < 2 {
		return nil, 0, errors.New("motion content requires distinct point times")
	}
	start := points[0].TimeMillis
	for index := range points {
		points[index].TimeMillis -= start
		points[index].PositionPercent = clampFloat(points[index].PositionPercent, 0, 100)
	}
	duration := points[len(points)-1].TimeMillis
	if requestedDuration > 0 && duration > 0 && requestedDuration != duration {
		points = scalePointTimes(points, duration, requestedDuration)
		duration = requestedDuration
	}
	if duration <= 0 {
		return nil, 0, errors.New("motion content duration must be positive")
	}
	if closeLoop && math.Abs(points[len(points)-1].PositionPercent-points[0].PositionPercent) > 0.001 {
		points = closeCurve(points, duration)
	}
	return points, duration, nil
}

func deduplicateTimes(points []CurvePoint) []CurvePoint {
	result := make([]CurvePoint, 0, len(points))
	for _, point := range points {
		if len(result) > 0 && result[len(result)-1].TimeMillis == point.TimeMillis {
			result[len(result)-1] = point
			continue
		}
		result = append(result, point)
	}
	return result
}

func closeCurve(points []CurvePoint, duration int64) []CurvePoint {
	closeWindow := max(int64(250), duration/20)
	if closeWindow >= duration {
		closeWindow = duration / 2
	}
	points = scalePointTimes(points, duration, duration-closeWindow)
	return append(points, CurvePoint{TimeMillis: duration, PositionPercent: points[0].PositionPercent})
}

func scalePointTimes(points []CurvePoint, from, to int64) []CurvePoint {
	if from <= 0 || from == to {
		return append([]CurvePoint(nil), points...)
	}
	scaled := make([]CurvePoint, len(points))
	for index, point := range points {
		scaled[index] = point
		scaled[index].TimeMillis = int64(math.Round(float64(point.TimeMillis) * float64(to) / float64(from)))
		if index > 0 && scaled[index].TimeMillis <= scaled[index-1].TimeMillis {
			scaled[index].TimeMillis = scaled[index-1].TimeMillis + 1
		}
	}
	scaled[len(scaled)-1].TimeMillis = to
	return scaled
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, min(len(tags), 12))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || len(tag) > 32 {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
		if len(result) == 12 {
			break
		}
	}
	return result
}

func generateStrokePattern() PatternDefinition {
	points := make([]CurvePoint, 13)
	for index := range points {
		position := 0.0
		if index%2 == 1 {
			position = 100
		}
		points[index] = CurvePoint{TimeMillis: int64(index) * 550, PositionPercent: position}
	}
	return mustFitCatalog(PatternDefinition{
		ID: PatternStroke, Name: "Stroke", Description: "Even full-span reversals.",
		Kind: PatternKindRoutine, CycleMillis: RoutineCycleFloorMillis, Points: points,
		Tags: []string{"steady", "full", "balanced"},
	})
}

func generatePulsePattern() PatternDefinition {
	positions := []float64{15, 100, 25, 85}
	points := make([]CurvePoint, 13)
	for index := range points {
		points[index] = CurvePoint{TimeMillis: int64(index) * 550, PositionPercent: positions[index%len(positions)]}
	}
	points[len(points)-1].PositionPercent = points[0].PositionPercent
	return mustFitCatalog(PatternDefinition{
		ID: PatternPulse, Name: "Pulse", Description: "Alternating deep and shorter peaks.",
		Kind: PatternKindRoutine, CycleMillis: RoutineCycleFloorMillis, Points: points,
		Tags: []string{"rhythmic", "varied", "peaks"},
	})
}

func generateTeasePattern() PatternDefinition {
	peaks := []float64{45, 60, 80, 100, 75, 55}
	points := make([]CurvePoint, 0, len(peaks)*2+1)
	points = append(points, CurvePoint{PositionPercent: 20})
	for index, peak := range peaks {
		points = append(points,
			CurvePoint{TimeMillis: int64(index*2+1) * 550, PositionPercent: peak},
			CurvePoint{TimeMillis: int64(index*2+2) * 550, PositionPercent: 20},
		)
	}
	return mustFitCatalog(PatternDefinition{
		ID: PatternTease, Name: "Tease", Description: "Progressive peaks with a consistent return.",
		Kind: PatternKindRoutine, CycleMillis: RoutineCycleFloorMillis, Points: points,
		Tags: []string{"progressive", "varied", "build"},
	})
}

// generateWavesPattern swells stroke amplitude up and back down across one
// cycle, so intensity breathes instead of holding one level.
func generateWavesPattern() PatternDefinition {
	positions := []float64{30, 55, 25, 75, 20, 95, 25, 75, 30, 55}
	points := make([]CurvePoint, 0, len(positions)+1)
	for index, position := range positions {
		points = append(points, CurvePoint{TimeMillis: int64(index) * 550, PositionPercent: position})
	}
	points = append(points, CurvePoint{TimeMillis: int64(len(positions)) * 550, PositionPercent: positions[0]})
	return mustFitCatalog(PatternDefinition{
		ID: PatternWaves, Name: "Waves", Description: "Experimental: strokes swell deeper, crest, and recede.",
		Kind: PatternKindRoutine, CycleMillis: RoutineCycleFloorMillis, Points: points,
		Tags: []string{TagExperimental, "swell", "varied", "breathing"},
	})
}

// generateClimbPattern ratchets progressively deeper with shallow recoveries,
// then releases across the full span.
func generateClimbPattern() PatternDefinition {
	positions := []float64{10, 40, 20, 55, 30, 70, 40, 85, 50, 100}
	points := make([]CurvePoint, 0, len(positions)+1)
	for index, position := range positions {
		points = append(points, CurvePoint{TimeMillis: int64(index) * 550, PositionPercent: position})
	}
	points = append(points, CurvePoint{TimeMillis: int64(len(positions)) * 550, PositionPercent: positions[0]})
	return mustFitCatalog(PatternDefinition{
		ID: PatternClimb, Name: "Climb", Description: "Experimental: a ratcheting build — each stroke reaches further before the release.",
		Kind: PatternKindRoutine, CycleMillis: RoutineCycleFloorMillis, Points: points,
		Tags: []string{TagExperimental, "build", "progressive", "release"},
	})
}

// generateFlutterPattern holds quick shallow mid-span strokes, then opens one
// full sweep before returning to the flutter.
func generateFlutterPattern() PatternDefinition {
	positions := []float64{45, 70, 45, 70, 45, 70, 45, 70, 5, 95}
	points := make([]CurvePoint, 0, len(positions)+1)
	for index, position := range positions {
		points = append(points, CurvePoint{TimeMillis: int64(index) * 550, PositionPercent: position})
	}
	points = append(points, CurvePoint{TimeMillis: int64(len(positions)) * 550, PositionPercent: positions[0]})
	return mustFitCatalog(PatternDefinition{
		ID: PatternFlutter, Name: "Flutter", Description: "Experimental: tight mid-span flutter broken by one full sweep.",
		Kind: PatternKindRoutine, CycleMillis: RoutineCycleFloorMillis, Points: points,
		Tags: []string{TagExperimental, "flutter", "contrast", "tight"},
	})
}

func mustFitCatalog(definition PatternDefinition) PatternDefinition {
	normalized, err := NormalizePatternDefinition(definition)
	if err != nil {
		panic(err)
	}
	for range 6 {
		metrics, measureErr := MeasureCurve(normalized.Points, normalized.CycleMillis, true)
		if measureErr != nil {
			panic(measureErr)
		}
		factor := 1.0
		if metrics.MinReversalGapMillis > 0 && metrics.MinReversalGapMillis < catalogMinReversalGap {
			factor = math.Max(factor, float64(catalogMinReversalGap)/float64(metrics.MinReversalGapMillis))
		}
		if metrics.MaxAccelerationPercentPerSecond2 > catalogMaxAcceleration {
			factor = math.Max(factor, math.Sqrt(metrics.MaxAccelerationPercentPerSecond2/catalogMaxAcceleration))
		}
		if factor <= 1.001 {
			return normalized
		}
		nextDuration := int64(math.Ceil(float64(normalized.CycleMillis) * factor * 1.01))
		normalized.Points = scalePointTimes(normalized.Points, normalized.CycleMillis, nextDuration)
		normalized.CycleMillis = nextDuration
	}
	panic("generated pattern could not satisfy motion budgets")
}

func reversalGap(points []CurvePoint) int64 {
	minimum := int64(0)
	lastReversal := int64(-1)
	previousDirection := 0
	for index := 1; index < len(points); index++ {
		delta := points[index].PositionPercent - points[index-1].PositionPercent
		direction := signFloat(delta)
		if direction == 0 {
			continue
		}
		if previousDirection != 0 && direction != previousDirection {
			at := points[index-1].TimeMillis
			if lastReversal >= 0 && (minimum == 0 || at-lastReversal < minimum) {
				minimum = at - lastReversal
			}
			lastReversal = at
		}
		previousDirection = direction
	}
	return minimum
}

func signFloat(value float64) int {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	default:
		return 0
	}
}

func clonePatternDefinition(definition PatternDefinition) PatternDefinition {
	definition.Points = append([]CurvePoint(nil), definition.Points...)
	definition.Tags = append([]string(nil), definition.Tags...)
	return definition
}

func clampFloat(value, minimum, maximum float64) float64 {
	return math.Max(minimum, math.Min(maximum, value))
}
