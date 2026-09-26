package motion

import (
	"errors"
	"math"
	"slices"
)

// StrokeSpec is an LLM Lab experiment that describes continuous motion by
// where each stroke turns, in absolute slider positions: its bottom turn, how
// deep the stroke goes (0 is the base), and its top turn, how close to the tip
// it turns. Each turn has a usual position, a position it may go instead, and
// a character for how it moves between them. Accents are short motifs woven
// into the stream at a seeded rate. The shared joint leg fit times the strokes,
// as it does for Creative v2, so the ordinary plan, sampler and Stop own
// playback.
type StrokeSpec struct {
	Bottom StrokeTurn `json:"bottom"`
	Top    StrokeTurn `json:"top"`
	// Accents are at most three distinct motifs.
	Accents []StrokeAccent `json:"accents,omitempty"`
	// VariationPercent lets timing breathe from stroke to stroke; 0 repeats
	// every stroke's timing exactly.
	VariationPercent int `json:"variation_percent"`
}

// StrokeTurn is one turning point of every stroke.
type StrokeTurn struct {
	// AtPercent is the usual turning point, 0 the base and 100 the tip.
	AtPercent int `json:"at_percent"`
	// VaryToPercent is where the turn goes instead, deeper or shallower. It
	// equals AtPercent when the turn holds still.
	VaryToPercent int `json:"vary_to_percent"`
	// Character is how the turn moves between the two positions.
	Character string `json:"character"`
}

// StrokeAccent is a short motif that appears at a seeded rate.
type StrokeAccent struct {
	Move string `json:"move"`
	Rate string `json:"rate"`
}

// StrokeCharacters are the ways a turn moves between its two positions:
// holding still, wandering smoothly, switching every few strokes, visiting the
// other position now and then, or wandering in step with the other roaming
// turn so the whole stroke moves.
var StrokeCharacters = []string{"steady", "drift", "alternate", "occasional", "roam"}

// StrokeAccentMoves are the motifs an accent can weave in.
var StrokeAccentMoves = []string{"plunge", "full", "tip_flicks", "deep_grind", "pause", "slow"}

// StrokeAccentRates are how often an accent appears.
var StrokeAccentRates = []string{"rarely", "sometimes", "often"}

const (
	strokeInertia = 0.25
	// strokeWorkSpan is the length, in points, of the short strokes an accent
	// works at the top or bottom turn.
	strokeWorkSpan = 14.0
	// strokeOccasionalChance is the share of strokes an occasional turn spends
	// at its other position.
	strokeOccasionalChance = 0.2
	strokeMinimumSpan      = 10.0
	strokeDefaultCycles    = 64
)

// strokeAccentChance is the chance per stroke that an accent starts there.
var strokeAccentChance = map[string]float64{"rarely": 1.0 / 16, "sometimes": 1.0 / 8, "often": 1.0 / 4}

func (s FlowSpec) validateStrokes() error {
	k := s.Strokes
	if k == nil {
		return nil
	}
	if s.Gesture != nil || len(s.Steps) != 0 || len(s.Layers) != 0 {
		return errors.New("a stroke score has no gesture, sections or modulation layers")
	}
	if err := validateStrokeTurns(k.Bottom, k.Top); err != nil {
		return err
	}
	if k.VariationPercent < 0 || k.VariationPercent > 100 || len(k.Accents) > 3 {
		return errors.New("stroke variation is 0-100, with at most three accents")
	}
	seen := make(map[string]bool, len(k.Accents))
	for _, accent := range k.Accents {
		if !slices.Contains(StrokeAccentMoves, accent.Move) || !slices.Contains(StrokeAccentRates, accent.Rate) || seen[accent.Move] {
			return errors.New("accents need distinct moves and a rate of rarely, sometimes or often")
		}
		seen[accent.Move] = true
	}
	return nil
}

