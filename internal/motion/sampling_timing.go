package motion

import (
	"fmt"
	"math"
)

// fitSpacedMotionSamples retains true reversals and fits each intervening leg
// from the dense shared-path probes. The timing floor limits emitted commands,
// not the resolution at which we inspect the curve. Aliasing an unrepresentable
// reversal would change direction/rhythm, so reject it instead.
func fitSpacedMotionSamples(probes []MotionSample, floor int64) ([]MotionSample, map[int64]struct{}, error) {
	anchors := motionSampleReversalAnchors(probes)
	// A lookahead window can end just after a turn. End at that turn rather
	// than forcing a short artificial tail; the next window will fit its leg.
	if len(anchors) > 2 && probes[anchors[len(anchors)-1]].TimeMillis-probes[anchors[len(anchors)-2]].TimeMillis < floor {
		anchors = anchors[:len(anchors)-1]
	}
	mandatory := make(map[int64]struct{}, len(anchors))
	mandatory[probes[0].TimeMillis] = struct{}{}
	result := []MotionSample{probes[0]}
	var fit func(int, int)
	fit = func(left, right int) {
		worst, split := wireApproximationTolerance, -1
		for index := left + 1; index < right; index++ {
			if probes[index].TimeMillis-probes[left].TimeMillis < floor || probes[right].TimeMillis-probes[index].TimeMillis < floor {
				continue
			}
			if deviation := motionSampleError(probes[left], probes[right], probes[index]); deviation > worst {
				worst, split = deviation, index
			}
		}
		if split >= 0 {
			fit(left, split)
			fit(split, right)
			return
		}
		result = append(result, probes[right])
	}
	for index := 1; index < len(anchors); index++ {
		left, right := anchors[index-1], anchors[index]
		if probes[right].TimeMillis-probes[left].TimeMillis < floor {
			return nil, nil, fmt.Errorf("motion reversal at %dms after %dms is shorter than the transport's %dms command interval; slow the motion or use a buffered transport", probes[right].TimeMillis, probes[left].TimeMillis, floor)
		}
		fit(left, right)
		// The final probe is a window boundary, not necessarily a true knot.
		if right < len(probes)-1 {
			mandatory[probes[right].TimeMillis] = struct{}{}
		}
	}
	return result, mandatory, nil
}

// fitQuantizedMotionTiming aligns optional points with the time the plan
// reaches their encoded position. Rounding position while keeping its original
// time creates avoidable speed pulses, particularly beside a reversal. Fixed
// knots and the already submitted append tail keep their exact times.
// A final interval projection enforces the velocity ceiling after rounding.
func fitQuantizedMotionTiming(samples, probes []MotionSample, mandatory map[int64]struct{}, resolution, maximumVelocity float64, floor int64) ([]MotionSample, error) {
	if len(samples) < 2 {
		return samples, nil
	}
	result := append([]MotionSample(nil), samples...)
	encoded := func(sample MotionSample) float64 {
		if resolution <= 0 {
			return sample.PositionPercent
		}
		return quantizedMotionPosition(sample.PositionPercent, resolution)
	}
	minimum := func(left, right MotionSample) int64 {
		return max(floor, int64(math.Ceil(math.Abs(encoded(right)-encoded(left))*1000/maximumVelocity-1e-9)))
	}
	for start := 0; start < len(result)-1; {
		end := start + 1
		for end < len(result)-1 {
			if _, fixed := mandatory[result[end].TimeMillis]; fixed {
				break
			}
			end++
		}
		// Integer millisecond rounding can consume the slack of a very fast
		// straight leg. Remove only optional samples, never a reversal/hold.
		for {
			var needed int64
			for index := start + 1; index <= end; index++ {
				needed += minimum(result[index-1], result[index])
			}
			if needed <= result[end].TimeMillis-result[start].TimeMillis {
				break
			}
			if end-start == 1 {
				return nil, fmt.Errorf("quantized motion exceeds the transport timing or velocity limit between %dms and %dms; slow the motion", result[start].TimeMillis, result[end].TimeMillis)
			}
			remove, cost := start+1, math.Inf(1)
			for index := start + 1; index < end; index++ {
				if deviation := motionSampleError(result[index-1], result[index+1], result[index]); deviation < cost {
					remove, cost = index, deviation
				}
			}
			result = append(result[:remove], result[remove+1:]...)
			end--
		}
		latest := make([]int64, end-start+1)
		latest[end-start] = result[end].TimeMillis
		for index := end - 1; index > start; index-- {
			latest[index-start] = latest[index+1-start] - minimum(result[index], result[index+1])
		}
		for index := start + 1; index < end; index++ {
			at := result[index].TimeMillis
			if resolution > 0 && floor <= 1 {
				at = smootherQuantizedTime(result, probes, index, start, end, resolution, minimum)
			}
			result[index].TimeMillis = min(latest[index-start], max(result[index-1].TimeMillis+minimum(result[index-1], result[index]), at))
		}
		start = end
	}
	return result, nil
}

