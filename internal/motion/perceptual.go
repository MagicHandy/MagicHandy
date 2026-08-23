package motion

import "math"

const perceptualWindowMillis = int64(12_000)

// PerceptualSummary is a compact description of the curve that the
// shared engine actually commands after speed calibration and safety fitting.
// It contains no transport payload and is safe to expose to the mode planner
// and diagnostics without retaining raw curve points.
type PerceptualSummary struct {
	PositionMinPercent             float64 `json:"position_min_percent"`
	PositionMaxPercent             float64 `json:"position_max_percent"`
	MeanStrokePercent              float64 `json:"mean_stroke_percent"`
	StrokeLengthCV                 float64 `json:"stroke_length_cv"`
	MinimumLocalStrokeCV           float64 `json:"minimum_local_stroke_cv"`
	MinimumLocalStrokeRange        float64 `json:"minimum_local_stroke_range_percent"`
	CommandedMeanTravelPerSecond   float64 `json:"commanded_mean_travel_percent_per_second"`
	CommandedPeakVelocityPerSecond float64 `json:"commanded_peak_velocity_percent_per_second"`
	AnchorCount                    int     `json:"anchor_count"`
	SectionCount                   int     `json:"section_count"`
	SpanProfile                    string  `json:"span_profile,omitempty"`
}

func summarizeMotionPlan(
	target MotionTarget,
	curve Curve,
	focus focusProjection,
	periodMillis int64,
) PerceptualSummary {
	if len(curve.authoredKnots) < 2 || curve.duration <= 0 || periodMillis <= 0 {
		return PerceptualSummary{}
	}
	summary := PerceptualSummary{
		PositionMinPercent: focus.apply(curve.minPosition),
		PositionMaxPercent: focus.apply(curve.maxPosition),
	}
	if target.Dynamic != nil {
		dynamic := NormalizeDynamicDefinition(*target.Dynamic)
		summary.AnchorCount = len(dynamic.Anchors)
		summary.SectionCount = len(dynamic.Sections)
		summary.SpanProfile = dynamic.SpanProfile
	}
	gain := focus.gain()
	travel := totalCurveTravel(curve.authoredKnots) * gain
	summary.CommandedMeanTravelPerSecond = travel * 1000 / float64(periodMillis)
	timeFactor := float64(periodMillis) / float64(curve.duration)
	summary.CommandedPeakVelocityPerSecond = curve.maximumVelocityPerMillis() * gain / timeFactor * 1000

	reversals := perceptualReversals(curve, focus, periodMillis)
	if len(reversals) < 2 {
		return summary
	}
	type stroke struct {
		at     int64
		length float64
	}
	strokes := make([]stroke, 0, len(reversals))
	lengths := make([]float64, 0, len(reversals))
	for index, left := range reversals {
		right := reversals[(index+1)%len(reversals)]
		rightAt := right.at
		if index == len(reversals)-1 {
			rightAt += periodMillis
		}
		length := math.Abs(right.position - left.position)
		strokes = append(strokes, stroke{at: left.at + (rightAt-left.at)/2, length: length})
		lengths = append(lengths, length)
	}
	summary.MeanStrokePercent, summary.StrokeLengthCV, _ = perceptualLengthMetrics(lengths)

	repeatCount := max(2, int((perceptualWindowMillis+periodMillis-1)/periodMillis)+1)
	repeated := make([]stroke, 0, len(strokes)*repeatCount)
	for pass := range repeatCount {
		for _, candidate := range strokes {
			repeated = append(repeated, stroke{
				at: candidate.at + int64(pass)*periodMillis, length: candidate.length,
			})
		}
	}
	minimumCV, minimumRange := math.MaxFloat64, math.MaxFloat64
	for start := 0; start < len(strokes); start++ {
		windowStart := repeated[start].at
		windowLengths := make([]float64, 0, len(strokes))
		for index := start; index < len(repeated) && repeated[index].at < windowStart+perceptualWindowMillis; index++ {
			windowLengths = append(windowLengths, repeated[index].length)
		}
		if len(windowLengths) < 2 {
			continue
		}
		_, cv, lengthRange := perceptualLengthMetrics(windowLengths)
		minimumCV = math.Min(minimumCV, cv)
		minimumRange = math.Min(minimumRange, lengthRange)
	}
	if minimumCV != math.MaxFloat64 {
		summary.MinimumLocalStrokeCV = minimumCV
		summary.MinimumLocalStrokeRange = minimumRange
	}
	return summary
}

type perceptualReversal struct {
	at       int64
	position float64
}

func perceptualReversals(curve Curve, focus focusProjection, periodMillis int64) []perceptualReversal {
	knotCount := len(curve.authoredKnots) - 1
	result := make([]perceptualReversal, 0, knotCount)
	for index := range knotCount {
		previous := (index + knotCount - 1) % knotCount
		incoming := curve.authoredKnots[index].PositionPercent - curve.authoredKnots[previous].PositionPercent
		outgoing := curve.authoredKnots[index+1].PositionPercent - curve.authoredKnots[index].PositionPercent
		if incoming*outgoing >= 0 {
			continue
		}
		result = append(result, perceptualReversal{
			at: int64(math.Round(float64(curve.authoredKnots[index].TimeMillis) /
				float64(curve.duration) * float64(periodMillis))),
			position: focus.apply(curve.authoredKnots[index].PositionPercent),
		})
	}
	return result
}

func perceptualLengthMetrics(lengths []float64) (mean, cv, lengthRange float64) {
	minimum, maximum := math.MaxFloat64, 0.0
	for _, length := range lengths {
		mean += length
		minimum = math.Min(minimum, length)
		maximum = math.Max(maximum, length)
	}
	mean /= float64(len(lengths))
	variance := 0.0
	for _, length := range lengths {
		variance += (length - mean) * (length - mean)
	}
	if mean > 0 {
		cv = math.Sqrt(variance/float64(len(lengths))) / mean
	}
	return mean, cv, maximum - minimum
}

// MateriallyDifferent reports whether two commanded curves differ enough to
// count as a new felt phrase. It is deliberately one normalized perceptual
// distance rather than a list of schema-field exceptions. Speed is absent: the
// scheduler already tracks pace age independently.
func (s PerceptualSummary) MateriallyDifferent(other PerceptualSummary) bool {
	if s.SectionCount != other.SectionCount || s.AnchorCount != other.AnchorCount {
		return true
	}
	profilePenalty := 0.0
	if s.SpanProfile != other.SpanProfile {
		profilePenalty = 1
	}
	leftCenter := (s.PositionMinPercent + s.PositionMaxPercent) / 2
	rightCenter := (other.PositionMinPercent + other.PositionMaxPercent) / 2
	leftSpan := s.PositionMaxPercent - s.PositionMinPercent
	rightSpan := other.PositionMaxPercent - other.PositionMinPercent
	components := []float64{
		(leftCenter - rightCenter) / 6,
		(leftSpan - rightSpan) / 8,
		(s.MeanStrokePercent - other.MeanStrokePercent) / 6,
		(s.StrokeLengthCV - other.StrokeLengthCV) / 0.05,
		(s.MinimumLocalStrokeRange - other.MinimumLocalStrokeRange) / 5,
		profilePenalty,
	}
	distanceSquared := 0.0
	for _, component := range components {
		distanceSquared += component * component
	}
	// A phrase boundary should represent a coherent combined change, not the
	// accumulated effect of a few nearby scalar edits. Keep enough separation to
	// reject numerical nudges without suppressing a compiled near-full expansion
	// whose center, span, stroke length, and texture change together.
	return math.Sqrt(distanceSquared/float64(len(components))) >= 1.1
}
