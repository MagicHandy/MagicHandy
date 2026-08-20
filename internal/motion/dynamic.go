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
			position := dynamicVariedPosition(base[index], base, definition, phase, cycles)
			if len(points) > 0 {
				elapsed += dynamicLegMillis(
					points[len(points)-1].PositionPercent, position,
					definition, len(points)-1, totalLegs,
				)
			}
			points = append(points, CurvePoint{TimeMillis: elapsed, PositionPercent: position})
		}
	}
	closing := dynamicVariedPosition(base[0], base, definition, 1, cycles)
	elapsed += dynamicLegMillis(
		points[len(points)-1].PositionPercent, closing,
		definition, len(points)-1, totalLegs,
	)
	points = append(points, CurvePoint{TimeMillis: elapsed, PositionPercent: closing})
	return resolvedContent{
		points: points, duration: elapsed, loop: true, maximumPoints: maximumCurvePoints,
		reversalProfile: curveReversalWholeLeg,
	}
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
			position := dynamicVariedPosition(base[index], base, definition, phase, cycles)
			if hasPrevious {
				travel += math.Abs(position - previous)
			}
			previous = position
			hasPrevious = true
		}
	}
	closing := dynamicVariedPosition(base[0], base, definition, 1, cycles)
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

func dynamicVariedPosition(position float64, base []float64, definition DynamicDefinition, phase float64, cycles int) float64 {
	phase -= math.Floor(phase)
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
	linearAmount := float64(variation) / 100
	// A square-root response gives the lower half of the model-facing control a
	// useful perceptual range. The previous linear map made the ordinary 20-40
	// band almost indistinguishable from a mechanically even loop.
	amount := math.Sqrt(linearAmount)
	seed := definition.PhraseSeed
	if seed == 0 {
		seed = dynamicPhraseSeed(definition)
	}
	// A small harmonic field is deterministic and loop-closed like the former
	// sine. A correlated, faster phrase texture prevents the same center from
	// repeating through a long envelope plateau without adding sample jitter.
	centerWave := 0.58*math.Sin(2*math.Pi*phase+0.35) +
		0.27*math.Sin(6*math.Pi*phase+1.10) +
		0.15*math.Sin(10*math.Pi*phase+2.20)
	centerTexture := 2*dynamicSeededPeriodicControl(
		seed^0x8f4d3b21, phase, dynamicTextureKnotCount(cycles, 5, 8, 96),
	) - 1
	variedSpan := span
	switch definition.SpanProfile {
	case DynamicSpanProfileSteady:
		// Explicit steady span separates range from center/rhythm texture.
	case DynamicSpanProfileBreathe, DynamicSpanProfileWander, DynamicSpanProfileContrast:
		variedSpan = dynamicSpanEnvelopeValue(definition, span, phase, cycles)
	default:
		// Compatibility for alpha.25 targets that predate explicit envelopes.
		spanWave := 0.68*math.Cos(2*math.Pi*phase+0.80) +
			0.32*math.Sin(8*math.Pi*phase+0.15)
		variedSpan = math.Max(
			MinimumDynamicSpanPercent,
			math.Min(100, span*(1+0.14*linearAmount*spanWave)),
		)
	}
	variedCenter := center + amount*(14*centerWave+6*centerTexture)
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

func dynamicSpanEnvelopeValue(definition DynamicDefinition, outerSpan, phase float64, cycles int) float64 {
	innerSpan := float64(definition.SpanMinPercent)
	factor := dynamicSpanEnvelopeFactor(definition, phase, cycles)
	return math.Max(innerSpan, math.Min(outerSpan, innerSpan+(outerSpan-innerSpan)*factor))
}

func dynamicSpanEnvelopeFactor(definition DynamicDefinition, phase float64, cycles int) float64 {
	phase -= math.Floor(phase)
	switch definition.SpanProfile {
	case DynamicSpanProfileBreathe:
		// One asymmetric swell with smaller secondary breaths. All harmonics are
		// periodic, so both value and slope match at the loop seam. A restrained
		// faster layer keeps the carrier from becoming exact during the long swell.
		value := 0.50 - 0.43*math.Cos(2*math.Pi*phase) +
			0.08*math.Sin(4*math.Pi*phase+0.35) +
			0.04*math.Sin(6*math.Pi*phase-0.60)
		texture := dynamicSeededPeriodicControl(
			definition.PhraseSeed^0x37c6a5d9, phase,
			dynamicTextureKnotCount(cycles, 7, 6, 48),
		)
		return math.Max(0.03, math.Min(0.97, value+0.12*(texture-0.5)))
	case DynamicSpanProfileContrast:
		return dynamicContrastEnvelope(definition.PhraseSeed, phase, cycles)
	case DynamicSpanProfileWander:
		return dynamicWanderEnvelope(definition.PhraseSeed, phase, cycles)
	default:
		return 1
	}
}

func dynamicWanderEnvelope(seed uint32, phase float64, cycles int) float64 {
	knotCount := dynamicTextureKnotCount(cycles, 2, 12, 128)
	return dynamicPeriodicControl(phase, knotCount, func(index int) float64 {
		// A seeded permutation visits lower, middle, and upper portions of the
		// band every few strokes. Its narrower levels and smooth interpolation
		// meander rather than making Contrast's tight-to-broad jumps.
		// index is generated by dynamicPeriodicControl in [0, knotCount).
		jitter := dynamicSeedUnit(seed^0x1b873593, uint64(index)) //nolint:gosec
		switch dynamicControlCategory(seed^0x85ebca6b, index, knotCount) {
		case 0:
			return 0.10 + 0.20*jitter
		case 1:
			return 0.37 + 0.26*jitter
		default:
			return 0.70 + 0.20*jitter
		}
	})
}

func dynamicContrastEnvelope(seed uint32, phase float64, cycles int) float64 {
	knotCount := dynamicTextureKnotCount(cycles, 2, 12, 160)
	return dynamicPeriodicControl(phase, knotCount, func(index int) float64 {
		// Each block is a seeded permutation of tight, medium, and broad. That
		// creates short human-readable groupings without a literal repeating table
		// or the former many-second plateau at one nearly identical span.
		// index is generated by dynamicPeriodicControl in [0, knotCount).
		jitter := dynamicSeedUnit(seed^0x63d83595, uint64(index)) //nolint:gosec
		switch dynamicControlCategory(seed^0xa511e9b3, index, knotCount) {
		case 0:
			return 0.05 + 0.15*jitter
		case 1:
			return 0.38 + 0.24*jitter
		default:
			return 0.82 + 0.16*jitter
		}
	})
}

func dynamicControlCategory(seed uint32, index, total int) int {
	if total <= 0 {
		return 1
	}
	index = (index%total + total) % total
	category := dynamicLinearControlCategory(seed, index)
	if index != total-1 || total <= 1 {
		return category
	}
	first := dynamicLinearControlCategory(seed, 0)
	if category != first {
		return category
	}
	previous := dynamicLinearControlCategory(seed, index-1)
	for candidate := range 3 {
		if candidate != first && candidate != previous {
			return candidate
		}
	}
	return category
}

func dynamicLinearControlCategory(seed uint32, index int) int {
	permutations := [...][3]int{
		{0, 1, 2}, {0, 2, 1}, {1, 0, 2},
		{1, 2, 0}, {2, 0, 1}, {2, 1, 0},
	}
	last := -1
	selected := permutations[0]
	for block := 0; block <= index/3; block++ {
		choices := [6]int{}
		choiceCount := 0
		for permutationIndex, permutation := range permutations {
			if permutation[0] == last {
				continue
			}
			choices[choiceCount] = permutationIndex
			choiceCount++
		}
		choice := int(dynamicSeedUnit(seed, uint64(block)) * float64(choiceCount))
		selected = permutations[choices[min(choice, choiceCount-1)]]
		last = selected[2]
	}
	return selected[index%3]
}

func dynamicPeriodicControl(phase float64, knotCount int, value func(int) float64) float64 {
	if knotCount <= 0 {
		return 0.5
	}
	scaled := (phase - math.Floor(phase)) * float64(knotCount)
	left := int(math.Floor(scaled)) % knotCount
	right := (left + 1) % knotCount
	fraction := scaled - math.Floor(scaled)
	smooth := fraction * fraction * (3 - 2*fraction)
	leftValue, rightValue := value(left), value(right)
	return leftValue + (rightValue-leftValue)*smooth
}

func dynamicSeededPeriodicControl(seed uint32, phase float64, knotCount int) float64 {
	return dynamicPeriodicControl(phase, knotCount, func(index int) float64 {
		// index is generated by dynamicPeriodicControl in [0, knotCount).
		return dynamicSeedUnit(seed, uint64(index)) //nolint:gosec
	})
}

func dynamicTextureKnotCount(samples, samplesPerKnot, minimum, maximum int) int {
	if samplesPerKnot <= 0 {
		samplesPerKnot = 1
	}
	return clamp((max(1, samples)+samplesPerKnot-1)/samplesPerKnot, minimum, maximum)
}

func dynamicSeedUnit(seed uint32, index uint64) float64 {
	value := uint64(seed) + uint64(index+1)*0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	value ^= value >> 31
	return float64(value>>11) / float64(uint64(1)<<53)
}

func dynamicLegMillis(left, right float64, definition DynamicDefinition, legIndex, totalLegs int) int64 {
	base := max(int64(180), int64(math.Round(math.Abs(right-left)*10)))
	variation := definition.VariationPercent
	if variation <= 0 {
		return base
	}
	amount := math.Sqrt(float64(variation) / 100)
	phase := (float64(legIndex) + 0.5) / float64(max(1, totalLegs))
	direction := 1.0
	if right < left {
		direction = -1
	}
	breathing := 0.64*math.Sin(2*math.Pi*phase+0.55) +
		0.36*math.Sin(6*math.Pi*phase+1.75)
	seed := definition.PhraseSeed
	if seed == 0 {
		seed = dynamicPhraseSeed(definition)
	}
	rhythm := 2*dynamicContrastEnvelope(
		seed^0xd1b54a35, phase,
		dynamicTextureKnotCount(totalLegs, 4, 8, 160)*3,
	) - 1
	// Directional skew makes the two halves of a stroke unequal, while seeded
	// grouped rhythm keeps a moderate setting from collapsing into a metronome.
	// The exact curve still passes through the normal acceleration and reversal
	// limiter, and the phrase remains loop-closed rather than using sample noise.
	skew := direction * math.Sin(4*math.Pi*phase+0.90)
	scale := 1 + amount*(0.14*breathing+0.10*skew+0.22*rhythm)
	scale = math.Max(0.65, math.Min(1.45, scale))
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
