package motion

import (
	"errors"
	"math"
)

// GestureSpec generates stroke destinations and travel character, rather than
// superimposing position oscillators. It is semantic content for the shared
// engine: no timers, transport access, device coordinates or private playback.
type GestureSpec struct {
	FocusPercent        int    `json:"focus_percent"`
	FocusWidthPercent   int    `json:"focus_width_percent"`
	FocusMixPercent     int    `json:"focus_mix_percent"`
	FasterDirection     string `json:"faster_direction"`
	ContrastPercent     int    `json:"contrast_percent"`
	InertiaPercent      int    `json:"inertia_percent"`
	ReboundCount        int    `json:"rebound_count"`
	ReboundDecayPercent int    `json:"rebound_decay_percent"`
	VariationPercent    int    `json:"variation_percent"`
}

// DefaultGestureSpec is a neutral vocabulary starting point, not a named path.
func DefaultGestureSpec() GestureSpec {
	return GestureSpec{FocusPercent: 100, FocusWidthPercent: 25, FocusMixPercent: 40,
		FasterDirection: "even", InertiaPercent: 25, ReboundDecayPercent: 60, VariationPercent: 35}
}

func (s FlowSpec) validateGesture() error {
	g := s.Gesture
	if g == nil {
		return nil
	}
	if len(s.Steps) != 0 || len(s.Layers) != 0 || s.LoopCycles > 32 {
		return errors.New("creative v2 uses generated strokes without sections or modulation layers, with at most 32 primary cycles")
	}
	if g.FasterDirection != "even" && g.FasterDirection != "tip" && g.FasterDirection != "base" {
		return errors.New("creative v2 sweep direction must be even, tip or base")
	}
	for _, bound := range [][3]int{{g.FocusPercent, 0, 100}, {g.FocusWidthPercent, 10, s.MaxPercent - s.MinPercent},
		{g.FocusMixPercent, 0, 100}, {g.ContrastPercent, 0, 80}, {g.InertiaPercent, 0, 100},
		{g.ReboundCount, 0, 4}, {g.ReboundDecayPercent, 25, 85}, {g.VariationPercent, 0, 100}} {
		if bound[0] < bound[1] || bound[0] > bound[2] {
			return errors.New("creative v2 controls exceed their supported bounds; focus width must fit inside the outer band")
		}
	}
	return nil
}

type gestureLeg struct {
	from, to float64
	weight   float64
	inertia  float64
}

// gestureLegs schedules bounded local groups among broad strokes. Rebounds
// contract geometrically toward the chosen focus; the next primary excursion
// restores the requested reach. Variation changes groups and correlated pace,
// never individual samples. A seed makes the accepted realization replayable.
func gestureLegs(s FlowSpec) []gestureLeg {
	g := *s.Gesture
	cycles := 32
	if s.LoopCycles > 0 {
		cycles = s.LoopCycles
	}
	lo, hi := float64(s.MinPercent), float64(s.MaxPercent)
	focus := lo + (hi-lo)*float64(g.FocusPercent)/100
	width := float64(g.FocusWidthPercent)
	localLo := clampFloat(focus-width*float64(g.FocusPercent)/100, lo, hi-width)
	// Begin at an endpoint, so a compiled seam cannot hide a pass-through turn.
	start := lo
	if g.FocusMixPercent == 100 {
		start = localLo
	}
	position := start
	legs := make([]gestureLeg, 0, cycles*4)
	trend, groupLeft, localRun := 0.0, 0, 0
	local := false
	variation := float64(g.VariationPercent) / 100
	random := func(index, salt int) float64 {
		return dynamicSeedUnit(s.Seed^uint32(salt), uint64(index)) //nolint:gosec // bounded cycle index and fixed salts
	}
	appendLeg := func(destination, pace float64) {
		if math.Abs(destination-position) < 0.001 {
			return
		}
		weight := gestureLegWeight(g, position, destination, pace)
		legs = append(legs, gestureLeg{position, destination, weight, float64(g.InertiaPercent) / 100})
		position = destination
	}
	for cycle := range cycles {
		trend = 0.7*trend + 0.3*(2*random(cycle, 0x46a32)-1)
		pace := 1 + 0.5*variation*trend
		if groupLeft == 0 {
			local = random(cycle, 0x96185)*100 < float64(g.FocusMixPercent)
			groupLeft = 1 + int(random(cycle, 0x75ea1)*3)
		}
		groupLeft--
		if g.FocusMixPercent == 0 || (g.FocusMixPercent < 100 && localRun >= 6) {
			local = false
		}
		if g.FocusMixPercent == 100 {
			local = true
		}
		a, b := lo, hi
		if local {
			localRun++
			// Change local width around its anchor, retaining at least 10% travel.
			span := math.Max(10, width*(1-0.35*variation*random(cycle, 0x75319)))
			a = localLo + (width-span)*float64(g.FocusPercent)/100
			b = a + span
		} else {
			localRun = 0
		}
		// End the primary stroke at the focus-side end. Transfers replace a leg;
		// no independent center wave can create incidental reversals.
		origin, destination := a, b
		if g.FocusPercent < 50 {
			origin, destination = b, a
		}
		appendLeg(origin, pace)
		appendLeg(destination, pace)
		if !local {
			continue
		}
		span := math.Abs(destination - origin)
		decay := float64(g.ReboundDecayPercent) / 100
		for bounce := range g.ReboundCount {
			span *= decay
			// Sub-resolution rebounds would produce stalls after wire quantization.
			// End the group instead of repeatedly clamping its shrinking tail.
			if span < 10 {
				break
			}
			away := destination - math.Copysign(span, destination-origin)
			// Shorter travel has shorter duration, with a bounded inertial decay.
			bouncePace := pace * (1 - 0.04*float64(bounce+1))
			appendLeg(away, bouncePace)
			appendLeg(destination, bouncePace)
		}
	}
	appendLeg(start, 1)
	return legs
}

func gestureLegWeight(g GestureSpec, from, to, pace float64) float64 {
	if g.FasterDirection == "even" {
		return pace
	}
	contrast := 0.65 * float64(g.ContrastPercent) / 100
	if (to > from) == (g.FasterDirection == "tip") {
		return pace * (1 - contrast)
	}
	return pace * (1 + contrast)
}
