package motion

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func gestureFixture() FlowSpec {
	s := DefaultFlowSpec()
	g := DefaultGestureSpec()
	s.Gesture, s.RangeFloorPercent = &g, 10
	return s
}

func TestGestureKinematicsAcrossSeedsDirectionsAndDeviceEnvelopes(t *testing.T) {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Pro} {
		limits.HandyModel = model
		for _, speed := range []int{10, 45, 85} {
			for _, focus := range []int{0, 50, 100} {
				for _, seed := range []uint32{1, 17, 23981} {
					s := gestureFixture()
					s.SpeedPercent, s.Seed = speed, seed
					s.Gesture.FocusPercent, s.Gesture.FocusWidthPercent, s.Gesture.FocusMixPercent = focus, 45, 55
					s.Gesture.InertiaPercent, s.Gesture.ReboundCount, s.Gesture.ReboundDecayPercent = 100, 4, 85
					s.Gesture.FasterDirection, s.Gesture.ContrastPercent = "tip", 80
					target, err := FlowTarget(s, limits)
					if err != nil {
						t.Fatal(err)
					}
					plan := NewMotionPlan("gesture-test", target, limits, 0, 0, time.Unix(0, 0))
					factor := float64(plan.PeriodMillis) / float64(plan.curve.duration)
					if factor > 1.02 {
						t.Fatalf("local stroke fitting fell back to whole-score slowdown: %.4f", factor)
					}
					if plan.Perceptual.CommandedPeakVelocityPerSecond > referenceTravelRateForSpeed(100, model)*1.001 {
						t.Fatal("velocity limit")
					}
					if a := plan.curve.maximumAccelerationPerMillis2() * 1e6 / (factor * factor); a > runtimeMaxAccelerationPercentPerSecond2*1.001 {
						t.Fatalf("acceleration %f", a)
					}
					if j := plan.curve.maximumJerkPerMillis3() * 1e9 / (factor * factor * factor); j > runtimeMaxJerkPercentPerSecond3*1.001 {
						t.Fatalf("jerk %f", j)
					}
					if gap := reversalGap(plan.curve.authoredKnots, plan.curve.duration, true); float64(gap)*factor < float64(runtimeMinimumReversalGapMillis)-0.01 {
						t.Fatal("reversal gap")
					}
					for index, left := range plan.curve.quintics {
						right := plan.curve.quintics[(index+1)%len(plan.curve.quintics)]
						if math.Abs(left.position(1)-right.position(0)) > 1e-7 || math.Abs(left.velocity(1)-right.velocity(0)) > 1e-7 || math.Abs(left.acceleration(1)-right.acceleration(0)) > 1e-7 {
							t.Fatalf("C2 seam %d", index)
						}
						for _, u := range []float64{0, .25, .5, .75, 1} {
							if x := left.position(u); x < 4.9999 || x > 95.0001 {
								t.Fatal("escaped outer band")
							}
						}
					}
				}
			}
		}
	}
}

func TestGestureNativeCharacterAndReplay(t *testing.T) {
	s := gestureFixture()
	s.Gesture.FocusPercent = 0
	s.Gesture.FocusWidthPercent = 45
	s.Gesture.ReboundCount, s.Gesture.ReboundDecayPercent = 3, 75
	legs := gestureLegs(s)
	full, local, shrinking := false, false, false
	for index, leg := range legs {
		span := math.Abs(leg.to - leg.from)
		full = full || span == 90
		local = local || (span < 46 && span >= 10)
		if index > 3 && span < math.Abs(legs[index-2].to-legs[index-2].from) && leg.to == 5 {
			shrinking = true
		}
	}
	if !full || !local || !shrinking {
		t.Fatalf("missing broad/local/rebound travel: %v/%v/%v", full, local, shrinking)
	}
	if !reflect.DeepEqual(legs, gestureLegs(s)) {
		t.Fatal("saved realization not replayable")
	}
	s.Seed++
	if reflect.DeepEqual(legs, gestureLegs(s)) {
		t.Fatal("seed does not develop motion")
	}
	s.Gesture.FocusMixPercent = 100
	for _, leg := range gestureLegs(s) {
		if leg.from > 50.001 || leg.to > 50.001 {
			t.Fatal("focus-only stroke escaped local band")
		}
	}
	s.Gesture.FocusMixPercent = 0
	for _, leg := range gestureLegs(s) {
		if math.Abs(leg.to-leg.from) != 90 {
			t.Fatal("full-only requested but local motion remains")
		}
	}
}

