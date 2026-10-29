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
func fitLayeredCurve(legs []gestureLeg, durations []int64, budget legBudget, name string, preserveRhythm bool) (Curve, error) {
	fit := fitLegCurves
	if preserveRhythm {
		fit = fitLayeredRhythmCurves
	}
	fittedDurations := slices.Clone(durations)
	curves, err := fit(legs, fittedDurations, budget)
	if err != nil {
		return Curve{}, err
	}
	// Distinguish physically constrained legs from slower companions stretched
	// only to preserve the authored rhythm. The latter keep their rounded shape.
	constrainedDurations := fittedDurations
	if preserveRhythm {
		constrainedDurations = slices.Clone(durations)
		if _, err := fitLegCurves(legs, constrainedDurations, budget); err != nil {
			return Curve{}, err
		}
	}
	best, err := assembleLegCurve(legs, curves, name)
	if err != nil {
		return Curve{}, err
	}
	softened := false
	for i, leg := range legs {
		softened = softened || leg.softness > 0
		best.plannedDuration += durations[i]
	}
	if softened || best.duration <= best.plannedDuration {
		return best, nil
	}
	for _, candidate := range cruiseCandidates(budget) {
		candidateLegs := slices.Clone(legs)
		if !fitGestureCruise(withCruiseRamp(candidateLegs, candidate.ramp), candidate.amount, budget) {
			continue
		}
		changed := false
		for i := range candidateLegs {
			// A fast neighbor must not flatten an already feasible slow stroke.
			// The joint fitter still reconciles their common turnaround state.
			if constrainedDurations[i] <= durations[i] {
				candidateLegs[i] = legs[i]
			}
			changed = changed || candidateLegs[i].cruise != legs[i].cruise
		}
		if !changed {
			continue
		}
		candidateCurves, fitErr := fit(candidateLegs, slices.Clone(durations), budget)
		if fitErr != nil {
			continue // An optional profile must not invalidate a fitted score.
		}
		fitted, fitErr := assembleLegCurve(candidateLegs, candidateCurves, name)
		if fitErr == nil && fitted.duration < best.duration {
			fitted.plannedDuration = best.plannedDuration
			best = fitted
		}
	}
	return best, nil
}

type cruiseCandidate struct{ amount, ramp float64 }

// cruiseCandidates are the cruise profiles a constrained score tries: the
// ordinary ramps at increasing amounts, then, above the top-speed knee, full
// cruise with shorter ramps (ADR 0064).
func cruiseCandidates(budget legBudget) []cruiseCandidate {
	candidates := []cruiseCandidate{{.25, 1}, {.5, 1}, {.75, 1}, {1, 1}}
	for _, ramp := range cruiseRampCandidates(budget)[1:] {
		candidates = append(candidates, cruiseCandidate{1, ramp})
	}
	return candidates
}

// withCruiseRamp gives every stroke the ramp share; 1 keeps the ordinary ramps.
func withCruiseRamp(legs []gestureLeg, ramp float64) []gestureLeg {
	if ramp < 1 {
		for i := range legs {
			legs[i].cruiseRamp = ramp
		}
	}
	return legs
}
