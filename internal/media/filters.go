package media

import (
	"math"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	// peakRoundingLegFraction caps each corner against its own shorter leg, so
	// dense fast sections round proportionally less than sparse slow ones
	// without the user managing that. Two adjacent corners can therefore never
	// consume the leg between them.
	peakRoundingLegFraction = 0.4
	// A fillet shorter than this is not worth the knots it costs.
	minimumPeakRoundingMillis = 4
	// Samples inside one fillet, reduced when the point budget is tight.
	defaultPeakRoundingSamples = 3
)

// Filters are opt-in playback transforms over a paired script. The zero value
// plays the script exactly as authored, which is the default everywhere: a
// media amplitude transform shipped once and collapsed subtle strokes toward
// the device's resolution floor (docs/motion-pathway-review-2026-07-20.md,
// 2026-07-22), so a filter that quietly changes motion has to be asked for.
type Filters struct {
	// SmoothingPercent removes extrema whose prominence is below it. Zero is off.
	SmoothingPercent int
	// PeakRoundingMillis rounds each direction change over this window on both
	// sides. Zero is off.
	PeakRoundingMillis int
}

// Effect reports what the active filters actually changed, so the cost is
// visible rather than argued.
type Effect struct {
	// ActionsRemoved counts authored actions dropped by smoothing.
	ActionsRemoved int `json:"actions_removed,omitempty"`
	// PeakReductionPercent is the largest distance any rounded corner moved
	// away from its authored extreme.
	PeakReductionPercent float64 `json:"peak_reduction_percent,omitempty"`
}

func (f Filters) normalized() Filters {
	f.SmoothingPercent = clampInt(f.SmoothingPercent, 0, config.MaxScriptSmoothingPercent)
	f.PeakRoundingMillis = clampInt(f.PeakRoundingMillis, 0, config.MaxPeakRoundingMillis)
	return f
}

func (f Filters) active() bool {
	f = f.normalized()
	return f.SmoothingPercent > 0 || f.PeakRoundingMillis > 0
}

// apply runs the filters in the only order that makes sense: drop the noise
// first, then round what is left. Rounding first would carefully soften spikes
// that smoothing is about to delete.
func (f Filters) apply(points []motion.CurvePoint, maximumPoints int) ([]motion.CurvePoint, Effect) {
	f = f.normalized()
	effect := Effect{}
	if !f.active() || len(points) < 3 {
		return points, effect
	}
	if f.SmoothingPercent > 0 {
		before := len(points)
		points = motion.StabilizePatternReversals(points, float64(f.SmoothingPercent))
		effect.ActionsRemoved = before - len(points)
	}
	if f.PeakRoundingMillis > 0 {
		points, effect.PeakReductionPercent = roundPeaks(points, int64(f.PeakRoundingMillis), maximumPoints)
	}
	return points, effect
}

// roundPeaks replaces each direction-change vertex with a bounded quadratic
// fillet, moving a hand-authored triangle or sawtooth toward a sine.
//
// The straight body of every stroke keeps its authored slope, so peak velocity
// does not rise and acceleration becomes finite where a corner demanded it
// unbounded: this asks less of the device, not more. The cost is peak position,
// because a corner cannot be rounded without cutting it, and that reduction is
// returned so the caller can show it.
//
// Timestamps are never moved. Knots are only inserted between them.
func roundPeaks(points []motion.CurvePoint, windowMillis int64, maximumPoints int) ([]motion.CurvePoint, float64) {
	corners := directionChangeIndexes(points)
	if len(corners) == 0 {
		return points, 0
	}
	samples := peakRoundingSamples(len(points), len(corners), maximumPoints)
	if samples < 1 {
		// Rounding cannot fit inside the point budget. Playing the script
		// unrounded is honest; silently thinning it to fit is not.
		return points, 0
	}

	corner := make(map[int]struct{}, len(corners))
	for _, index := range corners {
		corner[index] = struct{}{}
	}
	result := make([]motion.CurvePoint, 0, len(points)+len(corners)*(samples+1))
	reduction := 0.0
	for index, point := range points {
		if _, ok := corner[index]; !ok {
			result = append(result, point)
			continue
		}
		fillet, moved := filletCorner(points[index-1], point, points[index+1], windowMillis, samples)
		if len(fillet) == 0 {
			result = append(result, point)
			continue
		}
		result = append(result, fillet...)
		reduction = math.Max(reduction, moved)
	}
	return result, reduction
}

