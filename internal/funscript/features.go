package funscript

const (
	minDelta         = 3.0
	minHalfStrokeMS  = 40
	minHalfStrokeAmp = 2.0
)

type halfStrokeSegment struct {
	amplitude  float64
	durationMS int
}

func halfStrokeSegments(actions []Action, minDeltaThreshold float64) []halfStrokeSegment {
	if len(actions) < 2 {
		return nil
	}
	if minDeltaThreshold <= 0 {
		minDeltaThreshold = minDelta
	}

	turnIndices := []int{0}
	direction := 0

	for index := 1; index < len(actions); index++ {
		delta := actions[index].Pos - actions[index-1].Pos
		if abs(delta) < minDeltaThreshold {
			continue
		}
		step := 1
		if delta < 0 {
			step = -1
		}
		if direction != 0 && step != direction {
			turnIndices = append(turnIndices, index)
		}
		direction = step
	}
	if turnIndices[len(turnIndices)-1] != len(actions)-1 {
		turnIndices = append(turnIndices, len(actions)-1)
	}

	segments := make([]halfStrokeSegment, 0, len(turnIndices))
	for idx := 0; idx < len(turnIndices)-1; idx++ {
		startI := turnIndices[idx]
		endI := turnIndices[idx+1]
		durationMS := actions[endI].At - actions[startI].At
		if durationMS < minHalfStrokeMS {
			continue
		}
		amplitude := abs(actions[endI].Pos - actions[startI].Pos)
		if amplitude < minHalfStrokeAmp {
			continue
		}
		segments = append(segments, halfStrokeSegment{amplitude: amplitude, durationMS: durationMS})
	}
	return segments
}

func strokeSpeedsPPS(segments []halfStrokeSegment) []float64 {
	speeds := make([]float64, 0, len(segments))
	for _, segment := range segments {
		if segment.durationMS <= 0 {
			continue
		}
		speeds = append(speeds, (segment.amplitude/float64(segment.durationMS))*1000.0)
	}
	return speeds
}

func summarizeStrokeMetrics(actions []Action) map[string]float64 {
	segments := halfStrokeSegments(actions, minDelta)
	amplitudes := make([]float64, 0, len(segments))
	for _, segment := range segments {
		amplitudes = append(amplitudes, segment.amplitude)
	}
	speeds := strokeSpeedsPPS(segments)

	durationMS := maxInt(actions[len(actions)-1].At-actions[0].At, 1)
	minP, maxP := actions[0].Pos, actions[0].Pos
	for _, action := range actions[1:] {
		if action.Pos < minP {
			minP = action.Pos
		}
		if action.Pos > maxP {
			maxP = action.Pos
		}
	}
	span := maxP - minP

	if len(segments) == 0 && span >= minHalfStrokeAmp {
		amplitudes = []float64{span}
		speeds = []float64{(span / float64(durationMS)) * 1000.0}
	}

	legCount := float64(len(segments))
	if legCount == 0 && span >= minHalfStrokeAmp {
		legCount = 1
	}

	return map[string]float64{
		"median_stroke_amplitude": round2(median(amplitudes)),
		"p25_stroke_amplitude":    round2(percentile(amplitudes, 0.25)),
		"p75_stroke_amplitude":    round2(percentile(amplitudes, 0.75)),
		"median_stroke_speed_pps": round2(median(speeds)),
		"p40_stroke_speed_pps":    round2(percentile(speeds, 0.4)),
		"p90_stroke_speed_pps":    round2(percentile(speeds, 0.9)),
		"stroke_leg_count":        legCount,
	}
}

func countStrokes(actions []Action, threshold float64) int {
	if len(actions) < 3 {
		return 0
	}
	if threshold <= 0 {
		threshold = 3.0
	}

	direction := 0
	strokes := 0
	for index := 1; index < len(actions); index++ {
		delta := actions[index].Pos - actions[index-1].Pos
		if abs(delta) < threshold {
			continue
		}
		step := 1
		if delta < 0 {
			step = -1
		}
		if direction != 0 && step != direction {
			strokes++
		}
		direction = step
	}
	return strokes
}

func computeSpeeds(actions []Action) []float64 {
	speeds := make([]float64, 0, len(actions)-1)
	for index := 1; index < len(actions); index++ {
		dt := actions[index].At - actions[index-1].At
		if dt <= 0 {
			continue
		}
		dp := abs(actions[index].Pos - actions[index-1].Pos)
		speeds = append(speeds, dp/float64(dt))
	}
	return speeds
}

// ExtractBlockFeatures computes motion metrics for one block.
func ExtractBlockFeatures(actions []Action) (BlockFeatures, error) {
	if len(actions) < 2 {
		return BlockFeatures{}, validationErrorf("block must contain at least two actions.")
	}

	pos := positions(actions)
	startMS := actions[0].At
	endMS := actions[len(actions)-1].At
	durationMS := endMS - startMS

	minP := pos[0]
	maxP := pos[0]
	for _, p := range pos[1:] {
		if p < minP {
			minP = p
		}
		if p > maxP {
			maxP = p
		}
	}
	minPos := int(minP + 0.5)
	maxPos := int(maxP + 0.5)
	avgPos := mean(pos)
	amplitude := maxPos - minPos

	speeds := computeSpeeds(actions)
	avgSpeed := mean(speeds)
	maxSpeed := 0.0
	for _, speed := range speeds {
		if speed > maxSpeed {
			maxSpeed = speed
		}
	}

	zeroMoveSamples := 0
	for index := 1; index < len(actions); index++ {
		if abs(actions[index].Pos-actions[index-1].Pos) < 0.5 {
			zeroMoveSamples++
		}
	}
	holdRatio := float64(zeroMoveSamples) / float64(maxInt(len(actions)-1, 1))
	strokeMetrics := summarizeStrokeMetrics(actions)

	return BlockFeatures{
		StartMS:               startMS,
		EndMS:                 endMS,
		DurationMS:            durationMS,
		MinPos:                minPos,
		MaxPos:                maxPos,
		AvgPos:                round2(avgPos),
		Amplitude:             amplitude,
		ActionCount:           len(actions),
		AvgSpeed:              round6(avgSpeed),
		MaxSpeed:              round6(maxSpeed),
		StrokeCount:           countStrokes(actions, 3.0),
		HoldRatio:             round4(holdRatio),
		MedianStrokeAmplitude: strokeMetrics["median_stroke_amplitude"],
		P25StrokeAmplitude:    strokeMetrics["p25_stroke_amplitude"],
		P75StrokeAmplitude:    strokeMetrics["p75_stroke_amplitude"],
		MedianStrokeSpeedPPS:  strokeMetrics["median_stroke_speed_pps"],
		P40StrokeSpeedPPS:     strokeMetrics["p40_stroke_speed_pps"],
		P90StrokeSpeedPPS:     strokeMetrics["p90_stroke_speed_pps"],
		StrokeLegCount:        strokeMetrics["stroke_leg_count"],
	}, nil
}

// ExtractFeaturesForBlocks computes features for every block in order.
func ExtractFeaturesForBlocks(blocks [][]Action) ([]BlockFeatures, error) {
	out := make([]BlockFeatures, 0, len(blocks))
	for _, block := range blocks {
		if len(block) < 2 {
			continue
		}
		features, err := ExtractBlockFeatures(block)
		if err != nil {
			return nil, err
		}
		out = append(out, features)
	}
	return out, nil
}