func validateStrokeTurns(b, t StrokeTurn) error {
	if b.AtPercent < 0 || b.AtPercent > 90 || b.VaryToPercent < 0 || b.VaryToPercent > 90 ||
		t.AtPercent < 10 || t.AtPercent > 100 || t.VaryToPercent < 10 || t.VaryToPercent > 100 ||
		t.AtPercent-b.AtPercent < 10 {
		return errors.New("stroke turns stay inside 0-100, with the usual top at least 10 above the usual bottom")
	}
	if !slices.Contains(StrokeCharacters, b.Character) || !slices.Contains(StrokeCharacters, t.Character) {
		return errors.New("a turn's character is steady, drift, alternate, occasional or roam")
	}
	return nil
}

// strokeUnit is one stroke of the realized stream: up from its bottom to its
// top, with its own time scale and optional rests at each turn.
type strokeUnit struct {
	bottom, top         float64
	pace                float64
	restTop, restBottom float64
}

func compileStrokeCurve(spec FlowSpec, handyModel string) (Curve, error) {
	legs := strokeLegs(spec)
	durations := make([]int64, len(legs))
	rate := referenceTravelRateForSpeed(spec.SpeedPercent, handyModel)
	budget := legBudget{velocity: referenceTravelRateForSpeed(100, handyModel),
		acceleration: runtimeMaxAccelerationPercentPerSecond2, jerk: runtimeMaxJerkPercentPerSecond3}
	for i, leg := range legs {
		if leg.rest > 0 {
			durations[i] = int64(math.Ceil(leg.rest * 1000))
			continue
		}
		floor := legTimeScale(gestureLegCurve(leg, 1000), budget)
		seconds := math.Max(math.Abs(leg.to-leg.from)/rate*leg.weight, floor)
		durations[i] = int64(math.Ceil(seconds * 1000 * 1.002))
	}
	curves, err := fitLegCurves(legs, durations, budget)
	if err != nil {
		return Curve{}, errors.New("stroke score timing did not converge inside motion limits")
	}
	return assembleLegCurve(legs, curves, "stroke score")
}

// strokeLegs turns the stream into alternating up and down strokes. Every
// joint is a genuine reversal, so the shared turn construction applies as it
// does to Creative v2.
func strokeLegs(spec FlowSpec) []gestureLeg {
	units := spec.strokeUnits()
	legs := make([]gestureLeg, 0, len(units)*3)
	for i, unit := range units {
		next := units[(i+1)%len(units)]
		legs = append(legs, gestureLeg{from: unit.bottom, to: unit.top, weight: unit.pace, inertia: strokeInertia})
		if unit.restTop > 0 {
			legs = append(legs, gestureLeg{from: unit.top, to: unit.top, rest: unit.restTop})
		}
		legs = append(legs, gestureLeg{from: unit.top, to: next.bottom, weight: (unit.pace + next.pace) / 2, inertia: strokeInertia})
		if unit.restBottom > 0 {
			legs = append(legs, gestureLeg{from: next.bottom, to: next.bottom, rest: unit.restBottom})
		}
	}
	return legs
}

func (s FlowSpec) strokeUnits() []strokeUnit {
	k := *s.Strokes
	cycles := s.LoopCycles
	if cycles == 0 {
		cycles = strokeDefaultCycles
	}
	s.LoopCycles = cycles
	variation := float64(k.VariationPercent) / 100
	bottoms := s.turnSeries(k.Bottom, cycles, 0x3b1d5)
	tops := s.turnSeries(k.Top, cycles, 0x7c2e9)
	units := make([]strokeUnit, 0, cycles+16)
	for i := range cycles {
		unit := strokeUnit{bottom: bottoms[i], top: tops[i], pace: s.strokePace(i, variation),
			restTop: s.linger(2*i, variation), restBottom: s.linger(2*i+1, variation)}
		units = append(units, s.accentUnits(i, unit)...)
	}
	return separateStrokeUnits(units)
}

