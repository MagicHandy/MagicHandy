package motion

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func strokeFixture() FlowSpec {
	s := DefaultFlowSpec()
	s.MinPercent, s.MaxPercent = 0, 100
	s.Strokes = &StrokeSpec{
		Bottom:           StrokeTurn{AtPercent: 10, VaryToPercent: 30, Character: "drift"},
		Top:              StrokeTurn{AtPercent: 95, VaryToPercent: 80, Character: "drift"},
		VariationPercent: 50,
	}
	return s
}

func TestStrokeScoreKinematicsAcrossCharactersAccentsAndDevices(t *testing.T) {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	turns := [][2]StrokeTurn{
		{{AtPercent: 0, VaryToPercent: 0, Character: "steady"}, {AtPercent: 100, VaryToPercent: 100, Character: "steady"}},
		{{AtPercent: 70, VaryToPercent: 0, Character: "occasional"}, {AtPercent: 100, VaryToPercent: 90, Character: "drift"}},
		{{AtPercent: 0, VaryToPercent: 60, Character: "alternate"}, {AtPercent: 100, VaryToPercent: 100, Character: "steady"}},
		{{AtPercent: 20, VaryToPercent: 50, Character: "roam"}, {AtPercent: 45, VaryToPercent: 75, Character: "roam"}},
	}
	accents := [][]StrokeAccent{nil, {{Move: "plunge", Rate: "often"}, {Move: "tip_flicks", Rate: "sometimes"}},
		{{Move: "deep_grind", Rate: "often"}, {Move: "pause", Rate: "sometimes"}, {Move: "slow", Rate: "rarely"}}}
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Pro} {
		limits.HandyModel = model
		for _, speed := range []int{10, 45, 85} {
			for index, pair := range turns {
				for _, set := range accents {
					for _, seed := range []uint32{1, 17, 23981} {
						s := strokeFixture()
						s.SpeedPercent, s.Seed = speed, seed
						s.Strokes.Bottom, s.Strokes.Top, s.Strokes.Accents = pair[0], pair[1], set
						target, err := FlowTarget(s, limits)
						if err != nil {
							t.Fatalf("turns %d accents %v: %v", index, set, err)
						}
						plan := NewMotionPlan("stroke-test", target, limits, 0, 0, time.Unix(0, 0))
						factor := float64(plan.PeriodMillis) / float64(plan.curve.duration)
						if plan.Perceptual.CommandedPeakVelocityPerSecond > referenceTravelRateForSpeed(100, model)*1.001 {
							t.Fatal("velocity limit")
						}
						if a := plan.curve.maximumAccelerationPerMillis2() * 1e6 / (factor * factor); a > runtimeMaxAccelerationPercentPerSecond2*1.001 {
							t.Fatalf("acceleration %f", a)
						}
						if j := plan.curve.maximumJerkPerMillis3() * 1e9 / (factor * factor * factor); j > runtimeMaxJerkPercentPerSecond3*1.001 {
							t.Fatalf("jerk %f", j)
						}
						if gap := reversalGap(plan.curve.authoredKnots, plan.curve.duration, true); float64(gap)*factor < float64(runtimeMinimumReversalGapMillis)-0.01 {
							t.Fatal("reversal gap")
						}
					}
				}
			}
		}
	}
}

