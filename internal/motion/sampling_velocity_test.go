package motion

import (
	"math"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestBufferedVelocityRefinementKeepsGeometryClockAndFrameEdges(t *testing.T) {
	for _, rate := range []int{12, 25} {
		t.Run(strconv.Itoa(rate), func(t *testing.T) { checkBufferedVelocityRefinement(t, rate) })
	}
}

func checkBufferedVelocityRefinement(t *testing.T, rate int) {
	t.Helper()
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	spec := DefaultFlowSpec()
	spec.MinPercent, spec.MaxPercent, spec.RangeFloorPercent, spec.RangeCeilingPercent = 0, 100, 28, 100
	spec.SpeedPercent, spec.PaceVariationPercent = 58, 0
	spec.Phrase = &FlowPhrase{Strokes: []PhraseStroke{
		{BottomPercent: 72, TopPercent: 100, Count: 2, RatePercent: rate},
		{BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: rate},
	}}
	target, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{plan: NewMotionPlan("slow-phrase", target, settings, 0, 0, time.Unix(0, 0)), settings: settings, preservePlanKnots: true}
	baseline, _, err := engine.fitMotionWindowLocked(0, 3000, bufferedProbeIntervalMillis, 128, 1)
	if err != nil {
		t.Fatal(err)
	}
	refined := engine.refineBufferedVelocityChunk(baseline, 1, 128)
	if len(refined) <= len(baseline) {
		t.Fatal("slow phrase received no representable velocity refinement")
	}
	if !reflect.DeepEqual(refined[:2], baseline[:2]) || !reflect.DeepEqual(refined[len(refined)-2:], baseline[len(baseline)-2:]) {
		t.Fatal("changed an append boundary or its segment velocity")
	}
	for _, anchor := range baseline {
		found := false
		for _, point := range refined {
			found = found || point == anchor
		}
		if !found {
			t.Fatal("moved or removed an existing point", anchor)
		}
	}
	if bufferedMaximumVelocityStep(refined, 1) > bufferedMaximumVelocityStep(baseline, 1)+1e-9 {
		t.Fatal("largest velocity step increased")
	}
	if velocityRefinementMeasuredError(engine.plan, refined) >= velocityRefinementMeasuredError(engine.plan, baseline) {
		t.Fatal("refinement did not reduce independently sampled velocity error")
	}
	checkRefinedWireBounds(t, engine.plan, refined, referenceTravelRateForSpeed(100, settings.HandyModel))
	for _, mode := range []string{"immediate", "transition", "capacity"} {
		t.Run(mode, func(t *testing.T) {
			maximum := 128
			engine.minimumPointIntervalMillis, engine.transition = 0, nil
			switch mode {
			case "immediate":
				engine.minimumPointIntervalMillis = 50
			case "transition":
				engine.transition = &planTransition{startMillis: 0, endMillis: 1000}
			case "capacity":
				maximum = len(baseline)
			}
			if got := engine.refineBufferedVelocityChunk(baseline, 1, maximum); !reflect.DeepEqual(got, baseline) {
				t.Fatal("changed a protected transport/transition or exceeded capacity")
			}
		})
	}
}

func checkRefinedWireBounds(t *testing.T, plan MotionPlan, samples []MotionSample, maximumVelocity float64) {
	t.Helper()
	for _, point := range samples {
		want := plan.SampleAt(point.TimeMillis)
		if point.PlanID != want.PlanID || point.PatternID != want.PatternID || point.ProgramID != want.ProgramID || point.MediaID != want.MediaID {
			t.Fatal("sample lost its source identity", point)
		}
	}
	for i := 1; i < len(samples); i++ {
		left, right := samples[i-1], samples[i]
		if right.TimeMillis <= left.TimeMillis || math.Abs(math.Round(right.PositionPercent)-math.Round(left.PositionPercent))*1000/float64(right.TimeMillis-left.TimeMillis) > maximumVelocity+1e-9 {
			t.Fatal("invalid time ordering or velocity", left, right)
		}
		for at := left.TimeMillis; at <= right.TimeMillis; at++ {
			fraction := float64(at-left.TimeMillis) / float64(right.TimeMillis-left.TimeMillis)
			wire := math.Round(left.PositionPercent) + fraction*(math.Round(right.PositionPercent)-math.Round(left.PositionPercent))
			if math.Abs(wire-plan.SampleAt(at).PositionPercent) > .8 {
				t.Fatal("wire position left its existing error bound", at)
			}
		}
	}
}

// One-millisecond centered differences are independent of the refinement's
// coarse velocity-cost probes. Endpoints are checked separately above.
func velocityRefinementMeasuredError(plan MotionPlan, samples []MotionSample) float64 {
	var errorValue float64
	for i := 1; i < len(samples); i++ {
		left, right := samples[i-1], samples[i]
		wireVelocity := (math.Round(right.PositionPercent) - math.Round(left.PositionPercent)) / float64(right.TimeMillis-left.TimeMillis)
		for at := left.TimeMillis + 1; at < right.TimeMillis; at++ {
			plannedVelocity := (plan.SampleAt(at+1).PositionPercent - plan.SampleAt(at-1).PositionPercent) / 2
			difference := plannedVelocity - wireVelocity
			errorValue += difference * difference
		}
	}
	return errorValue
}
