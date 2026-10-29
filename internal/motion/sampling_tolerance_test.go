package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestSamplingToleranceUsesTrajectoryRatherThanRoundedWireHistory(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, speed := range []int{5, 25, 60, 75, 85, 92, 100} {
		target, err := FlowTarget(layeredSpeedFixture("full", speed), settings)
		if err != nil {
			t.Fatal(err)
		}
		plan := NewMotionPlan("reserve", target, settings, 0, 0, time.Unix(0, 0))
		engine := &Engine{plan: plan, settings: settings}
		budget := engine.samplingTolerance(.3)
		if budget.acceleration < maximumPlanAcceleration(plan) {
			t.Fatal("interpolation bound is below the analytic trajectory acceleration")
		}
		// A retimed wire anchor can differ from the true path by half an
		// encoded position unit one millisecond before the next real probe.
		// It must not be differentiated into a fictitious acceleration spike.
		anchor := plan.SampleAt(1000)
		anchor.PositionPercent = math.Round(anchor.PositionPercent)
		anchor.TimeMillis += 3
		engine.lastSample = &anchor
		if engine.samplingTolerance(.3) != budget {
			t.Fatal("wire rounding changed the planned interpolation budget")
		}
		if speed <= 75 && budget.acceleration != runtimeMaxAccelerationPercentPerSecond2 {
			t.Fatal("ordinary-speed sparse sampling changed")
		}
	}
}

func TestSamplingReserveReleasesExpiredFastTransition(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	fast, _ := FlowTarget(layeredSpeedFixture("full", 100), settings)
	slow, _ := FlowTarget(layeredSpeedFixture("full", 5), settings)
	previous := NewMotionPlan("fast", fast, settings, 0, 0, time.Unix(0, 0))
	next := NewMotionPlan("slow", slow, settings, 0, 0, time.Unix(0, 0))
	engine := &Engine{plan: next, settings: settings, transition: newPlanTransition(previous, nil, 1000)}
	if engine.samplingTolerance(.3).acceleration <= runtimeMaxAccelerationPercentPerSecond2 {
		t.Fatal("live fast history lost its wider interpolation bound")
	}
	engine.nextSampleMillis = engine.transition.endMillis
	if engine.samplingTolerance(.3).acceleration != runtimeMaxAccelerationPercentPerSecond2 {
		t.Fatal("expired fast motion densified the slow destination")
	}
}

func TestTopSpeedDiagnosticsRespectContentEnvelope(t *testing.T) {
	legacy := MotionTarget{SpeedPercent: 100}
	a, j := targetMotionEnvelope(legacy)
	if a != runtimeMaxAccelerationPercentPerSecond2 || j != runtimeMaxJerkPercentPerSecond3 {
		t.Fatal("legacy content was reported against the sharper prepared-motion envelope")
	}
	legacy.prepared = &preparedMotion{acceleration: 2400, jerk: 24000}
	a, j = targetMotionEnvelope(legacy)
	if a != 2400 || j != 24000 {
		t.Fatal("content's narrower limits were lost")
	}
}
