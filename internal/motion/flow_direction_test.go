package motion

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestLayeredDirectionalPaceKeepsSmoothBoundedDistinctLegs(t *testing.T) {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		limits.HandyModel = model
		for _, speed := range []int{5, 45, 100} {
			for _, span := range []int{10, 30, 100} {
				for _, kind := range []string{"field", "strokes", "regions"} {
					for _, rate := range []int{1, 25, 55} {
						for _, mirror := range []bool{false, true} {
							t.Run(fmt.Sprintf("%s/%d/%d/%s/%d/%t", model, speed, span, kind, rate, mirror), func(t *testing.T) {
								s := layeredSpeedFixture("full", speed)
								s.MaxPercent, s.RangeFloorPercent, s.RangeCeilingPercent = span, span, span
								s.DirectionalPace = &DirectionalPace{rate, 100}
								if mirror {
									s.DirectionalPace = &DirectionalPace{100, rate}
								}
								if kind == "strokes" {
									s.Phrase = &FlowPhrase{Strokes: []PhraseStroke{{0, span, 2, 100}}}
								}
								if kind == "regions" {
									s.Phrase = &FlowPhrase{Regions: []PhraseRegion{{PhraseStroke{0, span, 2, 100}, "down_up"}}}
								}
								target, err := FlowTarget(s, limits)
								if err != nil {
									t.Fatal(err)
								}
								plan := NewMotionPlan("direction", target, limits, 0, 0, time.Unix(0, 0))
								assertDirectionPlan(t, plan, model, mirror)
							})
						}
					}
				}
			}
		}
	}
}

func TestLayeredDirectionalDefaultAndClone(t *testing.T) {
	s := layeredSpeedFixture("full", 45)
	a, err := compileFlowCurve(s, config.HandyModelOriginal)
	if err != nil {
		t.Fatal(err)
	}
	s.DirectionalPace = &DirectionalPace{100, 100}
	b, err := compileFlowCurve(s, config.HandyModelOriginal)
	if err != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("neutral rates changed the legacy curve", err)
	}
	clone := CloneFlowSpec(&s)
	clone.DirectionalPace.UpRatePercent = 20
	if s.DirectionalPace.UpRatePercent != 100 {
		t.Fatal("direction rates alias active score")
	}
	for _, p := range []DirectionalPace{{0, 100}, {100, 0}, {-1, 100}, {101, 100}, {100, 101}} {
		s.DirectionalPace = &p
		if s.Validate(config.DefaultSettings().Motion) == nil {
			t.Fatal("accepted invalid direction rate")
		}
	}
}

func TestDirectionalRegionalClockKeepsRelativeTimingAcrossSeam(t *testing.T) {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	s := regionalPhraseFixture(100, false)
	s.DirectionalPace = &DirectionalPace{30, 100}
	s.Phrase.Regions[0].RatePercent = 35
	_, requested := phraseLegs(s, limits.HandyModel)
	curve, err := compilePhraseCurve(s, limits.HandyModel)
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := math.Inf(1), 0.0
	for i, d := range requested {
		factor := float64(curve.authoredKnots[i+1].TimeMillis-curve.authoredKnots[i].TimeMillis) / float64(d)
		lo, hi = math.Min(lo, factor), math.Max(hi, factor)
	}
	if hi/lo > 1.03 {
		t.Fatalf("a connector or seam inverted authored relative timing: factors %g..%g", lo, hi)
	}
}

func assertDirectionPlan(t *testing.T, plan MotionPlan, model string, mirror bool) {
	t.Helper()
	if maximumPlanAcceleration(plan) > envelopeAcceleration(plan)*1.001 || maximumPlanJerk(plan) > envelopeJerk(plan)*1.001 || plan.Perceptual.CommandedPeakVelocityPerSecond > referenceTravelRateForSpeed(100, model)*1.001 {
		t.Fatal("directional motion exceeded shared envelope")
	}
	var up, down int64
	for i, p := range plan.curve.authoredKnots[1:] {
		before := plan.curve.authoredKnots[i]
		if p.PositionPercent > before.PositionPercent {
			up += p.TimeMillis - before.TimeMillis
		} else if p.PositionPercent < before.PositionPercent {
			down += p.TimeMillis - before.TimeMillis
		} else {
			t.Fatal("inserted stationary leg")
		}
	}
	fast, slow := down, up
	if mirror {
		fast, slow = up, down
	}
	if slow <= fast {
		t.Fatalf("direction contrast lost: up=%d down=%d", up, down)
	}
	for i, q := range plan.curve.quintics {
		next := plan.curve.quintics[(i+1)%len(plan.curve.quintics)]
		if math.Abs(q.velocity(1)-next.velocity(0)) > 1e-7 || math.Abs(q.acceleration(1)-next.acceleration(0)) > 1e-7 {
			t.Fatal("lost C2 join")
		}
	}
}
