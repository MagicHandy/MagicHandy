package motion

import (
	"errors"
	"fmt"
	"math"
)

// FlowPhrase supplies counted pairs of lower and upper turns. It is
// semantic content for the same Layered joint fitter, plan, sampler and Stop.
// No device coordinates, timestamps, private playback or implicit rests exist.
type FlowPhrase struct {
	Strokes []PhraseStroke `json:"strokes"`
}

// PhraseStroke contributes Count lower/upper turn pairs, directly joined to
// the preceding upper turn. Shared descents belong to the narrowest adjacent
// group containing the whole leg; otherwise they use the destination's rate.
// RatePercent is a fraction of the score's selected travel rate, not a second
// absolute speed. The model selects reach; there is no built-in tip width.
type PhraseStroke struct {
	BottomPercent int `json:"bottom_percent"`
	TopPercent    int `json:"top_percent"`
	Count         int `json:"count"`
	RatePercent   int `json:"rate_percent"`
}

func (s FlowSpec) validatePhrase() error {
	p := s.Phrase
	if p == nil {
		return nil
	}
	if s.Gesture != nil || s.Strokes != nil || s.Freestyle != nil || len(s.Steps) != 0 {
		return errors.New("a counted Layered phrase replaces other stroke grammars")
	}
	for _, layer := range s.Layers {
		if layer.Axis != "pace" {
			return errors.New("counted phrase geometry cannot also use range or center layers")
		}
	}
	if len(p.Strokes) < 1 || len(p.Strokes) > 6 {
		return errors.New("a Layered phrase needs one to six stroke groups")
	}
	total := 0
	bottom, top := 100, 0
	previousTop := p.Strokes[len(p.Strokes)-1].TopPercent
	for i, stroke := range p.Strokes {
		if err := stroke.validateWithin(s.MinPercent, s.MaxPercent, previousTop, i, len(p.Strokes)); err != nil {
			return err
		}
		previousTop = stroke.TopPercent
		bottom, top = min(bottom, stroke.BottomPercent), max(top, stroke.TopPercent)
		total += stroke.Count
	}
	if total > 24 {
		return errors.New("a Layered phrase supports at most 24 complete strokes")
	}
	if s.MinPercent != bottom || s.MaxPercent != top {
		return errors.New("phrase outer bounds must match its turning positions")
	}
	return nil
}

func (s PhraseStroke) validateWithin(bottom, top, previousTop, index, groups int) error {
	group := index + 1
	if s.BottomPercent < bottom || s.TopPercent > top {
		return fmt.Errorf("phrase group %d turns %d..%d must stay within the outer bounds %d..%d", group, s.BottomPercent, s.TopPercent, bottom, top)
	}
	if s.TopPercent-s.BottomPercent < 10 {
		return fmt.Errorf("phrase group %d top_percent (%d) must be at least 10 points above its bottom_percent (%d)", group, s.TopPercent, s.BottomPercent)
	}
	if s.Count < 1 || s.Count > 8 {
		return fmt.Errorf("phrase group %d count must be 1..8; got %d", group, s.Count)
	}
	if s.RatePercent < 1 || s.RatePercent > 100 {
		return fmt.Errorf("phrase group %d rate_percent must be 1..100; got %d", group, s.RatePercent)
	}
	if previousTop-s.BottomPercent < 10 {
		previousGroup := index
		join := "join"
		if index == 0 {
			previousGroup, join = groups, "loop seam"
		}
		return fmt.Errorf("phrase %s from group %d to group %d is not a downward leg: previous top_percent (%d) must be at least 10 points above the next bottom_percent (%d); every group join, including last-to-first, must alternate lower and upper turns", join, previousGroup, group, previousTop, s.BottomPercent)
	}
	return nil
}

func compilePhraseCurve(spec FlowSpec, handyModel string) (Curve, error) {
	legs, durations := phraseLegs(spec, handyModel)
	budget := legBudget{velocity: referenceTravelRateForSpeed(100, handyModel), acceleration: flowAccelerationBudget, jerk: flowJerkBudget}
	curve, err := fitLayeredCurve(legs, durations, budget, "Layered phrase")
	if err != nil {
		return Curve{}, err
	}
	curve.flowPeriod = float64(len(legs)) / 2
	curve.flowCycles = make([]float64, len(legs)+1)
	for i := range curve.flowCycles {
		curve.flowCycles[i] = float64(i) / 2
	}
	return curve, nil
}

func phraseLegs(spec FlowSpec, handyModel string) ([]gestureLeg, []int64) {
	p := spec.Phrase
	count := 0
	for _, stroke := range p.Strokes {
		count += stroke.Count
	}
	// Repeat whole motifs over the ordinary Layered horizon. A three-stroke
	// motif must not compress a long pace layer into just three strokes.
	repeats := max(1, (int(spec.cycleCount())+count-1)/count)
	spec.LoopCycles = count * repeats
	legs := make([]gestureLeg, 0, 2*count*repeats)
	durations := make([]int64, 0, 2*count*repeats)
	previousTop := float64(p.Strokes[len(p.Strokes)-1].TopPercent)
	previous := p.Strokes[len(p.Strokes)-1]
	for range repeats {
		for _, stroke := range p.Strokes {
			for range stroke.Count {
				for half := range 2 {
					from, to := previousTop, float64(stroke.BottomPercent)
					rate := phraseDescentRate(previous, stroke)
					if half == 1 {
						from, to = float64(stroke.BottomPercent), float64(stroke.TopPercent)
						rate = stroke.RatePercent
					}
					u := float64(len(legs)) / 2
					variation := float64(spec.PaceVariationPercent) / 100
					pace := (1 - variation + variation*spec.field(u+0.25, 0x85ebca6b)) * float64(rate) / 100
					for _, layer := range spec.Layers {
						pace *= flowPaceFactor(layer, spec.layerEnvelope(u+0.25, layer))
					}
					span := math.Abs(to - from)
					millis := spec.flowClock(span, float64(spec.SpeedPercent), pace, span, handyModel) / 2
					legs = append(legs, gestureLeg{from: from, to: to, weight: 1, softness: float64(spec.TurnSoftnessPercent) / 100, sourceCycle: u})
					durations = append(durations, layeredDurationMillis(millis))
				}
				previousTop = float64(stroke.TopPercent)
				previous = stroke
			}
		}
	}
	return legs, durations
}

// The return from a full base-anchored sweep belongs to that sweep, just as
// both legs of its tip-anchored mirror do. Equal spans prefer the destination.
func phraseDescentRate(previous, next PhraseStroke) int {
	previousContains := next.BottomPercent >= previous.BottomPercent
	nextContains := previous.TopPercent <= next.TopPercent
	if previousContains && (!nextContains || previous.TopPercent-previous.BottomPercent < next.TopPercent-next.BottomPercent) {
		return previous.RatePercent
	}
	return next.RatePercent
}
