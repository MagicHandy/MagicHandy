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

const hostedLayeredPhraseGuide = `Counted rhythm: composition:"phrase", phrase:{strokes:[{bottom_percent,top_percent,count,rate_percent},...]}. These are absolute slider turning positions: bottom is the lower/deeper turnaround, top the upper/shallower turnaround. Each group contributes count lower/upper turn pairs, in order; the whole phrase repeats. Adjacent pairs join directly without resets or added pauses. Keep at least10 points between each bottom and both adjacent tops. Choose1..6 groups, count1..8 each, total<=24. rate_percent1..100 scales base travel rate (100 preserves it). Each ascent uses its group's rate. A shared descent uses the narrowest adjacent group containing that whole leg; ties or neither-containing use the destination group. Thus a full sweep's outbound and return both use its rate, whether anchored at the base or tip. Choose regional coverage for the request, without a fixed tip width. A complete device sweep uses bottom0/top100, regardless of the preceding band's endpoints. Phrase replaces range/center layers and derives its band from the chosen turns. Previously copied pace timing becomes independent; phrase pace supports edit/remove, never follow. Returning to field motion keeps a shared base/tip anchor when one exists; otherwise the prior anchor remains. Counts and reach do not specify pace. Shorter travel already produces more strokes at the same travel rate; do not raise speed to make a counted group quicker. For a count/reach-only edit, omit controls, layers and evolve, and retain existing per-group rates when editing a phrase. Repetition means the ordered groups recur; it does not mean removing seeded pace variation. memory_cycles controls variation development, never the number of strokes in a motif. Independent pace changes remain available through speed_percent,memory_cycles,pace_variation_percent and the pace layer when requested or when autonomous pacing is permitted. Independent pace-control/layer edits retain an existing phrase. Geometry/stroke_width/shape-control/range-or-center-layer edits return to field motion, replacing the phrase. Evolve changes seeded pace detail only. Use field motion for uncounted wandering; do not substitute it for requested counts. Keeping the exact motion means no edits: its internal variation already repeats.`
