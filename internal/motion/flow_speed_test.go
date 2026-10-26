package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func layeredSpeedFixture(shape string, speed int) FlowSpec {
	spec := DefaultFlowSpec()
	spec.MinPercent, spec.MaxPercent = 0, 100
	spec.RangeFloorPercent, spec.RangeCeilingPercent = 100, 100
	spec.SpeedPercent, spec.PaceVariationPercent, spec.LoopCycles = speed, 0, 6
	if shape == "short" {
		spec.MinPercent, spec.RangeFloorPercent, spec.RangeCeilingPercent = 80, 20, 20
	}
	if shape == "compound" {
		spec.RangeFloorPercent = 28
		spec.Phrase = &FlowPhrase{Strokes: []PhraseStroke{
			{BottomPercent: 72, TopPercent: 100, Count: 2, RatePercent: 100},
			{BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: 100},
		}}
	}
	return spec
}

func TestLayeredSpeedMatchesPhysicallyAcceptedTiming(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	// Six-cycle periods captured from the separately auditioned candidate.
	for shape, period := range map[string]int64{"full": 4116, "short": 1536, "compound": 2660} {
		t.Run(shape, func(t *testing.T) {
			spec := layeredSpeedFixture(shape, 100)
			target, err := FlowTarget(spec, settings)
			if err != nil {
				t.Fatal(err)
			}
			plan := NewMotionPlan("accepted", target, settings, 0, 0, time.Unix(0, 0))
			if plan.PeriodMillis != period || len(plan.curve.authoredKnots) != 13 {
				t.Fatalf("auditioned timing or stroke count changed: period=%d turns=%d", plan.PeriodMillis, len(plan.curve.authoredKnots))
			}
			if plan.Perceptual.CommandedPeakVelocityPerSecond > referenceTravelRateForSpeed(100, settings.HandyModel) ||
				maximumPlanAcceleration(plan) > runtimeMaxAccelerationPercentPerSecond2 || maximumPlanJerk(plan) > runtimeMaxJerkPercentPerSecond3 {
				t.Fatal("accepted fit exceeded shared motion envelope")
			}
		})
	}
}

func TestLayeredSlowPaceAndExplicitSoftnessStayAuthored(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, shape := range []string{"full", "short", "compound"} {
		for _, speed := range []int{1, 5, 10} {
			spec := layeredSpeedFixture(shape, speed)
			target, err := FlowTarget(spec, settings)
			if err != nil {
				t.Fatal(err)
			}
			plan := NewMotionPlan("slow", target, settings, 0, 0, time.Unix(0, 0))
			rate := referenceTravelRateForSpeed(speed, settings.HandyModel)
			if got := plan.Perceptual.CommandedMeanTravelPerSecond; got < rate*.97 || got > rate*1.001 {
				t.Fatalf("%s at %d%% changed unconstrained travel rate: %.3f versus %.3f", shape, speed, got, rate)
			}
		}
	}
	for _, softness := range []int{25, 70, 100} {
		spec := layeredSpeedFixture("full", 100)
		spec.TurnSoftnessPercent = softness
		legs, durations := flowLegs(spec, settings.HandyModel)
		budget := legBudget{referenceTravelRateForSpeed(100, settings.HandyModel), flowAccelerationBudget, flowJerkBudget}
		curves, err := fitLegCurves(legs, durations, budget)
		if err != nil {
			t.Fatal(err)
		}
		wanted, err := assembleLegCurve(legs, curves, "soft")
		if err != nil {
			t.Fatal(err)
		}
		actual, err := compileFlowCurve(spec, settings.HandyModel)
		if err != nil {
			t.Fatal(err)
		}
		for at := 0.; at < float64(actual.duration); at += 7 {
			if math.Abs(actual.sampleFloat(at)-wanted.sampleFloat(at)) > 1e-8 {
				t.Fatal("cruise shaping erased authored turn softness")
			}
		}
	}
}

func TestLayeredSpeedRetainsPhraseRateContrast(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	spec := layeredSpeedFixture("compound", 100)
	spec.Phrase.Strokes[0].RatePercent = 15
	target, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	plan := NewMotionPlan("contrast", target, settings, 0, 0, time.Unix(0, 0))
	knots := plan.curve.authoredKnots
	shortRate := 28 * 1000 / float64(knots[1].TimeMillis-knots[0].TimeMillis)
	fullRate := 100 * 1000 / float64(knots[5].TimeMillis-knots[4].TimeMillis)
	if shortRate > fullRate*.4 || len(knots) != 13 {
		t.Fatalf("explicit short-stroke slowdown/count lost: short=%f full=%f turns=%d", shortRate, fullRate, len(knots))
	}
}

func TestQuantizedIntervalBudgetUsesOnlyBracketingProbes(t *testing.T) {
	probes := []MotionSample{{TimeMillis: 0}, {TimeMillis: 100}, {TimeMillis: 105}, {TimeMillis: 110}, {TimeMillis: 115}, {TimeMillis: 200}}
	got := quantizedIntervalTolerance(probes, 102, 109, 1, .3)
	want := .5 + .3 - runtimeMaxAccelerationPercentPerSecond2*25/8e6
	if math.Abs(got-want) > 1e-12 || quantizedIntervalTolerance(probes, 99, 109, 1, .3) != .5 {
		t.Fatal("interpolation reserve ignored either local coverage or a real overlapping gap")
	}
}
