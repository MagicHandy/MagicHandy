package motion

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestLayeredChangedPhraseJoinsWithoutCrossfadeAndPreservesTargets(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, profile := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = profile
		for _, speed := range []int{15, 35, 94} {
			for _, fraction := range []float64{.17, .43, .71, .93} {
				t.Run(fmt.Sprintf("%s/%d/%.2f", profile, speed, fraction), func(t *testing.T) {
					a := phraseFixture(0, 35, 5, speed)
					a.PaceVariationPercent = 0
					b := phraseFixture(100, 72, 3, max(7, speed-10))
					b.PaceVariationPercent = 0
					ta, err := FlowTarget(a, settings)
					if err != nil {
						t.Fatal(err)
					}
					tb, err := FlowTarget(b, settings)
					if err != nil {
						t.Fatal(err)
					}
					old := NewMotionPlan("old", ta, settings, 0, 0, time.Unix(0, 0))
					at := int64(float64(old.PeriodMillis) * fraction)
					next := old.Retarget("next", tb, settings, at, time.Unix(0, 0))
					original := *CloneFlowSpec(next.Target.Flow)
					bridge := layeredTurnTransition(old, nil, &next, at)
					if bridge == nil {
						t.Fatal("representable phrase join used oscillator blend")
					}
					assertLayeredSplice(t, old, next, bridge, settings)
					assertSampledJoinContinuity(t, old, next, bridge)
					if !singleCrestJoin(*bridge.stroke, bridge.stroke.position(1)-bridge.stroke.position(0)) {
						t.Fatal("join introduced a braking shoulder")
					}
					if !reflect.DeepEqual(*next.Target.Flow, original) || next.Target.SpeedPercent != b.SpeedPercent {
						t.Fatal("transition changed the requested score")
					}
					if bridge.endMillis-at > 3000 {
						t.Fatal("join delayed longer than one slow connecting stroke")
					}
				})
			}
		}
	}
}

func TestConnectingStrokeRejectsInteriorBrakingValley(t *testing.T) {
	// These monotone cubic positions have positive velocity throughout. Only
	// the first slows down and accelerates again away from any authored turn.
	valley := quinticSegment{duration: 1000, coefficients: [6]float64{0, .35, -.5, 1.0 / 3}}
	crest := quinticSegment{duration: 1000, coefficients: [6]float64{0, .1, .5, -1.0 / 3}}
	for _, sign := range []float64{1, -1} {
		a, b := valley, crest
		for i := range a.coefficients {
			a.coefficients[i] *= sign
			b.coefficients[i] *= sign
		}
		if singleCrestJoin(a, sign) || !singleCrestJoin(b, sign) {
			t.Fatal("direction-independent shoulder classification failed")
		}
	}
}

func TestLayeredShapeJoinAcceptedGeometryStaysInsideSharedEnvelope(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	accepted := 0
	for _, profile := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = profile
		for i := range 20 {
			a := DefaultFlowSpec()
			a.SpeedPercent = 1 + i*29%100
			a.Seed = uint32(915 + i)
			a.VariationMode = "drift"
			b := *CloneFlowSpec(&a)
			b.RangeCeilingPercent = 25 + i%31
			b.AnchorPercent = i * 19 % 101
			ta, err := FlowTarget(a, settings)
			if err != nil {
				t.Fatal(err)
			}
			tb, err := FlowTarget(b, settings)
			if err != nil {
				t.Fatal(err)
			}
			old := NewMotionPlan("old", ta, settings, 0, 0, time.Unix(0, 0))
			at := old.PeriodMillis * int64(3+i*13%94) / 100
			next := old.Retarget("next", tb, settings, at, time.Unix(0, 0))
			if bridge := layeredTurnTransition(old, nil, &next, at); bridge != nil {
				accepted++
				assertLayeredSplice(t, old, next, bridge, settings)
				assertSampledJoinContinuity(t, old, next, bridge)
			}
		}
	}
	if accepted == 0 {
		t.Fatal("matrix tested no accepted joins")
	}
}

