//go:build magichandy_labs

package motion

import (
	"fmt"
	"os"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Inert release matrix: every score enters the production compiler and Cloud
// sampler. Failed entries remain in the report before the test fails.
func TestLayeredReleaseSpeedReview(t *testing.T) {
	path := os.Getenv("MAGICHANDY_LAYERED_RELEASE_REPORT")
	if path == "" {
		t.Skip("set an ignored release atlas path")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	var entries []Review
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		for _, speed := range []int{5, 25, 55, 100} {
			for name, spec := range layeredReleaseFixtures(speed) {
				target, err := FlowTarget(spec, settings)
				if err != nil {
					t.Errorf("%s/%d/%s: %v", model, speed, name, err)
					entries = append(entries, Review{ID: fmt.Sprintf("%s-%s-%d", model, name, speed), Name: name, Error: err.Error()})
					continue
				}
				entry := speedSupportCloudReview(target, settings)
				entry.ID, entry.Name, entry.Group = fmt.Sprintf("%s-%s-%d", model, name, speed), name, "layered-release"
				entries = append(entries, entry)
				if entry.Error != "" || entry.PeakAcceleration > flowAccelerationBudget*1.001 || entry.PeakJerk > flowJerkBudget*1.001 ||
					entry.Summary.CommandedPeakVelocityPerSecond > referenceTravelRateForSpeed(100, model)*1.001 || entry.VelocityJump > .01 || entry.AccelerationJump > .01 {
					t.Errorf("%s: error=%s accel=%f jerk=%f jumps=%f/%f", entry.ID, entry.Error, entry.PeakAcceleration, entry.PeakJerk, entry.VelocityJump, entry.AccelerationJump)
				}
			}
		}
	}
	writeSpeedReviewReport(t, path, map[string]any{"schema": "magichandy.motion-atlas.v1", "entries": entries})
}

func layeredReleaseFixtures(speed int) map[string]FlowSpec {
	full, short, compound := layeredSpeedFixture("full", speed), layeredSpeedFixture("short", speed), layeredSpeedFixture("compound", speed)
	soft, softest, contrast, cadence := full, full, *CloneFlowSpec(&compound), short
	soft.TurnSoftnessPercent, softest.TurnSoftnessPercent = 70, 100
	contrast.Phrase.Strokes[0].RatePercent = 15
	cadence.MinPercent, cadence.RangeCeilingPercent, cadence.CadenceHoldPercent = 0, 100, 100
	pace := full
	pace.LoopCycles = 16
	pace.Layers = []FlowLayer{{Axis: "pace", AmountPercent: 100, LowRatePercent: 15, PeriodCycles: 8, Shape: "wave"}}
	roaming := layeredTransferFixture()
	roaming.SpeedPercent, roaming.LoopCycles = speed, 16
	combined := roaming
	combined.Layers = append([]FlowLayer(nil), roaming.Layers...)
	combined.Layers = append(combined.Layers, FlowLayer{Axis: "range", AmountPercent: 70, PeriodCycles: 5, Shape: "drift"})
	sections := full
	sections.RangeFloorPercent, sections.RangeCeilingPercent = 15, 0
	sections.Steps = []FlowStep{{MinPercent: 0, MaxPercent: 100, SpeedPercent: speed, Cycles: 4}, {MinPercent: 55, MaxPercent: 100, SpeedPercent: max(1, speed/3), Cycles: 4}}
	return map[string]FlowSpec{"full": full, "short": short, "compound": compound, "soft": soft, "softest": softest,
		"phrase-contrast": contrast, "cadence": cadence, "pace-layer": pace, "roaming": roaming, "combined": combined, "sections": sections}
}

func TestLayeredReleaseRetargetRuntime(t *testing.T) {
	path := os.Getenv("MAGICHANDY_LAYERED_RELEASE_RUNTIME_REPORT")
	if path == "" {
		t.Skip("set an ignored runtime capture path")
	}
	runSpeedReviewRuntime(t, path, layeredSpeedFixture("compound", 7), layeredSpeedFixture("compound", 100))
}
