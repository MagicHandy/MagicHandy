package motion

import (
	"fmt"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// preparedMotion is immutable, backend-authored content with explicit
// position, velocity and acceleration states. It has no public JSON input.
// It uses the same plan, transition, sampler, sanitizer and dispatch as every
// other source; preserving its derivatives avoids rebuilding a continuous
// signal as a sequence of independently shaped strokes.
type preparedMotion struct {
	id, name      string
	libraryID     PatternID
	curve         Curve
	referenceRate float64
	acceleration  float64
	jerk          float64
}

func preparedPlan(id string, target MotionTarget, settings config.MotionSettings, phase float64, handoff int64, created time.Time) MotionPlan {
	content := target.prepared
	curve := content.curve
	focus := newFocusProjection(target, resolvedContent{points: curve.authoredKnots, duration: curve.duration, loop: true})
	gain := focus.gain()
	requestedRate := referenceTravelRateForSpeed(target.SpeedPercent, settings.HandyModel)
	factor := content.referenceRate * gain / requestedRate
	factor = math.Max(factor, totalCurveTravel(curve.authoredKnots)*gain*1000/float64(curve.duration)/requestedRate)
	factor = math.Max(factor, curve.maximumVelocityPerMillis()*gain*1000/referenceTravelRateForSpeed(100, settings.HandyModel))
	acceleration := math.Min(content.acceleration, runtimeMaxAccelerationPercentPerSecond2)
	jerk := math.Min(content.jerk, runtimeMaxJerkPercentPerSecond3)
	factor = math.Max(factor, math.Sqrt(curve.maximumAccelerationPerMillis2()*gain*1e6/acceleration))
	factor = math.Max(factor, math.Cbrt(curve.maximumJerkPerMillis3()*gain*1e9/jerk))
	if gap := reversalGap(curve.authoredKnots, curve.duration, curve.loop); gap > 0 {
		factor = math.Max(factor, float64(runtimeMinimumReversalGapMillis)/float64(gap))
	}
	period := max(int64(1), int64(math.Ceil(float64(curve.duration)*factor)))
	if id == "" {
		id = fmt.Sprintf("%s-%d", content.id, created.UnixNano())
	}
	return MotionPlan{ID: id, Target: target, PatternID: target.PatternID, PeriodMillis: period,
		HandoffMillis: handoff, PhaseOffset: phaseForContent(phase, curve.loop), Loop: curve.loop,
		CreatedAt: created.UTC().Format(time.RFC3339Nano), curve: curve, focus: focus,
		Perceptual: summarizeMotionPlan(target, curve, focus, period, settings.HandyModel), timingModel: settings.HandyModel}
}

func (target MotionTarget) continuousCurve() bool {
	return target.Dynamic != nil || target.prepared != nil
}
