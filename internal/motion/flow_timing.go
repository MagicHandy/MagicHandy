package motion

import (
	"math"
	"slices"
)

// Numerical turning-point refinement can put the integrated clock just above
// an exact millisecond on alternate legs of an otherwise uniform pattern.
// Discard that tiny relative search error before rounding up, so it cannot
// seed alternating joint-fit times. This is far below one wire millisecond.
func layeredDurationMillis(millis float64) int64 {
	return max(1, int64(math.Ceil(millis-math.Max(1e-6, millis*2e-8))))
}

// fitLayeredCurve preserves the score's destinations, counts and requested
// timing. The shared joint fitter only lengthens legs when exact velocity,
// acceleration or jerk bounds require it. If rounded neutral strokes are
// constrained, try the existing cruise profiles and retain a strictly faster
// valid fit. Unconstrained timing and explicitly softened turns keep their
// original shape; no language-dependent speed or range rules are involved.
func fitLayeredCurve(legs []gestureLeg, durations []int64, budget legBudget, name string) (Curve, error) {
	curves, err := fitLegCurves(legs, slices.Clone(durations), budget)
	if err != nil {
		return Curve{}, err
	}
	best, err := assembleLegCurve(legs, curves, name)
	if err != nil {
		return Curve{}, err
	}
	requested := int64(0)
	for i, leg := range legs {
		if leg.softness > 0 {
			return best, nil
		}
		requested += durations[i]
	}
	if best.duration <= requested {
		return best, nil
	}
	for _, amount := range []float64{.25, .5, .75, 1} {
		candidateLegs := slices.Clone(legs)
		if !fitGestureCruise(candidateLegs, amount, budget) {
			continue
		}
		candidateCurves, fitErr := fitLegCurves(candidateLegs, slices.Clone(durations), budget)
		if fitErr != nil {
			continue // An optional profile must not invalidate a fitted score.
		}
		candidate, fitErr := assembleLegCurve(candidateLegs, candidateCurves, name)
		if fitErr == nil && candidate.duration < best.duration {
			best = candidate
		}
	}
	return best, nil
}
