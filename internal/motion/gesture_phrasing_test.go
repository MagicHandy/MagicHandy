package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestGestureBreathOnlyEasesAndRecovers(t *testing.T) {
	s := gestureFixture()
	for _, seed := range []uint32{1, 17, 23981, 31415} {
		s.Seed = seed
		eased, resting := false, false
		for i := range 64 {
			breath := s.breath(i, 0.5)
			if breath < 1 || breath > 1+breathDepth*0.5+1e-12 {
				t.Fatalf("seed %d: breath %.3f quickened or overshot", seed, breath)
			}
			eased = eased || breath > 1.1
			resting = resting || breath == 1
		}
		if !eased || !resting {
			t.Fatalf("seed %d: breath never eased (%t) or never recovered (%t)", seed, eased, resting)
		}
		if s.breath(9, 0) != 1 {
			t.Fatal("breath without variation")
		}
	}
}

func TestGestureFlurriesAreShortQuickAndKeepHeldAnchors(t *testing.T) {
	s := gestureFixture()
	s.Gesture.VariationPercent = 100
	flurries := 0
	for i := range 64 {
		quick, span, ok := s.flurry(i, 64, 1)
		if !ok {
			continue
		}
		flurries++
		if quick < 1.2 || quick > 1.4 || span != 0.6 {
			t.Fatal("flurry outside its bounds", quick, span)
		}
	}
	if flurries == 0 || flurries > 24 {
		t.Fatalf("%d flurry strokes in a 64-stroke phrase", flurries)
	}
	if _, _, ok := s.flurry(3, 64, 0); ok {
		t.Fatal("flurry without variation")
	}
	low, span := shortenForFlurry(60, 35, 0.6, 1)
	if low+span != 95 || span != 21 {
		t.Fatal("a tip-anchored flurry left its anchor", low, span)
	}
	low, span = shortenForFlurry(5, 12, 0.6, 0)
	if low != 5 || span != 10 {
		t.Fatal("a flurry shortened a stroke below ten points or moved a base anchor", low, span)
	}
	// Full-only reach is a hard request: flurries quicken it but never shorten it.
	s.Gesture.FocusMixPercent = 0
	for _, leg := range travelLegs(t, gestureLegs(s)) {
		if math.Abs(leg.to-leg.from) < 90*(1-2*landingInset)-1e-9 {
			t.Fatal("a flurry shortened a full-only stroke")
		}
	}
}

func TestGestureLingeringTurnsAreBriefRareAndInsideTheBand(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, variation := range []int{35, 100} {
		s := gestureFixture()
		s.Gesture.VariationPercent = variation
		rests, turns := 0, 0
		for seed := uint32(1); seed <= 20; seed++ {
			s.Seed = seed
			for _, leg := range gestureLegs(s) {
				if leg.rest > 0 {
					rests++
				} else {
					turns++
				}
			}
			target, err := FlowTarget(s, settings)
			if err != nil {
				t.Fatal(err)
			}
			plan := NewMotionPlan("phrasing", target, settings, 0, 0, time.Unix(0, 0))
			for at := int64(0); at <= plan.PeriodMillis; at += 20 {
				if x := plan.SampleAt(at).PositionPercent; x < 4.9999 || x > 95.0001 {
					t.Fatal("phrasing escaped the outer band", x)
				}
			}
		}
		share := float64(rests) / float64(turns)
		if want := lingerChance * float64(variation) / 100; share == 0 || share > 2*want {
			t.Fatalf("variation %d: %.3f of turns linger, want about %.3f", variation, share, want)
		}
	}
}

func TestGesturePhrasingIsExactWithoutVariation(t *testing.T) {
	s := gestureFixture()
	s.Gesture.VariationPercent, s.Gesture.FocusMixPercent = 0, 0
	s.Gesture.FasterDirection, s.Gesture.ContrastPercent = "tip", 40
	for i, band := range gestureBands(s) {
		if band.pace != 1 || band.contrast != 0.4 || band.inertia != 0.25 || band.lingerTop != 0 || band.lingerBottom != 0 ||
			band.low != 5 || band.high != 95 {
			t.Fatalf("stroke %d varied without variation: %+v", i, band)
		}
	}
}
