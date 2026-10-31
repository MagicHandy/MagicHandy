package motion

import (
	"math"
	"slices"
	"testing"
)

func TestDirectionalFieldLimitsSlowdownToNeighboringTurns(t *testing.T) {
	for _, upFast := range []bool{false, true} {
		legs := []gestureLeg{{from: 0, to: 100}, {from: 100, to: 0}, {from: 0, to: 100}, {from: 100, to: 90}, {from: 90, to: 100}, {from: 100, to: 0}, {from: 0, to: 100}, {from: 100, to: 0}}
		requested := make([]int64, len(legs))
		for i, leg := range legs {
			rate := 200.
			if (leg.to > leg.from) != upFast {
				rate *= .85
			}
			requested[i] = int64(math.Ceil(math.Abs(leg.to-leg.from) / rate * 1000))
		}
		budget := topSpeedBudget(78, "handy_original")
		fitted := slices.Clone(requested)
		curves, err := fitDirectionalFieldCurves(legs, fitted, budget, upFast)
		if err != nil {
			t.Fatal(err)
		}
		uniform := slices.Clone(requested)
		if _, err := fitLayeredRhythmCurves(legs, uniform, budget); err != nil {
			t.Fatal(err)
		}
		var localMillis, uniformMillis int64
		for i, leg := range legs {
			localMillis += fitted[i]
			uniformMillis += uniform[i]
			if fitted[i] < requested[i] {
				t.Fatal("sped up an authored leg")
			}
			if legTimeScale(curves[i], budget.forLeg(leg)) > 1.000001 {
				t.Fatal("exceeded kinematic bounds")
			}
			next := (i + 1) % len(legs)
			if math.Abs(curves[i].accelerationFloat(float64(curves[i].duration))-curves[next].accelerationFloat(0)) > 1e-8 {
				t.Fatal("lost acceleration continuity")
			}
			if (leg.to > leg.from) == upFast {
				for _, neighbor := range []int{(i + len(legs) - 1) % len(legs), next} {
					if float64(fitted[neighbor])/float64(requested[neighbor])+1e-10 < float64(fitted[i])/float64(requested[i]) {
						t.Fatal("inverted the authored directional contrast")
					}
				}
			}
		}
		if float64(localMillis) > .9*float64(uniformMillis) {
			t.Fatalf("one short reversal still slowed the whole field: local=%d uniform=%d", localMillis, uniformMillis)
		}
	}
}

func TestDirectionalFieldFallsBackWhenLocalPassBudgetIsExhausted(t *testing.T) {
	legs := []gestureLeg{{from: 0, to: 100}, {from: 100, to: 90}, {from: 90, to: 100}, {from: 100, to: 0}}
	requested := []int64{500, 59, 50, 589}
	budget := topSpeedBudget(100, "handy_original")
	durations := slices.Clone(requested)
	// One pass cannot propagate and then re-fit this constrained short turn.
	curves, err := fitDirectionalFieldPasses(legs, durations, budget, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	common := slices.Clone(requested)
	if _, err := fitLayeredRhythmCurves(legs, common, budget); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(durations, common) {
		t.Fatalf("fallback did not use the original requested clock: %v vs %v", durations, common)
	}
	for i, curve := range curves {
		if legTimeScale(curve, budget.forLeg(legs[i])) > 1.000001 {
			t.Fatal("fallback exceeded the shared envelope")
		}
		if durations[i] < requested[i] {
			t.Fatal("fallback sped up an authored leg")
		}
	}
}

func TestDirectionalFieldLeavesDistantUnconstrainedTimingAlone(t *testing.T) {
	legs := []gestureLeg{{from: 0, to: 100}, {from: 100, to: 0}, {from: 0, to: 100}, {from: 100, to: 90}, {from: 90, to: 100}, {from: 100, to: 0}, {from: 0, to: 100}, {from: 100, to: 0}}
	requested := []int64{2000, 2353, 2000, 59, 50, 2353, 2000, 2353}
	for _, speed := range []int{78, 100} {
		fitted := slices.Clone(requested)
		if _, err := fitDirectionalFieldCurves(legs, fitted, topSpeedBudget(speed, "handy_original"), true); err != nil {
			t.Fatal(err)
		}
		for _, i := range []int{0, 1, 7} {
			if fitted[i] != requested[i] {
				t.Fatalf("speed=%d distant leg %d changed from %d to %d", speed, i, requested[i], fitted[i])
			}
		}
		if fitted[4] <= requested[4] {
			t.Fatal("fixture did not constrain the short flick")
		}
	}
}
