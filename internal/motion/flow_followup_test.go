package motion

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestLayeredMaximumToGentlePaceAcrossDeviceProfiles(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		for _, shape := range []string{"full", "short", "compound"} {
			for _, speed := range []int{1, 5} {
				for _, phase := range []float64{.01, .17, .43, .71, .93, .999} {
					t.Run(fmt.Sprintf("%s/%s/100-%d/%.3f", model, shape, speed, phase), func(t *testing.T) {
						before, err := FlowTarget(layeredSpeedFixture(shape, 100), settings)
						if err != nil {
							t.Fatal(err)
						}
						after, err := FlowTarget(layeredSpeedFixture(shape, speed), settings)
						if err != nil {
							t.Fatal(err)
						}
						old := NewMotionPlan("before", before, settings, 0, 0, time.Unix(0, 0))
						at := int64(float64(old.PeriodMillis) * phase)
						next := old.Retarget("after", after, settings, at, time.Unix(0, 0))
						bridge := layeredTurnTransition(old, nil, &next, at)
						if bridge == nil {
							t.Fatal("fast reduction fell back to oscillator crossfade")
						}
						assertLayeredSplice(t, old, next, bridge, settings)
					})
				}
			}
		}
	}
}

func TestLayeredInteriorSpliceKeepsTheAuthoredTurn(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	before, err := FlowTarget(layeredSpeedFixture("full", 5), settings)
	if err != nil {
		t.Fatal(err)
	}
	after, err := FlowTarget(layeredSpeedFixture("full", 100), settings)
	if err != nil {
		t.Fatal(err)
	}
	old := NewMotionPlan("before", before, settings, 0, 0, time.Unix(0, 0))
	at := int64(10)
	next := old.Retarget("after", after, settings, at, time.Unix(0, 0))
	bridge := fitLayeredInteriorSplice(old, &next, at, at, 1, layeredJoinBudget(old, next))
	if bridge == nil {
		t.Fatal("no bounded interior join")
	}
	assertLayeredSplice(t, old, next, bridge, settings)
	if math.Abs(bridge.stroke.velocity(1)) < .001 {
		t.Fatal("fixture joined a turn instead of an interior point")
	}
	knots := next.knotTimesBetween(bridge.endMillis, bridge.endMillis+next.PeriodMillis)
	if len(knots) == 0 {
		t.Fatal("interior join lost the next turn")
	}
	position := next.SampleAt(knots[0]).PositionPercent
	// The wire knot time is rounded to an integer millisecond; allow the
	// resulting sub-hundredth-percent displacement, not a different turn.
	if math.Abs(position-next.focus.apply(next.curve.authoredKnots[1].PositionPercent)) > .01 {
		t.Fatalf("interior join skipped its destination stroke: %f at %dms, phase %.8f, expected %f", position, knots[0], next.PhaseAt(knots[0]), next.focus.apply(next.curve.authoredKnots[1].PositionPercent))
	}
}

func TestLayeredDeliberateVariationIsNotPhysicalLimiting(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	variation, layer := layeredSpeedFixture("full", 10), layeredSpeedFixture("full", 10)
	variation.PaceVariationPercent = 25
	layer.Layers = []FlowLayer{{Axis: "pace", AmountPercent: 100, LowRatePercent: 15, PeriodCycles: 4, Shape: "wave"}}
	phrase := layeredSpeedFixture("compound", 10)
	phrase.Phrase.Strokes[1].RatePercent = 35
	cadence := layeredSpeedFixture("short", 10)
	cadence.MinPercent, cadence.RangeCeilingPercent, cadence.CadenceHoldPercent = 0, 100, 100
	for name, spec := range map[string]FlowSpec{"variation": variation, "pace layer": layer, "phrase rate": phrase, "cadence hold": cadence} {
		t.Run(name, func(t *testing.T) {
			target, err := FlowTarget(spec, settings)
			if err != nil {
				t.Fatal(err)
			}
			plan := NewMotionPlan("variation", target, settings, 0, 0, time.Unix(0, 0))
			pace := plan.Perceptual.Pace
			if pace.EffectivePercent >= 9 || pace.Limited || len(pace.Limiters) != 0 || math.Abs(pace.PlannedMeanTravelPerSecond-pace.CommandedMeanTravelPerSecond) > .01 {
				t.Fatalf("deliberate pace variation was described as physical limiting: %+v", pace)
			}
		})
	}
}

