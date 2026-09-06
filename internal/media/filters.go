package media

import (
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Filters are opt-in playback transforms. Zero preserves the authored script.
// Smoothing only removes small, rapid excursions; rounding is compiled by the
// shared engine after the optional speed cap, with its actual effect reported.
type Filters struct {
	SmoothingPercent   int
	PeakRoundingMillis int
}

// Effect records source actions removed before engine compilation. Continuous
// rounding measurements come from the resulting engine plan, not a point plot.
type Effect struct {
	ActionsRemoved int `json:"actions_removed,omitempty"`
}

func (f Filters) normalized() Filters {
	f.SmoothingPercent = max(0, min(f.SmoothingPercent, config.MaxScriptSmoothingPercent))
	f.PeakRoundingMillis = max(0, min(f.PeakRoundingMillis, config.MaxPeakRoundingMillis))
	return f
}

func (f Filters) apply(points []motion.CurvePoint) ([]motion.CurvePoint, Effect) {
	f = f.normalized()
	if f.SmoothingPercent == 0 || len(points) < 3 {
		return points, Effect{}
	}
	filtered := motion.SmoothMediaReversals(points, float64(f.SmoothingPercent))
	return filtered, Effect{ActionsRemoved: len(points) - len(filtered)}
}
