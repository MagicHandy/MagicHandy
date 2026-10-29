package motion

import (
	"errors"
	"fmt"
)

func validatePhraseReversals(turns []PhraseTurn, minimum, maximum int) error {
	bottom, top := 100, 0
	for i, turn := range turns {
		p := turn.PositionPercent
		if p < minimum || p > maximum {
			return fmt.Errorf("phrase turn %d position %d must stay within %d..%d", i+1, p, minimum, maximum)
		}
		if turn.RatePercent < 1 || turn.RatePercent > 100 {
			return fmt.Errorf("phrase turn %d rate_percent must be 1..100", i+1)
		}
		before, after := turns[(i+len(turns)-1)%len(turns)].PositionPercent, turns[(i+1)%len(turns)].PositionPercent
		if !phraseReversal(before, p, after) {
			return fmt.Errorf("phrase turn %d is not a reversal of at least 10 points: %d -> %d -> %d, including the loop seam; author connecting travel explicitly without stopping midway", i+1, before, p, after)
		}
		bottom, top = min(bottom, p), max(top, p)
	}
	if minimum != bottom || maximum != top {
		return errors.New("phrase outer bounds must match its turning positions")
	}
	return nil
}

// OrderedTurns expands counted strokes or local region cycles into semantic
// reversals. Only Regions explicitly authorize connecting travel. Compilation
// validates first; malformed inspection inputs return no turns.
func (p FlowPhrase) OrderedTurns() []PhraseTurn {
	if len(p.Regions) > 0 {
		turns, _ := regionTurns(p.Regions)
		return turns
	}
	if len(p.Strokes) > 6 {
		return nil
	}
	if len(p.Strokes) == 0 {
		return nil
	}
	previous := p.Strokes[len(p.Strokes)-1]
	turns := make([]PhraseTurn, 0, 48)
	for _, stroke := range p.Strokes {
		if stroke.Count < 1 || stroke.Count > 8 || len(turns)+2*stroke.Count > 48 {
			return nil
		}
		for range stroke.Count {
			turns = append(turns, PhraseTurn{stroke.BottomPercent, float64(phraseDescentRate(previous, stroke)), 0}, PhraseTurn{stroke.TopPercent, float64(stroke.RatePercent), 0})
			previous = stroke
		}
	}
	return turns
}

func phraseReversal(before, at, after int) bool {
	return (at >= before+10 && at >= after+10) || (at <= before-10 && at <= after-10)
}
