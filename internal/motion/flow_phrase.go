package motion

import (
	"errors"
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
	for _, stroke := range p.Strokes {
		if !stroke.validWithin(s.MinPercent, s.MaxPercent, previousTop) {
			return errors.New("phrase pairs need at least ten points between each lower turn and both adjacent upper turns, counts 1-8 and rates 1-100")
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

func (s PhraseStroke) validWithin(bottom, top, previousTop int) bool {
	return s.BottomPercent >= bottom && s.TopPercent <= top && s.TopPercent-s.BottomPercent >= 10 && previousTop-s.BottomPercent >= 10 && s.Count >= 1 && s.Count <= 8 && s.RatePercent >= 1 && s.RatePercent <= 100
}

func compilePhraseCurve(spec FlowSpec, handyModel string) (Curve, error) {
	legs, durations := phraseLegs(spec, handyModel)
	budget := legBudget{velocity: referenceTravelRateForSpeed(100, handyModel), acceleration: flowAccelerationBudget, jerk: flowJerkBudget}
	curves, err := fitLegCurves(legs, durations, budget)
	if err != nil {
		return Curve{}, err
	}
	curve, err := assembleLegCurve(legs, curves, "Layered phrase")
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
					durations = append(durations, int64(math.Ceil(millis)))
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
