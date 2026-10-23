package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestHoldingTargetKeepsBoundsAndIgnoresClickPhase(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 5, 95
	before := MotionTarget{SpeedPercent: 70, Flow: &FlowSpec{MinPercent: 30, MaxPercent: 70}}
	var first MotionTarget
	for _, position := range []float64{30, 50, 70} {
		target, err := HoldingTarget(ActiveMotionState{Target: before, CurrentSample: &MotionSample{PositionPercent: position}}, settings)
		if err != nil {
			t.Fatal(err)
		}
		if target.SpeedPercent > before.SpeedPercent || target.SpeedPercent < settings.SpeedMinPercent || target.Flow.MinPercent < 30 || target.Flow.MaxPercent > 70 {
			t.Fatalf("escaped working bounds: %+v", target)
		}
		if first.Flow != nil && (target.Flow.MinPercent != first.Flow.MinPercent || target.Flow.MaxPercent != first.Flow.MaxPercent) {
			t.Fatal("click phase changed the holding region")
		}
		first = target
	}
	settings.SpeedMinPercent = 70
	if target, err := HoldingTarget(ActiveMotionState{Target: before}, settings); err != nil || target.SpeedPercent != 70 {
		t.Fatalf("ignored saved minimum: %+v %v", target, err)
	}
}

func TestHoldingTargetNeverWidensAnUnusableMeasuredRegion(t *testing.T) {
	settings := config.DefaultSettings().Motion
	for _, end := range []float64{30, 35} {
		_, err := HoldingTarget(ActiveMotionState{Target: MotionTarget{PatternID: "user-pattern", SpeedPercent: 30}, Perceptual: PerceptualSummary{PositionMinPercent: 30, PositionMaxPercent: end, Pace: PaceSummary{RequestedPercent: 30}}}, settings)
		if err == nil {
			t.Fatal("unusable measured region fell back to full range")
		}
	}
}

func TestEdgeHoldTransitionsDoNotInsertTinyReversals(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, creative := range []bool{false, true} {
		for _, speed := range []int{15, 50, 90} {
			spec := DefaultFlowSpec()
			spec.SpeedPercent = speed
			spec.PaceVariationPercent = 0
			if creative {
				g := DefaultGestureSpec()
				spec.Gesture = &g
			}
			before, err := FlowTarget(spec, settings)
			if err != nil {
				t.Fatal(err)
			}
			hold, err := HoldingTarget(ActiveMotionState{Target: before}, settings)
			if err != nil {
				t.Fatal(err)
			}
			for _, pair := range [][2]MotionTarget{{before, hold}, {hold, before}} {
				old := NewMotionPlan("old", pair[0], settings, 0, 0, time.Unix(0, 0))
				for _, phase := range []float64{.17, .43, .71, .93} {
					at := int64(float64(old.PeriodMillis) * phase)
					next := old.Retarget("new", pair[1], settings, at, time.Unix(0, 0))
					bridge := layeredTurnTransition(old, nil, &next, at)
					if bridge == nil {
						bridge = newPlanTransition(old, nil, at)
					}
					turns, direction := 0, 0
					last := old.SampleAt(at).PositionPercent
					for now := at; now <= bridge.endMillis; now += 5 {
						x := sampleMotionPath(next, bridge, now).PositionPercent
						v := sampleMotionPath(next, bridge, now+1).PositionPercent - sampleMotionPath(next, bridge, now-1).PositionPercent
						nextDirection := curveDirection(v)
						if nextDirection != 0 && direction != 0 && nextDirection != direction {
							if turns > 0 && math.Abs(x-last) < 1 {
								t.Fatalf("tiny reversal creative=%t speed=%d phase=%.2f", creative, speed, phase)
							}
							turns++
							last = x
						}
						if nextDirection != 0 {
							direction = nextDirection
						}
					}
				}
			}
		}
	}
}