func TestGestureDirectionalTimingAndInertia(t *testing.T) {
	s := gestureFixture()
	s.Gesture.FocusMixPercent = 0
	plain, _, _ := gestureProgress(.5, 0)
	inertial, _, _ := gestureProgress(.5, 1)
	if plain < .49999 || plain > .50001 || inertial >= .3 {
		t.Fatal("inertia did not move the velocity crest")
	}
	s.Gesture.VariationPercent = 0
	s.Gesture.FasterDirection = "tip"
	s.Gesture.ContrastPercent = 80
	legs := gestureLegs(s)
	if legs[0].weight >= legs[1].weight {
		t.Fatal("directional timing is reversed")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	s.Gesture.FocusMixPercent = 100
	s.Gesture.InertiaPercent = 70
	for _, speed := range []int{10, 45, 85} {
		s.SpeedPercent = speed
		curve, err := compileGestureCurve(s, settings.HandyModel)
		if err != nil {
			t.Fatal(err)
		}
		knots := curve.authoredKnots
		up := knots[1].TimeMillis - knots[0].TimeMillis
		down := knots[2].TimeMillis - knots[1].TimeMillis
		if float64(down)/float64(up) < 2.5 {
			t.Fatalf("speed %d erased directional contrast: up=%d down=%d", speed, up, down)
		}
	}
}

func TestGestureTargetRoundTripIsolationAndLiveLimits(t *testing.T) {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	s := gestureFixture()
	target, err := FlowTarget(s, limits)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(target)
	var restored MotionTarget
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	plan := NewMotionPlan("replay", restored, limits, 0, 0, time.Unix(0, 0))
	if err := plan.compilationError(); err != nil {
		t.Fatal(err)
	}
	s.Gesture.FocusPercent = 0
	if target.Flow.Gesture.FocusPercent != 100 {
		t.Fatal("aliasing caller gesture")
	}
	limits.SpeedMaxPercent = 10
	limited := NewMotionPlan("limited", target, limits, 0, 0, time.Unix(0, 0))
	if limited.Target.SpeedPercent != 10 || limited.Perceptual.CommandedMeanTravelPerSecond > referenceTravelRateForSpeed(10, limits.HandyModel)*1.001 {
		t.Fatal("live speed limit bypass")
	}
	for _, bad := range []FlowSpec{func() FlowSpec { v := gestureFixture(); v.Gesture.ReboundCount = 9; return v }(), func() FlowSpec { v := gestureFixture(); v.MinPercent = 80; return v }(), func() FlowSpec { v := gestureFixture(); v.Layers = []FlowLayer{{Axis: "pace"}}; return v }()} {
		if _, err := FlowTarget(bad, limits); err == nil {
			t.Fatal("invalid grammar accepted")
		}
	}
}

func TestCreativeFreshnessPreservesMeaningAndReplay(t *testing.T) {
	d := NormalizeDynamicDefinition(DynamicDefinition{CenterPercent: 50, SpanPercent: 80, SpanMinPercent: 25, SpanProfile: "wander", VariationPercent: 65})
	a := FreshDynamicPhrase(d, 0)
	b := FreshDynamicPhrase(d, a.PhraseSeed)
	if a.PhraseSeed == 0 || a.PhraseSeed == b.PhraseSeed {
		t.Fatal("reused phrase")
	}
	if !reflect.DeepEqual(dynamicContent(a), dynamicContent(a)) {
		t.Fatal("unstable replay")
	}
	a.PhraseSeed, b.PhraseSeed = d.PhraseSeed, d.PhraseSeed
	if !reflect.DeepEqual(a, d) || !reflect.DeepEqual(b, d) {
		t.Fatal("fresh realization changed semantic request")
	}
}
