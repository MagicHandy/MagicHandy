package motion

import (
	"fmt"
	"hash/fnv"
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
	// An explicit span envelope must outlive the short carrier motif. The
	// target-travel calculation below translates this duration into enough
	// whole route traversals at the fastest calibrated Handy profile.
	minimumDynamicSpanEnvelopeLoopSeconds = 30.0
	minimumDynamicSpanEnvelopeCycles      = 24
	maximumDynamicSpanEnvelopeCycles      = 512
)

// DynamicSpanProfileSteady and its siblings are semantic texture choices, not
// transport modes. The empty profile retains alpha.25 compatibility, where
// variation_percent also produces a small implicit span swell.
const (
	DynamicSpanProfileSteady   = "steady"
	DynamicSpanProfileBreathe  = "breathe"
	DynamicSpanProfileWander   = "wander"
	DynamicSpanProfileContrast = "contrast"
)

// DynamicSpanProfiles lists the explicit model-facing profile vocabulary.
func DynamicSpanProfiles() []string {
	return []string{
		DynamicSpanProfileSteady,
		DynamicSpanProfileBreathe,
		DynamicSpanProfileWander,
		DynamicSpanProfileContrast,
	}
}

// ValidDynamicSpanProfile reports whether value is an explicit profile.
func ValidDynamicSpanProfile(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case DynamicSpanProfileSteady, DynamicSpanProfileBreathe,
		DynamicSpanProfileWander, DynamicSpanProfileContrast:
		return true
	default:
		return false
	}
}

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
	SpanMinPercent   int             `json:"span_min_percent,omitempty"`
	SpanProfile      string          `json:"span_profile,omitempty"`
	PhraseSeed       uint32          `json:"phrase_seed,omitempty"`
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
	} else {
		if definition.CenterPercent == 0 && definition.SpanPercent == 0 {
			definition.CenterPercent = defaultDynamicCenter
			definition.SpanPercent = defaultDynamicSpan
		}
		definition.CenterPercent = clamp(definition.CenterPercent, 0, 100)
		definition.SpanPercent = clamp(definition.SpanPercent, MinimumDynamicSpanPercent, 100)
		minimum, maximum := dynamicWindow(definition.CenterPercent, definition.SpanPercent)
		definition.CenterPercent = (minimum + maximum) / 2
		definition.SpanPercent = maximum - minimum
	}
	return normalizeDynamicSpanEnvelope(definition)
}

func normalizeDynamicSpanEnvelope(definition DynamicDefinition) DynamicDefinition {
	profile := strings.ToLower(strings.TrimSpace(definition.SpanProfile))
	if !ValidDynamicSpanProfile(profile) {
		// Empty and unknown profiles use the bounded alpha.25 compatibility
		// behavior. Chat rejects unknown values before they reach this layer;
		// direct targets still fail safe instead of inventing a new texture.
		definition.SpanProfile = ""
		definition.SpanMinPercent = 0
		definition.PhraseSeed = 0
		return definition
	}
	definition.SpanProfile = profile
	if profile == DynamicSpanProfileSteady {
		definition.SpanMinPercent = definition.SpanPercent
		definition.PhraseSeed = 0
		return definition
	}
	if definition.SpanMinPercent == 0 {
		// A variable profile without a model-selected floor must not silently
		// choose how much range to remove. It becomes an honest steady target.
		definition.SpanProfile = DynamicSpanProfileSteady
		definition.SpanMinPercent = definition.SpanPercent
		definition.PhraseSeed = 0
		return definition
	}
	definition.SpanMinPercent = clamp(
		definition.SpanMinPercent,
		MinimumDynamicSpanPercent,
		definition.SpanPercent,
	)
	if definition.SpanMinPercent >= definition.SpanPercent {
		definition.SpanProfile = DynamicSpanProfileSteady
		definition.PhraseSeed = 0
		return definition
	}
	if definition.PhraseSeed == 0 {
		definition.PhraseSeed = dynamicPhraseSeed(definition)
	}
	return definition
}