func assertSampledJoinContinuity(t *testing.T, previous, next MotionPlan, bridge *planTransition) {
	t.Helper()
	for _, at := range []int64{bridge.strokeStartMillis, bridge.endMillis} {
		p := func(offset int64) float64 { return sampleMotionPath(next, bridge, at+offset).PositionPercent }
		leftV, rightV := (3*p(0)-4*p(-1)+p(-2))/2, (-3*p(0)+4*p(1)-p(2))/2
		leftA, rightA := p(0)-2*p(-1)+p(-2), p(2)-2*p(1)+p(0)
		// One-sided finite differences span two milliseconds; allow the bounded
		// change in acceleration over that interval, not a discontinuous jump.
		// Each acceleration estimate is centered one millisecond away from
		// the join. Their allowed difference follows the actual jerk envelope.
		// assertLayeredSplice separately checks exact C2 continuity at 1e-7.
		// One-sided differences also carry truncation error in proportion to
		// jerk; the velocity bound scales with the join's jerk envelope.
		_, jerk := joinEnvelope(previous, next)
		accelerationDifference := 2*jerk/1e9 + 1e-8
		velocityDifference := 1e-4 * math.Max(1, jerk/flowJerkBudget)
		if math.Abs(leftV-rightV) > velocityDifference || math.Abs(leftA-rightA) > accelerationDifference {
			t.Fatalf("sampled join is discontinuous at %d: velocity %g/%g acceleration %g/%g", at, leftV, rightV, leftA, rightA)
		}
	}
}

func TestLayeredSoftTurnJoinFollowsAuthoredDirectionAtRest(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	a := phraseFixture(0, 35, 5, 35)
	a.PaceVariationPercent, a.TurnSoftnessPercent = 0, 100
	b := phraseFixture(100, 72, 3, 25)
	b.PaceVariationPercent, b.TurnSoftnessPercent = 0, 100
	ta, _ := FlowTarget(a, settings)
	tb, _ := FlowTarget(b, settings)
	old := NewMotionPlan("old", ta, settings, 0, 0, time.Unix(0, 0))
	accepted := 0
	for i := 1; i < len(old.curve.authoredKnots)-1; i++ {
		at := int64(math.Ceil(float64(old.curve.authoredKnots[i].TimeMillis) * float64(old.PeriodMillis) / float64(old.curve.duration)))
		next := old.Retarget("next", tb, settings, at, time.Unix(0, 0))
		if bridge := joinContinuousTurn(old, &next, at, at, 0); bridge != nil {
			accepted++
			direction := old.curve.authoredKnots[i+1].PositionPercent - old.curve.authoredKnots[i].PositionPercent
			if direction*(bridge.stroke.position(1)-bridge.stroke.position(0)) <= 0 {
				t.Fatal("soft turn continued the old stroke past its destination")
			}
			assertLayeredSplice(t, old, next, bridge, settings)
			assertSampledJoinContinuity(t, old, next, bridge)
		}
	}
	if accepted == 0 {
		t.Fatal("fixture exercised no joins")
	}
}

func TestLayeredShapeJoinSharesPlaybackAndStop(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(64), time.Hour)
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	a := phraseFixture(0, 35, 5, 35)
	a.PaceVariationPercent = 0
	b := phraseFixture(100, 72, 3, 25)
	b.PaceVariationPercent = 0
	ta, err := FlowTarget(a, settings)
	if err != nil {
		t.Fatal(err)
	}
	tb, err := FlowTarget(b, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Start(t.Context(), ta, settings); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	if _, err = engine.ApplyTarget(t.Context(), tb, "compound edit"); err != nil {
		t.Fatal(err)
	}
	engine.mu.Lock()
	spliced := engine.transition != nil && engine.transition.stroke != nil
	engine.mu.Unlock()
	if !spliced || countCommands(fake.Commands(), transport.CommandKindPointsPlay) != 1 {
		t.Fatal("shape join bypassed shared playback")
	}
	if _, err = engine.Stop(t.Context(), "review stop"); err != nil {
		t.Fatal(err)
	}
	count := len(fake.Commands())
	if _, err = engine.RefreshSettings(t.Context(), settings, "after stop"); err != nil {
		t.Fatal(err)
	}
	if engine.Snapshot().Running || len(fake.Commands()) != count {
		t.Fatal("stopped join produced device work")
	}
}

func TestLayeredShapeJoinLeavesOtherMotionAndActiveTransitionsAlone(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	a := phraseFixture(0, 35, 5, 35)
	target, err := FlowTarget(a, settings)
	if err != nil {
		t.Fatal(err)
	}
	old := NewMotionPlan("old", target, settings, 0, 0, time.Unix(0, 0))
	next := old
	if layeredTurnTransition(old, newPlanTransition(old, nil, 0), &next, 1) != nil {
		t.Fatal("discarded an active transition")
	}
	g := DefaultGestureSpec()
	a.Phrase = nil
	a.Gesture = &g
	target, err = FlowTarget(a, settings)
	if err != nil {
		t.Fatal(err)
	}
	next = NewMotionPlan("creative", target, settings, 0, 0, time.Unix(0, 0))
	if layeredTurnTransition(old, nil, &next, 1) != nil {
		t.Fatal("unreviewed Creative v2 transition was changed")
	}
}
