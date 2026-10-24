package chat

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func validateHostedPhraseComposition(edits map[string]json.RawMessage) error {
	if edits["phrase"] == nil {
		return errors.New("phrase composition requires a counted phrase")
	}
	for name := range edits {
		if name != "composition" && name != "phrase" && name != "controls" && name != "evolve" && name != "layers" && name != "remove_layers" {
			return errors.New("phrase composition cannot include field geometry or layers")
		}
	}
	return validateHostedGeometryLayers(edits)
}

func validatePhraseObject(raw json.RawMessage) error {
	phrase, err := uniqueLayeredObject(raw)
	if err != nil {
		return err
	}
	if len(phrase) != 1 || len(phrase["strokes"]) == 0 {
		return errors.New("phrase requires explicit stroke groups")
	}
	var strokes []json.RawMessage
	if json.Unmarshal(phrase["strokes"], &strokes) != nil || strokes == nil {
		return errors.New("phrase strokes must be an array")
	}
	for _, raw := range strokes {
		fields, err := uniqueLayeredObject(raw)
		if err != nil {
			return err
		}
		if len(fields) != 4 {
			return errors.New("each phrase group requires bottom_percent, top_percent, count and rate_percent")
		}
		for _, key := range []string{"bottom_percent", "top_percent", "count", "rate_percent"} {
			if len(fields[key]) == 0 || string(fields[key]) == "null" {
				return errors.New("phrase values must be explicit numbers")
			}
		}
	}
	return nil
}

func applyLayeredPhrase(next *motion.FlowSpec, edit LayeredEdit) error {
	shapeEdit := edit.Geometry != "" || edit.StrokeWidth != nil
	for _, layer := range edit.Layers {
		shapeEdit = shapeEdit || layer.Axis != "pace"
	}
	for _, axis := range edit.RemoveLayers {
		shapeEdit = shapeEdit || axis != "pace"
	}
	for _, name := range []string{"min_percent", "max_percent", "anchor_percent", "variation_mode"} {
		shapeEdit = shapeEdit || layeredControlSpecified(edit, name)
	}
	if edit.Phrase == nil {
		if shapeEdit {
			next.Phrase = nil
		}
		return nil
	}
	if shapeEdit {
		return errors.New("a counted phrase owns its geometry and per-group pace; do not combine it with field geometry or layers")
	}
	phrase := *edit.Phrase
	phrase.Strokes = slices.Clone(phrase.Strokes)
	next.Phrase = &phrase
	next.MinPercent, next.MaxPercent = 100, 0
	shortest := 100
	for _, stroke := range phrase.Strokes {
		next.MinPercent = min(next.MinPercent, stroke.BottomPercent)
		next.MaxPercent = max(next.MaxPercent, stroke.TopPercent)
		span := stroke.TopPercent - stroke.BottomPercent
		shortest = min(shortest, span)
	}
	next.RangeFloorPercent, next.RangeCeilingPercent = shortest, next.MaxPercent-next.MinPercent
	allBottom, allTop := true, true
	for _, stroke := range phrase.Strokes {
		allBottom = allBottom && stroke.BottomPercent == next.MinPercent
		allTop = allTop && stroke.TopPercent == next.MaxPercent
	}
	if allTop && !allBottom {
		next.AnchorPercent = 100
	} else if allBottom && !allTop {
		next.AnchorPercent = 0
	}
	next.Layers = slices.DeleteFunc(next.Layers, func(layer motion.FlowLayer) bool { return layer.Axis != "pace" })
	return nil
}

func hostedPhraseSchema() map[string]any {
	integer := func(lo, hi int) map[string]any {
		return map[string]any{"type": "integer", "minimum": lo, "maximum": hi}
	}
	stroke := map[string]any{"type": "object", "additionalProperties": false,
		"properties": map[string]any{"bottom_percent": integer(0, 90), "top_percent": integer(10, 100), "count": integer(1, 8), "rate_percent": integer(1, 100)},
		"required":   []string{"bottom_percent", "top_percent", "count", "rate_percent"}}
	return map[string]any{"type": "object", "additionalProperties": false,
		"properties": map[string]any{"strokes": map[string]any{"type": "array", "minItems": 1, "maxItems": 6, "items": stroke}},
		"required":   []string{"strokes"}}
}

const hostedLayeredPhraseGuide = `Counted rhythm: composition:"phrase", phrase:{strokes:[{bottom_percent,top_percent,count,rate_percent},...]}. Absolute turns; repeat each pair count times, then loop the motif without resets/rests. Use 1..6 groups, counts1..8, total<=24; bottom+10<=own and preceding top, including wrap. rate_percent1..100 multiplies base travel rate. Ascents use their group; descents use the narrowest adjacent group containing the whole leg, or destination if tied/neither. Full sweeps thus keep their rate both ways. Coverage follows the request, never a fixed tip width; full-device=bottom0/top100.
Phrase derives bounds and replaces range/center layers. ` + hostedPhraseTimingGuide + ` Short travel already raises frequency; counts do not authorize speed increases. Repetition keeps seeded pace variation; memory_cycles develops variation, not counts. Speed, memory, pace_variation_percent and pace-layer edits preserve phrases. Geometry/width/range/center edits restore field motion, keeping a shared tip/base anchor or the prior anchor. Evolve changes seeded pace only. Exact motion needs no edits. Use field for uncounted wandering.`

const hostedPhraseTimingGuide = "Phrase pace allows edit/remove, not follow. When a new motif replaces a regional field, its old region-specific timing belongs to the replaced pattern, not a standing restriction. Carry that contrast into group rates only if it remains requested; crossing the old region alone is not a reason to slow a new sweep. Explicitly remove pace coupled to range/center layers that the phrase replaces, unless the user retains that speed cycle: otherwise it becomes an unrelated whole-phrase slowdown. Preserve independently requested pace variation and base speed. Count/reach-only edits within an existing phrase preserve group rates and independent pace; omit unrelated controls/layers/evolve."

// Hosted-only composition guidance; no language rules or local prompt cost.
const hostedLayeredRegionGuide = `Uncounted upper/lower: field alternate_ends, pace follow center (same=faster upper; opposite=faster lower). Allow several strokes per region before center travel consumes them. Bridge only failed phrase joins: high-to-low already descends; low-to-high needs a climb from the last lower turn to the next upper turn. A requested full sweep supplies that climb. Preserve counts, describe connecting travel; never widen regions or change counts just to pass validation.`