func dynamicPhraseSeed(definition DynamicDefinition) uint32 {
	hash := fnv.New32a()
	_, _ = fmt.Fprintf(hash, "%s:%d:%d", definition.SpanProfile,
		definition.SpanPercent, definition.SpanMinPercent)
	for _, anchor := range definition.Anchors {
		_, _ = fmt.Fprintf(hash, ":%s=%d", anchor.Name, anchor.PositionPercent)
	}
	seed := hash.Sum32()
	if seed == 0 {
		return 1
	}
	return seed
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
	if dynamicHasVariableSpanEnvelope(definition) {
		cycles = dynamicSpanEnvelopeCycleCount(base, definition)
	} else if definition.VariationPercent > 0 {
		cycles = dynamicVariationCycleCount(base, definition.VariationPercent)
	}
	indices := dynamicTraversalIndices(len(base))
	points := make([]CurvePoint, 0, cycles*len(indices)+1)
	totalLegs := cycles * len(indices)
	var elapsed int64
	for cycle := range cycles {
		phase := float64(cycle) / float64(cycles)
		for _, index := range indices {
			position := dynamicVariedPosition(base[index], base, definition, phase)
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
	closing := dynamicVariedPosition(base[0], base, definition, 1)
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
	return dynamicPhraseTravel(base, indices, DynamicDefinition{VariationPercent: variation}, cycles)
}

func dynamicSpanEnvelopeCycleCount(base []float64, definition DynamicDefinition) int {
	indices := dynamicTraversalIndices(len(base))
	if len(indices) < 2 {
		return minimumDynamicSpanEnvelopeCycles
	}
	maximumCycles := min(
		maximumDynamicSpanEnvelopeCycles,
		max(1, (maximumCurvePoints-1)/len(indices)),
	)
	minimumCycles := min(minimumDynamicSpanEnvelopeCycles, maximumCycles)
	targetTravel := minimumDynamicSpanEnvelopeLoopSeconds * maximumSupportedReferenceTravelRatePercentPerSecond
	const coarseStep = 8
	for cycles := minimumCycles; cycles <= maximumCycles; cycles += coarseStep {
		if dynamicPhraseTravel(base, indices, definition, cycles) < targetTravel {
			continue
		}
		for candidate := max(minimumCycles, cycles-coarseStep+1); candidate <= cycles; candidate++ {
			if dynamicPhraseTravel(base, indices, definition, candidate) >= targetTravel {
				return candidate
			}
		}
		return cycles
	}
	return maximumCycles
}

func dynamicPhraseTravel(base []float64, indices []int, definition DynamicDefinition, cycles int) float64 {
	travel := 0.0
	previous := 0.0
	hasPrevious := false
	for cycle := range cycles {
		phase := float64(cycle) / float64(cycles)
		for _, index := range indices {
			position := dynamicVariedPosition(base[index], base, definition, phase)
			if hasPrevious {
				travel += math.Abs(position - previous)
			}
			previous = position
			hasPrevious = true
		}
	}
	closing := dynamicVariedPosition(base[0], base, definition, 1)
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

func dynamicVariedPosition(position float64, base []float64, definition DynamicDefinition, phase float64) float64 {
	variation := definition.VariationPercent
	if variation <= 0 && !dynamicHasVariableSpanEnvelope(definition) {
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
	variedSpan := span
	switch definition.SpanProfile {
	case DynamicSpanProfileSteady:
		// Explicit steady span separates range from center/rhythm texture.
	case DynamicSpanProfileBreathe, DynamicSpanProfileWander, DynamicSpanProfileContrast:
		variedSpan = dynamicSpanEnvelopeValue(definition, span, phase)
	default:
		// Compatibility for alpha.25 targets that predate explicit envelopes.
		spanWave := 0.68*math.Cos(2*math.Pi*phase+0.80) +
			0.32*math.Sin(8*math.Pi*phase+0.15)
		variedSpan = math.Max(
			MinimumDynamicSpanPercent,
			math.Min(100, span*(1+0.14*amount*spanWave)),
		)
	}
	variedCenter := center + 9*amount*centerWave
	variedCenter = math.Max(variedSpan/2, math.Min(100-variedSpan/2, variedCenter))
	// The algebraic bounds above can still land a few ulps beyond an endpoint
	// (for example, a full-span route at 96% variation produced
	// 100.00000000000001). Curve validation is deliberately strict because its
	// output reaches a real device, so clamp the final floating-point result as
	// well as the conceptual center/span window.
	return math.Max(0, math.Min(100, variedCenter+(position-center)*variedSpan/span))
}

func dynamicHasVariableSpanEnvelope(definition DynamicDefinition) bool {
	return definition.SpanMinPercent >= MinimumDynamicSpanPercent &&
		definition.SpanMinPercent < definition.SpanPercent &&
		(definition.SpanProfile == DynamicSpanProfileBreathe ||
			definition.SpanProfile == DynamicSpanProfileWander ||
			definition.SpanProfile == DynamicSpanProfileContrast)
}

func dynamicSpanEnvelopeValue(definition DynamicDefinition, outerSpan, phase float64) float64 {
	innerSpan := float64(definition.SpanMinPercent)
	factor := dynamicSpanEnvelopeFactor(definition, phase)
	return math.Max(innerSpan, math.Min(outerSpan, innerSpan+(outerSpan-innerSpan)*factor))
}

func dynamicSpanEnvelopeFactor(definition DynamicDefinition, phase float64) float64 {
	phase -= math.Floor(phase)
	switch definition.SpanProfile {
	case DynamicSpanProfileBreathe:
		// One asymmetric swell with smaller secondary breaths. All harmonics are
		// periodic, so both value and slope match at the loop seam.
		value := 0.50 - 0.43*math.Cos(2*math.Pi*phase) +
			0.08*math.Sin(4*math.Pi*phase+0.35) +
			0.04*math.Sin(6*math.Pi*phase-0.60)
		return math.Max(0.04, math.Min(0.96, value))
	case DynamicSpanProfileContrast:
		return dynamicContrastEnvelope(definition.PhraseSeed, phase)
	case DynamicSpanProfileWander:
		return dynamicWanderEnvelope(definition.PhraseSeed, phase)
	default:
		return 1
	}
}

func dynamicWanderEnvelope(seed uint32, phase float64) float64 {
	const knotCount uint64 = 13
	values := make([]float64, knotCount)
	minimum, maximum := 1.0, 0.0
	for index := range knotCount {
		values[index] = dynamicSeedUnit(seed, index)
		minimum = math.Min(minimum, values[index])
		maximum = math.Max(maximum, values[index])
	}
	for index := range values {
		if maximum > minimum {
			values[index] = (values[index] - minimum) / (maximum - minimum)
		}
		// Spend most of the phrase away from hard bounds while still making
		// occasional narrow and broad strokes perceptible.
		values[index] = 0.08 + 0.84*values[index]
	}
	return periodicSmoothControl(values, phase)
}

func dynamicContrastEnvelope(seed uint32, phase float64) float64 {
	values := [...]float64{
		0.16, 0.18, 0.20, 0.54, 0.90, 0.88, 0.42, 0.24,
		0.22, 0.72, 0.48, 0.92, 0.90, 0.34, 0.64, 0.18,
	}
	rotated := make([]float64, len(values))
	offset := int(seed % uint32(len(values)))
	reverse := seed&1 != 0
	for index := range rotated {
		source := (index + offset) % len(values)
		if reverse {
			source = (offset - index + len(values)) % len(values)
		}
		rotated[index] = values[source]
	}
	return periodicSmoothControl(rotated, phase)
}

func periodicSmoothControl(values []float64, phase float64) float64 {
	if len(values) == 0 {
		return 1
	}
	scaled := (phase - math.Floor(phase)) * float64(len(values))
	left := int(math.Floor(scaled)) % len(values)
	right := (left + 1) % len(values)
	fraction := scaled - math.Floor(scaled)
	smooth := fraction * fraction * (3 - 2*fraction)
	return values[left] + (values[right]-values[left])*smooth
}

func dynamicSeedUnit(seed uint32, index uint64) float64 {
	value := uint64(seed) + uint64(index+1)*0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	value ^= value >> 31
	return float64(value>>11) / float64(uint64(1)<<53)
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
	_, _ = fmt.Fprintf(&builder, "dynamic:%d:%d:%d:%s:%d:%d", definition.CenterPercent,
		definition.SpanPercent, definition.SpanMinPercent, definition.SpanProfile,
		definition.PhraseSeed, definition.VariationPercent)
	for _, anchor := range definition.Anchors {
		_, _ = fmt.Fprintf(&builder, ":%s=%d", anchor.Name, anchor.PositionPercent)
	}
	return builder.String()
}
