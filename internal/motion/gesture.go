package motion

import "errors"

// GestureSpec generates stroke destinations and travel character, rather than
// superimposing position oscillators. It is semantic content for the shared
// engine: no timers, transport access, device coordinates or private playback.
type GestureSpec struct {
	FocusPercent        int    `json:"focus_percent"`
	FocusWidthPercent   int    `json:"focus_width_percent"`
	FocusMixPercent     int    `json:"focus_mix_percent"`
	FocusRoamPercent    int    `json:"focus_roam_percent"`
	FasterDirection     string `json:"faster_direction"`
	ContrastPercent     int    `json:"contrast_percent"`
	InertiaPercent      int    `json:"inertia_percent"`
	ReboundCount        int    `json:"rebound_count"`
	ReboundDecayPercent int    `json:"rebound_decay_percent"`
	VariationPercent    int    `json:"variation_percent"`
}

// DefaultGestureSpec is a neutral vocabulary starting point, not a named path.
func DefaultGestureSpec() GestureSpec {
	return GestureSpec{FocusPercent: 50, FocusWidthPercent: 25, FocusMixPercent: 40, FocusRoamPercent: 100,
		FasterDirection: "even", InertiaPercent: 25, ReboundDecayPercent: 60, VariationPercent: 50}
}

func (s FlowSpec) validateGesture() error {
	g := s.Gesture
	if g == nil {
		return nil
	}
	if len(s.Steps) != 0 || len(s.Layers) != 0 || s.LoopCycles > 64 {
		return errors.New("creative v2 uses generated strokes without sections or modulation layers, with at most 64 cycles")
	}
	if g.FasterDirection != "even" && g.FasterDirection != "tip" && g.FasterDirection != "base" {
		return errors.New("creative v2 sweep direction must be even, tip or base")
	}
	for _, bound := range [][3]int{{g.FocusPercent, 0, 100}, {g.FocusWidthPercent, 10, s.MaxPercent - s.MinPercent},
		{g.FocusMixPercent, 0, 100}, {g.FocusRoamPercent, 0, 100}, {g.ContrastPercent, 0, 80}, {g.InertiaPercent, 0, 100},
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
	// contrast is this stroke's realized direction contrast, 0..0.8.
	contrast float64
	// rest is a lingering turn: no travel, held for this many seconds. The
	// strokes on either side arrive at and leave it from rest.
	rest float64
	// softness blends the stroke toward rest-to-rest turns (Layered's soft
	// turns); Creative v2 strokes keep their shared turn acceleration.
	softness float64
}

// gestureLegs follows one evolving reach envelope. Each destination is the
// next reversal, never a transfer to the start of a separate local pattern.
// A saved seed reproduces the field; live evolution changes its realization.
func gestureLegs(s FlowSpec) []gestureLeg {
	g := *s.Gesture
	bands := gestureBands(s)
	legs := make([]gestureLeg, 0, len(bands)*2)
	for index, band := range bands {
		next := bands[(index+1)%len(bands)]
		returning := (band.contrast + next.contrast) / 2
		legs = append(legs, gestureLeg{from: band.low, to: band.high, weight: gestureLegWeight(g, band.contrast, band.low, band.high, band.pace),
			inertia: band.inertia, contrast: band.contrast})
		if band.lingerTop > 0 {
			legs = append(legs, gestureLeg{from: band.high, to: band.high, rest: band.lingerTop})
		}
		legs = append(legs, gestureLeg{from: band.high, to: next.low, weight: gestureLegWeight(g, returning, band.high, next.low, (band.pace+next.pace)/2),
			inertia: (band.inertia + next.inertia) / 2, contrast: returning})
		if band.lingerBottom > 0 {
			legs = append(legs, gestureLeg{from: next.low, to: next.low, rest: band.lingerBottom})
		}
	}
	return legs
}

func gestureLegWeight(g GestureSpec, contrast, from, to, pace float64) float64 {
	if g.FasterDirection == "even" {
		return pace
	}
	contrast *= 0.65
	if (to > from) == (g.FasterDirection == "tip") {
		return pace * (1 - contrast)
	}
	return pace * (1 + contrast)
}
