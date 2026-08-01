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

	minimumBurstCycleMillis = 500
	maximumCurvePoints      = 4096
	// MaximumMediaTimelinePoints keeps feature-length funscripts bounded while
	// leaving normal pattern and program authoring on the smaller content cap.
	MaximumMediaTimelinePoints = 100_000
	catalogMinReversalGap      = 450
	catalogMaxAcceleration     = 3000.0
	catalogMinStrokeAmplitude  = 22.0
	catalogMinStrokeVelocity   = 42.0
	catalogMaxSpeedRatio       = 3.3
	catalogMinMeanVelocity     = 55.0
	catalogMaxStallMillis      = int64(200)
	catalogStallVelocity       = 30.0

	// TagExperimental marks catalog content that is fully playable in the
	// library but exposed to the model only behind the user's
	// experimental-patterns capability gate.
	TagExperimental = "experimental"
	// TagCurated marks exact user-tested curves promoted into the built-in catalog.
	TagCurated = "curated"
)

const catalogSampleMillis int64 = 25

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

// MediaTimelineDefinition is finite, clock-locked motion authored against a
// media file. It is deliberately separate from ProgramDefinition: feature-
// length funscripts need a larger bound and linear interpolation, and they are
// never persisted in the pattern library.
type MediaTimelineDefinition struct {
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

type catalogFeelMetrics struct {
	MinimumAmplitude  float64
	MinimumVelocity   float64
	MaximumVelocity   float64
	MeanVelocity      float64
	LongestStallMilli int64
}

func (metrics catalogFeelMetrics) acceptable() bool {
	return metrics.MinimumAmplitude >= catalogMinStrokeAmplitude &&
		metrics.MinimumVelocity >= catalogMinStrokeVelocity &&
		metrics.MaximumVelocity/metrics.MinimumVelocity <= catalogMaxSpeedRatio &&
		metrics.MeanVelocity >= catalogMinMeanVelocity &&
		metrics.LongestStallMilli <= catalogMaxStallMillis
}

// Curve is a validated time-parameterized sampler. Pattern curves use
// monotone cubic interpolation; media timelines preserve linear segments.
type Curve struct {
	points        []CurvePoint
	authoredKnots []CurvePoint
	slopes        []float64
	duration      int64
	loop          bool
	linear        bool
	// minPosition and maxPosition bound the authored span. Shape-preserving
	// interpolation never overshoots a knot, so the authored extremes are the
	// curve's extremes.
	minPosition float64
	maxPosition float64
}

var builtinPatternCatalog = buildBuiltinPatternCatalog()

func buildBuiltinPatternCatalog() []PatternDefinition {
	definitions := []PatternDefinition{
		generateStrokePattern(),
		generatePulsePattern(),
		generateTeasePattern(),
	}
	definitions = append(definitions, generateCatalogPatterns()...)
	definitions = append(definitions, PromotedBuiltinPatternDefinitions()...)
	definitions = append(definitions, loadCuratedBuiltinPatterns()...)
	return definitions
}

// NewCurve validates points and builds PCHIP-style wall-time derivatives.
// Authoring, preview, and budget measurement use unscaled playback; only the
// engine plan knows the speed and focus a curve will actually be played at.
func NewCurve(points []CurvePoint, durationMillis int64, loop bool) (Curve, error) {
	return newCurve(points, durationMillis, loop, false, maximumCurvePoints, neutralPlaybackScale())
}

func newCurve(
	points []CurvePoint,
	durationMillis int64,
	loop bool,
	linear bool,
	maximumPoints int,
	scale playbackScale,
) (Curve, error) {
	if len(points) < 2 {
		return Curve{}, errors.New("a motion curve requires at least two points")
	}
	if len(points) > maximumPoints {
		return Curve{}, fmt.Errorf("a motion curve supports at most %d points", maximumPoints)
	}
	copyPoints := append([]CurvePoint(nil), points...)
	if err := validateCurvePoints(copyPoints, durationMillis); err != nil {
		return Curve{}, err
	}
	curvePoints := copyPoints
	var guideSlopes map[int64]float64
	if loop && !linear {
		curvePoints, guideSlopes = withBoundedLoopReversalGuides(copyPoints, scale)
	}
	minimum, maximum := curvePointBounds(copyPoints)
	curve := Curve{
		points:        curvePoints,
		authoredKnots: copyPoints,
		duration:      durationMillis,
		loop:          loop,
		linear:        linear,
		minPosition:   minimum,
		maxPosition:   maximum,
	}
	if !linear {
		curve.slopes = monotoneSlopes(curvePoints, loop)
		for index, point := range curvePoints {
			if slope, ok := guideSlopes[point.TimeMillis]; ok {
				curve.slopes[index] = slope
			}
		}
	}
	return curve, nil
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

// UsesExactImportedCurve marks the small explicit allowlist of hardware-accepted
// timing exceptions. A bulk-imported filename is never sufficient evidence.
func UsesExactImportedCurve(definition PatternDefinition) bool {
	if !slices.Contains(definition.Tags, TagCurated) {
		return false
	}
	return definition.ID == PatternHardAndRegular || definition.ID == PatternPlayfulJerk
}

// BuiltinPatternDefinitions returns the parametrically generated catalog.
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
	points = StabilizePatternReversals(points, MinimumPatternReversalProminence)
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

// NormalizeMediaTimelineDefinition validates a bounded feature-length media
// curve without applying pattern-library normalization or loop closure.
func NormalizeMediaTimelineDefinition(definition MediaTimelineDefinition) (MediaTimelineDefinition, error) {
	definition.ID = strings.TrimSpace(definition.ID)
	definition.Name = strings.TrimSpace(definition.Name)
	if definition.ID == "" || definition.Name == "" {
		return MediaTimelineDefinition{}, errors.New("media timeline id and name are required")
	}
	points, duration, err := normalizePointsWithLimit(
		definition.Points,
		definition.DurationMillis,
		false,
		MaximumMediaTimelinePoints,
	)
	if err != nil {
		return MediaTimelineDefinition{}, err
	}
	definition.DurationMillis = duration
	definition.Points = points
	if _, err := newCurve(points, duration, false, true, MaximumMediaTimelinePoints, neutralPlaybackScale()); err != nil {
		return MediaTimelineDefinition{}, err
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

func exceedsCatalogSafetyBudgets(metrics CurveMetrics) bool {
	return metrics.MaxAccelerationPercentPerSecond2 > catalogMaxAcceleration ||
		(metrics.MinReversalGapMillis > 0 && metrics.MinReversalGapMillis < catalogMinReversalGap)
}

func measureCatalogPatternFeel(definition PatternDefinition) (catalogFeelMetrics, error) {
	curve, err := NewCurve(definition.Points, definition.CycleMillis, true)
	if err != nil {
		return catalogFeelMetrics{}, err
	}
	metrics := catalogFeelMetrics{
		MinimumAmplitude: math.Inf(1),
		MinimumVelocity:  math.Inf(1),
	}
	travel := 0.0
	for index := 1; index < len(definition.Points); index++ {
		previous, point := definition.Points[index-1], definition.Points[index]
		amplitude := math.Abs(point.PositionPercent - previous.PositionPercent)
		millis := point.TimeMillis - previous.TimeMillis
		if amplitude == 0 || millis <= 0 {
			continue
		}
		velocity := amplitude / (float64(millis) / 1000)
		metrics.MinimumAmplitude = math.Min(metrics.MinimumAmplitude, amplitude)
		metrics.MinimumVelocity = math.Min(metrics.MinimumVelocity, velocity)
		metrics.MaximumVelocity = math.Max(metrics.MaximumVelocity, velocity)
		travel += amplitude
	}
	if math.IsInf(metrics.MinimumAmplitude, 1) {
		metrics.MinimumAmplitude = 0
		metrics.MinimumVelocity = 0
	}
	if definition.CycleMillis > 0 {
		metrics.MeanVelocity = travel / (float64(definition.CycleMillis) / 1000)
	}
	run := int64(0)
	for at := int64(0); at < definition.CycleMillis; at += catalogSampleMillis {
		if math.Abs(curve.Velocity(at)) < catalogStallVelocity {
			run += catalogSampleMillis
			metrics.LongestStallMilli = max(metrics.LongestStallMilli, run)
			continue
		}
		run = 0
	}
	return metrics, nil
}

func curvePointBounds(points []CurvePoint) (float64, float64) {
	minimum := points[0].PositionPercent
	maximum := points[0].PositionPercent
	for _, point := range points[1:] {
		minimum = math.Min(minimum, point.PositionPercent)
		maximum = math.Max(maximum, point.PositionPercent)
	}
	return minimum, maximum
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
	if c.linear {
		return y0 + (y1-y0)*u
	}
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
		if c.linear {
			return 0
		}
		return c.slopes[left]
	}
	h := float64(c.points[right].TimeMillis - c.points[left].TimeMillis)
	if c.linear {
		return (c.points[right].PositionPercent - c.points[left].PositionPercent) / h
	}
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
		slopes[index] = interiorSlope(h[index-1], h[index], delta[index-1], delta[index])
	}
	if loop {
		seam := interiorSlope(
			h[count-2], h[0], delta[count-2], delta[0],
		)
		slopes[0] = seam
		slopes[count-1] = seam
	} else if count > 2 {
		slopes[0] = endpointSlope(h[0], h[1], delta[0], delta[1])
		slopes[count-1] = endpointSlope(h[count-2], h[count-3], delta[count-2], delta[count-3])
	}
	return slopes
}

func interiorSlope(previousWidth, nextWidth, previousDelta, nextDelta float64) float64 {
	if previousDelta*nextDelta <= 0 {
		return 0
	}
	w1 := 2*nextWidth + previousWidth
	w2 := nextWidth + 2*previousWidth
	return (w1 + w2) / (w1/previousDelta + w2/nextDelta)
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
	return normalizePointsWithLimit(raw, requestedDuration, closeLoop, maximumCurvePoints)
}

func normalizePointsWithLimit(raw []CurvePoint, requestedDuration int64, closeLoop bool, maximumPoints int) ([]CurvePoint, int64, error) {
	if len(raw) < 2 || len(raw) > maximumPoints {
		return nil, 0, fmt.Errorf("motion content requires 2..%d points", maximumPoints)
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

type catalogPatternSpec struct {
	ID           PatternID
	Name         string
	Description  string
	Positions    []float64
	TravelMillis []int64
	Tags         []string
	Experimental bool
}

// catalogPatternSpecs are deliberately selected complete cycles. None is
// a random excerpt; every final travel interval closes the shape back onto its
// first point before the hardware-budget pass runs.
//
// Travel times for the velocity-authored entries are derived, not chosen: each
// is amplitude divided by an intended stroke speed. Authoring positions and
// times as unrelated lists is what let the retired catalog put a 14-unit stroke
// and an 84-unit stroke on the same 760ms, giving 18%/s next to 116%/s inside
// one pattern. Those entries also reach the cycle floor by repeating their
// phrase rather than by letting mustFitCatalog stretch every timestamp, which
// would divide every velocity by the same factor.
var catalogPatternSpecs = []catalogPatternSpec{
	{
		ID: PatternFlutter, Name: "Flutter", Description: "Tight mid-span strokes open into one full sweep.",
		Positions:    []float64{45, 70, 45, 70, 45, 70, 45, 70, 5, 95},
		TravelMillis: []int64{500, 500, 500, 500, 500, 500, 500, 850, 900, 850},
		Tags:         []string{"flutter", "contrast", "tight"},
	},
	{
		ID: PatternDrift, Name: "Drift", Description: "A steady-width stroke migrates upward and returns.",
		Positions:    []float64{15, 45, 22, 55, 30, 65, 40, 78, 48, 82, 38, 68, 28, 55},
		TravelMillis: []int64{520, 520, 540, 540, 560, 560, 580, 580, 560, 560, 540, 540, 520, 580},
		Tags:         []string{"migrating", "progressive", "smooth"},
	},
	{
		ID: PatternEasingDown, Name: "Easing Down",
		Description:  "Steps steadily lower without losing pace. For winding down from a peak.",
		Positions:    []float64{100, 56, 86, 42, 72, 28, 58, 14, 100, 56, 86, 42, 72, 28, 58, 14},
		TravelMillis: []int64{518, 484, 518, 484, 518, 484, 518, 717, 518, 484, 518, 484, 518, 484, 518, 717},
		Tags:         []string{"easing", "descending", "calming"},
	},
	{
		ID: PatternBuildingUp, Name: "Building Up",
		Description:  "Climbs step by step, then one full sweep resets it. For building intensity.",
		Positions:    []float64{0, 44, 14, 58, 28, 72, 42, 86, 0, 44, 14, 58, 28, 72, 42, 86},
		TravelMillis: []int64{518, 484, 518, 484, 518, 484, 518, 717, 518, 484, 518, 484, 518, 484, 518, 717},
		Tags:         []string{"building", "ascending", "progressive"},
	},
	{
		ID: PatternBroadAndTight, Name: "Broad and Tight",
		Description:  "One wide sweep, then a run of tight centered strokes. Playful contrast.",
		Positions:    []float64{6, 94, 34, 66, 34, 66, 6, 94, 34, 66, 34, 66},
		TravelMillis: []int64{733, 500, 516, 516, 516, 968, 733, 500, 516, 516, 516, 968},
		Tags:         []string{"contrast", "paired", "centered"},
	},
	{
		ID: PatternUpperAccents, Name: "Upper Accents",
		Description:  "Quick accents up top, answered by one broad sweep.",
		Positions:    []float64{8, 96, 62, 96, 62, 96, 8, 96, 62, 96, 62, 96},
		TravelMillis: []int64{721, 486, 486, 486, 486, 721, 721, 486, 486, 486, 486, 721},
		Tags:         []string{"upper", "accent", "teasing"},
	},
	{
		ID: PatternLowerAccents, Name: "Lower Accents",
		Description:  "Quick accents down low, answered by one broad sweep.",
		Positions:    []float64{92, 4, 38, 4, 38, 4, 92, 4, 38, 4, 38, 4},
		TravelMillis: []int64{721, 486, 486, 486, 486, 721, 721, 486, 486, 486, 486, 721},
		Tags:         []string{"lower", "accent", "deep"},
	},
	{
		ID: PatternSteadyDrift, Name: "Steady Drift",
		Description:  "One unchanging pace while the window wanders up and back. Calm baseline.",
		Positions:    []float64{10, 52, 20, 62, 30, 72, 10, 52, 20, 62, 30, 72},
		TravelMillis: []int64{600, 457, 600, 457, 600, 886, 600, 457, 600, 457, 600, 886},
		Tags:         []string{"steady", "migrating", "calm"},
	},
	{
		ID: PatternNarrowing, Name: "Narrowing",
		Description:  "Strokes close toward the center and the pace eases with them.",
		Positions:    []float64{15, 85, 21, 79, 28, 72, 35, 65, 15, 85, 21, 79, 28, 72, 35, 65},
		TravelMillis: []int64{522, 525, 518, 520, 512, 514, 484, 510, 522, 525, 518, 520, 512, 514, 484, 510},
		Tags:         []string{"narrowing", "easing", "centered"},
	},
	{
		ID: PatternOpeningUp, Name: "Opening Up",
		Description:  "Strokes widen out from the center and pick up pace.",
		Positions:    []float64{35, 65, 28, 72, 21, 79, 15, 85, 35, 65, 28, 72, 21, 79, 15, 85},
		TravelMillis: []int64{484, 514, 512, 520, 518, 525, 522, 510, 484, 514, 512, 520, 518, 525, 522, 510},
		Tags:         []string{"widening", "building", "centered"},
	},
	{
		ID: PatternRocking, Name: "Rocking",
		Description:  "Even mid-range strokes at one steady pace. A plain rhythm to settle into.",
		Positions:    []float64{25, 75, 25, 75, 25, 75, 25, 75, 25, 75, 25, 75},
		TravelMillis: []int64{556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 556},
		Tags:         []string{"steady", "even", "centered"},
	},
	{
		ID: PatternThreeAndOne, Name: "Three and One",
		Description:  "Three tight strokes up high, then one full plunge and recovery.",
		Positions:    []float64{95, 65, 95, 65, 95, 65, 95, 5, 95, 65, 95, 65, 95, 65, 95, 5},
		TravelMillis: []int64{484, 484, 484, 484, 484, 484, 738, 738, 484, 484, 484, 484, 484, 484, 738, 738},
		Tags:         []string{"grouped", "resolving", "upper"},
	},
	{
		ID: PatternOffbeat, Name: "Offbeat",
		Description:  "Even strokes broken by one deeper reach off the beat. Stays unpredictable.",
		Positions:    []float64{16, 64, 16, 64, 16, 92, 16, 64, 16, 64, 16, 92},
		TravelMillis: []int64{667, 667, 667, 667, 1056, 623, 667, 667, 667, 667, 1056, 623},
		Tags:         []string{"syncopated", "accent", "varied"}, Experimental: true,
	},
	{
		ID: PatternLongReturn, Name: "Long Return",
		Description:  "A quick reach answered by an unhurried return. Leaning, asymmetric.",
		Positions:    []float64{10, 78, 10, 78, 10, 78, 10, 78},
		TravelMillis: []int64{567, 1097, 567, 1097, 567, 1097, 567, 1097},
		Tags:         []string{"asymmetric", "leaning", "paired"}, Experimental: true,
	},
	{
		ID: PatternSwell, Name: "Swell",
		Description:  "The window rises across the cycle and settles back. One arc, not a beat.",
		Positions:    []float64{5, 45, 15, 55, 25, 65, 35, 75, 25, 65, 15, 55, 5, 45, 15, 55, 25, 65, 35, 75, 25, 65, 15, 55},
		TravelMillis: []int64{500, 484, 500, 484, 500, 484, 500, 510, 500, 510, 500, 510, 500, 484, 500, 484, 500, 484, 500, 510, 500, 510, 500, 510},
		Tags:         []string{"arc", "migrating", "long"}, Experimental: true,
	},
	{
		ID: PatternSurgeAndSettle, Name: "Surge and Settle",
		Description:  "One fast full sweep, then a long settled run of mid strokes.",
		Positions:    []float64{2, 98, 35, 68, 35, 68, 35, 68, 35, 68, 35, 68, 2, 98, 35, 68, 35, 68, 35, 68, 35, 68, 35, 68},
		TravelMillis: []int64{686, 573, 485, 485, 485, 485, 485, 485, 485, 485, 485, 589, 686, 573, 485, 485, 485, 485, 485, 485, 485, 485, 485, 589},
		Tags:         []string{"accent", "recovery", "long"}, Experimental: true,
	},
	{
		ID: PatternCrosscut, Name: "Crosscut",
		Description:  "Blocks of broad strokes trade with blocks of tight ones. Restless but on a beat.",
		Positions:    []float64{8, 88, 8, 88, 8, 88, 55, 85, 55, 85, 55, 85},
		TravelMillis: []int64{640, 640, 640, 640, 640, 485, 484, 484, 484, 484, 484, 631},
		Tags:         []string{"alternating", "blocks", "restless"}, Experimental: true,
	},
	{
		ID: PatternFourLevelCircuit, Name: "Four-Level Circuit", Description: "Full and partial strokes rotate through both halves of the range.",
		Positions:    []float64{99, 0, 25, 0, 99, 74, 99, 0, 25, 0},
		TravelMillis: []int64{942, 472, 471, 943, 472, 471, 943, 471, 472, 943},
		Tags:         []string{"multi-level", "alternating", "full"},
	},
	{
		ID: PatternHighLowBlocks, Name: "High-Low Blocks", Description: "Upper-zone pulses switch to lower-zone pulses between full sweeps.",
		Positions:    []float64{0, 99, 70, 99, 70, 99, 70, 99, 0, 30, 0, 30},
		TravelMillis: []int64{942, 472, 472, 471, 472, 472, 472, 942, 472, 471, 472, 470},
		Tags:         []string{"zones", "grouped", "contrast"},
	},
	{
		ID: PatternDeepShallowSequence, Name: "Deep-Shallow Sequence", Description: "Upper returns move between medium and full-depth strokes in an uneven phrase.",
		Positions:    []float64{99, 63, 99, 0, 99, 63, 99, 0, 99, 63},
		TravelMillis: []int64{446, 446, 1337, 625, 446, 446, 1337, 625, 446, 446},
		Tags:         []string{"uneven", "deep", "upper-return"},
	},
	{
		ID: PatternSlowFastFull, Name: "Slow-to-Fast Full", Description: "Two measured full strokes transition into a run of faster full strokes.",
		Positions:    []float64{0, 100, 0, 100, 0, 100, 0, 100, 0, 100},
		TravelMillis: []int64{702, 1170, 701, 1204, 471, 470, 470, 470, 471, 471},
		Tags:         []string{"full", "tempo-change", "accelerating"},
	},
	{
		ID: PatternDeepPartialSequence, Name: "Deep-Partial Sequence", Description: "Lower returns mix full-depth and partial-depth strokes with uneven accents.",
		Positions:    []float64{0, 100, 0, 80, 0, 100, 0, 100, 0, 80},
		TravelMillis: []int64{462, 462, 462, 463, 1418, 496, 1452, 453, 471, 461},
		Tags:         []string{"uneven", "deep", "partial"},
	},
	{
		ID: PatternRisingReach, Name: "Rising Reach", Description: "Alternating returns reach progressively higher before a full release.",
		Positions:    []float64{10, 50, 0, 60, 20, 60, 10, 70, 0, 90},
		TravelMillis: []int64{501, 467, 501, 467, 467, 467, 500, 501, 1068, 834},
		Tags:         []string{"progressive", "ascending", "varied"}, Experimental: true,
	},
}

func generateCatalogPatterns() []PatternDefinition {
	definitions := make([]PatternDefinition, 0, len(catalogPatternSpecs))
	for _, spec := range catalogPatternSpecs {
		definitions = append(definitions, generateCatalogPattern(spec))
	}
	return definitions
}

func generateCatalogPattern(spec catalogPatternSpec) PatternDefinition {
	if len(spec.Positions) < 2 || len(spec.TravelMillis) != len(spec.Positions) {
		panic("catalog pattern requires one closing travel interval per position")
	}
	points := make([]CurvePoint, 0, len(spec.Positions)+1)
	points = append(points, CurvePoint{PositionPercent: spec.Positions[0]})
	elapsed := int64(0)
	for index, travelMillis := range spec.TravelMillis {
		if travelMillis <= 0 {
			panic("catalog pattern travel time must be positive")
		}
		elapsed += travelMillis
		next := spec.Positions[(index+1)%len(spec.Positions)]
		points = append(points, CurvePoint{TimeMillis: elapsed, PositionPercent: next})
	}
	description := spec.Description
	tags := append([]string(nil), spec.Tags...)
	if spec.Experimental {
		description = "Experimental: " + description
		tags = append([]string{TagExperimental}, tags...)
	}
	return mustFitCatalog(PatternDefinition{
		ID: spec.ID, Name: spec.Name, Description: description,
		Kind: PatternKindRoutine, CycleMillis: elapsed, Points: points, Tags: tags,
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
		if factor <= 1.0001 {
			return normalized
		}
		nextDuration := int64(math.Ceil(float64(normalized.CycleMillis) * factor * 1.01))
		normalized.Points = scalePointTimes(normalized.Points, normalized.CycleMillis, nextDuration)
		normalized.CycleMillis = nextDuration
	}
	panic(fmt.Sprintf("pattern %q could not satisfy motion budgets", definition.ID))
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
