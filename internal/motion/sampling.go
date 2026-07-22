package motion

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

const (
	wireApproximationTolerance  = 0.3
	maximumAdaptiveChunkPoints  = 128
	bufferedProbeIntervalMillis = int64(25)
)

// knotTimesBetween maps authored curve knots onto stream time. Buffered owners
// can therefore preserve a short reversal or dwell even when it falls between
// unrelated fixed sampler ticks.
func (p MotionPlan) knotTimesBetween(startMillis, endMillis int64) []int64 {
	if endMillis <= startMillis || p.PeriodMillis <= 0 || p.curve.duration <= 0 {
		return nil
	}
	times := make([]int64, 0, min(len(p.curve.points), 32))
	if !p.Loop {
		streamTime := func(index int) int64 {
			phase := float64(p.curve.points[index].TimeMillis) / float64(p.curve.duration)
			return p.HandoffMillis + int64(math.Round((phase-p.PhaseOffset)*float64(p.PeriodMillis)))
		}
		first := sort.Search(len(p.curve.points), func(index int) bool {
			return streamTime(index) >= startMillis
		})
		for index := first; index < len(p.curve.points); index++ {
			at := streamTime(index)
			if at >= endMillis {
				break
			}
			if at >= p.HandoffMillis {
				times = append(times, at)
			}
		}
		return uniqueMillis(times)
	}

	for _, point := range p.curve.points {
		phase := float64(point.TimeMillis) / float64(p.curve.duration)

		firstCycle := int64(math.Ceil(
			float64(startMillis-p.HandoffMillis)/float64(p.PeriodMillis) - phase + p.PhaseOffset,
		))
		for cycle := firstCycle; ; cycle++ {
			at := p.HandoffMillis + int64(math.Round(
				(phase-p.PhaseOffset+float64(cycle))*float64(p.PeriodMillis),
			))
			if at >= endMillis {
				break
			}
			if at >= startMillis {
				times = append(times, at)
			}
		}
	}
	sort.Slice(times, func(left, right int) bool { return times[left] < times[right] })
	return uniqueMillis(times)
}

func (e *Engine) nextMotionSamplesLocked() ([]MotionSample, error) {
	intervalMillis := e.sampleInterval.Milliseconds()
	chunkStart := e.nextSampleMillis
	chunkEnd := chunkStart + int64(e.chunkSize)*intervalMillis
	transitionInChunk := transitionOverlaps(e.transition, chunkStart, chunkEnd)
	probeIntervalMillis := intervalMillis
	if e.preservePlanKnots && probeIntervalMillis > bufferedProbeIntervalMillis {
		probeIntervalMillis = bufferedProbeIntervalMillis
	}
	maximumPoints := e.maximumChunkPoints
	if maximumPoints < 2 {
		maximumPoints = maximumAdaptiveChunkPoints
	}
	minimumBoundedProbe := max(int64(1), (chunkEnd-chunkStart+int64(maximumPoints)-1)/int64(maximumPoints))
	probeIntervalMillis = max(probeIntervalMillis, minimumBoundedProbe)
	times := make([]int64, 0, int((chunkEnd-chunkStart)/probeIntervalMillis)+min(len(e.plan.curve.points), maximumPoints))
	for streamMillis := chunkStart; streamMillis < chunkEnd; streamMillis += probeIntervalMillis {
		times = append(times, streamMillis)
	}
	if e.preservePlanKnots {
		times = append(times, motionPathKnotTimes(e.plan, e.transition, chunkStart, chunkEnd)...)
	}
	mandatory := make(map[int64]struct{}, 2)
	if transitionInChunk {
		for _, at := range transitionBoundaryTimes(e.transition, chunkStart, chunkEnd) {
			times = append(times, at)
			mandatory[at] = struct{}{}
		}
	}
	sort.Slice(times, func(left, right int) bool { return times[left] < times[right] })
	times = uniqueMillis(times)

	samples := make([]MotionSample, 0, len(times)+1)
	hasPreviousAnchor := e.lastSample != nil && (len(times) == 0 || e.lastSample.TimeMillis < times[0])
	if hasPreviousAnchor {
		samples = append(samples, *e.lastSample)
	}
	for _, streamMillis := range times {
		samples = append(samples, sampleMotionPath(e.plan, e.transition, streamMillis))
	}
	if transitionInChunk {
		samples = stabilizeTransitionSamples(samples, mandatory)
	}
	samples = simplifyMotionSamples(samples, wireApproximationTolerance, mandatory)
	if transitionInChunk {
		samples = stabilizeTransitionSamples(samples, mandatory)
	}
	positionResolution := e.effectivePositionResolutionPercentLocked()
	if positionResolution > 0 {
		samples = simplifyQuantizedMotionSamples(
			samples,
			positionResolution,
			wireApproximationTolerance+positionResolution/2,
			mandatory,
		)
	}
	if hasPreviousAnchor {
		samples = samples[1:]
	}
	if len(samples) == 0 {
		return nil, errors.New("motion sampler produced an empty output window")
	}
	if len(samples) > maximumPoints {
		return nil, fmt.Errorf(
			"motion content has %d essential points in the %dms output window; trim or slow the content",
			len(samples), chunkEnd-chunkStart,
		)
	}

	e.nextSampleMillis = chunkEnd
	lastSample := samples[len(samples)-1]
	e.lastSample = &lastSample
	return samples, nil
}

