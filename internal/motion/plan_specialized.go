package motion

import (
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Select an already-resolved grammar before compiling catalog/media content.
func specializedMotionPlan(id string, target MotionTarget, settings config.MotionSettings, phase float64, handoff int64, created time.Time) (MotionPlan, bool) {
	switch {
	case target.TempoOnly:
		return tempoOnlyMotionPlan(id, target, created), true
	case target.Flow != nil:
		return flowPlan(id, target, settings, phase, handoff, created), true
	case target.prepared != nil && target.prepared.libraryID == "":
		return preparedPlan(id, target, settings, phase, handoff, created), true
	default:
		return MotionPlan{}, false
	}
}
