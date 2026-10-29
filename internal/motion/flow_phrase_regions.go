package motion

import (
	"errors"
	"fmt"
	"math"
)

func (s FlowSpec) validatePhraseRegions() error {
	regions := s.Phrase.Regions
	if len(regions) < 1 || len(regions) > 6 {
		return errors.New("a regional phrase needs one to six regions")
	}
	total := 0
	for i, r := range regions {
		if err := r.validateWithin(s.MinPercent, s.MaxPercent, r.TopPercent, i, len(regions)); err != nil {
			return err
		}
		if r.Cycle != "up_down" && r.Cycle != "down_up" {
			return fmt.Errorf("phrase region %d cycle must be up_down or down_up", i+1)
		}
		total += r.Count
	}
	if total > 24 {
		return errors.New("a regional phrase supports at most 24 complete local cycles")
	}
	turns, err := regionTurns(regions)
	if err != nil {
		return err
	}
	if err := validatePhraseReversals(turns, s.MinPercent, s.MaxPercent); err != nil {
		return fmt.Errorf("regional travel: %w; choose compatible cycle directions or endpoints instead of adding a tiny connecting reversal", err)
	}
	return nil
}

func regionEndpoints(r PhraseRegion) (start, other int) {
	if r.Cycle == "down_up" {
		return r.TopPercent, r.BottomPercent
	}
	return r.BottomPercent, r.TopPercent
}

// regionTurns first writes exactly the selected local cycles and straight
// connecting travel. A region boundary passed in the same direction is a
// waypoint, not a reversal: merge it without adding a detour or a stop. The
// merged leg retains the summed travel time at the selected regional rates.
func regionTurns(regions []PhraseRegion) ([]PhraseTurn, error) {
	if len(regions) < 1 || len(regions) > 6 {
		return nil, errors.New("invalid regional phrase length")
	}
	total := 0
	for _, r := range regions {
		if r.BottomPercent < 0 || r.TopPercent > 100 || r.TopPercent-r.BottomPercent < 10 || r.RatePercent < 1 || r.RatePercent > 100 || (r.Cycle != "up_down" && r.Cycle != "down_up") {
			return nil, errors.New("invalid region bounds, rate or cycle")
		}
		if r.Count < 1 || r.Count > 8 {
			return nil, errors.New("region count must be 1..8")
		}
		total += r.Count
	}
	if total > 24 {
		return nil, errors.New("too many local cycles")
	}
	turns := make([]PhraseTurn, 0, 2*total+len(regions))
	for i, r := range regions {
		start, other := regionEndpoints(r)
		turns = append(turns, PhraseTurn{start, float64(r.RatePercent), 100 * i})
		for cycle := range r.Count {
			turns = append(turns, PhraseTurn{other, float64(r.RatePercent), 100*i + 2*cycle + 1}, PhraseTurn{start, float64(r.RatePercent), 100*i + 2*cycle + 2})
		}
	}
	return mergeRegionWaypoints(turns), nil
}

func mergeRegionWaypoints(turns []PhraseTurn) []PhraseTurn {
	for len(turns) > 2 {
		merged := false
		for i, at := range turns {
			previous, next := (i+len(turns)-1)%len(turns), (i+1)%len(turns)
			a, b := turns[previous], turns[next]
			incoming, outgoing := at.PositionPercent-a.PositionPercent, b.PositionPercent-at.PositionPercent
			if incoming*outgoing < 0 {
				continue
			}
			travel := math.Abs(float64(incoming)) + math.Abs(float64(outgoing))
			if travel > 0 && at.RatePercent != b.RatePercent {
				cost := math.Abs(float64(incoming))/at.RatePercent + math.Abs(float64(outgoing))/b.RatePercent
				turns[next].RatePercent = travel / cost
			}
			turns = append(turns[:i], turns[i+1:]...)
			merged = true
			break
		}
		if !merged {
			break
		}
	}
	return turns
}