func TestStrokeTurnsLandWhereRequested(t *testing.T) {
	s := strokeFixture()
	s.Strokes.VariationPercent = 0
	s.Strokes.Bottom = StrokeTurn{AtPercent: 0, VaryToPercent: 0, Character: "steady"}
	s.Strokes.Top = StrokeTurn{AtPercent: 100, VaryToPercent: 100, Character: "steady"}
	for _, unit := range s.strokeUnits() {
		if unit.bottom != 0 || unit.top != 100 {
			t.Fatalf("steady full strokes landed at %.1f-%.1f", unit.bottom, unit.top)
		}
	}

	s.Strokes.Bottom = StrokeTurn{AtPercent: 70, VaryToPercent: 5, Character: "occasional"}
	deep := 0
	for _, unit := range s.strokeUnits() {
		if unit.bottom != 70 && unit.bottom != 5 {
			t.Fatalf("an occasional turn landed between its positions: %.1f", unit.bottom)
		}
		if unit.bottom == 5 {
			deep++
		}
	}
	if deep < 4 || deep > 24 {
		t.Fatalf("occasional turn visited its other position on %d of 64 strokes", deep)
	}

	s.Strokes.Bottom = StrokeTurn{AtPercent: 10, VaryToPercent: 40, Character: "drift"}
	low, high := 100.0, 0.0
	for _, unit := range s.strokeUnits() {
		low, high = math.Min(low, unit.bottom), math.Max(high, unit.bottom)
	}
	if low < 10 || high > 40 || high-low < 15 {
		t.Fatalf("a drifting turn stayed at %.1f-%.1f instead of wandering inside 10-40", low, high)
	}

	s.Strokes.Bottom = StrokeTurn{AtPercent: 20, VaryToPercent: 50, Character: "roam"}
	s.Strokes.Top = StrokeTurn{AtPercent: 45, VaryToPercent: 75, Character: "roam"}
	moved := false
	for _, unit := range s.strokeUnits() {
		if math.Abs(unit.top-unit.bottom-25) > 1e-9 {
			t.Fatalf("roaming turns moved apart: %.2f-%.2f", unit.bottom, unit.top)
		}
		moved = moved || unit.bottom > 30
	}
	if !moved {
		t.Fatal("roaming turns never moved the stroke")
	}
}

func TestStrokeAlternationRunsTwoToSixStrokes(t *testing.T) {
	s := strokeFixture()
	s.Strokes.VariationPercent = 0
	s.Strokes.Bottom = StrokeTurn{AtPercent: 0, VaryToPercent: 60, Character: "alternate"}
	units := s.strokeUnits()
	run := 1
	for i := 1; i < len(units); i++ {
		if units[i].bottom == units[i-1].bottom {
			run++
			continue
		}
		if run < 2 || run > 6 {
			t.Fatalf("an alternation run lasted %d strokes", run)
		}
		run = 1
	}
}

func TestStrokeAccentsWeaveMotifsIntoTheStream(t *testing.T) {
	s := strokeFixture()
	s.Strokes.VariationPercent = 0
	s.Strokes.Bottom = StrokeTurn{AtPercent: 60, VaryToPercent: 60, Character: "steady"}
	s.Strokes.Top = StrokeTurn{AtPercent: 95, VaryToPercent: 95, Character: "steady"}
	s.Strokes.Accents = []StrokeAccent{{Move: "plunge", Rate: "often"}}
	plunges := 0
	for _, unit := range s.strokeUnits() {
		if unit.bottom == 0 {
			plunges++
		}
	}
	if plunges < 8 || plunges > 26 {
		t.Fatalf("often plunged on %d of 64 strokes", plunges)
	}

	s.Strokes.Bottom = StrokeTurn{AtPercent: 20, VaryToPercent: 20, Character: "steady"}
	s.Strokes.Accents = []StrokeAccent{{Move: "tip_flicks", Rate: "sometimes"}}
	units := s.strokeUnits()
	flicks := 0
	for _, unit := range units {
		if unit.top == 95 && unit.bottom == 95-strokeWorkSpan {
			flicks++
		}
	}
	if len(units) <= 64 || flicks == 0 {
		t.Fatal("tip flicks did not add short strokes at the top turn")
	}

	s.Strokes.Accents = []StrokeAccent{{Move: "deep_grind", Rate: "sometimes"}}
	grinds := 0
	for _, unit := range s.strokeUnits() {
		if unit.bottom == 20 && unit.top == 20+strokeWorkSpan {
			grinds++
		}
	}
	if grinds == 0 {
		t.Fatal("deep grinding did not add short strokes at the bottom turn")
	}
}

