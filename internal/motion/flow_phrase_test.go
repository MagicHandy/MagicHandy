package motion

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func phraseFixture(anchor, turn, count, speed int) FlowSpec {
	s := DefaultFlowSpec()
	s.MinPercent, s.MaxPercent, s.RangeFloorPercent, s.RangeCeilingPercent = 0, 100, 10, 100
	s.SpeedPercent = speed
	full := 0
	if anchor == 0 {
		full = 100
	}
	s.Phrase = &FlowPhrase{Strokes: []PhraseStroke{{BottomPercent: min(anchor, turn), TopPercent: max(anchor, turn), Count: count, RatePercent: 100}, {BottomPercent: min(anchor, full), TopPercent: max(anchor, full), Count: 1, RatePercent: 100}}}
	return s
}

func TestLayeredPhraseCountsReachAndSmoothTurns(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, profile := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = profile
		for _, speed := range []int{7, 22, 46, 83, 100} {
			for _, shape := range [][3]int{{100, 87, 2}, {100, 62, 2}, {0, 28, 3}, {0, 47, 4}} {
				t.Run(fmt.Sprintf("%s/%d/%v", profile, speed, shape), func(t *testing.T) {
					s := phraseFixture(shape[0], shape[1], shape[2], speed)
					target, err := FlowTarget(s, settings)
					if err != nil {
						t.Fatal(err)
					}
					plan := NewMotionPlan("phrase", target, settings, 0, 0, time.Unix(0, 0))
					assertPhrasePlan(t, plan, shape, settings)
				})
			}
		}
	}
}

func TestLayeredPhrasePaceSplicePreservesStroke(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, speeds := range [][2]int{{22, 46}, {83, 17}, {7, 85}} {
		for _, fraction := range []float64{.01, .18, .37, .62, .92} {
			s := phraseFixture(100, 63, 2, speeds[0])
			a, err := FlowTarget(s, settings)
			if err != nil {
				t.Fatal(err)
			}
			previous := NewMotionPlan("before", a, settings, 0, 0, time.Unix(0, 0))
			s.SpeedPercent = speeds[1]
			b, err := FlowTarget(s, settings)
			if err != nil {
				t.Fatal(err)
			}
			at := int64(float64(previous.PeriodMillis) * fraction)
			next := previous.Retarget("after", b, settings, at, time.Unix(0, 0))
			bridge := layeredPaceTransition(previous, nil, &next, at)
			if bridge == nil {
				t.Fatalf("phrase pace edit lost bounded splice at %v/%f", speeds, fraction)
			}
			assertLayeredSplice(t, previous, next, bridge, settings)
		}
	}
}

func TestLayeredPhraseReachEditKeepsCountAndFeasibleHandoff(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, width := range [][2]int{{78, 53}, {53, 83}, {84, 30}} {
		for _, fraction := range []float64{.01, .18, .37, .62, .92} {
			s := phraseFixture(100, width[0], 2, 46)
			before, err := FlowTarget(s, settings)
			if err != nil {
				t.Fatal(err)
			}
			previous := NewMotionPlan("before", before, settings, 0, 0, time.Unix(0, 0))
			s.Phrase.Strokes[0].BottomPercent = width[1]
			after, err := FlowTarget(s, settings)
			if err != nil {
				t.Fatal(err)
			}
			at := int64(float64(previous.PeriodMillis) * fraction)
			next := previous.Retarget("after", after, settings, at, time.Unix(0, 0))
			bridge := layeredPaceTransition(previous, nil, &next, at)
			if bridge == nil {
				t.Fatalf("phrase reach edit fell back to opposed crossfade: %v/%f", width, fraction)
			}
			assertLayeredSplice(t, previous, next, bridge, settings)
		}
	}
}

func TestLayeredPhraseRejectsAmbiguousContentAndClones(t *testing.T) {
	s := phraseFixture(100, 65, 2, 30)
	clone := CloneFlowSpec(&s)
	clone.Phrase.Strokes[0].BottomPercent = 70
	if s.Phrase.Strokes[0].BottomPercent != 65 {
		t.Fatal("phrase backing slice aliased")
	}
	for _, mutate := range []func(*FlowSpec){
		func(s *FlowSpec) { s.Phrase.Strokes[0].TopPercent = 50 },
		func(s *FlowSpec) { s.Phrase.Strokes[0].Count = 0 },
		func(s *FlowSpec) { s.Phrase.Strokes[0].RatePercent = 0 },
		func(s *FlowSpec) { s.Phrase.Strokes[0].BottomPercent = 99 },
		func(s *FlowSpec) { s.Phrase.Strokes[0].BottomPercent = -1 },
		func(s *FlowSpec) { s.MaxPercent = 99 },
		func(s *FlowSpec) {
			for i := range s.Phrase.Strokes {
				s.Phrase.Strokes[i].TopPercent = 99
			}
		},
		func(s *FlowSpec) { s.Layers = []FlowLayer{{Axis: "range", PeriodCycles: 8}} },
		func(s *FlowSpec) { s.Gesture = &GestureSpec{} },
	} {
		bad := CloneFlowSpec(&s)
		mutate(bad)
		if bad.Validate(config.DefaultSettings().Motion) == nil {
			t.Fatalf("invalid phrase accepted: %+v", bad)
		}
	}
}

