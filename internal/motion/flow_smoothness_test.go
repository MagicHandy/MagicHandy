package motion

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func layeredTransferFixture() FlowSpec {
	return FlowSpec{MinPercent: 0, MaxPercent: 100, SpeedPercent: 37,
		RangeFloorPercent: 15, RangeCeilingPercent: 30, AnchorPercent: 50,
		MemoryCycles: 10, PaceVariationPercent: 12, VariationMode: "drift", Seed: 841,
		Layers: []FlowLayer{
			{Axis: "pace", AmountPercent: 20, PeriodCycles: 22, PhasePercent: 53, Shape: "drift"},
			{Axis: "center", AmountPercent: 100, PeriodCycles: 8, Shape: "alternate"},
		}}
}

func TestLayeredMovingCenterRetainsBoundsAndContinuousTurnsAcrossProfiles(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, profile := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = profile
		for i := range 48 {
			t.Run(fmt.Sprintf("%s/%d", profile, i), func(t *testing.T) {
				spec := DefaultFlowSpec()
				spec.MinPercent, spec.MaxPercent = 0, 100
				spec.RangeFloorPercent = 10 + i*13%31
				spec.RangeCeilingPercent = spec.RangeFloorPercent + i*7%(101-spec.RangeFloorPercent)
				spec.Seed, spec.MemoryCycles = []uint32{17, 841, 139, 7231}[i%4], 2+i*3%31
				spec.SpeedPercent, spec.PaceVariationPercent = 1+i*29%100, i%41
				spec.TurnSoftnessPercent, spec.LoopCycles = i*11%101, []int{4, 16, 64}[i%3]
				spec.Layers = []FlowLayer{{Axis: "center", AmountPercent: 100, PeriodCycles: 2 + i%31, PhasePercent: i * 17 % 101, Shape: []string{"wave", "drift", "alternate"}[i%3]}}
				target, err := FlowTarget(spec, settings)
				if err != nil {
					t.Fatal(err)
				}
				plan := NewMotionPlan("bounds", target, settings, 0, 0, time.Unix(0, 0))
				if maximumPlanAcceleration(plan) > envelopeAcceleration(plan)*1.001 || maximumPlanJerk(plan) > envelopeJerk(plan)*1.001 {
					t.Fatal("smoothing bypassed the derivative envelope")
				}
				for index, segment := range plan.curve.quintics {
					next := plan.curve.quintics[(index+1)%len(plan.curve.quintics)]
					if math.Abs(segment.velocity(1)-next.velocity(0)) > 1e-7 || math.Abs(segment.acceleration(1)-next.acceleration(0)) > 1e-7 {
						t.Fatal("lost continuous turn derivatives")
					}
					for _, u := range []float64{0, .25, .5, .75, 1} {
						if x := segment.position(u); x < -1e-6 || x > 100+1e-6 {
							t.Fatal("left the requested band")
						}
					}
				}
			})
		}
	}
}

func TestFlowMicroReversalCoalescingKeepsClockAndOuterTurns(t *testing.T) {
	positions := []float64{0, 40, 39.6, 100, 0}
	for rotation := range 4 {
		var legs []gestureLeg
		var times []int64
		for n := range 4 {
			i := (n + rotation) % 4
			legs = append(legs, gestureLeg{from: positions[i], to: positions[i+1], weight: 1, softness: .35})
			times = append(times, int64(100*(i+1)))
		}
		legs, times = coalesceFlowChatter(legs, times)
		if len(legs) != 2 || times[0]+times[1] != 1000 {
			t.Fatalf("rotation %d lost the clock or retained the cancelled stroke: %+v / %v", rotation, legs, times)
		}
		for i, leg := range legs {
			if math.Abs(leg.to-leg.from) != 100 || leg.to != legs[(i+1)%len(legs)].from || leg.softness != .35 {
				t.Fatalf("rotation %d changed outer geometry or softness: %+v", rotation, legs)
			}
		}
	}
	// Slow, subtle travel remains part of the score, even if it is small.
	legs := []gestureLeg{{from: 0, to: 40}, {from: 40, to: 39}, {from: 39, to: 100}, {from: 100, to: 0}}
	times := []int64{100, 400, 300, 400}
	wantLegs, wantTimes := append([]gestureLeg(nil), legs...), append([]int64(nil), times...)
	gotLegs, gotTimes := coalesceFlowChatter(legs, times)
	if !reflect.DeepEqual(gotLegs, wantLegs) || !reflect.DeepEqual(gotTimes, wantTimes) {
		t.Fatal("changed a representable authored turn")
	}
}

