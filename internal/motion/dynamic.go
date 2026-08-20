package motion

import (
	"fmt"
	"math"
	"strings"
)

const (
	// DynamicMotionName is the user-facing content label for model-authored loops.
	DynamicMotionName = "Creative"
	// MinimumDynamicSpanPercent prevents narrow model targets from collapsing
	// into whole-percent device stalls at slow speeds.
	MinimumDynamicSpanPercent = 20
	defaultDynamicCenter      = 50
	defaultDynamicSpan        = 70
	defaultDynamicSegment     = 12
	// Spatial and timing variation must establish a slow texture rather than a
	// short motif that repeats every few strokes. The cycle count adapts to the
	// route's travel so even the minimum 20% span takes roughly this long to
	// repeat at the calibrated maximum reference speed.
	minimumDynamicVariationLoopSeconds = 8.0
	minimumDynamicVariationCycles      = 12
	maximumDynamicVariationCycles      = 72
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
		cycles = dynamicVariationCycleCount(base, definition.VariationPercent)
	}
	indices := dynamicTraversalIndices(len(base))
	points := make([]CurvePoint, 0, cycles*len(indices)+1)
	totalLegs := cycles * len(indices)
	var elapsed int64
	for cycle := range cycles {
		phase := float64(cycle) / float64(cycles)
		for _, index := range indices {
			position := dynamicVariedPosition(base[index], base, definition.VariationPercent, phase)
			if len(points) > 0 {
				legPhase := (float64(len(points)-1) + 0.5) / float64(totalLegs)
				elapsed += dynamicLegMillis(
					points[len(points)-1].PositionPercent, position,
					definition.VariationPercent, legPhase,
				)
			}
			points = append(points, CurvePoint{TimeMillis: elapsed, PositionPercent: position})
		}
	}
	closing := dynamicVariedPosition(base[0], base, definition.VariationPercent, 1)
	closingPhase := (float64(totalLegs) - 0.5) / float64(totalLegs)
	elapsed += dynamicLegMillis(
		points[len(points)-1].PositionPercent, closing,
		definition.VariationPercent, closingPhase,
	)
	points = append(points, CurvePoint{TimeMillis: elapsed, PositionPercent: closing})
	return resolvedContent{points: points, duration: elapsed, loop: true, maximumPoints: maximumCurvePoints}
}

func dynamicVariationCycleCount(base []float64, variation int) int {
	indices := dynamicTraversalIndices(len(base))
	if len(indices) < 2 {
		return minimumDynamicVariationCycles
	}
	targetTravel := minimumDynamicVariationLoopSeconds * maximumSupportedReferenceTravelRatePercentPerSecond
	for cycles := minimumDynamicVariationCycles; cycles <= maximumDynamicVariationCycles; cycles++ {
		if dynamicVariationTravel(base, indices, variation, cycles) >= targetTravel {
			return cycles
		}
	}
	return maximumDynamicVariationCycles
}

func dynamicVariationTravel(base []float64, indices []int, variation, cycles int) float64 {
	travel := 0.0
	previous := 0.0
	hasPrevious := false
	for cycle := range cycles {
		phase := float64(cycle) / float64(cycles)
		for _, index := range indices {
			position := dynamicVariedPosition(base[index], base, variation, phase)
			if hasPrevious {
				travel += math.Abs(position - previous)
			}
			previous = position
			hasPrevious = true
		}
	}
	closing := dynamicVariedPosition(base[0], base, variation, 1)
	return travel + math.Abs(closing-previous)
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
	span := maximum - minimum
	if span <= 0 {
		return position
	}
	amount := float64(variation) / 100
	// A small harmonic field is deterministic and loop-closed like the former
	// sine, but its mixed periods avoid a single obvious mechanical swell. It
	// transforms the whole anchor route together, preserving order and spacing.
	centerWave := 0.58*math.Sin(2*math.Pi*phase+0.35) +
		0.27*math.Sin(6*math.Pi*phase+1.10) +
		0.15*math.Sin(10*math.Pi*phase+2.20)
	spanWave := 0.68*math.Cos(2*math.Pi*phase+0.80) +
		0.32*math.Sin(8*math.Pi*phase+0.15)
	variedSpan := math.Max(
		MinimumDynamicSpanPercent,
		math.Min(100, span*(1+0.14*amount*spanWave)),
	)
	variedCenter := center + 9*amount*centerWave
	variedCenter = math.Max(variedSpan/2, math.Min(100-variedSpan/2, variedCenter))
	return variedCenter + (position-center)*variedSpan/span
}

func dynamicLegMillis(left, right float64, variation int, phase float64) int64 {
	base := max(int64(180), int64(math.Round(math.Abs(right-left)*10)))
	if variation <= 0 {
		return base
	}
	amount := float64(variation) / 100
	direction := 1.0
	if right < left {
		direction = -1
	}
	breathing := 0.64*math.Sin(2*math.Pi*phase+0.55) +
		0.36*math.Sin(6*math.Pi*phase+1.75)
	// Directional skew makes the two halves of a stroke subtly unequal, while
	// a slower harmonic changes which half leads. The bound prevents variation
	// from becoming a hidden speed multiplier or abrupt jitter.
	skew := direction * math.Sin(4*math.Pi*phase+0.90)
	scale := 1 + amount*(0.18*breathing+0.09*skew)
	scale = math.Max(0.75, math.Min(1.25, scale))
	return max(int64(120), int64(math.Round(float64(base)*scale)))
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
