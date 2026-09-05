package motion

import (
	"math"
	"reflect"
	"testing"
)

func TestGestureFreeRoamingHasNoPersistentEndpoint(t *testing.T) {
	s := gestureFixture()
	*s.Gesture = DefaultGestureSpec()
	if s.Gesture.FocusRoamPercent != 100 {
		t.Fatal("fresh motion holds a default location")
	}
	for _, seed := range []uint32{1, 17, 23981, 31415} {
		s.Seed = seed
		bands := gestureBands(s)
		minLow, maxLow, minHigh, maxHigh := 100.0, 0.0, 100.0, 0.0
		for _, band := range bands {
			minLow, maxLow = math.Min(minLow, band.low), math.Max(maxLow, band.low)
			minHigh, maxHigh = math.Min(minHigh, band.high), math.Max(maxHigh, band.high)
		}
		if maxLow-minLow < 10 || maxHigh-minHigh < 10 {
			t.Fatalf("seed %d: an endpoint stayed fixed", seed)
		}
		s.Gesture.FocusPercent = 0
		atBase := gestureBands(s)
		s.Gesture.FocusPercent = 100
		if !reflect.DeepEqual(atBase, gestureBands(s)) {
			t.Fatal("full roaming still inherits the previous anchor")
		}
	}
}

func TestGestureRoamingKeepsOverlappingWindowsAndReplay(t *testing.T) {
	for _, cycles := range []int{4, 16, 64} {
		for _, roam := range []int{0, 25, 60, 100} {
			for seed := uint32(1); seed <= 12; seed++ {
				s := gestureFixture()
				s.LoopCycles, s.Seed = cycles, seed
				s.Gesture.FocusRoamPercent, s.Gesture.FocusMixPercent, s.Gesture.FocusWidthPercent = roam, 100, 10
				legs := gestureLegs(s)
				for i, leg := range legs {
					next := legs[(i+1)%len(legs)]
					if leg.to != next.from || (leg.to-leg.from)*(next.to-next.from) >= 0 || math.Abs(leg.to-leg.from) < 4.9999 {
						t.Fatalf("cycles %d roam %d seed %d: lost a full directional reversal", cycles, roam, seed)
					}
				}
				if !reflect.DeepEqual(legs, gestureLegs(s)) {
					t.Fatal("roaming replay changed")
				}
				if roam == 0 {
					for _, band := range gestureBands(s) {
						if math.Abs(band.high-float64(s.MaxPercent)) > 1e-9 {
							t.Fatal("explicit tip anchor moved")
						}
					}
				}
			}
		}
	}
}
