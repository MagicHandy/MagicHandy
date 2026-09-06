package motion

import (
	"math"
	"sort"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

const minimumMediaRoundingMillis int64 = 4

// MediaRoundingEffect describes the compiled, optionally speed-limited curve.
// These are commanded geometry measurements, never physical-device telemetry.
type MediaRoundingEffect struct {
	RoundedCorners       int     `json:"rounded_corners"`
	LimitedCorners       int     `json:"rounding_limited_corners"`
	SkippedCorners       int     `json:"rounding_skipped_corners"`
	PeakReductionPercent float64 `json:"peak_reduction_percent"`
	PeakShiftMillis      float64 `json:"peak_shift_ms"`
}

// mediaFillet uses the same Hermite polynomial evaluator as continuous motion.
// Symmetric windows and the original slopes make its velocity a monotone cubic
// smoothstep between the two incoming/outgoing rates. Acceleration is zero at
// both joins: position, velocity and acceleration all meet the linear body.
type mediaFillet struct {
	start, end int64
	segment    quinticSegment
}

func (c *Curve) roundMediaCorners(windowMillis int) {
	windowMillis = clamp(windowMillis, 0, config.MaxPeakRoundingMillis)
	if windowMillis == 0 || len(c.points) < 3 {
		return
	}
	// Count once to allocate exactly, including dense 100k-action scripts. This
	// avoids growing a large polynomial array repeatedly during a live seek.
	eligible := 0
	for i := 1; i < len(c.points)-1; i++ {
		previous, vertex, next := c.points[i-1], c.points[i], c.points[i+1]
		if (vertex.PositionPercent-previous.PositionPercent)*(next.PositionPercent-vertex.PositionPercent) < 0 &&
			min(vertex.TimeMillis-previous.TimeMillis, next.TimeMillis-vertex.TimeMillis)*2/5 >= minimumMediaRoundingMillis && int64(windowMillis) >= minimumMediaRoundingMillis {
			eligible++
		}
	}
	c.mediaFillets = make([]mediaFillet, 0, eligible)
	knots := make([]CurvePoint, 0, len(c.points)+2*eligible)
	for i, vertex := range c.points {
		if i == 0 || i == len(c.points)-1 {
			knots = append(knots, vertex)
			continue
		}
		previous, next := c.points[i-1], c.points[i+1]
		before, after := vertex.PositionPercent-previous.PositionPercent, next.PositionPercent-vertex.PositionPercent
		if before*after >= 0 {
			knots = append(knots, vertex)
			continue
		}
		// Two adjacent windows leave at least 20% of their shared leg exact.
		window := min(int64(windowMillis), (min(vertex.TimeMillis-previous.TimeMillis, next.TimeMillis-vertex.TimeMillis)*2)/5)
		if window < minimumMediaRoundingMillis {
			c.mediaRounding.SkippedCorners++
			knots = append(knots, vertex)
			continue
		}
		if window < int64(windowMillis) {
			c.mediaRounding.LimitedCorners++
		}
		leftRate := before / float64(vertex.TimeMillis-previous.TimeMillis)
		rightRate := after / float64(next.TimeMillis-vertex.TimeMillis)
		left := CurvePoint{TimeMillis: vertex.TimeMillis - window, PositionPercent: vertex.PositionPercent - float64(window)*leftRate}
		right := CurvePoint{TimeMillis: vertex.TimeMillis + window, PositionPercent: vertex.PositionPercent + float64(window)*rightRate}
		segment := newQuinticSegment(left, right, leftRate, rightRate, 0, 0)
		c.mediaFillets = append(c.mediaFillets, mediaFillet{start: left.TimeMillis, end: right.TimeMillis, segment: segment})
		// Invert cubic smoothstep at the unique zero of velocity. The closed
		// form avoids iterative root searches for every corner during a seek.
		// Asymmetric slopes move the apex within the window; report that cost.
		fraction := leftRate / (leftRate - rightRate)
		apex := 0.5 - math.Sin(math.Asin(1-2*fraction)/3)
		apexTime := float64(left.TimeMillis) + float64(2*window)*apex
		apexPosition := segment.position(apex)
		c.mediaRounding.RoundedCorners++
		c.mediaRounding.PeakReductionPercent = math.Max(c.mediaRounding.PeakReductionPercent, math.Abs(vertex.PositionPercent-apexPosition))
		c.mediaRounding.PeakShiftMillis = math.Max(c.mediaRounding.PeakShiftMillis, math.Abs(float64(vertex.TimeMillis)-apexTime))
		knots = append(knots, left)
		if at := int64(math.Round(apexTime)); at > left.TimeMillis && at < right.TimeMillis {
			knots = append(knots, CurvePoint{TimeMillis: at, PositionPercent: apexPosition})
		}
		knots = append(knots, right)
	}
	// Mandatory fitter landmarks describe the compiled curve, not the removed
	// sharp vertex. Source points remain bounded and unchanged in the target.
	c.authoredKnots = knots
	c.minPosition, c.maxPosition = curvePointBounds(knots)
}

func (c Curve) mediaFilletAt(at float64) (quinticSegment, float64, bool) {
	if len(c.mediaFillets) == 0 {
		return quinticSegment{}, 0, false
	}
	i := sort.Search(len(c.mediaFillets), func(i int) bool { return float64(c.mediaFillets[i].end) >= at })
	if i == len(c.mediaFillets) || at < float64(c.mediaFillets[i].start) {
		return quinticSegment{}, 0, false
	}
	fillet := c.mediaFillets[i]
	return fillet.segment, (at - float64(fillet.start)) / float64(fillet.end-fillet.start), true
}