func (e *Engine) effectivePositionResolutionPercentLocked() float64 {
	resolution := e.positionResolutionPercent
	if resolution <= 0 || !e.resolutionAfterStrokeWindow {
		return resolution
	}
	strokeSpan := e.settings.StrokeMaxPercent - e.settings.StrokeMinPercent
	if strokeSpan <= 0 {
		return resolution
	}
	return math.Min(100, resolution*100/float64(strokeSpan))
}

func stabilizeTransitionSamples(samples []MotionSample, mandatory map[int64]struct{}) []MotionSample {
	result := append([]MotionSample(nil), samples...)
	for len(result) > 2 {
		anchors := motionSampleReversalAnchors(result)
		removed := false
		for index := 1; index < len(anchors)-1; index++ {
			pointIndex := anchors[index]
			if _, protected := mandatory[result[pointIndex].TimeMillis]; protected {
				continue
			}
			left := result[anchors[index-1]]
			current := result[pointIndex]
			right := result[anchors[index+1]]
			prominence := math.Min(
				math.Abs(current.PositionPercent-left.PositionPercent),
				math.Abs(current.PositionPercent-right.PositionPercent),
			)
			flankMillis := min(
				current.TimeMillis-left.TimeMillis,
				right.TimeMillis-current.TimeMillis,
			)
			if prominence > MinimumPatternReversalProminence ||
				flankMillis > maximumPatternChatterFlankMillis {
				continue
			}
			result = append(result[:pointIndex], result[pointIndex+1:]...)
			removed = true
			break
		}
		if !removed {
			break
		}
	}
	return result
}

func motionSampleReversalAnchors(samples []MotionSample) []int {
	anchors := []int{0}
	previousDirection := 0
	for index := 1; index < len(samples); index++ {
		direction := curveDirection(samples[index].PositionPercent - samples[index-1].PositionPercent)
		if direction == 0 {
			continue
		}
		if previousDirection != 0 && direction != previousDirection {
			anchors = append(anchors, index-1)
		}
		previousDirection = direction
	}
	last := len(samples) - 1
	if anchors[len(anchors)-1] != last {
		anchors = append(anchors, last)
	}
	return anchors
}

