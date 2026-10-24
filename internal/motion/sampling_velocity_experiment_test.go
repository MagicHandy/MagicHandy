package motion

// Rejected production experiment retained for inert comparison. Hardware
// auditions favored fewer points despite this approach's numerical gains.

import "math"

func (e *Engine) refineBufferedVelocityChunk(samples []MotionSample, resolution float64, maximumPoints int) []MotionSample {
	if !e.preservePlanKnots || e.minimumPointIntervalMillis > 0 || resolution <= 0 || !e.plan.Target.continuousCurve() || len(samples) < 4 {
		return samples
	}
	start, end := samples[0].TimeMillis, samples[len(samples)-1].TimeMillis
	if transitionOverlaps(e.transition, start, end) {
		return samples
	}
	step := max(bufferedProbeIntervalMillis, (end-start+maximumInternalProbePoints-1)/maximumInternalProbePoints)
	probes := make([]MotionSample, 0, int((end-start)/step)+2)
	for at := start; at < end; at += step {
		probes = append(probes, sampleMotionPath(e.plan, e.transition, at))
	}
	probes = append(probes, sampleMotionPath(e.plan, e.transition, end))
	result := refineBufferedVelocity(samples, probes, resolution, referenceTravelRateForSpeed(100, e.settings.HandyModel), e.plan.SampleAt)
	if len(result) > maximumPoints || bufferedMaximumVelocityStep(result, resolution) > bufferedMaximumVelocityStep(samples, resolution)+1e-9 {
		return samples
	}
	return result
}

// refineBufferedVelocity evaluates representable splits against the shared
// curve's velocity. It keeps the clock, reversals and submitted endpoints.
// Immediate transports keep their existing command-spacing fitter.
func refineBufferedVelocity(samples, probes []MotionSample, resolution, maximumVelocity float64, sampleAt func(int64) MotionSample) []MotionSample {
	if len(samples) < 2 || len(probes) < 3 {
		return samples
	}
	const minimumSpacing = int64(40)
	tolerance := quantizedProbeTolerance(probes, resolution, wireApproximationTolerance)
	result := []MotionSample{samples[0]}
	var split func(MotionSample, MotionSample)
	split = func(left, right MotionSample) {
		duration := right.TimeMillis - left.TimeMillis
		from, to := quantizedMotionPosition(left.PositionPercent, resolution), quantizedMotionPosition(right.PositionPercent, resolution)
		cost := bufferedVelocityError(left, right, probes, resolution)
		mean := (to - from) / float64(duration)
		bestCost, found := cost*.8, false
		var best MotionSample
		if duration >= 2*minimumSpacing && math.Abs(to-from) > resolution && cost > mean*mean*float64(duration)*.0144 {
			for _, fraction := range []float64{.25, .5, .75} {
				position := quantizedMotionPosition(from+(to-from)*fraction, resolution)
				if position == from || position == to {
					continue
				}
				candidate := MotionSample{TimeMillis: left.TimeMillis + int64(float64(duration)*fraction), PositionPercent: position}
				candidate.TimeMillis = quantizedCrossingTime(probes, candidate, position, left.TimeMillis, right.TimeMillis)
				if candidate.TimeMillis-left.TimeMillis < minimumSpacing || right.TimeMillis-candidate.TimeMillis < minimumSpacing ||
					math.Abs(position-from)*1000/float64(candidate.TimeMillis-left.TimeMillis) > maximumVelocity ||
					math.Abs(to-position)*1000/float64(right.TimeMillis-candidate.TimeMillis) > maximumVelocity ||
					!wireSegmentFits(left, candidate, probes, resolution, tolerance) || !wireSegmentFits(candidate, right, probes, resolution, tolerance) {
					continue
				}
				candidateCost := bufferedVelocityError(left, candidate, probes, resolution) + bufferedVelocityError(candidate, right, probes, resolution)
				if candidateCost < bestCost {
					// Retain the shared plan's identity and phase for observation
					// and diagnostics as well as its quantized wire coordinate.
					candidate = sampleAt(candidate.TimeMillis)
					candidate.PositionPercent = position
					best, bestCost, found = candidate, candidateCost, true
				}
			}
		}
		if found {
			split(left, best)
			split(best, right)
			return
		}
		result = append(result, right)
	}
	for i := 1; i < len(samples); i++ {
		// Keep boundary segment velocities and the original append cutoff:
		// improving one frame must not create a speed step at its neighbors.
		if i == 1 || i == len(samples)-1 {
			result = append(result, samples[i])
		} else {
			split(samples[i-1], samples[i])
		}
	}
	return result
}

func bufferedMaximumVelocityStep(samples []MotionSample, resolution float64) float64 {
	var previous, maximum float64
	for i := 1; i < len(samples); i++ {
		left, right := samples[i-1], samples[i]
		velocity := (quantizedMotionPosition(right.PositionPercent, resolution) - quantizedMotionPosition(left.PositionPercent, resolution)) * 1000 / float64(right.TimeMillis-left.TimeMillis)
		if i > 1 {
			maximum = math.Max(maximum, math.Abs(velocity-previous))
		}
		previous = velocity
	}
	return maximum
}

func bufferedVelocityError(left, right MotionSample, probes []MotionSample, resolution float64) float64 {
	velocity := (quantizedMotionPosition(right.PositionPercent, resolution) - quantizedMotionPosition(left.PositionPercent, resolution)) / float64(right.TimeMillis-left.TimeMillis)
	errorValue := 0.0
	for i := 1; i < len(probes); i++ {
		a, b := probes[i-1], probes[i]
		duration := min(b.TimeMillis, right.TimeMillis) - max(a.TimeMillis, left.TimeMillis)
		if duration <= 0 {
			continue
		}
		planned := (b.PositionPercent - a.PositionPercent) / float64(b.TimeMillis-a.TimeMillis)
		errorValue += (planned - velocity) * (planned - velocity) * float64(duration)
	}
	return errorValue
}
