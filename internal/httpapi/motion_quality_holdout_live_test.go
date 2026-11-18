//go:build liveeval && magichandy_labs

package httpapi

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// These sequences were reserved for the comparison after prompt selection.
// Their wording, parameters and requested transformations differ from the
// endpoint examples used during development.
func qualityHoldoutCases(mirror bool) []qualityTurn {
	lo, hi, speed, move := 9, 63, 32, 12
	if mirror {
		lo, hi, speed, move = 26, 88, 43, -11
	}
	direction := "toward the tip"
	distance := move
	if move < 0 {
		direction = "toward the base"
		distance = -move
	}
	return []qualityTurn{
		{"marks", fmt.Sprintf("Begin at pace %d. The lower mark is %d and the upper mark is %d. Go from mark to mark on every pass and keep within them.", speed, lo, hi), "start", func(_, a *motion.FlowSpec, r *motion.Review) bool {
			return a != nil && a.SpeedPercent == speed && qualityBounds(r, lo, hi) && r.Summary.MeanStrokePercent >= float64(hi-lo-3)
		}},
		{"increment", "Same exact movement, six pace points higher, please.", "update", qualitySpeedDelta(6)},
		{"translate", fmt.Sprintf("Slide that whole working interval %d percentage points %s. Keep its width and how fast it is going.", distance, direction), "update", func(b, a *motion.FlowSpec, r *motion.Review) bool {
			return b != nil && a != nil && a.SpeedPercent == b.SpeedPercent && qualityBounds(r, b.MinPercent+move, b.MaxPercent+move)
		}},
		{"one_end", "Bring just the lower mark five points higher. Keep the upper mark and pace. Continue covering the full interval on every pass.", "update", func(b, a *motion.FlowSpec, r *motion.Review) bool {
			return b != nil && a != nil && a.SpeedPercent == b.SpeedPercent && qualityBounds(r, b.MinPercent+5, b.MaxPercent)
		}},
		{"explain", "What part of the slider is in use now? Just answer, keep playing the same pattern.", "none", nil},
		{"stop", "Stop.", "stop", nil},
	}
}

func qualityPhraseGroups(s *motion.FlowSpec) []motion.PhraseStroke {
	if s == nil || s.Phrase == nil {
		return nil
	}
	if len(s.Phrase.Strokes) > 0 {
		return slices.Clone(s.Phrase.Strokes)
	}
	out := make([]motion.PhraseStroke, 0, len(s.Phrase.Regions))
	for _, r := range s.Phrase.Regions {
		out = append(out, r.PhraseStroke)
	}
	return out
}

func qualityPhraseCases(mirror bool) []qualityTurn {
	shortLo, shortHi, wideLo, wideHi, n, speed := 65, 95, 10, 95, 3, 38
	if mirror {
		shortLo, shortHi, wideLo, wideHi, n, speed = 12, 42, 12, 88, 4, 32
	}
	groups := []motion.PhraseStroke{
		{BottomPercent: shortLo, TopPercent: shortHi, Count: n, RatePercent: 100},
		{BottomPercent: wideLo, TopPercent: wideHi, Count: 1, RatePercent: 100},
	}
	edit := func(change func([]motion.PhraseStroke)) func(*motion.FlowSpec, *motion.FlowSpec, *motion.Review) bool {
		return func(b, a *motion.FlowSpec, _ *motion.Review) bool {
			target := qualityPhraseGroups(b)
			if len(target) != 2 || a == nil || a.SpeedPercent != b.SpeedPercent {
				return false
			}
			change(target)
			return reflect.DeepEqual(target, qualityPhraseGroups(a))
		}
	}
	return []qualityTurn{
		{"counted", fmt.Sprintf("Start a repeating figure at overall speed %d. Make %d strokes from %d to %d, then one wide sweep from %d to %d. Both groups initially travel at the full selected rate.", speed, n, shortLo, shortHi, wideLo, wideHi), "start", func(_, a *motion.FlowSpec, r *motion.Review) bool {
			return a != nil && a.SpeedPercent == speed && reflect.DeepEqual(qualityPhraseGroups(a), groups) && qualityBounds(r, wideLo, wideHi)
		}},
		{"count_only", "One extra short stroke in each repetition. Leave the wide sweep, endpoints and travel rates exactly as they are.", "update", edit(func(g []motion.PhraseStroke) { g[0].Count++ })},
		{"rate_only", "Make just the broad sweep take twice as long. Leave the short strokes, counts and overall speed alone.", "update", edit(func(g []motion.PhraseStroke) { g[1].RatePercent /= 2 })},
		{"undo_rate", "Undo just that last timing change. Keep the extra short stroke.", "update", edit(func(g []motion.PhraseStroke) { g[1].RatePercent *= 2 })},
		{"explain", "What sequence will repeat? Answer without changing anything.", "none", nil},
		{"stop", "Stop.", "stop", nil},
	}
}
