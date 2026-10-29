package motion

import (
	"fmt"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestTopSpeedSweepDoesNotReverseTheSpeedControl(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		for _, shape := range []string{"full", "short", "compound", "default", "creative-v2"} {
			t.Run(fmt.Sprintf("%s/%s", model, shape), func(t *testing.T) {
				previous := 0.0
				for speed := 75; speed <= 100; speed++ {
					spec := layeredSpeedFixture(shape, speed)
					switch shape {
					case "default":
						spec = DefaultFlowSpec()
					case "creative-v2":
						spec = gestureFixture()
					}
					spec.SpeedPercent = speed
					target, err := FlowTarget(spec, settings)
					if err != nil {
						t.Fatal(err)
					}
					plan := NewMotionPlan("sweep", target, settings, 0, 0, time.Unix(0, 0))
					rate := plan.Perceptual.CommandedMeanTravelPerSecond
					// Every stroke is timed in integer milliseconds. A different
					// feasible fit can round up by one millisecond per stroke; a
					// larger regression is not timing resolution.
					rounding := float64(len(plan.curve.authoredKnots)-1) / float64(plan.PeriodMillis)
					if rate < previous*(1-rounding)-1e-8 {
						t.Errorf("%d%% became slower: %.4f from %.4f percent/s", speed, rate, previous)
					}
					previous = rate
				}
			})
		}
	}
}

func BenchmarkTopSpeedCompile(b *testing.B) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, shape := range []string{"full", "compound", "default"} {
		for _, speed := range []int{25, 100} {
			b.Run(fmt.Sprintf("%s/%d", shape, speed), func(b *testing.B) {
				spec := layeredSpeedFixture(shape, speed)
				if shape == "default" {
					spec = DefaultFlowSpec()
					spec.SpeedPercent = speed
				}
				b.ReportAllocs()
				for b.Loop() {
					if _, err := FlowTarget(spec, settings); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
