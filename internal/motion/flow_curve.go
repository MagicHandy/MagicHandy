package motion

import "math"

// compileFlowCurve turns a layered score into strokes between the carrier's
// turning points. The layers still decide where each stroke starts and ends
// and how long it takes, but every stroke now travels monotonically from one
// reversal to the next. Evaluating the layered position continuously let a
// moving center or width outrun the carrier near a turn, adding small
// incidental reversals; the shared minimum reversal spacing then slowed the
// whole score, which capped alternating ends near 37-40 %/s whatever speed was
// requested. Strokes are fitted jointly inside Flow's authoring budget, with
// the same shared-turn construction Creative v2 uses.
func compileFlowCurve(spec FlowSpec, handyModel string) (Curve, error) {
	legs, durations := flowLegs(spec, handyModel)
	budget := legBudget{velocity: referenceTravelRateForSpeed(100, handyModel),
		acceleration: flowAccelerationBudget, jerk: flowJerkBudget}
	curves, err := fitLegCurves(legs, durations, budget)
	if err != nil {
		return Curve{}, err
	}
	return assembleLegCurve(legs, curves, "flow")
}

// flowLegs samples the score at every half-cycle turning point. A leg's time is
// the layered clock integrated over its half-cycle, lengthened when the layers
// carry the stroke farther than the local window, so a transfer between regions
// keeps the requested travel rate instead of rushing. Consecutive half-cycles
// that travel the same way (a transfer outrunning the stroke) become one leg,
// so every remaining joint is a genuine reversal and no transfer stops midway.
func flowLegs(spec FlowSpec, handyModel string) ([]gestureLeg, []int64) {
	halves := 2 * int(math.Round(spec.cycleCount()))
	positions := make([]float64, halves)
	for i := range positions {
		positions[i], _, _ = spec.flowState(0.5*float64(i), handyModel)
	}
	times := make([]float64, halves)
	for i := range times {
		start := 0.5 * float64(i)
		times[i] = flowClockIntegral(spec, handyModel, start, start+0.5)
		_, span, _ := spec.flowState(start+0.25, handyModel)
		if travel := math.Abs(positions[(i+1)%halves] - positions[i]); span > 0 && travel > span {
			times[i] *= travel / span
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
	var legs []gestureLeg
	var durations []int64
	for offset := 0; offset < halves; {
		start := (first + offset) % halves
		direction, millis := flowDirection(positions, start), 0.0
		run := 0
		for offset+run < halves && (run == 0 || flowDirection(positions, (start+run)%halves)*direction >= 0) {
			millis += times[(start+run)%halves]
			run++
		}
		legs = append(legs, gestureLeg{from: positions[start], to: positions[(start+run)%halves], weight: 1, softness: softness})
		durations = append(durations, int64(math.Ceil(millis)))
		offset += run
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
