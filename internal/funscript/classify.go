package funscript

// PaceFromBPM maps strokes-per-minute to library speed tags.
func PaceFromBPM(bpm float64) string {
	if bpm <= 0 {
		return "unknown"
	}
	switch {
	case bpm < 45:
		return "slow"
	case bpm < 95:
		return "medium"
	case bpm < 160:
		return "fast"
	default:
		return "very_fast"
	}
}

// ComputeStrokeBPM estimates full in-out stroke cycles per minute.
func ComputeStrokeBPM(actions []Action, durationMS int) map[string]any {
	durationMS = maxInt(durationMS, 1)
	if len(actions) < 2 {
		return map[string]any{
			"bpm":              0.0,
			"stroke_legs":      0,
			"stroke_reversals": 0,
			"pace":             "unknown",
		}
	}

	metrics := summarizeStrokeMetrics(actions)
	legs := maxInt(0, int(metrics["stroke_leg_count"]+0.5))
	reversals := countStrokes(actions, 3.0)
	minutes := float64(durationMS) / 60000.0

	var fullCycles float64
	switch {
	case legs >= 2:
		fullCycles = float64(legs) / 2.0
	case legs == 1:
		fullCycles = 1.0
	default:
		fullCycles = 0.0
	}

	bpm := 0.0
	if minutes > 0 {
		bpm = fullCycles / minutes
	}
	return map[string]any{
		"bpm":              round1(bpm),
		"stroke_legs":      legs,
		"stroke_reversals": reversals,
		"pace":             PaceFromBPM(bpm),
	}
}

func classifyZone(features BlockFeatures) string {
	span := features.MaxPos - features.MinPos
	avg := features.AvgPos

	if span >= 55 && features.MinPos <= 20 && features.MaxPos >= 80 {
		return "full"
	}

	topTime, middleTime, bottomTime := 0, 0, 0
	switch {
	case avg >= 67:
		topTime = 2
	case avg <= 33:
		bottomTime = 2
	default:
		middleTime = 2
	}

	if features.MinPos >= 55 {
		topTime += 2
	} else if features.MaxPos <= 45 {
		bottomTime += 2
	} else {
		if features.MinPos > 45 {
			topTime++
		}
		if features.MaxPos < 55 {
			bottomTime++
		}
		middleTime++
	}

	scores := []struct {
		zone  string
		score int
	}{
		{"top", topTime},
		{"middle", middleTime},
		{"bottom", bottomTime},
	}
	dominant := "middle"
	best := -1
	activeZones := 0
	for _, item := range scores {
		if item.score >= 2 {
			activeZones++
		}
		if item.score > best {
			dominant = item.zone
			best = item.score
		}
	}
	if activeZones >= 2 && span >= 30 {
		return "mixed"
	}
	return dominant
}

func classifyStrokeLength(features BlockFeatures) string {
	effective := features.P25StrokeAmplitude
	if effective <= 0 {
		effective = features.MedianStrokeAmplitude
	}
	if effective <= 0 {
		effective = float64(features.Amplitude) * 0.35
	}
	switch {
	case effective < 6:
		return "micro"
	case effective < 14:
		return "short"
	case effective < 28:
		return "medium"
	default:
		return "full"
	}
}

func strokeIntervals(actions []Action) []int {
	if len(actions) < 3 {
		return nil
	}

	intervals := make([]int, 0, len(actions))
	var lastTurnMS *int
	direction := 0

	for index := 1; index < len(actions); index++ {
		delta := actions[index].Pos - actions[index-1].Pos
		if abs(delta) < 3.0 {
			continue
		}
		step := 1
		if delta < 0 {
			step = -1
		}
		atMS := actions[index].At
		if direction != 0 && step != direction {
			if lastTurnMS != nil {
				intervals = append(intervals, atMS-*lastTurnMS)
			}
			lastTurnMS = &atMS
		} else if lastTurnMS == nil {
			lastTurnMS = &atMS
		}
		direction = step
	}
	return intervals
}

func speedFromStrokeIntervals(actions []Action) string {
	intervals := strokeIntervals(actions)
	if len(intervals) < 3 {
		return ""
	}
	medianMS := median(floatsFromInts(intervals))
	switch {
	case medianMS >= 1100:
		return "slow"
	case medianMS >= 650:
		return "medium"
	case medianMS >= 380:
		return "fast"
	default:
		return "very_fast"
	}
}

