package funscript

const (
	gapThresholdMS      = 5000
	positionEPS         = 0.5
	maxTailHoldMS       = 800
	activeWindowMS      = 1200
	activeWindowMinAmp  = 4.0
	meaningfulStepPos   = 3.0
	meaningfulStepSpeed = 6.0
	flatGapMinMS        = 1500
	maxFlatHoldMS       = 900
	flatPosMax          = 1.5
)

type heatmapAction struct {
	at  float64
	pos float64
}

func roundActionsForHeatmap(actions []Action) []heatmapAction {
	cleaned := make([]heatmapAction, 0, len(actions))
	for _, item := range actions {
		at := float64(maxInt(item.At, 0))
		pos := item.Pos
		if pos < 0 {
			pos = 0
		}
		if pos > 100 {
			pos = 100
		}
		cleaned = append(cleaned, heatmapAction{at: at, pos: pos})
	}
	for i := 0; i < len(cleaned)-1; i++ {
		for j := i + 1; j < len(cleaned); j++ {
			if cleaned[j].at < cleaned[i].at {
				cleaned[i], cleaned[j] = cleaned[j], cleaned[i]
			}
		}
	}
	return cleaned
}

// MetadataDurationMS parses metadata.duration (seconds or ms).
func MetadataDurationMS(metadata map[string]any) int {
	if metadata == nil {
		return 0
	}
	raw, ok := metadata["duration"]
	if !ok {
		return 0
	}
	value, err := toFloat(raw)
	if err != nil || value <= 0 {
		return 0
	}
	if value < 100000 {
		return int(value * 1000)
	}
	return int(value)
}

func meaningfulStep(prevPos, currPos float64, gapMS int) bool {
	dp := abs(currPos - prevPos)
	if dp >= meaningfulStepPos {
		return true
	}
	if gapMS <= 0 || dp < 1.0 {
		return false
	}
	speed := 1000.0 * dp / float64(gapMS)
	return speed >= meaningfulStepSpeed
}

func lastActiveMotionIndex(actions []heatmapAction) int {
	if len(actions) == 0 {
		return 0
	}
	if len(actions) == 1 {
		return 0
	}

	lastActive := 0
	for index := range actions {
		tEnd := actions[index].at
		tStart := tEnd - activeWindowMS
		window := make([]float64, 0, 8)
		for _, item := range actions {
			if item.at >= tStart && item.at <= tEnd {
				window = append(window, item.pos)
			}
		}
		if len(window) >= 2 {
			minP, maxP := window[0], window[0]
			for _, p := range window[1:] {
				if p < minP {
					minP = p
				}
				if p > maxP {
					maxP = p
				}
			}
			if maxP-minP >= activeWindowMinAmp {
				lastActive = index
			}
		}
	}

	if lastActive > 0 {
		lastActive = trimTrailingLowAmpRun(actions, lastActive)
	}
	if lastActive > 0 {
		return lastActive
	}

	for index := 1; index < len(actions); index++ {
		gap := int(actions[index].at - actions[index-1].at)
		if meaningfulStep(actions[index-1].pos, actions[index].pos, gap) {
			lastActive = index
		}
	}
	return lastActive
}

func trimTrailingLowAmpRun(ordered []heatmapAction, anchor int) int {
	last := anchor
	for index := anchor + 1; index < len(ordered); index++ {
		tail := ordered[anchor : index+1]
		span := int(tail[len(tail)-1].at - tail[0].at)
		minP, maxP := tail[0].pos, tail[0].pos
		for _, item := range tail[1:] {
			if item.pos < minP {
				minP = item.pos
			}
			if item.pos > maxP {
				maxP = item.pos
			}
		}
		amplitude := maxP - minP
		gap := int(ordered[index].at - ordered[index-1].at)
		if span > 5000 && amplitude < activeWindowMinAmp {
			break
		}
		if meaningfulStep(ordered[index-1].pos, ordered[index].pos, gap) {
			last = index
			anchor = index
		}
	}
	return last
}

// MotionPlaybackEndMS returns the last meaningful motion time.
func MotionPlaybackEndMS(actions []Action) int {
	rounded := roundActionsForHeatmap(actions)
	if len(rounded) == 0 {
		return 0
	}
	if len(rounded) == 1 {
		return int(rounded[0].at) + 100
	}

	lastActiveIdx := lastActiveMotionIndex(rounded)
	lastAt := int(rounded[lastActiveIdx].at)
	step := 100
	if lastActiveIdx > 0 {
		step = maxInt(1, lastAt-int(rounded[lastActiveIdx-1].at))
	}
	tail := min(float64(step), float64(maxTailHoldMS))

	if lastActiveIdx+1 < len(rounded) {
		nextAt := int(rounded[lastActiveIdx+1].at)
		gap := nextAt - lastAt
		if gap <= gapThresholdMS {
			return nextAt
		}
		return lastAt + int(tail)
	}
	return lastAt + int(tail)
}

// EffectiveScriptDurationMS returns timeline length for stats and playback.
func EffectiveScriptDurationMS(actions []Action, metadata map[string]any) int {
	rounded := roundActionsForHeatmap(actions)
	if len(rounded) == 0 {
		return 0
	}

	metaMS := MetadataDurationMS(metadata)
	motionEnd := MotionPlaybackEndMS(actions)
	firstAt := int(rounded[0].at)

	if len(rounded) < 2 {
		if metaMS > 0 {
			return metaMS
		}
		if motionEnd > 0 {
			return motionEnd
		}
		return firstAt
	}

	activeSpan := maxInt(1, motionEnd-firstAt)
	if metaMS > 0 {
		trailing := metaMS - motionEnd
		if trailing > gapThresholdMS {
			motionSpan := maxInt(1, motionEnd-firstAt)
			if trailing > 15000 || float64(trailing)/float64(motionSpan) > 0.25 {
				if firstAt <= 0 {
					return activeSpan
				}
				return motionEnd
			}
		}
		return metaMS
	}
	if firstAt <= 0 {
		return activeSpan
	}
	return motionEnd
}

// TrimActionsForPlayback drops trailing dead keyframes for device playback.
func TrimActionsForPlayback(actions []Action, metadata map[string]any) []Action {
	if len(actions) < 2 {
		return append([]Action(nil), actions...)
	}

	ordered := append([]Action(nil), actions...)
	for i := 0; i < len(ordered)-1; i++ {
		for j := i + 1; j < len(ordered); j++ {
			if ordered[j].At < ordered[i].At {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}

	lastAt := ordered[len(ordered)-1].At
	endMS := MotionPlaybackEndMS(ordered)
	metaMS := MetadataDurationMS(metadata)

	trailing := lastAt - endMS
	metaTrailing := 0
	if metaMS > 0 {
		metaTrailing = metaMS - endMS
	}
	motionSpan := maxInt(1, endMS-ordered[0].At)
	shouldTrim := trailing > gapThresholdMS || (metaTrailing > gapThresholdMS && (metaTrailing > 15000 || float64(metaTrailing)/float64(motionSpan) > 0.25))
	if !shouldTrim {
		return ordered
	}

	trimmed := make([]Action, 0, len(ordered))
	for _, item := range ordered {
		if item.At <= endMS {
			trimmed = append(trimmed, item)
		}
	}
	if len(trimmed) >= 2 {
		return trimmed
	}
	return ordered
}