func TestFlowRegionTransfersDoNotIntroduceSubPercentReversals(t *testing.T) {
	spec := layeredTransferFixture()
	legs, _ := flowLegs(spec, config.HandyModelOriginal)
	for _, leg := range legs {
		if span := math.Abs(leg.to - leg.from); span < 1 {
			t.Fatalf("moving center introduced a %.4f%% reversal despite a %d%% minimum stroke width", span, spec.RangeFloorPercent)
		}
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	target, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	plan := NewMotionPlan("region-transfer", target, settings, 0, 0, time.Unix(0, 0))
	points := catalogSamples(t, plan, 1)
	assertNoStationaryWireEdges(t, "layered region transfer", points, 1)
}

func TestLayeredTurnsTrackTheCombinedFieldAcrossTheLoopSeam(t *testing.T) {
	for _, softness := range []int{0, 35, 100} {
		spec := layeredTransferFixture()
		spec.TurnSoftnessPercent = softness
		cycles := flowTurningCycles(spec)
		if len(cycles) < 2 {
			t.Fatal("lost the oscillation")
		}
		for _, u := range cycles {
			x, _, _ := spec.flowState(u, config.HandyModelOriginal)
			left, _, _ := spec.flowState(u-.001, config.HandyModelOriginal)
			right, _, _ := spec.flowState(u+.001, config.HandyModelOriginal)
			if (x-left)*(right-x) > 1e-10 {
				t.Fatalf("cycle %.6f is not a combined-field reversal", u)
			}
		}
		curve, err := compileFlowCurve(spec, config.HandyModelOriginal)
		if err != nil {
			t.Fatal(err)
		}
		if len(curve.flowCycles) != len(curve.authoredKnots) {
			t.Fatal("lost field coordinates")
		}
		for i := 1; i < len(curve.flowCycles); i++ {
			if curve.flowCycles[i] <= curve.flowCycles[i-1] {
				t.Fatal("field clock ran backward at the loop seam")
			}
		}
		if math.Abs(curve.flowCycles[len(curve.flowCycles)-1]-curve.flowCycles[0]-spec.cycleCount()) > 1e-8 {
			t.Fatal("changed the field period")
		}
	}
}

func BenchmarkLayeredCompile(b *testing.B) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	spec := layeredTransferFixture()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := FlowTarget(spec, settings); err != nil {
			b.Fatal(err)
		}
	}
}

func TestLayeredPaceEditsRetainNearbyStrokeContext(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	spec := DefaultFlowSpec()
	spec.SpeedPercent, spec.VariationMode = 22, "drift"
	spec.Layers = []FlowLayer{{Axis: "center", AmountPercent: 30, PeriodCycles: 13, Shape: "drift"}}
	target, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	previous := NewMotionPlan("before", target, settings, 0, 0, time.Unix(0, 0))
	spec.SpeedPercent = 35
	target, err = FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range []int{9, 22, 37, 50, 61, 109, 125} {
		knots := previous.curve.authoredKnots
		clock := float64(knots[slot].TimeMillis) + .4*float64(knots[slot+1].TimeMillis-knots[slot].TimeMillis)
		at := int64(clock * float64(previous.PeriodMillis) / float64(previous.curve.duration))
		next := previous.Retarget("after", target, settings, at, time.Unix(0, 0))
		nextTime := next.PhaseAt(at) * float64(next.curve.duration)
		nextSlot := max(0, sort.Search(len(next.curve.authoredKnots), func(i int) bool { return float64(next.curve.authoredKnots[i].TimeMillis) > nextTime })-1)
		if math.Abs(float64(nextSlot-slot)) > 2 || next.DirectionAt(at) != previous.DirectionAt(at) {
			t.Fatalf("pace edit jumped to another part of the Layered phrase: %d -> %d", slot, nextSlot)
		}
		if math.Abs(next.SampleAt(at).PositionPercent-previous.SampleAt(at).PositionPercent) > 2 {
			t.Fatal("nearby handoff lost position continuity")
		}
	}
}
