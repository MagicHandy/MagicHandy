package motion

import "math"

// The carrier's half-cycles are not the turning points of a moving window.
// Locate the extrema of the combined semantic field first, then let the shared
// stroke fitter give those actual excursions a bounded monotone trajectory.
func flowTurningCycles(spec FlowSpec) []float64 {
	const probesPerCycle = 32
	count := int(spec.cycleCount()) * probesPerCycle
	step := 1.0 / probesPerCycle
	position := func(u float64) float64 { x, _, _, _, _ := spec.flowGeometry(u); return x }
	values := make([]float64, count)
	for i := range values {
		values[i] = position(float64(i) * step)
	}
	turns := make([]float64, 0, int(spec.cycleCount())*2)
	for i, current := range values {
		before, after := values[(i+count-1)%count], values[(i+1)%count]
		if (current-before)*(after-current) >= 0 {
			continue
		}
		maximum := current > before
		left, right := (float64(i)-1)*step, (float64(i)+1)*step
		// Each bracket is much shorter than the supported modulation periods.
		// Golden-section refinement reuses one evaluation and handles a soft
		// carrier's flat top without dividing by a near-zero derivative.
		const ratio = .6180339887498949
		a, b := right-ratio*(right-left), left+ratio*(right-left)
		va, vb := position(a), position(b)
		for range 32 {
			if (va < vb) == maximum {
				left, a, va = a, b, vb
				b = left + ratio*(right-left)
				vb = position(b)
			} else {
				right, b, vb = b, a, va
				a = right - ratio*(right-left)
				va = position(a)
			}
		}
		turns = append(turns, (left+right)/2)
	}
	// Keep the loop origin at the reversal nearest cycle zero, rather than
	// changing the starting end when the bottom turn drifts across the seam.
	if len(turns) > 1 && spec.cycleCount()-turns[len(turns)-1] < math.Abs(turns[0]) {
		last := turns[len(turns)-1] - spec.cycleCount()
		turns = append([]float64{last}, turns[:len(turns)-1]...)
	}
	return turns
}
