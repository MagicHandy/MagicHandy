package motion

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestRegionalCyclesCompileWithOnlyNecessaryConnectingTravel(t *testing.T) {
	s := layeredSpeedFixture("full", 75)
	s.RangeFloorPercent = 30
	s.Phrase = &FlowPhrase{Regions: []PhraseRegion{
		{PhraseStroke{35, 65, 2, 100}, "down_up"},
		{PhraseStroke{70, 100, 3, 80}, "down_up"},
		{PhraseStroke{0, 30, 2, 60}, "up_down"},
	}}
	// The second middle return passes through65 on its climb to100. It must
	// not turn at65, descend again to35, and invent a third middle cycle.
	want := []int{65, 35, 65, 35, 100, 70, 100, 70, 100, 70, 100, 0, 30, 0, 30, 0}
	turns := s.Phrase.OrderedTurns()
	if len(turns) != len(want) {
		t.Fatalf("unexpected connector count: %v", turns)
	}
	for i, p := range want {
		if turns[i].PositionPercent != p {
			t.Fatalf("turn%d: %d != %d", i, turns[i].PositionPercent, p)
		}
	}
	clone := CloneFlowSpec(&s)
	clone.Phrase.Regions[0].Count = 1
	if s.Phrase.Regions[0].Count != 2 {
		t.Fatal("regions alias the active score")
	}
	if _, err := FlowTarget(s, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	for i := range s.Phrase.Regions {
		s.Phrase.Regions[i].RatePercent = 100
	}
	if _, err := FlowTarget(s, config.DefaultSettings().Motion); err != nil {
		t.Fatal("equal regional rates accumulated floating-point bounds error", err)
	}
}

func TestRegionalRandomScoresStaySmoothBoundedAndStopFree(t *testing.T) {
	rng := rand.New(rand.NewPCG(73, 29)) // #nosec G404 -- reproducible geometric test data, not security randomness.
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for trial := range 120 {
		settings.HandyModel = []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro}[trial%3]
		s := DefaultFlowSpec()
		s.SpeedPercent = 1 + rng.IntN(100)
		s.PaceVariationPercent = 0
		if trial%2 == 0 {
			s.DirectionalPace = &DirectionalPace{1 + rng.IntN(100), 1 + rng.IntN(100)}
		}
		s.MinPercent, s.MaxPercent, s.RangeFloorPercent = 100, 0, 10
		s.Phrase = &FlowPhrase{}
		for range 1 + rng.IntN(6) {
			low := 10 * rng.IntN(10)
			high := low + 10 + 10*rng.IntN((100-low)/10)
			s.MinPercent, s.MaxPercent = min(s.MinPercent, low), max(s.MaxPercent, high)
			s.Phrase.Regions = append(s.Phrase.Regions, PhraseRegion{PhraseStroke{low, high, 1 + rng.IntN(4), 1 + rng.IntN(100)}, []string{"up_down", "down_up"}[rng.IntN(2)]})
		}
		target, err := FlowTarget(s, settings)
		if err != nil {
			t.Fatalf("trial%d: %v", trial, err)
		}
		plan := NewMotionPlan("regional", target, settings, 0, 0, time.Unix(0, 0))
		if maximumPlanAcceleration(plan) > flowAccelerationBudget*1.001 || maximumPlanJerk(plan) > flowJerkBudget*1.001 {
			t.Fatal("regional plan escaped envelope")
		}
		for i, q := range plan.curve.quintics {
			next := plan.curve.quintics[(i+1)%len(plan.curve.quintics)]
			if math.Abs(q.velocity(1)-next.velocity(0)) > 1e-7 || math.Abs(q.acceleration(1)-next.acceleration(0)) > 1e-7 {
				t.Fatal("regional seam is not C2")
			}
		}
		for i, k := range plan.curve.authoredKnots[1:] {
			if k.PositionPercent == plan.curve.authoredKnots[i].PositionPercent {
				t.Fatal("regional compiler inserted a rest")
			}
		}
	}
}

func TestRegionTransferDoesNotInventAnotherLocalCycle(t *testing.T) {
	for _, cycle := range []string{"up_down", "down_up"} {
		regions := []PhraseRegion{{PhraseStroke{0, 40, 5, 100}, "up_down"}, {PhraseStroke{60, 100, 3, 50}, cycle}}
		turns, err := regionTurns(regions)
		if err != nil {
			t.Fatal(err)
		}
		upperFar := 100
		if cycle == "down_up" {
			upperFar = 60
		}
		upper, lower := 0, 0
		for _, turn := range turns {
			if turn.PositionPercent == upperFar {
				upper++
			}
			if turn.PositionPercent == 40 {
				lower++
			}
		}
		if upper != 3 || lower != 5 {
			t.Fatalf("connector added local excursions: upper=%d lower=%d turns=%v", upper, lower, turns)
		}
	}
}

func TestRegionWaypointMergePreservesTravelTime(t *testing.T) {
	turns := mergeRegionWaypoints([]PhraseTurn{{0, 100, 0}, {60, 100, 1}, {100, 50, 2}, {60, 50, 3}})
	if len(turns) != 2 || turns[0].PositionPercent != 0 || turns[1].PositionPercent != 100 {
		t.Fatal("same-direction waypoint became a stop", turns)
	}
	for _, turn := range turns {
		if math.Abs(100/turn.RatePercent-(60.0/100+40.0/50)) > 1e-9 {
			t.Fatal("merging changed selected travel time")
		}
	}
}

func TestRegionalHandoffMatchesSemanticSlots(t *testing.T) {
	settings := config.DefaultSettings().Motion
	a := regionalPhraseFixture(45, false)
	b := *CloneFlowSpec(&a)
	b.SpeedPercent = 55
	plan := func(s FlowSpec) MotionPlan {
		target, err := FlowTarget(s, settings)
		if err != nil {
			t.Fatal(err)
		}
		return NewMotionPlan("slots", target, settings, 0, 0, time.Unix(0, 0))
	}
	if !samePhraseSlots(plan(a), plan(b)) {
		t.Fatal("pace edit lost ordinal slots")
	}
	b.Phrase.Regions[0].Count--
	b.Phrase.Regions[1].Count++
	if samePhraseSlots(plan(a), plan(b)) {
		t.Fatal("same total count concealed moved regional slots")
	}
}
