package motion

import (
	"math"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// PlanningEnvelope exposes the current semantic coordinate system and the
// profile-derived velocity ceiling. It is a planning reference, not telemetry.
// Physical stroke-window mapping and final curve fitting stay in the engine.
type PlanningEnvelope struct {
	PositionMinPercent           int     `json:"position_min_percent"`
	PositionMaxPercent           int     `json:"position_max_percent"`
	ProfilePeakVelocityPerSecond float64 `json:"profile_peak_velocity_percent_per_second"`
}

// CurrentPlanningEnvelope derives values from the same profile as the shared
// sampler; callers need no knowledge of hardware names or version differences.
func CurrentPlanningEnvelope(settings config.MotionSettings) PlanningEnvelope {
	return PlanningEnvelope{PositionMinPercent: 0, PositionMaxPercent: 100,
		ProfilePeakVelocityPerSecond: math.Round(referenceTravelRateForSpeed(100, settings.HandyModel)*10) / 10}
}
