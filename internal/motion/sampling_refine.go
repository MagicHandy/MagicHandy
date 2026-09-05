package motion

import (
	"math"
	"sort"
)

// refineQuantizedMotionSamples checks the final quantized segments after
// stationary-edge removal. Splits come only from the shared path probes and
// must round differently from both segment ends. Mandatory reversals, prior
// append tails and intentional holds stay intact. The usual chunk point cap
// still rejects content that cannot be represented within transport capacity.
func refineQuantizedMotionSamples(samples, probes []MotionSample, resolution float64) []MotionSample {
	if len(samples) < 2 || len(probes) < 3 {
		return samples
	}
	tolerance := wireApproximationTolerance + resolution/2
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
