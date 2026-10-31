package motion

import (
	"errors"
	"math"
	"slices"
)

// DirectionalPace scales travel toward the tip (up) and base (down), before
// the shared joint fitter. It never changes endpoints or turnaround shaping.
// Nil retains the historical symmetric clock; explicit rates are 1..100.
type DirectionalPace struct {
	UpRatePercent   int `json:"up_rate_percent"`
	DownRatePercent int `json:"down_rate_percent"`
}

func (s FlowSpec) validateDirectionalPace() error {
	if s.DirectionalPace == nil {
		return nil
	}
	p := s.DirectionalPace
	if p.UpRatePercent < 1 || p.UpRatePercent > 100 || p.DownRatePercent < 1 || p.DownRatePercent > 100 {
		return errors.New("directional_pace requires up_rate_percent and down_rate_percent within 1..100")
	}
	if s.Gesture != nil || s.Strokes != nil || s.Freestyle != nil || len(s.Steps) != 0 {
		return errors.New("directional_pace is supported by Layered field and phrase motion only")
	}
	return nil
}

func (s FlowSpec) directionRate(from, to float64) float64 {
	if s.DirectionalPace == nil {
		return 1
	}
	if to > from {
		return float64(s.DirectionalPace.UpRatePercent) / 100
	}
	return float64(s.DirectionalPace.DownRatePercent) / 100
}

func (s FlowSpec) hasDirectionalContrast() bool {
	return s.DirectionalPace != nil && s.DirectionalPace.UpRatePercent != s.DirectionalPace.DownRatePercent
}

// Preserve all explicitly authored relative leg timing around the whole loop,
// including region connectors and its seam. Fixed index pairs do not identify
// real cycles in a mixed motif. Fit one common clock scale rather than allowing
// a constrained fast leg to become slower than its intended slow neighbor.
// Counted phrases opt in through unequal direction rates. Fields instead use
// the local contrast constraints below; ordinary symmetric Layered is unchanged.
func fitLayeredRhythmCurves(legs []gestureLeg, durations []int64, budget legBudget) ([]Curve, error) {
	curves := make([]Curve, len(legs))
	for range 16 {
		// Evaluate the common clock directly. Fitting individual legs first
		// changes their shared turns and can overestimate the scale this
		// uniformly stretched curve actually needs.
		turns := gestureTurnAccelerations(legs, durations)
		factor := 1.0
		for i, leg := range legs {
			curves[i] = gestureTurnCurve(leg, durations[i], turns[i], turns[(i+1)%len(legs)])
			factor = math.Max(factor, legTimeScale(curves[i], budget.forLeg(leg)))
		}
		if factor <= 1 {
			return curves, nil
		}
		for i := range durations {
			durations[i] = int64(math.Ceil(float64(durations[i]) * factor * 1.002))
		}
	}
	return nil, errors.New("directional timing did not converge inside the shared motion envelope")
}

// A field has no counted motif whose entire clock must stretch together.
// Preserve its authored directional contrast locally at every reversal:
// when a fast-direction leg needs extra time, its two slow neighbors receive
// at least that same relative slowdown. Slow legs need not delay distant fast
// legs. This keeps a single tight reversal from throttling all 64 cycles.
// The ordinary joint fitter still owns bounds and acceleration continuity.
func fitDirectionalFieldCurves(legs []gestureLeg, durations []int64, budget legBudget, upFast bool) ([]Curve, error) {
	requested := slices.Clone(durations)
	for range 16 {
		curves, err := fitLegCurves(legs, durations, budget)
		if err != nil {
			return nil, err
		}
		changed := false
		for i, leg := range legs {
			if (leg.to > leg.from) != upFast {
				continue
			}
			for _, neighbor := range []int{(i + len(legs) - 1) % len(legs), (i + 1) % len(legs)} {
				// Integer ceiling preserves the normalized contrast without
				// floating rounding accumulating another millisecond each pass.
				minimum := (durations[i]*requested[neighbor] + requested[i] - 1) / requested[i]
				if durations[neighbor] < minimum {
					durations[neighbor], changed = minimum, true
				}
			}
		}
		if !changed {
			return curves, nil
		}
	}
	return nil, errors.New("directional field timing did not converge inside the shared motion envelope")
}