// turnSeries is where one turn lands on each stroke. Every character draws
// from the score's seed, so a saved score replays exactly.
func (s FlowSpec) turnSeries(turn StrokeTurn, cycles int, salt uint32) []float64 {
	home, other := float64(turn.AtPercent), float64(turn.VaryToPercent)
	values := make([]float64, cycles)
	for i := range values {
		values[i] = home
	}
	switch turn.Character {
	case "drift", "roam":
		if turn.Character == "roam" {
			// Both roaming turns follow one field, so the stroke moves as a whole.
			salt = 0x40a3b
		}
		for i := range values {
			values[i] = home + (other-home)*clampFloat((s.driftField(float64(i), salt)-0.2)/0.6, 0, 1)
		}
	case "alternate":
		away, remaining := false, s.alternateRun(salt, 0)
		for i := range values {
			if remaining == 0 {
				away, remaining = !away, s.alternateRun(salt, i)
			}
			if away {
				values[i] = other
			}
			remaining--
		}
	case "occasional":
		for i := range values {
			if flowHashUnit(s.Seed^salt, strokeKey(i)) < strokeOccasionalChance {
				values[i] = other
			}
		}
	}
	return values
}

// alternateRun is a seeded run of two to six strokes, so alternation has no
// fixed period.
func (s FlowSpec) alternateRun(salt uint32, i int) int {
	return 2 + min(4, int(flowHashUnit(s.Seed^salt^0x5a17, strokeKey(i))*5))
}

// strokePace is the stroke's time scale, above 1 slower: the phrase eases off
// and recovers, the pace wanders over a few strokes, and neighbors never take
// identical time. It matches Creative v2's phrasing without its flurries.
func (s FlowSpec) strokePace(i int, variation float64) float64 {
	pace := s.breath(i, variation)
	pace *= 1 + 0.4*variation*(2*s.driftField(float64(i), 0x46a32)-1)
	return pace * (1 + 0.15*variation*(2*flowHashUnit(s.Seed^0x3c6f, strokeKey(i))-1))
}

// accentUnits applies the first accent drawn for this stroke, if any.
func (s FlowSpec) accentUnits(i int, unit strokeUnit) []strokeUnit {
	for index, accent := range s.Strokes.Accents {
		salt := 0xacce5 ^ strokeKey(index+1)*0x9e3779b1
		if flowHashUnit(s.Seed^salt, strokeKey(i)) >= strokeAccentChance[accent.Rate] {
			continue
		}
		switch accent.Move {
		case "plunge":
			unit.bottom = 0
		case "full":
			unit.bottom, unit.top = 0, 100
		case "tip_flicks", "deep_grind":
			return s.workTurn(i, unit, accent.Move == "tip_flicks")
		case "pause":
			unit.restTop = math.Max(unit.restTop, 0.4+0.4*flowHashUnit(s.Seed^0x9a05e, strokeKey(i)))
		case "slow":
			unit.pace *= 2
		}
		return []strokeUnit{unit}
	}
	return []strokeUnit{unit}
}

// workTurn replaces one stroke with two to four short, quicker strokes worked
// at its top or bottom turn. The motif is entered or left by an ordinary
// stroke, so it joins the stream without a jump.
func (s FlowSpec) workTurn(i int, unit strokeUnit, atTop bool) []strokeUnit {
	count := 2 + min(2, int(flowHashUnit(s.Seed^0xf11c3, strokeKey(i))*3))
	units := make([]strokeUnit, count)
	for j := range units {
		short := unit
		short.pace *= 0.8
		short.restTop, short.restBottom = 0, 0
		if atTop {
			short.bottom = math.Max(unit.bottom, unit.top-strokeWorkSpan)
		} else {
			short.top = math.Min(unit.top, unit.bottom+strokeWorkSpan)
		}
		units[j] = short
	}
	if atTop {
		units[0].bottom, units[0].pace = unit.bottom, unit.pace
	} else {
		units[count-1].top, units[count-1].pace = unit.top, unit.pace
	}
	return units
}

// separateStrokeUnits keeps every stroke at least ten points long and makes
// each descent a genuine reversal: the next bottom sits at least ten points
// below this top. Lowering a bottom only lengthens its own stroke.
func separateStrokeUnits(units []strokeUnit) []strokeUnit {
	for i := range units {
		if units[i].top-units[i].bottom < strokeMinimumSpan {
			units[i].top = math.Min(100, units[i].bottom+strokeMinimumSpan)
			units[i].bottom = units[i].top - strokeMinimumSpan
		}
	}
	for i := range units {
		next := &units[(i+1)%len(units)]
		if next.bottom > units[i].top-strokeMinimumSpan {
			next.bottom = math.Max(0, units[i].top-strokeMinimumSpan)
		}
	}
	return units
}
