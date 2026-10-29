//go:build magichandy_labs

package motion

import (
	"fmt"
	"os"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Inert session review: compiled geometry and shipping Cloud samples only.
func TestSessionMotionReview(t *testing.T) {
	path := os.Getenv("MAGICHANDY_SESSION_MOTION_REPORT")
	if path == "" {
		t.Skip("set an ignored report path")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	entries := []Review{}
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard} {
		settings.HandyModel = model
		for _, speed := range []int{5, 45, 75, 100} {
			full := layeredSpeedFixture("full", speed)
			full.LoopCycles = 64
			held := *CloneFlowSpec(&full)
			held.TurnSoftnessPercent = 85
			varied := *CloneFlowSpec(&full)
			varied.PaceVariationPercent = 18
			varied.Layers = []FlowLayer{{Axis: "pace", AmountPercent: 75, LowRatePercent: 40, PeriodCycles: 12, Shape: "wave"}}
			mixed := *CloneFlowSpec(&varied)
			mixed.RangeFloorPercent, mixed.RangeCeilingPercent = 35, 85
			mixed.Layers = append(mixed.Layers, FlowLayer{Axis: "range", AmountPercent: 80, PeriodCycles: 8, PhasePercent: 20, Shape: "wave"}, FlowLayer{Axis: "center", AmountPercent: 75, PeriodCycles: 10, Shape: "wave"})
			gesture := gestureFixture()
			gesture.SpeedPercent = speed
			holdTarget, err := HoldingTarget(ActiveMotionState{Target: MotionTarget{SpeedPercent: speed, Flow: &full}}, settings)
			if err != nil {
				t.Fatal(err)
			}
			edge := *CloneFlowSpec(holdTarget.Flow)
			edgePrior, edgeMiddle := edge, edge
			edgePrior.TurnSoftnessPercent, edgeMiddle.TurnSoftnessPercent = 85, 35
			for name, spec := range map[string]FlowSpec{"full": full, "inherited-hold": held, "pace-texture": varied, "mixed": mixed, "creative-varied": gesture, "regional": regionalPhraseFixture(speed, false), "regional-mirror": regionalPhraseFixture(speed, true), "edge-rounded": edge, "edge-prior": edgePrior, "edge-middle": edgeMiddle, "down-fast-full": directionalReviewSpec("full", speed, false), "up-fast-full": directionalReviewSpec("full", speed, true), "down-fast-compound": directionalReviewSpec("compound", speed, false), "up-fast-compound": directionalReviewSpec("compound", speed, true)} {
				target, err := FlowTarget(spec, settings)
				if err != nil {
					t.Fatal(err)
				}
				e := speedSupportCloudReview(target, settings)
				e.ID, e.Name, e.Group = fmt.Sprintf("%s-%s-%d", model, name, speed), name, "session-motion"
				entries = append(entries, e)
				if e.Error != "" || e.PeakAcceleration > flowAccelerationBudget*1.001 || e.PeakJerk > flowJerkBudget*1.001 || e.AccelerationJump > .01 || e.VelocityJump > .01 {
					t.Errorf("%s invalid fit: %s jumps %g/%g", e.ID, e.Error, e.VelocityJump, e.AccelerationJump)
				}
				logSpeedSupport(t, e)
			}
		}
	}
	writeSpeedReviewReport(t, path, map[string]any{"schema": "magichandy.motion-atlas.v1", "entries": entries})
}

func directionalReviewSpec(shape string, speed int, upFast bool) FlowSpec {
	s := layeredSpeedFixture(shape, speed)
	s.DirectionalPace = &DirectionalPace{30, 100}
	if upFast {
		s.DirectionalPace = &DirectionalPace{100, 30}
	}
	return s
}

func TestSessionDirectionalRetargetAndStop(t *testing.T) {
	path := os.Getenv("MAGICHANDY_SESSION_RUNTIME_REPORT")
	if path == "" {
		t.Skip("set ignored report path for inert Cloud timing test")
	}
	runSpeedReviewRuntime(t, path, regionalPhraseFixture(75, false), directionalReviewSpec("compound", 75, false))
}
