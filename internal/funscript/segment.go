package funscript

const (
	minBlockMS           = 1500
	targetMaxBlockMS     = 30000
	absMaxBlockMS        = 45000
	pauseGapMS           = 1500
	zoneTop              = 67.0
	zoneBottom           = 33.0
	speedChangeRatio     = 2.0
	amplitudeChangeRatio = 1.8
)

// ActionSegment is a contiguous slice of actions representing one motion block.
type ActionSegment struct {
	Actions    []Action
	StartIndex int
	EndIndex   int
}

func (s ActionSegment) StartMS() int { return s.Actions[0].At }
func (s ActionSegment) EndMS() int   { return s.Actions[len(s.Actions)-1].At }
func (s ActionSegment) DurationMS() int {
	return s.EndMS() - s.StartMS()
}

func zoneOf(pos float64) string {
	switch {
	case pos >= zoneTop:
		return "top"
	case pos <= zoneBottom:
		return "bottom"
	default:
		return "middle"
	}
}

func segmentSpeeds(actions []Action) []float64 {
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

func segmentAmplitude(actions []Action) float64 {
	minP, maxP := actions[0].Pos, actions[0].Pos
	for _, action := range actions[1:] {
		if action.Pos < minP {
			minP = action.Pos
		}
		if action.Pos > maxP {
			maxP = action.Pos
		}
	}
	return maxP - minP
}

func dominantZone(actions []Action) string {
	counts := map[string]int{"top": 0, "middle": 0, "bottom": 0}
	for _, action := range actions {
		counts[zoneOf(action.Pos)]++
	}
	best := "middle"
	bestCount := -1
	for zone, count := range counts {
		if count > bestCount {
			best = zone
			bestCount = count
		}
	}
	return best
}

func isPause(actions []Action, index int) bool {
	if index <= 0 || index >= len(actions) {
		return false
	}
	prev := actions[index-1]
	current := actions[index]
	gap := current.At - prev.At
	if gap < pauseGapMS {
		return false
	}
	posDelta := abs(current.Pos - prev.Pos)
	return posDelta < 3.0 || gap >= pauseGapMS*2
}

func shouldSplit(left, right []Action, forceMaxDuration bool) bool {
	if len(left) < 2 || len(right) < 2 {
		return false
	}

	leftDuration := left[len(left)-1].At - left[0].At
	if forceMaxDuration && leftDuration >= targetMaxBlockMS {
		return true
	}

	leftAmp := segmentAmplitude(left)
	rightAmp := segmentAmplitude(right)
	if leftAmp >= 5 && rightAmp >= 5 {
		ratio := max(leftAmp, rightAmp) / max(min(leftAmp, rightAmp), 1.0)
		if ratio >= amplitudeChangeRatio {
			return true
		}
	}

	leftSpeeds := segmentSpeeds(left)
	rightSpeeds := segmentSpeeds(right)
	if len(leftSpeeds) > 0 && len(rightSpeeds) > 0 {
		leftAvg := mean(leftSpeeds)
		rightAvg := mean(rightSpeeds)
		if leftAvg > 0 && rightAvg > 0 {
			speedRatio := max(leftAvg, rightAvg) / min(leftAvg, rightAvg)
			if speedRatio >= speedChangeRatio {
				return true
			}
		}
	}

	if dominantZone(left) != dominantZone(right) {
		leftMin, leftMax := left[0].Pos, left[0].Pos
		rightMin, rightMax := right[0].Pos, right[0].Pos
		for _, action := range left {
			if action.Pos < leftMin {
				leftMin = action.Pos
			}
			if action.Pos > leftMax {
				leftMax = action.Pos
			}
		}
		for _, action := range right {
			if action.Pos < rightMin {
				rightMin = action.Pos
			}
			if action.Pos > rightMax {
				rightMax = action.Pos
			}
		}
		if leftMax < zoneBottom && rightMin > zoneTop {
			return true
		}
		if rightMax < zoneBottom && leftMin > zoneTop {
			return true
		}
		leftMean := mean(positions(left))
		rightMean := mean(positions(right))
		if abs(leftMean-rightMean) >= 25 {
			return true
		}
	}
	return false
}

func findSplitPoints(actions []Action) []int {
	if len(actions) < 3 {
		return nil
	}

	points := map[int]struct{}{}
	blockStart := 0

	for index := 1; index < len(actions); index++ {
		if isPause(actions, index) {
			points[index] = struct{}{}
		}

		segmentDuration := actions[index-1].At - actions[blockStart].At
		forceMax := segmentDuration >= targetMaxBlockMS

		if index-blockStart >= 2 {
			left := actions[blockStart:index]
			rightEnd := index + maxInt(2, len(actions)-index)
			if rightEnd > len(actions) {
				rightEnd = len(actions)
			}
			right := actions[index:rightEnd]
			if shouldSplit(left, right, forceMax) {
				points[index] = struct{}{}
			}
		}

		if forceMax {
			points[index] = struct{}{}
			blockStart = index
		}
	}

	out := make([]int, 0, len(points))
	for point := range points {
		out = append(out, point)
	}
	for i := 0; i < len(out)-1; i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func buildSegments(actions []Action, splitPoints []int) []ActionSegment {
	if len(actions) == 0 {
		return nil
	}

	boundaries := append([]int{0}, splitPoints...)
	boundaries = append(boundaries, len(actions))

	unique := make([]int, 0, len(boundaries))
	for _, boundary := range boundaries {
		if len(unique) == 0 || boundary != unique[len(unique)-1] {
			unique = append(unique, boundary)
		}
	}

	segments := make([]ActionSegment, 0, len(unique))
	for i := 0; i < len(unique)-1; i++ {
		start, end := unique[i], unique[i+1]
		if end <= start {
			continue
		}
		chunk := actions[start:end]
		if len(chunk) >= 2 {
			segments = append(segments, ActionSegment{
				Actions:    append([]Action(nil), chunk...),
				StartIndex: start,
				EndIndex:   end - 1,
			})
		}
	}
	return segments
}

func mergeShortSegments(segments []ActionSegment) []ActionSegment {
	if len(segments) == 0 {
		return nil
	}

	merged := make([]ActionSegment, 0, len(segments))
	var buffer *ActionSegment

	for _, segment := range segments {
		if buffer == nil {
			copySeg := segment
			buffer = &copySeg
			continue
		}

		if buffer.DurationMS() < minBlockMS || (buffer.DurationMS()+segment.DurationMS() <= targetMaxBlockMS && segment.DurationMS() < minBlockMS) {
			combined := append(append([]Action(nil), buffer.Actions...), segment.Actions[1:]...)
			buffer = &ActionSegment{
				Actions:    combined,
				StartIndex: buffer.StartIndex,
				EndIndex:   segment.EndIndex,
			}
			continue
		}
		merged = append(merged, *buffer)
		copySeg := segment
		buffer = &copySeg
	}

	if buffer != nil {
		if len(merged) > 0 && buffer.DurationMS() < minBlockMS {
			prev := merged[len(merged)-1]
			combined := append(append([]Action(nil), prev.Actions...), buffer.Actions[1:]...)
			merged[len(merged)-1] = ActionSegment{
				Actions:    combined,
				StartIndex: prev.StartIndex,
				EndIndex:   buffer.EndIndex,
			}
		} else {
			merged = append(merged, *buffer)
		}
	}
	return merged
}

func splitLongSegments(segments []ActionSegment) []ActionSegment {
	output := make([]ActionSegment, 0, len(segments))

	for _, segment := range segments {
		if segment.DurationMS() <= targetMaxBlockMS {
			output = append(output, segment)
			continue
		}

		actions := segment.Actions
		chunkStart := 0
		for index := 2; index < len(actions); index++ {
			chunkDuration := actions[index-1].At - actions[chunkStart].At
			if chunkDuration >= targetMaxBlockMS {
				chunk := actions[chunkStart:index]
				if len(chunk) >= 2 {
					output = append(output, ActionSegment{
						Actions:    append([]Action(nil), chunk...),
						StartIndex: segment.StartIndex + chunkStart,
						EndIndex:   segment.StartIndex + index - 1,
					})
				}
				chunkStart = index - 1
			}
		}

		tail := actions[chunkStart:]
		if len(tail) >= 2 {
			output = append(output, ActionSegment{
				Actions:    append([]Action(nil), tail...),
				StartIndex: segment.StartIndex + chunkStart,
				EndIndex:   segment.EndIndex,
			})
		}
	}
	return output
}

// SegmentActions splits a normalized action list into motion blocks.
func SegmentActions(actions []Action) [][]Action {
	if len(actions) < 2 {
		return nil
	}

	splitPoints := findSplitPoints(actions)
	segments := buildSegments(actions, splitPoints)
	segments = mergeShortSegments(segments)
	segments = splitLongSegments(segments)
	segments = mergeShortSegments(segments)

	blocks := make([][]Action, 0, len(segments))
	for _, segment := range segments {
		if len(segment.Actions) >= 2 {
			blocks = append(blocks, append([]Action(nil), segment.Actions...))
		}
	}
	return blocks
}

func positions(actions []Action) []float64 {
	out := make([]float64, len(actions))
	for i, action := range actions {
		out[i] = action.Pos
	}
	return out
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
