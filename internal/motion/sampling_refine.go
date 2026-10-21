package motion

import (
	"math"
	"sort"
)

func (e *Engine) refineContinuousSamples(samples, reference []MotionSample, mandatory map[int64]struct{}, resolution float64, hasPrevious, inTransition bool) []MotionSample {
	samples = removeRedundantQuantizedSamples(samples, resolution, mandatory)
	if hasPrevious {
		samples = removeLeadingQuantizedDuplicates(samples, *e.lastSample, resolution, mandatory)
		samples = append([]MotionSample{*e.lastSample}, samples...)
	}
	// Removing duplicate positions can erase easing near a reversal. Restore
	// distinct points against the path, including the immutable append tail.
	samples = refineQuantizedMotionSamples(samples, reference, resolution)
	if inTransition {
		samples = stabilizeTransitionSamples(samples, mandatory)
	}
	if hasPrevious {
		samples = samples[1:]
	}
	return samples
}

// refineQuantizedMotionSamples checks the final quantized segments after
// stationary-edge removal. Splits come only from the shared path probes and
// must round differently from both segment ends. Mandatory reversals, prior
// append tails and intentional holds stay intact. The usual chunk point cap
// still rejects content that cannot be represented within transport capacity.
func refineQuantizedMotionSamples(samples, probes []MotionSample, resolution float64) []MotionSample {
	if len(samples) < 2 || len(probes) < 3 {
		return samples
	}
	tolerance := quantizedProbeTolerance(probes, resolution)
	result := []MotionSample{samples[0]}
	var split func(MotionSample, MotionSample)
	split = func(left, right MotionSample) {
		lo := sort.Search(len(probes), func(i int) bool { return probes[i].TimeMillis > left.TimeMillis })
		hi := sort.Search(len(probes), func(i int) bool { return probes[i].TimeMillis >= right.TimeMillis })
		leftPosition := quantizedMotionPosition(left.PositionPercent, resolution)
		rightPosition := quantizedMotionPosition(right.PositionPercent, resolution)
		worstError, candidateError, candidate := 0.0, -1.0, -1
		for index := lo; index < hi; index++ {
			probe := probes[index]
			fraction := float64(probe.TimeMillis-left.TimeMillis) / float64(right.TimeMillis-left.TimeMillis)
			errorValue := math.Abs(probe.PositionPercent - (leftPosition + fraction*(rightPosition-leftPosition)))
			worstError = math.Max(worstError, errorValue)
			position := quantizedMotionPosition(probe.PositionPercent, resolution)
			if position != leftPosition && position != rightPosition && errorValue > candidateError {
				candidate, candidateError = index, errorValue
			}
		}
		if worstError > tolerance && candidate >= 0 {
			split(left, probes[candidate])
			split(probes[candidate], right)
			return
		}
		result = append(result, right)
	}
	for index := 1; index < len(samples); index++ {
		split(samples[index-1], samples[index])
	}
	return result
}

// Probe agreement alone can hide a small error crest between probes. Reserve
// the linear-interpolation error A*dt²/8 under the existing acceleration bound,
// so a faster rounded curve still meets the same 0.8% whole-percent wire bound.
func quantizedProbeTolerance(probes []MotionSample, resolution float64) float64 {
	var gap int64
	for i := 1; i < len(probes); i++ {
		gap = max(gap, probes[i].TimeMillis-probes[i-1].TimeMillis)
	}
	reserve := runtimeMaxAccelerationPercentPerSecond2 * float64(gap) * float64(gap) / 8e6
	return resolution/2 + math.Max(0, wireApproximationTolerance-reserve)
}
