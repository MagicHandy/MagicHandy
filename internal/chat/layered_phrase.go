package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func validateHostedPhraseComposition(edits map[string]json.RawMessage) error {
	if edits["phrase"] == nil {
		return errors.New("phrase composition requires a counted phrase")
	}
	for name := range edits {
		if name != "composition" && name != "phrase" && name != "controls" && name != "evolve" && name != "layers" && name != "remove_layers" && name != "recall" && name != "directional_pace" {
			return fmt.Errorf("phrase composition cannot include %q; use counted groups for geometry and only pace for layers", name)
		}
	}
	// A phrase already removes field geometry. Explicit removals are redundant,
	// not competing geometry; the ordinary domain parser still validates axes.
	if layers, present := edits["layers"]; present {
		return validateHostedGeometryLayers(map[string]json.RawMessage{"layers": layers})
	}
	return nil
}

func validatePhraseObject(raw json.RawMessage) error {
	phrase, err := uniqueLayeredObject(raw)
	if err != nil {
		return err
	}
	if len(phrase) != 1 || (len(phrase["strokes"]) == 0 && len(phrase["regions"]) == 0) {
		return errors.New("phrase requires exactly one of strokes or regions")
	}
	key, required := "strokes", []string{"bottom_percent", "top_percent", "count", "rate_percent"}
	if phrase["regions"] != nil {
		key, required = "regions", []string{"bottom_percent", "top_percent", "count", "rate_percent", "cycle"}
	}
	var strokes []json.RawMessage
	if json.Unmarshal(phrase[key], &strokes) != nil || strokes == nil {
		return fmt.Errorf("phrase %s must be an array", key)
	}
	if len(strokes) < 1 || len(strokes) > 6 {
		return fmt.Errorf("phrase %s requires one to six groups", key)
	}
	for _, raw := range strokes {
		fields, err := uniqueLayeredObject(raw)
		if err != nil {
			return err
		}
		if len(fields) != len(required) {
			return fmt.Errorf("each phrase %s item requires exactly %v", key, required)
		}
		for _, key := range required {
			if len(fields[key]) == 0 || string(fields[key]) == "null" {
				return errors.New("phrase values must be explicit and non-null")
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
		shapeEdit = shapeEdit || (edit.Phrase == nil && axis != "pace" && slices.ContainsFunc(next.Layers, func(layer motion.FlowLayer) bool { return layer.Axis == axis }))
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
	phrase.Regions = slices.Clone(phrase.Regions)
	next.Phrase = &phrase
	applyPhraseGeometry(next, phrase)
	next.Layers = slices.DeleteFunc(next.Layers, func(layer motion.FlowLayer) bool { return layer.Axis != "pace" })
	return nil
}

func applyPhraseGeometry(next *motion.FlowSpec, phrase motion.FlowPhrase) {
	groups := phrase.Strokes
	if len(phrase.Regions) > 0 {
		groups = make([]motion.PhraseStroke, len(phrase.Regions))
		for i, r := range phrase.Regions {
			groups[i] = r.PhraseStroke
		}
	}
	next.MinPercent, next.MaxPercent = 100, 0
	shortest := 100
	for _, stroke := range groups {
		next.MinPercent = min(next.MinPercent, stroke.BottomPercent)
		next.MaxPercent = max(next.MaxPercent, stroke.TopPercent)
		span := stroke.TopPercent - stroke.BottomPercent
		shortest = min(shortest, span)
	}
	next.RangeFloorPercent, next.RangeCeilingPercent = shortest, next.MaxPercent-next.MinPercent
	allBottom, allTop := true, true
	for _, stroke := range groups {
		allBottom = allBottom && stroke.BottomPercent == next.MinPercent
		allTop = allTop && stroke.TopPercent == next.MaxPercent
	}
	if allTop && !allBottom {
		next.AnchorPercent = 100
	} else if allBottom && !allTop {
		next.AnchorPercent = 0
	}
}

func hostedPhraseSchema() map[string]any {
	integer := func(lo, hi int) map[string]any {
		return map[string]any{"type": "integer", "minimum": lo, "maximum": hi}
	}
	stroke := map[string]any{"type": "object", "additionalProperties": false,
		"properties": map[string]any{"bottom_percent": integer(0, 90), "top_percent": integer(10, 100), "count": integer(1, 8), "rate_percent": integer(1, 100)},
		"required":   []string{"bottom_percent", "top_percent", "count", "rate_percent"}}
	groups := map[string]any{"type": "object", "additionalProperties": false,
		"properties": map[string]any{"strokes": map[string]any{"type": "array", "minItems": 1, "maxItems": 6, "items": stroke}},
		"required":   []string{"strokes"}}
	regionProperties := maps.Clone(stroke["properties"].(map[string]any))
	regionProperties["cycle"] = map[string]any{"type": "string", "enum": []string{"up_down", "down_up"}}
	region := map[string]any{"type": "object", "additionalProperties": false, "properties": regionProperties, "required": []string{"bottom_percent", "top_percent", "count", "rate_percent", "cycle"}}
	regions := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"regions": map[string]any{"type": "array", "minItems": 1, "maxItems": 6, "items": region}}, "required": []string{"regions"}}
	return map[string]any{"oneOf": []any{groups, regions}}
}

const hostedLayeredPhraseGuide = `Counted rhythm: composition:"phrase", phrase:{strokes:[{bottom_percent,top_percent,count,rate_percent},...]}. Absolute turns; repeat each pair count times, then loop the motif without resets/rests. Use 1..6 groups, counts1..8, total<=24; bottom+10<=own and preceding top, including wrap. rate_percent1..100 multiplies base travel rate. Ascents use their group; descents use the narrowest adjacent group containing the whole leg, or destination if tied/neither. Full sweeps thus keep their rate both ways. Coverage follows the request, never a fixed tip width; full-device=bottom0/top100.
Phrase derives bounds and replaces range/center layers. ` + hostedPhraseTimingGuide + ` Short travel already raises frequency; counts do not authorize speed increases. Repetition keeps seeded pace variation; memory_cycles develops variation, not counts. Speed, memory, pace_variation_percent and pace-layer edits preserve phrases. Geometry/width/range/center edits restore field motion, keeping a shared tip/base anchor or the prior anchor. Evolve changes seeded pace only. Exact motion needs no edits. Use field for uncounted wandering.`

const hostedPhraseTimingGuide = "Phrase pace allows edit/remove, not follow. When a new motif replaces a regional field, its old region-specific timing belongs to the replaced pattern, not a standing restriction. Carry that contrast into group rates only if it remains requested; crossing the old region alone is not a reason to slow a new sweep. Explicitly remove pace coupled to range/center layers that the phrase replaces, unless the user retains that speed cycle: otherwise it becomes an unrelated whole-phrase slowdown. Preserve independently requested pace variation and base speed. Count/reach-only edits within an existing phrase preserve group rates and independent pace; omit unrelated controls/layers/evolve."

// Hosted-only composition guidance; no language rules or local prompt cost.
const hostedLayeredRegionGuide = `For counted motion in separate regions, use composition:"phrase" with phrase:{regions:[{bottom_percent,top_percent,count,rate_percent,cycle},...]}, instead of strokes. Each region performs count COMPLETE local out-and-back cycles; cycle:"up_down" starts and returns at bottom, "down_up" at top. Use 1..6 regions, each width>=10, count1..8, total<=24. Choose meaningful endpoints and rates for the request. The shared compiler joins regions directly, including the loop seam, without added excursions or rests. A boundary crossed in the same direction is a waypoint, not a reversal; a final return and the following approach can share one continuous leg. Connecting travel uses the destination region's rate; merged travel retains the combined time from its selected rates. Do not add bridging groups or enumerate turning points. Describe the local cycles and necessary connecting travel separately. Regional summaries and pace periods include connecting travel as well as local cycles.
Use compact strokes for overlapping/shared-endpoint motifs already expressed by lower/upper pairs. Uncounted upper/lower motion uses field alternate_ends, with pace follow center when location and pace should correspond. More natural flow does not require reduced speed.`
