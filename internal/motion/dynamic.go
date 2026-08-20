package motion

import (
	"fmt"
	"math"
	"strings"
)

const (
	// DynamicMotionName is the user-facing content label for model-authored loops.
	DynamicMotionName = "Dynamic"
	// MinimumDynamicSpanPercent prevents narrow model targets from collapsing
	// into whole-percent device stalls at slow speeds.
	MinimumDynamicSpanPercent = 20
	defaultDynamicCenter      = 50
	defaultDynamicSpan        = 70
	defaultDynamicSegment     = 12
)

// DynamicAnchor is one semantic pass-through target in a model-authored loop.
type DynamicAnchor struct {
	Name            string `json:"name"`
	PositionPercent int    `json:"position_percent"`
}

// DynamicDefinition is bounded geometry authored by the LLM and compiled by
// the backend into ordinary MotionPlan content. It is not a transport payload.
type DynamicDefinition struct {
	CenterPercent    int             `json:"center_percent"`
	SpanPercent      int             `json:"span_percent"`
	Anchors          []DynamicAnchor `json:"anchors,omitempty"`
	VariationPercent int             `json:"variation_percent"`
	SegmentSeconds   int             `json:"segment_seconds"`
}

// NormalizeDynamicDefinition returns concrete, bounded geometry suitable for
// engine sampling. Named anchors take precedence over center/span geometry.
func NormalizeDynamicDefinition(definition DynamicDefinition) DynamicDefinition {
	definition.VariationPercent = clamp(definition.VariationPercent, 0, 100)
	if definition.SegmentSeconds == 0 {
		definition.SegmentSeconds = defaultDynamicSegment
	}
	definition.SegmentSeconds = clamp(definition.SegmentSeconds, 4, 120)
	definition.Anchors = normalizeDynamicAnchors(definition.Anchors)
	if len(definition.Anchors) >= 2 {
		minimum, maximum := dynamicAnchorBounds(definition.Anchors)
		definition.CenterPercent = (minimum + maximum) / 2
		definition.SpanPercent = maximum - minimum
		return definition
	}

	if definition.CenterPercent == 0 && definition.SpanPercent == 0 {
		definition.CenterPercent = defaultDynamicCenter
		definition.SpanPercent = defaultDynamicSpan
	}
	definition.CenterPercent = clamp(definition.CenterPercent, 0, 100)
	definition.SpanPercent = clamp(definition.SpanPercent, MinimumDynamicSpanPercent, 100)
	minimum, maximum := dynamicWindow(definition.CenterPercent, definition.SpanPercent)
	definition.CenterPercent = (minimum + maximum) / 2
	definition.SpanPercent = maximum - minimum
	return definition
}

func normalizeDynamicAnchors(anchors []DynamicAnchor) []DynamicAnchor {
	if len(anchors) > 6 {
		anchors = anchors[:6]
	}
	result := make([]DynamicAnchor, 0, len(anchors))
	for _, anchor := range anchors {
		anchor.Name = strings.ToLower(strings.TrimSpace(anchor.Name))
		anchor.PositionPercent = clamp(anchor.PositionPercent, 0, 100)
		if len(result) > 0 && result[len(result)-1].PositionPercent == anchor.PositionPercent {
			continue
		}
		result = append(result, anchor)
	}
	if len(result) < 2 {
		return nil
	}
	minimum, maximum := dynamicAnchorBounds(result)
	if maximum-minimum < MinimumDynamicSpanPercent {
		return nil
	}
	return result
}

func dynamicAnchorBounds(anchors []DynamicAnchor) (int, int) {
	minimum, maximum := 100, 0
	for _, anchor := range anchors {
		minimum = min(minimum, anchor.PositionPercent)
		maximum = max(maximum, anchor.PositionPercent)
	}
	return minimum, maximum
}

func dynamicWindow(center, span int) (int, int) {
	minimum := center - span/2
	maximum := minimum + span
	if minimum < 0 {
		maximum -= minimum
		minimum = 0
	}
	if maximum > 100 {
		minimum -= maximum - 100
		maximum = 100
	}
	return clamp(minimum, 0, 100), clamp(maximum, 0, 100)
}

func dynamicContent(definition DynamicDefinition) resolvedContent {
	definition = NormalizeDynamicDefinition(definition)
	base := dynamicBasePositions(definition)
	cycles := 1
	if definition.VariationPercent > 0 {
		cycles = 4
	}
	indices := dynamicTraversalIndices(len(base))
	points := make([]CurvePoint, 0, cycles*len(indices)+1)
	var elapsed int64
	for cycle := range cycles {
		phase := float64(cycle) / float64(cycles)
		for _, index := range indices {
			position := dynamicVariedPosition(base[index], base, definition.VariationPercent, phase)
			if len(points) > 0 {
				elapsed += dynamicLegMillis(points[len(points)-1].PositionPercent, position)
			}
			points = append(points, CurvePoint{TimeMillis: elapsed, PositionPercent: position})
		}
	}
	closing := dynamicVariedPosition(base[0], base, definition.VariationPercent, 1)
	elapsed += dynamicLegMillis(points[len(points)-1].PositionPercent, closing)
	points = append(points, CurvePoint{TimeMillis: elapsed, PositionPercent: closing})
	return resolvedContent{points: points, duration: elapsed, loop: true, maximumPoints: maximumCurvePoints}
}

func dynamicBasePositions(definition DynamicDefinition) []float64 {
	if len(definition.Anchors) >= 2 {
		positions := make([]float64, len(definition.Anchors))
		for index, anchor := range definition.Anchors {
			positions[index] = float64(anchor.PositionPercent)
		}
		return positions
	}
	minimum, maximum := dynamicWindow(definition.CenterPercent, definition.SpanPercent)
	return []float64{float64(minimum), float64(maximum)}
}

// dynamicTraversalIndices visits interior anchors in both directions without
// duplicating either reversal. PCHIP therefore gives interior anchors a
// pass-through tangent while only the true endpoints reach zero velocity.
func dynamicTraversalIndices(count int) []int {
	indices := make([]int, 0, max(2, count*2-2))
	for index := range count {
		indices = append(indices, index)
	}
	for index := count - 2; index > 0; index-- {
		indices = append(indices, index)
	}
	return indices
}

func dynamicVariedPosition(position float64, base []float64, variation int, phase float64) float64 {
	if variation <= 0 {
		return position
	}
	minimum, maximum := base[0], base[0]
	for _, candidate := range base[1:] {
		minimum = math.Min(minimum, candidate)
		maximum = math.Max(maximum, candidate)
	}
	center := (minimum + maximum) / 2
	amount := float64(variation) / 100
	drift := 10 * amount * math.Sin(2*math.Pi*phase)
	scale := 1 + 0.12*amount*math.Cos(2*math.Pi*phase)
	return math.Max(0, math.Min(100, center+drift+(position-center)*scale))
}

func dynamicLegMillis(left, right float64) int64 {
	return max(int64(180), int64(math.Round(math.Abs(right-left)*10)))
}

func dynamicContentID(definition DynamicDefinition) string {
	definition = NormalizeDynamicDefinition(definition)
	var builder strings.Builder
	_, _ = fmt.Fprintf(&builder, "dynamic:%d:%d:%d", definition.CenterPercent,
		definition.SpanPercent, definition.VariationPercent)
	for _, anchor := range definition.Anchors {
		_, _ = fmt.Fprintf(&builder, ":%s=%d", anchor.Name, anchor.PositionPercent)
	}
	return builder.String()
}
