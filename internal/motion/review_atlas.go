//go:build magichandy_labs

package motion

import (
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Review is an inert rendering input, generated from the shared plan and
// buffered sampler. Wire points represent quantized commands, never telemetry.
type Review struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Group            string            `json:"group"`
	Description      string            `json:"description,omitempty"`
	Request          string            `json:"request,omitempty"`
	Raw              string            `json:"raw,omitempty"`
	Model            string            `json:"model,omitempty"`
	Error            string            `json:"error,omitempty"`
	PeriodMillis     int64             `json:"period_ms"`
	SpeedPercent     int               `json:"speed_percent"`
	HandyModel       string            `json:"handy_model"`
	Summary          PerceptualSummary `json:"summary"`
	PeakAcceleration float64           `json:"peak_acceleration"`
	PeakJerk         float64           `json:"peak_jerk"`
	AccelerationJump float64           `json:"acceleration_jump"`
	Outcome          string            `json:"outcome,omitempty"`
	Samples          [][4]float64      `json:"samples"`
	Wire             []CurvePoint      `json:"wire"`
}

// ReviewMotionOutput never constructs a transport or starts an engine loop.
// It exercises the same buffered frame fitter used by a real engine.
func ReviewMotionOutput(target MotionTarget, settings config.MotionSettings) Review {
	plan := NewMotionPlan("visual-review", target, settings, 0, 0, time.Unix(0, 0))
	review := Review{ID: string(plan.PatternID), Name: plan.Target.PatternName, PeriodMillis: plan.PeriodMillis,
		SpeedPercent: plan.Target.SpeedPercent, HandyModel: settings.HandyModel}
	if err := plan.compilationError(); err != nil {
		review.Error = err.Error()
		return review
	}
	review.Summary = summarizeMotionPlan(plan.Target, plan.curve, plan.focus, plan.PeriodMillis, settings.HandyModel)
	factor := float64(plan.PeriodMillis) / float64(plan.curve.duration)
	gain := plan.focus.gain()
	review.PeakAcceleration = plan.curve.maximumAccelerationPerMillis2() * gain * 1e6 / (factor * factor)
	review.PeakJerk = plan.curve.maximumJerkPerMillis3() * gain * 1e9 / (factor * factor * factor)
	for _, point := range plan.curve.points {
		at := float64(point.TimeMillis)
		before, after := positiveModulo(at-0.000001, float64(plan.curve.duration)), positiveModulo(at+0.000001, float64(plan.curve.duration))
		jump := math.Abs(plan.curve.accelerationFloat(after)-plan.curve.accelerationFloat(before)) * gain * 1e6 / (factor * factor)
		review.AccelerationJump = math.Max(review.AccelerationJump, jump)
	}
	for index := 0; index <= 1600; index++ {
		at := float64(plan.PeriodMillis) * float64(index) / 1600
		u := at / factor
		review.Samples = append(review.Samples, [4]float64{at / 1000, plan.focus.apply(plan.curve.sampleFloat(u)),
			plan.curve.velocityFloat(u) * gain * 1000 / factor, plan.curve.accelerationFloat(u) * gain * 1e6 / (factor * factor)})
	}
	engine := &Engine{plan: plan, settings: settings, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true, positionResolutionPercent: 1, maximumChunkPoints: maximumAdaptiveChunkPoints}
	for engine.nextSampleMillis < min(plan.PeriodMillis, int64(12000)) {
		points, err := engine.nextMotionSamplesLocked()
		if err != nil {
			review.Error = err.Error()
			return review
		}
		for _, point := range points {
			review.Wire = append(review.Wire, CurvePoint{TimeMillis: point.TimeMillis, PositionPercent: math.Round(point.PositionPercent)})
		}
	}
	return review
}