func smootherQuantizedTime(samples, probes []MotionSample, index, start, end int, resolution float64, minimum func(MotionSample, MotionSample) int64) int64 {
	original := samples[index].TimeMillis
	position := quantizedMotionPosition(samples[index].PositionPercent, resolution)
	candidate := quantizedCrossingTime(probes, samples[index], position, samples[start].TimeMillis, samples[end].TimeMillis)
	// A rounded extremum may be unreachable on this leg. Never chase it on a
	// different stroke, or trade a new speed pulse for positional accuracy.
	if absMillis(candidate-original) > wireTimingApproximationFloorMillis {
		return original
	}
	point := samples[index]
	point.TimeMillis = candidate
	before := localWireVelocityJump(samples, index, resolution)
	samples[index].TimeMillis = candidate
	after := localWireVelocityJump(samples, index, resolution)
	samples[index].TimeMillis = original
	if after < before && candidate-samples[index-1].TimeMillis >= minimum(samples[index-1], point) &&
		samples[index+1].TimeMillis-candidate >= minimum(point, samples[index+1]) &&
		wireSegmentFits(samples[index-1], point, probes, resolution) && wireSegmentFits(point, samples[index+1], probes, resolution) {
		return candidate
	}
	return original
}

func localWireVelocityJump(samples []MotionSample, at int, resolution float64) float64 {
	var previous, largest float64
	start := max(1, at-1)
	for index := start; index <= min(len(samples)-1, at+2); index++ {
		duration := samples[index].TimeMillis - samples[index-1].TimeMillis
		if duration <= 0 {
			return math.Inf(1)
		}
		velocity := (quantizedMotionPosition(samples[index].PositionPercent, resolution) - quantizedMotionPosition(samples[index-1].PositionPercent, resolution)) * 1000 / float64(duration)
		if index > start {
			largest = math.Max(largest, math.Abs(velocity-previous))
		}
		previous = velocity
	}
	return largest
}

func absMillis(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func wireSegmentFits(left, right MotionSample, probes []MotionSample, resolution float64) bool {
	if right.TimeMillis <= left.TimeMillis {
		return false
	}
	l, r := quantizedMotionPosition(left.PositionPercent, resolution), quantizedMotionPosition(right.PositionPercent, resolution)
	for _, probe := range probes {
		if probe.TimeMillis < left.TimeMillis || probe.TimeMillis > right.TimeMillis {
			continue
		}
		fraction := float64(probe.TimeMillis-left.TimeMillis) / float64(right.TimeMillis-left.TimeMillis)
		if math.Abs(l+(r-l)*fraction-probe.PositionPercent) > wireApproximationTolerance+resolution/2 {
			return false
		}
	}
	return true
}

func quantizedCrossingTime(probes []MotionSample, sample MotionSample, position float64, start, end int64) int64 {
	best, distance := sample.TimeMillis, math.Inf(1)
	for index := 1; index < len(probes); index++ {
		left, right := probes[index-1], probes[index]
		if left.TimeMillis < start || right.TimeMillis > end {
			continue
		}
		delta := right.PositionPercent - left.PositionPercent
		if delta == 0 || position < math.Min(left.PositionPercent, right.PositionPercent) || position > math.Max(left.PositionPercent, right.PositionPercent) {
			continue
		}
		at := float64(left.TimeMillis) + (position-left.PositionPercent)/delta*float64(right.TimeMillis-left.TimeMillis)
		if gap := math.Abs(at - float64(sample.TimeMillis)); gap < distance {
			best, distance = int64(math.Round(at)), gap
		}
	}
	return best
}
