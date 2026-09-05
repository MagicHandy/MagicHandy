package motion

import (
	"errors"
	"math"
)

// resolveExperimentalRhythm keeps relative leg timing when the desired mean
// pace is infeasible. Unlike production Creative's pace-priority local fit,
// this lab policy accepts a slower average instead of silently equalizing the
// outbound/return rhythm. The same exact C2 envelope checks remain mandatory.
func resolveExperimentalRhythm(content resolvedContent, authored []int64, target int64, peak float64) ([]int64, error) {
	if len(authored)%2 != 0 {
		return nil, errors.New("experimental rhythm requires complete two-leg strokes")
	}
	scale := float64(target) / float64(sumDynamicDurations(authored))
	candidate := make([]int64, len(authored))
	for index, duration := range authored {
		candidate[index] = max(int64(1), int64(math.Ceil(float64(duration)*scale)))
	}
	for range maximumDynamicTimingFitPasses {
		fitted, err := fitDynamicDurationsToEnvelope(content, candidate, peak)
		if err != nil {
			return nil, err
		}
		changed := false
		for index := 0; index < len(candidate); index += 2 {
			// Stretch a complete stroke together. One difficult reversal must
			// not slow the entire long phrase, nor erase the return-time ratio.
			factor := math.Max(float64(fitted[index])/float64(candidate[index]),
				float64(fitted[index+1])/float64(candidate[index+1]))
			if factor <= 1 {
				continue
			}
			for leg := index; leg < index+2; leg++ {
				candidate[leg] = int64(math.Ceil(float64(candidate[leg]) * factor * 1.002))
			}
			changed = true
		}
		if !changed {
			return fitted, nil
		}
	}
	return nil, errors.New("experimental rhythm did not converge inside the runtime envelope")
}
