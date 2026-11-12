//go:build liveeval || magichandy_labs

package motion

import "errors"

// TempoReviewSampler exposes the production envelope to opt-in evaluations.
// It creates no engine, transport, goroutine or hardware connection. Times are
// absolute milliseconds in the evaluator's session, not relative to each read.
func TempoReviewSampler(spec TempoSpec, at int64, from float64) (func(int64) float64, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if spec.Mode == TempoModeFollow {
		return nil, errors.New("tempo follow needs a position plan")
	}
	return newTempoPlan(spec, at, from, 1).LevelAt, nil
}
