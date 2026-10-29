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
	// Six-cycle periods. At the top-speed knee the fit is the separately
	// auditioned one. Above it turns sharpen (ADR 0064): 100% must beat the
	// auditioned alpha.75 periods inside its own envelope; its feel awaits
	// physical acceptance.
	atKnee := map[string]int64{"full": 4308, "short": 1536, "compound": 2700}
	auditionedTop := map[string]int64{"full": 4116, "short": 1536, "compound": 2660}
	for shape, period := range atKnee {
		t.Run(shape, func(t *testing.T) {
			for _, speed := range []int{topSpeedKneePercent, 100} {
				target, err := FlowTarget(layeredSpeedFixture(shape, speed), settings)
				if err != nil {
					t.Fatal(err)
				}
				plan := NewMotionPlan("accepted", target, settings, 0, 0, time.Unix(0, 0))
				if len(plan.curve.authoredKnots) != 13 {
					t.Fatalf("%d%%: stroke count changed: turns=%d", speed, len(plan.curve.authoredKnots))
				}
				if speed == topSpeedKneePercent && plan.PeriodMillis != period {
					t.Fatalf("auditioned timing changed at the knee: period=%d", plan.PeriodMillis)
				}
				if speed == 100 && plan.PeriodMillis >= auditionedTop[shape] {
					t.Fatalf("100%% is no faster than the auditioned fit: %d >= %d", plan.PeriodMillis, auditionedTop[shape])
				}
				if plan.Perceptual.CommandedPeakVelocityPerSecond > referenceTravelRateForSpeed(100, settings.HandyModel) ||
					maximumPlanAcceleration(plan) > envelopeAcceleration(plan) || maximumPlanJerk(plan) > envelopeJerk(plan) {
					t.Fatalf("%d%%: fit exceeded its motion envelope", speed)
				}
			}
		})
	}
}

// Requests above the knee kept playing alike near 79% effective; each step up
// must now be faster, while the knee and below keep the runtime envelope
// (ADR 0064).
func TestTopSpeedRequestsKeepGettingFaster(t *testing.T) {
	if acceleration, jerk := topSpeedEnvelope(topSpeedKneePercent); acceleration != runtimeMaxAccelerationPercentPerSecond2 || jerk != runtimeMaxJerkPercentPerSecond3 {
		t.Fatal("the knee must keep the runtime envelope")
	}
	if budget := topSpeedBudget(topSpeedKneePercent, config.HandyModelOriginal); len(cruiseRampCandidates(budget)) != 1 {
		t.Fatal("the knee must keep the ordinary cruise ramps")
	}
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings := config.DefaultSettings().Motion
		settings.HandyModel, settings.SpeedMinPercent, settings.SpeedMaxPercent = model, 1, 100
		previous := 0.0
		for _, speed := range []int{75, 85, 92, 100} {
			target, err := FlowTarget(layeredSpeedFixture("full", speed), settings)
			if err != nil {
				t.Fatal(err)
			}
			effective := NewMotionPlan("top", target, settings, 0, 0, time.Unix(0, 0)).Perceptual.Pace.EffectivePercent
			if effective <= previous {
				t.Fatalf("%s %d%%: effective %.1f%% is not above %.1f%%", model, speed, effective, previous)
			}
			previous = effective
		}
		if previous < 85 {
			t.Fatalf("%s: 100%% reaches only %.1f%% effective", model, previous)
		}
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
		budget := legBudget{velocity: referenceTravelRateForSpeed(100, settings.HandyModel), acceleration: flowAccelerationBudget, jerk: flowJerkBudget}
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
	got := quantizedIntervalTolerance(probes, 102, 109, 1, samplingTolerance{position: .3})
	want := .5 + .3 - runtimeMaxAccelerationPercentPerSecond2*25/8e6
	if math.Abs(got-want) > 1e-12 || quantizedIntervalTolerance(probes, 99, 109, 1, samplingTolerance{position: .3}) != .5 {
		t.Fatal("interpolation reserve ignored either local coverage or a real overlapping gap")
	}
}
