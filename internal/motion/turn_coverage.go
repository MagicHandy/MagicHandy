package motion

import (
	"math"
	"slices"
)

// TurnCoverage separates one isolated endpoint visit from typical stroke reach.
// Fractions describe compiled turns, never measured carriage position.
type TurnCoverage struct {
	LowerPercentiles [3]float64 `json:"lower_p10_p50_p90"`
	UpperPercentiles [3]float64 `json:"upper_p10_p50_p90"`
	BaseFraction     float64    `json:"lower_within_one_of_base_fraction"`
	TipFraction      float64    `json:"upper_within_one_of_tip_fraction"`
	LowerCount       int        `json:"lower_count"`
	UpperCount       int        `json:"upper_count"`
}

func summarizeTurnCoverage(turns []perceptualReversal) TurnCoverage {
	var result TurnCoverage
	var lower, upper []float64
	for i, turn := range turns {
		previous, next := turns[(i+len(turns)-1)%len(turns)], turns[(i+1)%len(turns)]
		if turn.position < previous.position && turn.position < next.position {
			lower = append(lower, turn.position)
			if turn.position <= 1 {
				result.BaseFraction++
			}
		}
		if turn.position > previous.position && turn.position > next.position {
			upper = append(upper, turn.position)
			if turn.position >= 99 {
				result.TipFraction++
			}
		}
	}
	result.LowerCount, result.UpperCount = len(lower), len(upper)
	if len(lower) > 0 {
		result.BaseFraction /= float64(len(lower))
	}
	if len(upper) > 0 {
		result.TipFraction /= float64(len(upper))
	}
	result.LowerPercentiles, result.UpperPercentiles = turnPercentiles(lower), turnPercentiles(upper)
	return result
}

func turnPercentiles(values []float64) [3]float64 {
	var result [3]float64
	if len(values) == 0 {
		return result
	}
	slices.Sort(values)
	for i, fraction := range []float64{.1, .5, .9} {
		index := fraction * float64(len(values)-1)
		lo, hi := int(math.Floor(index)), int(math.Ceil(index))
		result[i] = math.Round((values[lo]+(values[hi]-values[lo])*(index-float64(lo)))*100) / 100
	}
	return result
}