func speedFromPPS(features BlockFeatures) string {
	speedPPS := features.P40StrokeSpeedPPS
	if speedPPS <= 0 {
		speedPPS = features.MedianStrokeSpeedPPS
	}
	if speedPPS <= 0 {
		speedPPS = features.AvgSpeed * 1000.0
	}
	if features.HoldRatio >= 0.4 {
		speedPPS *= max(0.55, 1.0-features.HoldRatio*0.5)
	}
	switch {
	case speedPPS < 22:
		return "slow"
	case speedPPS < 48:
		return "medium"
	case speedPPS < 95:
		return "fast"
	default:
		return "very_fast"
	}
}

func classifySpeed(features BlockFeatures, actions []Action) string {
	bpmResult := ComputeStrokeBPM(actions, features.DurationMS)
	if bpm, ok := bpmResult["bpm"].(float64); ok && bpm > 0 {
		return PaceFromBPM(bpm)
	}
	if byInterval := speedFromStrokeIntervals(actions); byInterval != "" {
		return byInterval
	}
	return speedFromPPS(features)
}

func classifyRhythm(features BlockFeatures, actions []Action) string {
	if features.HoldRatio >= 0.65 && features.Amplitude < 12 {
		return "pause_hold"
	}

	intervals := strokeIntervals(actions)
	if len(intervals) < 3 {
		if features.StrokeCount >= 4 && features.DurationMS <= 4000 {
			return "pulsed"
		}
		return "steady"
	}

	floatIntervals := floatsFromInts(intervals)
	meanInterval := mean(floatIntervals)
	if meanInterval <= 0 {
		return "steady"
	}

	cv := pstdev(floatIntervals) / meanInterval
	mid := len(intervals) / 2
	firstHalf := floatsFromInts(intervals[:mid])
	secondHalf := floatsFromInts(intervals[mid:])
	if len(firstHalf) >= 2 && len(secondHalf) >= 2 {
		firstMean := mean(firstHalf)
		secondMean := mean(secondHalf)
		if secondMean <= firstMean*0.72 {
			return "accelerating"
		}
		if secondMean >= firstMean*1.35 {
			return "decelerating"
		}
	}

	if cv >= 0.55 {
		if features.StrokeCount >= 4 && cv < 0.95 && meanInterval <= 900 {
			return "pulsed"
		}
		return "chaotic"
	}
	if features.StrokeCount >= 4 && meanInterval <= 700 && cv <= 0.35 {
		return "pulsed"
	}
	return "steady"
}

func computeIntensity(features BlockFeatures) float64 {
	speedPPS := features.MedianStrokeSpeedPPS
	if speedPPS <= 0 {
		speedPPS = features.AvgSpeed * 1000.0
	}
	strokeAmp := features.MedianStrokeAmplitude
	if strokeAmp <= 0 {
		strokeAmp = float64(features.Amplitude) * 0.5
	}
	speedScore := min(100.0, speedPPS*1.05)
	amplitudeScore := min(100.0, strokeAmp*2.2)
	frequencyScore := 0.0
	if features.DurationMS > 0 {
		strokesPerSec := float64(features.StrokeCount) / (float64(features.DurationMS) / 1000.0)
		frequencyScore = min(100.0, strokesPerSec*22.0)
	}
	holdPenalty := features.HoldRatio * 18.0
	raw := (speedScore * 0.42) + (amplitudeScore * 0.33) + (frequencyScore * 0.25) - holdPenalty
	return round1(max(0.0, min(100.0, raw)))
}

// ClassifyBlock returns classification labels and intensity for a single block.
func ClassifyBlock(features BlockFeatures, actions []Action) Classification {
	zone := classifyZone(features)
	strokeLength := classifyStrokeLength(features)
	speed := classifySpeed(features, actions)
	rhythm := classifyRhythm(features, actions)
	intensity := computeIntensity(features)
	return Classification{
		Zone:         zone,
		StrokeLength: strokeLength,
		Speed:        speed,
		Rhythm:       rhythm,
		Intensity:    intensity,
		Tags:         []string{zone, strokeLength, speed, rhythm},
	}
}

// ClassifyBlocks classifies every block using precomputed features.
func ClassifyBlocks(blocks [][]Action, featuresList []BlockFeatures) ([]Classification, error) {
	if len(blocks) != len(featuresList) {
		return nil, validationErrorf("blocks and features_list must have the same length.")
	}
	out := make([]Classification, len(blocks))
	for i := range blocks {
		out[i] = ClassifyBlock(featuresList[i], blocks[i])
	}
	return out, nil
}

func floatsFromInts(values []int) []float64 {
	out := make([]float64, len(values))
	for i, v := range values {
		out[i] = float64(v)
	}
	return out
}
