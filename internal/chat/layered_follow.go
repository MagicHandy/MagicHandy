package chat

import (
	"errors"
	"slices"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Resolve after geometry and every axis edit, so JSON field order cannot affect
// which timing is copied. The engine receives an ordinary immutable score.
func applyLayeredFollowing(score *motion.FlowSpec, edits []LayeredLayerEdit) error {
	for _, edit := range edits {
		if edit.Follow == nil {
			continue
		}
		if score.Phrase != nil {
			return errors.New("a counted phrase has no range or center layer to follow; edit independent pace instead")
		}
		link := edit.Follow
		if edit.Axis != "pace" || (link.Axis != "center" && link.Axis != "range") || (link.Relation != "same" && link.Relation != "opposite") {
			return errors.New("pace follow requires center or range and a same or opposite relation")
		}
		if edit.PeriodCycles != nil || edit.PeriodChangeCycles != nil || edit.PhasePercent != nil || edit.Shape != nil || edit.Inverted != nil {
			return errors.New("follow selects timing; do not also specify independent pace timing")
		}
		source := slices.IndexFunc(score.Layers, func(l motion.FlowLayer) bool { return l.Axis == link.Axis })
		target := slices.IndexFunc(score.Layers, func(l motion.FlowLayer) bool { return l.Axis == "pace" })
		if source < 0 || target < 0 || score.Layers[source].AmountPercent == 0 || score.Layers[source].Shape == "drift" {
			return errors.New("pace follow needs an active periodic source layer in the resulting score")
		}
		from, to := score.Layers[source], &score.Layers[target]
		to.PeriodCycles, to.PhasePercent, to.Shape = from.PeriodCycles, from.PhasePercent, from.Shape
		to.Inverted = from.Inverted != (link.Relation == "opposite")
	}
	return nil
}