// filletCorner returns the knots replacing one vertex, and how far the apex
// moved off the authored extreme.
func filletCorner(
	previous, vertex, next motion.CurvePoint,
	windowMillis int64,
	samples int,
) ([]motion.CurvePoint, float64) {
	leftLeg := vertex.TimeMillis - previous.TimeMillis
	rightLeg := next.TimeMillis - vertex.TimeMillis
	window := min(windowMillis, int64(float64(min(leftLeg, rightLeg))*peakRoundingLegFraction))
	if window < minimumPeakRoundingMillis {
		return nil, 0
	}

	start := motion.CurvePoint{
		TimeMillis:      vertex.TimeMillis - window,
		PositionPercent: interpolate(previous, vertex, vertex.TimeMillis-window),
	}
	end := motion.CurvePoint{
		TimeMillis:      vertex.TimeMillis + window,
		PositionPercent: interpolate(vertex, next, vertex.TimeMillis+window),
	}

	// A quadratic Bezier with the vertex as its control point. Its knots are
	// equally spaced in time because the three control times are, so sampling
	// uniformly in u samples uniformly in time.
	knots := make([]motion.CurvePoint, 0, samples+2)
	knots = append(knots, start)
	for step := 1; step <= samples; step++ {
		u := float64(step) / float64(samples+1)
		position := quadraticBezier(start.PositionPercent, vertex.PositionPercent, end.PositionPercent, u)
		at := start.TimeMillis + int64(math.Round(float64(2*window)*u))
		if at <= knots[len(knots)-1].TimeMillis || at >= end.TimeMillis {
			continue
		}
		knots = append(knots, motion.CurvePoint{TimeMillis: at, PositionPercent: position})
	}
	knots = append(knots, end)

	// Report the closest emitted point to the authored extreme. The old metric
	// selected the knot farthest from the vertex, which is near the fillet edge
	// and overstates how much the actual rounded apex was cut.
	roundedExtreme := knots[0].PositionPercent
	if vertex.PositionPercent > start.PositionPercent && vertex.PositionPercent > end.PositionPercent {
		for _, knot := range knots[1:] {
			roundedExtreme = math.Max(roundedExtreme, knot.PositionPercent)
		}
	} else {
		for _, knot := range knots[1:] {
			roundedExtreme = math.Min(roundedExtreme, knot.PositionPercent)
		}
	}
	return knots, math.Abs(vertex.PositionPercent - roundedExtreme)
}

func quadraticBezier(start, control, end, u float64) float64 {
	inverse := 1 - u
	return inverse*inverse*start + 2*inverse*u*control + u*u*end
}

func interpolate(left, right motion.CurvePoint, at int64) float64 {
	span := right.TimeMillis - left.TimeMillis
	if span <= 0 {
		return right.PositionPercent
	}
	fraction := float64(at-left.TimeMillis) / float64(span)
	return left.PositionPercent + (right.PositionPercent-left.PositionPercent)*fraction
}

func directionChangeIndexes(points []motion.CurvePoint) []int {
	indexes := make([]int, 0, len(points)/2)
	for index := 1; index < len(points)-1; index++ {
		before := sign(points[index].PositionPercent - points[index-1].PositionPercent)
		after := sign(points[index+1].PositionPercent - points[index].PositionPercent)
		if before != 0 && after != 0 && before != after {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

// peakRoundingSamples fits the fillets inside the engine's point cap, spending
// fewer knots per corner on a dense script rather than rounding some corners
// and not others.
func peakRoundingSamples(pointCount, cornerCount, maximumPoints int) int {
	if cornerCount == 0 || maximumPoints <= 0 {
		return 0
	}
	for samples := defaultPeakRoundingSamples; samples >= 1; samples-- {
		if pointCount+cornerCount*(samples+1) <= maximumPoints {
			return samples
		}
	}
	return 0
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

func clampInt(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
