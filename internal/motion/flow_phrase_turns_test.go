package motion

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func regionalPhraseFixture(speed int, mirror bool) FlowSpec {
	s := layeredSpeedFixture("full", speed)
	s.RangeFloorPercent = 30
	s.Phrase = &FlowPhrase{Regions: []PhraseRegion{
		{PhraseStroke: PhraseStroke{0, 30, 4, 100}, Cycle: "up_down"},
		{PhraseStroke: PhraseStroke{70, 100, 2, 100}, Cycle: "down_up"},
	}}
	if mirror {
		for i := range s.Phrase.Regions {
			r := &s.Phrase.Regions[i]
			r.BottomPercent, r.TopPercent = 100-r.TopPercent, 100-r.BottomPercent
			if r.Cycle == "up_down" {
				r.Cycle = "down_up"
			} else {
				r.Cycle = "up_down"
			}
		}
	}
	return s
}

func TestRegionalPhraseKeepsCyclesAndBoundedSmoothLoop(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		for _, speed := range []int{5, 45, 100} {
			for _, mirror := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/%t", model, speed, mirror), func(t *testing.T) {
					s := regionalPhraseFixture(speed, mirror)
					target, err := FlowTarget(s, settings)
					if err != nil {
						t.Fatal(err)
					}
					plan := NewMotionPlan("ordered", target, settings, 0, 0, time.Unix(0, 0))
					knots := plan.curve.authoredKnots
					if (len(knots)-1)%len(s.Phrase.OrderedTurns()) != 0 {
						t.Fatal("motif gained or lost a reversal")
					}
					for i, k := range knots {
						want := s.Phrase.OrderedTurns()[(i+len(s.Phrase.OrderedTurns())-1)%len(s.Phrase.OrderedTurns())].PositionPercent
						if math.Abs(k.PositionPercent-float64(want)) > 1e-8 {
							t.Fatalf("turn%d=%f want%d", i, k.PositionPercent, want)
						}
					}
					if maximumPlanAcceleration(plan) > flowAccelerationBudget*1.001 || maximumPlanJerk(plan) > flowJerkBudget*1.001 || plan.Perceptual.CommandedPeakVelocityPerSecond > referenceTravelRateForSpeed(100, model)*1.001 {
						t.Fatal("ordered motif exceeded the shared envelope")
					}
					for i, q := range plan.curve.quintics {
						next := plan.curve.quintics[(i+1)%len(plan.curve.quintics)]
						if math.Abs(q.velocity(1)-next.velocity(0)) > 1e-7 || math.Abs(q.acceleration(1)-next.acceleration(0)) > 1e-7 {
							t.Fatal("lost C2 continuity")
						}
					}
					for i := 1; i < len(knots); i++ {
						if knots[i].PositionPercent == knots[i-1].PositionPercent {
							t.Fatal("inserted a stationary hold")
						}
					}
				})
			}
		}
	}
}

func TestLayeredTraceIncludesPaceProfileAndTurnShaping(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	spec := layeredSpeedFixture("full", 100)
	spec.TurnSoftnessPercent = 85
	target, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	plan := NewMotionPlan("trace", target, settings, 0, 0, time.Unix(0, 0))
	trace := traceTargetWithPace(target, settings, plan.Perceptual.Pace)
	if trace.MotionKind != "flow" || trace.HandyModel != config.HandyModelOriginal || trace.TurnSoftnessPercent != 85 || trace.CommandedMeanTravel < 170 || trace.CommandedMeanTravel > 180 || trace.EffectiveSpeedPercent >= 50 || !trace.PaceLimited {
		t.Fatalf("trace hides Layered shaping: %+v", trace)
	}
}
