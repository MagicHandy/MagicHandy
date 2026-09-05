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
		FasterDirection: "even", InertiaPercent: 25, ReboundDecayPercent: 60, VariationPercent: 35}
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
		legs = append(legs,
			gestureLeg{band.low, band.high, gestureLegWeight(g, band.low, band.high, band.pace), float64(g.InertiaPercent) / 100},
			gestureLeg{band.high, next.low, gestureLegWeight(g, band.high, next.low, (band.pace+next.pace)/2), float64(g.InertiaPercent) / 100})
	}
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