func BenchmarkLayeredPhraseCompile(b *testing.B) {
	s := phraseFixture(100, 63, 2, 46)
	settings := config.DefaultSettings().Motion
	for b.Loop() {
		if _, err := FlowTarget(s, settings); err != nil {
			b.Fatal(err)
		}
	}
}

func TestPhraseSinglePairMatchesFixedWidthLayered(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, speed := range []int{10, 25, 43, 85} {
		for _, softness := range []int{0, 70} {
			s := DefaultFlowSpec()
			s.MinPercent, s.MaxPercent, s.RangeFloorPercent, s.RangeCeilingPercent = 23, 87, 64, 64
			s.SpeedPercent, s.TurnSoftnessPercent, s.PaceVariationPercent = speed, softness, 0
			field, err := compileFlowCurve(s, settings.HandyModel)
			if err != nil {
				t.Fatal(err)
			}
			s.Phrase = &FlowPhrase{Strokes: []PhraseStroke{{BottomPercent: 23, TopPercent: 87, Count: 1, RatePercent: 100}}}
			phrase, err := compilePhraseCurve(s, settings.HandyModel)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(float64(field.duration)/64-float64(phrase.duration)/phrase.flowPeriod) > float64(phrase.duration)/phrase.flowPeriod*.01 {
				t.Fatal("phrase introduced a different speed mapping")
			}
		}
	}
}

func TestTurnCoverageReportsTypicalReachAndMultiRegionCounts(t *testing.T) {
	s := phraseFixture(100, 65, 2, 30)
	settings := config.DefaultSettings().Motion
	plan := NewMotionPlan("coverage", MotionTarget{Flow: &s, SpeedPercent: s.SpeedPercent}, settings, 0, 0, time.Unix(0, 0))
	c := plan.Perceptual.Turns
	if c.LowerCount < 3 || c.LowerCount%3 != 0 || c.UpperCount != c.LowerCount || c.LowerPercentiles[1] != 65 || math.Abs(c.BaseFraction-1.0/3) > 1e-9 || c.TipFraction != 1 {
		t.Fatalf("isolated full sweep was confused with typical reach: %+v", c)
	}
	s.Phrase.Strokes = []PhraseStroke{{BottomPercent: 70, TopPercent: 100, Count: 2, RatePercent: 100}, {BottomPercent: 0, TopPercent: 25, Count: 2, RatePercent: 65}, {BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: 100}}
	curve, err := compilePhraseCurve(s, settings.HandyModel)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{100, 70, 100, 70, 100, 0, 25, 0, 25, 0, 100}
	if (len(curve.authoredKnots)-1)%(len(want)-1) != 0 {
		t.Fatal("multi-region phrase added or lost a turn")
	}
	for i, k := range curve.authoredKnots {
		if math.Abs(k.PositionPercent-want[i%(len(want)-1)]) > 1e-8 {
			t.Fatalf("wrong turn %d", i)
		}
	}
}

func assertPhrasePlan(t *testing.T, plan MotionPlan, shape [3]int, settings config.MotionSettings) {
	t.Helper()
	knots := plan.curve.authoredKnots
	motifLegs := 2 * (shape[2] + 1)
	if (len(knots)-1)%motifLegs != 0 {
		t.Fatalf("count changed: %d knots", len(knots))
	}
	for index, k := range knots {
		i := index % motifLegs
		want := expectedPhraseTurn(shape, i, motifLegs)
		if math.Abs(k.PositionPercent-float64(want)) > 1e-7 {
			t.Fatalf("turn %d: got %f want %d", i, k.PositionPercent, want)
		}
	}
	if maximumPlanAcceleration(plan) > flowAccelerationBudget*1.001 || maximumPlanJerk(plan) > flowJerkBudget*1.001 {
		t.Fatal("phrase bypassed Layered envelope")
	}
	for i, segment := range plan.curve.quintics {
		next := plan.curve.quintics[(i+1)%len(plan.curve.quintics)]
		if math.Abs(segment.velocity(1)-next.velocity(0)) > 1e-7 || math.Abs(segment.acceleration(1)-next.acceleration(0)) > 1e-7 {
			t.Fatal("turn or loop seam lost C2 continuity")
		}
	}
	for i, k := range knots[:len(knots)-1] {
		if math.Abs(plan.curve.accelerationFloat(float64(k.TimeMillis))) < 1e-10 {
			t.Fatalf("turn %d was changed into a rest", i)
		}
	}
	engine := &Engine{plan: plan, settings: settings, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval, preservePlanKnots: true, positionResolutionPercent: 1, maximumChunkPoints: 100}
	minimum, maximum := 100.0, 0.0
	for engine.nextSampleMillis < knots[motifLegs].TimeMillis*plan.PeriodMillis/plan.curve.duration+1000 {
		points, err := engine.nextMotionSamplesLocked()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range points {
			minimum = math.Min(minimum, math.Round(p.PositionPercent))
			maximum = math.Max(maximum, math.Round(p.PositionPercent))
		}
	}
	if minimum != 0 || maximum != 100 {
		t.Fatalf("buffered output lost endpoint coverage: %f..%f", minimum, maximum)
	}
}

func expectedPhraseTurn(shape [3]int, i, motifLegs int) int {
	want := shape[0]
	if i%2 == 1 {
		want = shape[1]
		if i == motifLegs-1 {
			want = 100 - shape[0]
		}
	}
	if shape[0] == 0 {
		want = 0
		if i == 0 {
			want = 100
		} else if i%2 == 0 {
			want = shape[1]
		}
	}
	return want
}
