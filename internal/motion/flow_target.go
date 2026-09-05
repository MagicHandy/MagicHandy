package motion

import (
	"fmt"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func normalizeFlowTarget(target MotionTarget, settings config.MotionSettings) MotionTarget {
	target.Flow = CloneFlowSpec(target.Flow)
	target.Dynamic, target.Pattern, target.Program, target.Media = nil, nil, nil, nil
	target.ProgramID, target.MediaID = "", ""
	target.AreaFocus, target.SoftAnchor = nil, nil
	if target.SpeedPercent == 0 {
		target.SpeedPercent = target.Flow.SpeedPercent
	}
	target.SpeedPercent = clamp(target.SpeedPercent, settings.SpeedMinPercent, settings.SpeedMaxPercent)
	if len(target.Flow.Steps) == 0 && target.Flow.SpeedPercent != target.SpeedPercent {
		target.Flow.SpeedPercent, target.prepared = target.SpeedPercent, nil
	}
	if target.prepared != nil {
		target.PatternID, target.PatternName = PatternID(target.prepared.id), target.prepared.name
	} else {
		target.PatternID, target.PatternName = "", "Continuous flow"
	}
	return target
}

func flowPlan(id string, target MotionTarget, settings config.MotionSettings, phase float64, handoff int64, created time.Time) MotionPlan {
	if target.prepared == nil {
		compiled, err := FlowTarget(*target.Flow, settings)
		if err != nil {
			return MotionPlan{ID: id, Target: target, PeriodMillis: minimumBurstCycleMillis,
				curve: (resolvedContent{}).stationaryFallbackCurve(), compileErr: fmt.Errorf("compile flow: %w", err)}
		}
		target.prepared = compiled.prepared
		target.PatternID, target.PatternName = PatternID(compiled.prepared.id), compiled.prepared.name
	}
	return preparedPlan(id, target, settings, phase, handoff, created)
}
