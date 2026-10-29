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
// This policy is opt-in through unequal direction rates; ordinary Layered keeps
// its existing local fitting and full speed range.
func fitLayeredRhythmCurves(legs []gestureLeg, durations []int64, budget legBudget) ([]Curve, error) {
	for range 16 {
		fitted := slices.Clone(durations)
		curves, err := fitLegCurves(legs, fitted, budget)
		if err != nil {
			return nil, err
		}
		factor := 1.0
		for i := range durations {
			factor = math.Max(factor, float64(fitted[i])/float64(durations[i]))
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
