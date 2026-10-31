package motion

import "math"

// compileFlowCurve fits the combined layered field's actual turning points.
// The local joint fitter preserves monotonic strokes and prevents one tight
// reversal from slowing the entire phrase.
func compileFlowCurve(spec FlowSpec, handyModel string) (Curve, error) {
	legs, durations := flowLegs(spec, handyModel)
	budget := topSpeedBudget(spec.peakSpeedPercent(), handyModel)
	var rhythmFit layeredCurveFitter
	if spec.hasDirectionalContrast() {
		rhythmFit = func(legs []gestureLeg, durations []int64, budget legBudget) ([]Curve, error) {
			return fitDirectionalFieldCurves(legs, durations, budget, spec.DirectionalPace.UpRatePercent > spec.DirectionalPace.DownRatePercent)
		}
	}
	curve, err := fitLayeredCurve(legs, durations, budget, "flow", rhythmFit)
	if err != nil {
		return Curve{}, err
	}
	curve.flowPeriod = spec.cycleCount()
	curve.flowCycles = make([]float64, len(legs)+1)
	for i, leg := range legs {
		curve.flowCycles[i] = leg.sourceCycle
	}
	curve.flowCycles[len(legs)] = legs[0].sourceCycle + curve.flowPeriod
	return curve, nil
}

// flowLegs uses the score's actual turning points. A leg's time is
// the layered clock integrated over its excursion, lengthened when the layers
// carry the stroke farther than the local window, so a transfer between regions
// keeps the requested travel rate instead of rushing. Adjacent extrema with
// the same direction (possible at a numerically flat top) become one leg, so
// every remaining joint is a genuine reversal and no transfer stops midway.
func flowLegs(spec FlowSpec, handyModel string) ([]gestureLeg, []int64) {
	cycles := flowTurningCycles(spec)
	halves := len(cycles)
	if halves < 2 {
		return nil, nil
	}
	positions := make([]float64, halves)
	for i := range positions {
		positions[i], _, _ = spec.flowState(cycles[i], handyModel)
	}
	times := make([]float64, halves)
	for i := range times {
		start, end := cycles[i], cycles[(i+1)%halves]
		if i == halves-1 {
			end += spec.cycleCount()
		}
		times[i] = flowClockIntegral(spec, handyModel, start, end)
		_, span, _ := spec.flowState((start+end)/2, handyModel)
		nominalTravel := 2 * span * (end - start)
		if travel := math.Abs(positions[(i+1)%halves] - positions[i]); nominalTravel > 0 && travel > nominalTravel {
			times[i] *= travel / nominalTravel
		}
	}
	// Start the loop at a reversal so a same-direction run cannot straddle it.
	first := 0
	for i := range halves {
		if flowDirection(positions, i)*flowDirection(positions, (i+halves-1)%halves) < 0 {
			first = i
			break
		}
	}
	softness := float64(spec.TurnSoftnessPercent) / 100
	legs := make([]gestureLeg, 0, halves)
	durations := make([]int64, 0, halves)
	for offset := 0; offset < halves; {
		start := (first + offset) % halves
		direction, millis := flowDirection(positions, start), 0.0
		run := 0
		for offset+run < halves && (run == 0 || flowDirection(positions, (start+run)%halves)*direction >= 0) {
			millis += times[(start+run)%halves]
			run++
		}
		legs = append(legs, gestureLeg{from: positions[start], to: positions[(start+run)%halves], weight: 1, softness: softness, sourceCycle: cycles[start]})
		durations = append(durations, layeredDurationMillis(millis))
		offset += run
	}
	legs, durations = coalesceFlowChatter(legs, durations)
	// Apply direction after geometric cleanup so it cannot change which
	// reversals survive. The joint fitter reconciles the unequal travel times.
	for i, leg := range legs {
		durations[i] = layeredDurationMillis(float64(durations[i]) / spec.directionRate(leg.from, leg.to))
	}
	for i := 1; i < len(legs); i++ {
		for legs[i].sourceCycle < legs[i-1].sourceCycle {
			legs[i].sourceCycle += spec.cycleCount()
		}
	}
	return legs, durations
}

// Fold small, brief field reversals using the shared chatter criteria, retaining
// the outer endpoints and total clock. Slow subtle movement remains authored
// content; the compiler does not depend on a transport's encoding resolution.
func coalesceFlowChatter(legs []gestureLeg, durations []int64) ([]gestureLeg, []int64) {
	for len(legs) >= 4 {
		index, smallest := -1, MinimumPatternReversalProminence
		for i, leg := range legs {
			if span := math.Abs(leg.to - leg.from); span < smallest && durations[i] <= maximumPatternChatterFlankMillis {
				index, smallest = i, span
			}
		}
		if index < 0 {
			break
		}
		if index == 0 || index == len(legs)-1 {
			// Move a retained turn to the loop seam before merging a seam lobe.
			first := (index + len(legs) - 1) % len(legs)
			legs = append(append([]gestureLeg(nil), legs[first:]...), legs[:first]...)
			durations = append(append([]int64(nil), durations[first:]...), durations[:first]...)
			index = 1
		}
		legs[index-1].to = legs[index+1].to
		durations[index-1] += durations[index] + durations[index+1]
		legs = append(legs[:index], legs[index+2:]...)
		durations = append(durations[:index], durations[index+2:]...)
	}
	return legs, durations
}

func flowDirection(positions []float64, i int) float64 {
	return math.Copysign(1, positions[(i+1)%len(positions)]-positions[i])
}

// flowClockIntegral is the time, in milliseconds, the layered clock assigns to
// cycle positions a..b (composite Simpson over eight intervals).
func flowClockIntegral(spec FlowSpec, handyModel string, a, b float64) float64 {
	const intervals = 8
	clock := func(u float64) float64 {
		_, _, millis := spec.flowState(u, handyModel)
		return millis
	}
	step, total := (b-a)/intervals, 0.0
	for i := range intervals {
		left := a + step*float64(i)
		total += step * (clock(left) + 4*clock(left+step/2) + clock(left+step)) / 6
	}
	return total
}