func TestLayeredCruisePreservesFeasibleSlowLegsBesideFastStrokes(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, bottom := range []int{0, 40} {
		for _, rate := range []int{5, 15, 35} {
			spec := layeredSpeedFixture("compound", 100)
			spec.Phrase.Strokes[0].BottomPercent = bottom
			spec.Phrase.Strokes[1].RatePercent = rate
			legs, requested := phraseLegs(spec, settings.HandyModel)
			fitted := slices.Clone(requested)
			original, err := fitLegCurves(legs, fitted, legBudget{referenceTravelRateForSpeed(100, settings.HandyModel), flowAccelerationBudget, flowJerkBudget})
			if err != nil {
				t.Fatal(err)
			}
			curve, err := compilePhraseCurve(spec, settings.HandyModel)
			if err != nil {
				t.Fatal(err)
			}
			checked := 0
			for i, leg := range original {
				if fitted[i] != requested[i] {
					continue
				}
				checked++
				start, end := curve.authoredKnots[i].TimeMillis, curve.authoredKnots[i+1].TimeMillis
				for j := range 101 {
					u := float64(j) / 100
					want := leg.sampleFloat(u * float64(leg.duration))
					got := curve.sampleFloat(float64(start) + u*float64(end-start))
					if math.Abs(got-want) > 1e-8 {
						t.Fatalf("fast neighbor reshaped feasible slow leg: bottom=%d rate=%d leg=%d difference=%g", bottom, rate, i, got-want)
					}
				}
			}
			if checked != 4 {
				t.Fatal("fixture lost its four unconstrained slow legs")
			}
		}
	}
}

func TestLayeredGentleEdgeDoesNotUseFastJerkEnvelope(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, shape := range []string{"short", "compound"} {
		target, err := FlowTarget(layeredSpeedFixture(shape, 5), settings)
		if err != nil {
			t.Fatal(err)
		}
		hold, err := HoldingTarget(ActiveMotionState{Target: target}, settings)
		if err != nil {
			t.Fatal(err)
		}
		old := NewMotionPlan("before", target, settings, 0, 0, time.Unix(0, 0))
		for _, phase := range []float64{.01, .07, .31, .57} {
			at := int64(float64(old.PeriodMillis) * phase)
			next := old.Retarget("hold", hold, settings, at, time.Unix(0, 0))
			bridge := layeredTurnTransition(old, nil, &next, at)
			if bridge == nil {
				t.Fatal("gentle Edge fell back to a crossfade")
			}
			if bridge.stroke.maximumJerk()*1e9 > 24000 {
				t.Fatalf("slow %s Edge used fast jerk: %f", shape, bridge.stroke.maximumJerk()*1e9)
			}
			assertLayeredSplice(t, old, next, bridge, settings)
		}
	}
}

func TestLayeredCompletedBridgeCanSpliceAgainWithoutLosingPlaybackHistory(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	planFor := func(speed int) MotionPlan {
		target, err := FlowTarget(layeredSpeedFixture("full", speed), settings)
		if err != nil {
			t.Fatal(err)
		}
		return NewMotionPlan("first", target, settings, 0, 0, time.Unix(0, 0))
	}
	old := planFor(35)
	first := old.Retarget("first", planFor(85).Target, settings, 1200, time.Unix(0, 0))
	history := layeredTurnTransition(old, nil, &first, 1200)
	if history == nil {
		t.Fatal("fixture needs an initial splice")
	}
	handoff := history.endMillis + 350
	playback := history.endMillis - 50
	var samples []MotionSample
	for at := history.startMillis; at < handoff; at += 7 {
		samples = append(samples, sampleMotionPath(first, history, at))
	}
	engine := &Engine{running: true, runEpoch: 1, plan: first, settings: settings, transition: history, nextSampleMillis: handoff,
		startedAt: time.Unix(0, 0), now: func() time.Time { return time.Unix(0, 0).Add(time.Duration(playback) * time.Millisecond) }}
	if err := engine.retarget(1, planFor(7).Target, "rapid edit"); err != nil {
		t.Fatal(err)
	}
	if engine.transition == nil || engine.transition.stroke == nil {
		t.Fatal("finished bridge still forced an oscillator crossfade")
	}
	for _, sample := range samples {
		if got := sampleMotionPath(engine.plan, engine.transition, sample.TimeMillis).PositionPercent; math.Abs(got-sample.PositionPercent) > 1e-8 {
			t.Fatalf("retarget rewrote already queued history at %dms: %f -> %f", sample.TimeMillis, sample.PositionPercent, got)
		}
	}
	assertLayeredSplice(t, first, engine.plan, engine.transition, settings)
}
