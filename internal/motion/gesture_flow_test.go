package motion

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestGestureMixedReachTraversesIntermediateWidths(t *testing.T) {
	s := gestureFixture()
	s.Gesture.FocusMixPercent, s.Gesture.VariationPercent = 50, 0
	bands := gestureBands(s)
	intermediate := 0
	for _, band := range bands {
		if span := band.high - band.low; span > 30 && span < 85 {
			intermediate++
		}
	}
	if intermediate < len(bands)/3 {
		t.Fatalf("mixed motion still switches mainly between two widths: %d intermediate strokes", intermediate)
	}
	legs := gestureLegs(s)
	for i, leg := range legs {
		next := legs[(i+1)%len(legs)]
		if leg.to != next.from || (leg.to-leg.from)*(next.to-next.from) >= 0 {
			t.Fatal("a pattern transfer added a stationary pass-through or broke the stroke chain")
		}
	}
}

func TestGestureReversalsCarryContinuousAcceleration(t *testing.T) {
	s := gestureFixture()
	s.Gesture.InertiaPercent, s.Gesture.ReboundCount = 70, 3
	curve, err := compileGestureCurve(s, config.HandyModelOriginal)
	if err != nil {
		t.Fatal(err)
	}
	for _, knot := range curve.authoredKnots[:len(curve.authoredKnots)-1] {
		i := sort.Search(len(curve.points), func(i int) bool { return curve.points[i].TimeMillis >= knot.TimeMillis })
		if math.Abs(curve.accelerations[i]) < 1e-9 {
			t.Fatal("stroke settles to zero acceleration at its reversal")
		}
	}
}

func TestGesturePaceEditsRetainNearbyStrokeContext(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	s := gestureFixture()
	s.SpeedPercent, s.Gesture.VariationPercent = 40, 70
	target, err := FlowTarget(s, settings)
	if err != nil {
		t.Fatal(err)
	}
	previous := NewMotionPlan("before", target, settings, 0, 0, time.Unix(0, 0))
	s.SpeedPercent = 50
	target, err = FlowTarget(s, settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range []int{9, 22, 37, 50, 61, 109, 125} {
		knots := previous.curve.authoredKnots
		clock := float64(knots[slot].TimeMillis) + 0.4*float64(knots[slot+1].TimeMillis-knots[slot].TimeMillis)
		at := int64(clock * float64(previous.PeriodMillis) / float64(previous.curve.duration))
		next := previous.Retarget("after", target, settings, at, time.Unix(0, 0))
		nextTime := next.PhaseAt(at) * float64(next.curve.duration)
		nextSlot := max(0, sort.Search(len(next.curve.authoredKnots), func(i int) bool { return float64(next.curve.authoredKnots[i].TimeMillis) > nextTime })-1)
		if math.Abs(float64(nextSlot-slot)) > 2 || next.DirectionAt(at) != previous.DirectionAt(at) {
			t.Fatalf("pace edit restarted another part of the phrase: %d -> %d", slot, nextSlot)
		}
		if math.Abs(next.SampleAt(at).PositionPercent-previous.SampleAt(at).PositionPercent) > 2 {
			t.Fatal("stroke continuity discarded handoff position")
		}
	}
}

func TestGestureFlowAcrossNarrowBandsAndUnequalTurns(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for i := range 72 {
		s := gestureFixture()
		s.MinPercent = i * 29 % 91
		width := 10 + i*37%(91-s.MinPercent)
		s.MaxPercent = s.MinPercent + width
		s.SpeedPercent, s.Seed = 1+i*7%100, uint32(1+i*137)
		s.MemoryCycles, s.LoopCycles = 2+i%31, []int{0, 4, 64}[i%3]
		g := s.Gesture
		g.FocusPercent, g.FocusMixPercent = i*17%101, i*19%101
		g.FocusRoamPercent = i * 43 % 101
		g.FocusWidthPercent = 10 + i*23%(width-9)
		g.FasterDirection, g.ContrastPercent = []string{"even", "base", "tip"}[i%3], i*11%81
		g.InertiaPercent, g.VariationPercent = i*13%101, i*31%101
		g.ReboundCount, g.ReboundDecayPercent = i%5, 25+i%61
		settings.HandyModel = []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro}[(i/3)%3]
		target, err := FlowTarget(s, settings)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		plan := NewMotionPlan("varied-bounds", target, settings, 0, 0, time.Unix(0, 0))
		if err := plan.compilationError(); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		for _, segment := range plan.curve.quintics {
			for _, u := range []float64{0, .25, .5, .75, 1} {
				if x := segment.position(u); x < float64(s.MinPercent)-1e-6 || x > float64(s.MaxPercent)+1e-6 {
					t.Fatalf("case %d escaped its requested band", i)
				}
			}
		}
	}
}