func simplifyMotionSamples(samples []MotionSample, tolerance float64, mandatory map[int64]struct{}) []MotionSample {
	if len(samples) <= 2 {
		return append([]MotionSample(nil), samples...)
	}
	anchors := []int{0}
	for index := 1; index < len(samples)-1; index++ {
		if _, ok := mandatory[samples[index].TimeMillis]; ok {
			anchors = append(anchors, index)
		}
	}
	anchors = append(anchors, len(samples)-1)

	keep := make([]bool, len(samples))
	for index := 1; index < len(anchors); index++ {
		keep[anchors[index-1]] = true
		keep[anchors[index]] = true
		markMotionSamples(samples, anchors[index-1], anchors[index], tolerance, keep)
	}
	result := make([]MotionSample, 0, len(samples))
	for index, sample := range samples {
		if keep[index] {
			result = append(result, sample)
		}
	}
	return result
}

func markMotionSamples(samples []MotionSample, start, end int, tolerance float64, keep []bool) {
	if end-start <= 1 {
		return
	}
	maximumError := 0.0
	maximumIndex := -1
	for index := start + 1; index < end; index++ {
		errorValue := motionSampleError(samples[start], samples[end], samples[index])
		if errorValue > maximumError {
			maximumError = errorValue
			maximumIndex = index
		}
	}
	if maximumIndex < 0 || maximumError <= tolerance {
		return
	}
	keep[maximumIndex] = true
	markMotionSamples(samples, start, maximumIndex, tolerance, keep)
	markMotionSamples(samples, maximumIndex, end, tolerance, keep)
}

func motionSampleError(left, right, sample MotionSample) float64 {
	duration := right.TimeMillis - left.TimeMillis
	if duration <= 0 {
		return math.Abs(sample.PositionPercent - right.PositionPercent)
	}
	fraction := float64(sample.TimeMillis-left.TimeMillis) / float64(duration)
	expected := left.PositionPercent + (right.PositionPercent-left.PositionPercent)*fraction
	return math.Abs(sample.PositionPercent - expected)
}

func simplifyQuantizedMotionSamples(
	samples []MotionSample,
	resolution float64,
	tolerance float64,
	mandatory map[int64]struct{},
) []MotionSample {
	if len(samples) <= 2 || resolution <= 0 {
		return append([]MotionSample(nil), samples...)
	}
	anchors := []int{0}
	for index := 1; index < len(samples)-1; index++ {
		if _, ok := mandatory[samples[index].TimeMillis]; ok {
			anchors = append(anchors, index)
		}
	}
	anchors = append(anchors, len(samples)-1)

	keep := make([]bool, len(samples))
	for index := 1; index < len(anchors); index++ {
		keep[anchors[index-1]] = true
		keep[anchors[index]] = true
		markQuantizedMotionSamples(samples, anchors[index-1], anchors[index], resolution, tolerance, keep)
	}
	result := make([]MotionSample, 0, len(samples))
	for index, sample := range samples {
		if keep[index] {
			result = append(result, sample)
		}
	}
	return result
}

func markQuantizedMotionSamples(
	samples []MotionSample,
	start int,
	end int,
	resolution float64,
	tolerance float64,
	keep []bool,
) {
	if end-start <= 1 {
		return
	}
	left := quantizedMotionPosition(samples[start].PositionPercent, resolution)
	right := quantizedMotionPosition(samples[end].PositionPercent, resolution)
	duration := samples[end].TimeMillis - samples[start].TimeMillis
	maximumError := 0.0
	maximumIndex := -1
	for index := start + 1; index < end; index++ {
		fraction := float64(samples[index].TimeMillis-samples[start].TimeMillis) / float64(duration)
		expected := left + (right-left)*fraction
		errorValue := math.Abs(samples[index].PositionPercent - expected)
		if errorValue > maximumError {
			maximumError = errorValue
			maximumIndex = index
		}
	}
	if maximumIndex < 0 || maximumError <= tolerance {
		return
	}
	keep[maximumIndex] = true
	markQuantizedMotionSamples(samples, start, maximumIndex, resolution, tolerance, keep)
	markQuantizedMotionSamples(samples, maximumIndex, end, resolution, tolerance, keep)
}

func quantizedMotionPosition(position float64, resolution float64) float64 {
	return math.Max(0, math.Min(100, math.Round(position/resolution)*resolution))
}

func uniqueMillis(values []int64) []int64 {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
