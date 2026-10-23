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

const hostedLayeredPhraseGuide = `Counted rhythm: composition:"phrase", phrase:{strokes:[{bottom_percent,top_percent,count,rate_percent},...]}. Absolute lower/upper slider turns form ordered pairs; count repeats each pair, then the whole motif loops, without resets or rests. Use 1..6 groups, counts 1..8, total<=24; each bottom must be at least10 below its own and preceding top, including wrap. rate_percent1..100 multiplies base travel rate. Ascents use their group's rate. Descents use the narrowest adjacent group containing the entire leg; ties/neither use the destination rate. This keeps both directions of a full sweep at its rate, at either end. Choose regional coverage for the request, never a fixed tip width. Full-device sweeps use bottom0/top100.
Phrase owns geometry: it derives outer bounds and replaces range/center layers. Existing materialized pace timing persists independently; phrase pace allows edit/remove, not follow. Count/reach-only edits omit controls/layers/evolve and preserve existing group rates. Short travel already increases stroke frequency; counts do not authorize more speed. Repetition retains seeded pace variation. memory_cycles develops variation, not motif counts. Independent speed_percent, memory_cycles, pace_variation_percent and pace-layer edits preserve the phrase. Shape/geometry/stroke_width/range-or-center edits replace it with field motion, retaining a shared tip/base anchor when present, otherwise the prior anchor. Evolve changes seeded pace details only. Exact motion uses no edits. Use field motion for uncounted wandering; preserve explicitly requested counts.`

// Hosted-only composition guidance; no language rules or local prompt cost.
const hostedLayeredRegionGuide = `Uncounted upper/lower alternation fits field alternate_ends with pace follow center (same=faster upper; opposite=faster lower). Allow several strokes in each region before the center travels onward; overly rapid center changes consume the short strokes. Phrases are cyclic turns: bottom[i]+10 <= min(top[i],top[previous]), including wrap. Bridge only failed joins: high-to-low already descends; low-to-high needs a climb pair from the last lower turn to the next upper turn. A requested full sweep already supplies that climb; do not add another. Preserve regional counts and describe connecting travel; never silently widen regions or change counts to pass validation.`