func TestStrokeLegsAlwaysReverseAndChain(t *testing.T) {
	for _, seed := range []uint32{1, 17, 23981} {
		s := strokeFixture()
		s.Seed = seed
		s.Strokes.Bottom = StrokeTurn{AtPercent: 40, VaryToPercent: 85, Character: "drift"}
		s.Strokes.Top = StrokeTurn{AtPercent: 60, VaryToPercent: 15, Character: "drift"}
		s.Strokes.Accents = []StrokeAccent{{Move: "tip_flicks", Rate: "often"}, {Move: "pause", Rate: "often"}}
		legs := strokeLegs(s)
		var previous *gestureLeg
		for i := range legs {
			leg := legs[i]
			next := legs[(i+1)%len(legs)]
			if leg.to != next.from {
				t.Fatal("stroke legs do not chain")
			}
			if leg.rest > 0 {
				continue
			}
			if math.Abs(leg.to-leg.from) < strokeMinimumSpan-1e-9 {
				t.Fatalf("a stroke travels only %.2f points", math.Abs(leg.to-leg.from))
			}
			if previous != nil && (previous.to-previous.from)*(leg.to-leg.from) >= 0 {
				t.Fatal("two strokes in a row travel the same way")
			}
			previous = &legs[i]
		}
	}
}

func TestStrokeScoreValidation(t *testing.T) {
	limits := config.DefaultSettings().Motion
	for name, change := range map[string]func(*FlowSpec){
		"bottom too shallow":   func(s *FlowSpec) { s.Strokes.Bottom.AtPercent = 95 },
		"top too deep":         func(s *FlowSpec) { s.Strokes.Top.AtPercent, s.Strokes.Top.VaryToPercent = 5, 5 },
		"turns too close":      func(s *FlowSpec) { s.Strokes.Bottom.AtPercent, s.Strokes.Top.AtPercent = 50, 55 },
		"unknown character":    func(s *FlowSpec) { s.Strokes.Top.Character = "bounce" },
		"unknown accent":       func(s *FlowSpec) { s.Strokes.Accents = []StrokeAccent{{Move: "spin", Rate: "often"}} },
		"unknown rate":         func(s *FlowSpec) { s.Strokes.Accents = []StrokeAccent{{Move: "plunge", Rate: "always"}} },
		"repeated accent":      func(s *FlowSpec) { s.Strokes.Accents = []StrokeAccent{{"plunge", "often"}, {"plunge", "rarely"}} },
		"variation above 100":  func(s *FlowSpec) { s.Strokes.VariationPercent = 101 },
		"mixed with a gesture": func(s *FlowSpec) { g := DefaultGestureSpec(); s.Gesture = &g },
		"mixed with layers":    func(s *FlowSpec) { s.Layers = []FlowLayer{{Axis: "pace", AmountPercent: 20, PeriodCycles: 8}} },
	} {
		s := strokeFixture()
		change(&s)
		if err := s.Validate(limits); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	if err := strokeFixture().Validate(limits); err != nil {
		t.Fatal(err)
	}
}

func TestStrokeScoreReplaysBySeedAndClonesDeeply(t *testing.T) {
	s := strokeFixture()
	s.Strokes.Accents = []StrokeAccent{{Move: "plunge", Rate: "sometimes"}}
	if !reflect.DeepEqual(s.strokeUnits(), s.strokeUnits()) {
		t.Fatal("a saved stroke score did not replay exactly")
	}
	other := s
	other.Seed = 23981
	if reflect.DeepEqual(s.strokeUnits(), other.strokeUnits()) {
		t.Fatal("a fresh seed did not change the realization")
	}
	clone := CloneFlowSpec(&s)
	clone.Strokes.Accents[0].Rate = "often"
	clone.Strokes.Bottom.AtPercent = 50
	if s.Strokes.Accents[0].Rate != "sometimes" || s.Strokes.Bottom.AtPercent != 10 {
		t.Fatal("cloning shared stroke state")
	}
}
