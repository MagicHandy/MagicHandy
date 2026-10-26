//go:build magichandy_labs

package motion

import (
	"fmt"
	"math"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// TestLayeredSpeedSupportReview isolates the production clock from stroke
// width, deliberate pace variation and device calibration. Alternatives are
// inert experiments only: no transport, settings mutation or hardware commands.
// They are not eligible for release without matched physical assessment.
func TestLayeredSpeedSupportReview(t *testing.T) {
	path := os.Getenv("MAGICHANDY_SPEED_SUPPORT_REPORT")
	if path == "" {
		t.Skip("set MAGICHANDY_SPEED_SUPPORT_REPORT to an ignored report path")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	var entries []Review
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		speeds := []int{100}
		if model == config.HandyModelOriginal {
			speeds = []int{25, 50, 75, 100}
		}
		for _, speed := range speeds {
			for _, shape := range []string{"full", "upper-20", "two-upper-one-full"} {
				spec := speedSupportSpec(shape, speed)
				target, err := FlowTarget(spec, settings)
				if err != nil {
					t.Fatal(err)
				}
				entry := speedSupportCloudReview(target, settings)
				entry.ID = fmt.Sprintf("%s-%s-%d-production", model, shape, speed)
				entry.Name, entry.Group = entry.ID, "layered-speed-support"
				entries = append(entries, entry)
				logSpeedSupport(t, entry)
				if entry.Error != "" {
					t.Fatal(entry.Error)
				}
				if model != config.HandyModelOriginal || speed != 100 {
					continue
				}
				for _, variant := range []string{"exact-current-envelope", "creative-envelope", "creative-envelope-cruise"} {
					candidate := speedSupportAlternative(t, spec, settings, variant)
					candidate.ID = fmt.Sprintf("%s-%s-%d-%s", model, shape, speed, variant)
					candidate.Name, candidate.Group = candidate.ID, "inert-speed-alternatives"
					candidate.Description = "Test-only retiming at unchanged semantic turns/counts; physical feel untested. " + variant
					entries = append(entries, candidate)
					logSpeedSupport(t, candidate)
				}
			}
		}
	}
	writeSpeedReviewReport(t, path, map[string]any{"schema": "magichandy.motion-atlas.v1", "entries": entries})
}

func speedSupportSpec(shape string, speed int) FlowSpec {
	spec := DefaultFlowSpec()
	spec.MinPercent, spec.MaxPercent = 0, 100
	spec.RangeFloorPercent, spec.RangeCeilingPercent = 100, 100
	spec.SpeedPercent, spec.PaceVariationPercent, spec.LoopCycles = speed, 0, 6
	if shape == "upper-20" {
		spec.MinPercent, spec.RangeFloorPercent, spec.RangeCeilingPercent = 80, 20, 20
	}
	if shape == "two-upper-one-full" {
		spec.RangeFloorPercent = 28
		spec.Phrase = &FlowPhrase{Strokes: []PhraseStroke{
			{BottomPercent: 72, TopPercent: 100, Count: 2, RatePercent: 100},
			{BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: 100},
		}}
	}
	return spec
}

func speedSupportAlternative(t *testing.T, spec FlowSpec, settings config.MotionSettings, variant string) Review {
	t.Helper()
	legs, _ := flowLegs(spec, settings.HandyModel)
	if spec.Phrase != nil {
		legs, _ = phraseLegs(spec, settings.HandyModel)
	}
	// Freeze the pre-alpha.75 envelope so historical comparisons stay meaningful
	// after production Layered adopts the physically accepted faster candidate.
	budget := legBudget{referenceTravelRateForSpeed(100, settings.HandyModel), 2400, 24000}
	if variant != "exact-current-envelope" {
		budget.acceleration, budget.jerk = runtimeMaxAccelerationPercentPerSecond2, runtimeMaxJerkPercentPerSecond3
	}
	rate := referenceTravelRateForSpeed(spec.SpeedPercent, settings.HandyModel)
	durations := make([]int64, len(legs))
	for i, leg := range legs {
		// These fixtures have no rests, contrast, pace layers or unequal rates.
		durations[i] = int64(math.Ceil(math.Abs(leg.to-leg.from) * 1000 / rate))
	}
	curves, err := fitLegCurves(legs, slices.Clone(durations), budget)
	if err != nil {
		t.Fatal(err)
	}
	best, err := assembleLegCurve(legs, curves, "speed-support-review")
	if err != nil {
		t.Fatal(err)
	}
	if variant == "creative-envelope-cruise" {
		for _, amount := range []float64{.25, .5, .75, 1} {
			candidateLegs := slices.Clone(legs)
			if !fitGestureCruise(candidateLegs, amount, budget) {
				continue
			}
			candidateCurves, fitErr := fitLegCurves(candidateLegs, slices.Clone(durations), budget)
			if fitErr != nil {
				t.Fatal(fitErr)
			}
			candidate, fitErr := assembleLegCurve(candidateLegs, candidateCurves, "speed-support-review")
			if fitErr != nil {
				t.Fatal(fitErr)
			}
			if candidate.duration < best.duration {
				best = candidate
			}
		}
	}
	target := MotionTarget{SpeedPercent: spec.SpeedPercent, Flow: &spec, prepared: &preparedMotion{
		id: "speed-support-review", name: variant, curve: best, referenceRate: rate,
		acceleration: budget.acceleration, jerk: budget.jerk,
	}}
	entry := speedSupportCloudReview(target, settings)
	if entry.Error != "" || entry.PeakAcceleration > budget.acceleration*1.001 || entry.PeakJerk > budget.jerk*1.001 ||
		entry.Summary.CommandedPeakVelocityPerSecond > budget.velocity*1.001 || entry.VelocityJump > .01 || entry.AccelerationJump > .01 {
		t.Fatalf("invalid inert alternative %s: error=%s accel=%.3f jerk=%.3f peak=%.3f jumps=%.5f/%.5f",
			variant, entry.Error, entry.PeakAcceleration, entry.PeakJerk, entry.Summary.CommandedPeakVelocityPerSecond, entry.VelocityJump, entry.AccelerationJump)
	}
	return entry
}

// Use the shipping Cloud sampling preference for the quantized overlay. The
// ordinary review helper uses the conservative cross-transport approximation.
func speedSupportCloudReview(target MotionTarget, settings config.MotionSettings) Review {
	entry := ReviewMotionOutput(target, settings)
	if entry.Error != "" {
		return entry
	}
	plan := NewMotionPlan("speed-support-review", target, settings, 0, 0, time.Unix(0, 0))
	caps := (&transport.CloudRESTTransport{}).MotionSamplingCapabilities()
	engine := &Engine{plan: plan, settings: settings, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true, positionResolutionPercent: caps.PositionResolutionPercent, maximumChunkPoints: caps.MaximumPointsPerAppend,
		preferSparseGeneratedMotion: caps.PreferSparseGeneratedMotion}
	entry.Wire = nil
	for engine.nextSampleMillis < min(plan.PeriodMillis, int64(12000)) {
		points, err := engine.nextMotionSamplesLocked()
		if err != nil {
			entry.Error = err.Error()
			break
		}
		for _, point := range points {
			entry.Wire = append(entry.Wire, CurvePoint{TimeMillis: point.TimeMillis, PositionPercent: math.Round(point.PositionPercent)})
		}
	}
	return entry
}

func logSpeedSupport(t *testing.T, entry Review) {
	t.Helper()
	profile := handySpeedProfileFor(entry.HandyModel)
	t.Logf("%s: mean=%.1fmm/s effective=%.1f%% peak=%.1fmm/s accel=%.0f jerk=%.0f period=%dms",
		entry.ID, entry.Summary.CommandedMeanTravelPerSecond*profile.strokeMillimeters/100,
		entry.Summary.Pace.EffectivePercent, entry.Summary.CommandedPeakVelocityPerSecond*profile.strokeMillimeters/100,
		entry.PeakAcceleration, entry.PeakJerk, entry.PeriodMillis)
}
