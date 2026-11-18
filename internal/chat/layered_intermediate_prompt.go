package chat

import (
	"encoding/json"
	"strings"
)

const intermediateLayeredRangeGuide = "Outer range edits replace the previous endpoints. When the user specifies endpoint numbers, use those numbers for both endpoints; the current range is not a saved limit."
const intermediateLayeredListGuide = "Edits is a list of independent changes, exactly one group per item. Emit all requested groups, not just the first. Never repeat a group. For a compound range and geometry change use two items. Questions and exact holds use an empty list."
const intermediateLayeredGeometryGuide = "Absolute outer reach uses edits.range with both min_percent and max_percent; never put endpoints in controls. Changing only the shorter strokes preserves the widest strokes: keep stroke_width.max_percent at the current outer width.\nGeometry full_slider_and_tip/full_slider_and_base mix full-DEVICE 0..100 sweeps with shorter strokes at that end, replacing an old region. Use these for full-device mixtures. full_band_and_tip/full_band_and_base instead preserve the current outer endpoints. All preserve pace and allow stroke_width overrides for custom coverage."

// The full reference and grouped schema together exceed the Simple 16K floor
// even on a fresh chat. The lifecycle, current score, output and voice sections
// still come from the ordinary composer.
const compactGroupedLayeredContract = `Use action, edits and reply. Each item in edits changes one group. Include each requested group once. Omit unchanged groups. Questions and continuing an already playing pattern use action none and edits:[] unless a change is also requested. Describe only changes in the edits.
If a named motion's definition is missing, use action none and ask for its endpoints and speed in your own words. Say it is playing only after applying it. The latest definition wins for every value, including speed. Apply its two marks, geometry and overall speed as separate groups. For example, applying a corrected 24..71 full sweep at speed 36 uses "edits":[{"range":{"min_percent":24,"max_percent":71}},{"geometry":"full_band"},{"controls":{"speed_percent":36}}]. Substitute all three values from the latest definition.
For full sweeps between named marks, send range with BOTH requested endpoints and geometry full_band. Add controls.speed_percent when requested. The marks must be at least 10 apart. Reserve full_slider for full travel from 0 (base) through 100 (tip). full_band_and_tip and full_band_and_base mix full-band sweeps with short strokes at that end. full_slider_and_tip and full_slider_and_base open the whole slider first. alternate_ends uses short strokes at both ends. tip_anchor, base_anchor and centered anchor width variation. wander varies width and location.
stroke_width:{min_percent,max_percent} gives the shortest and widest strokes, 10 through the outer width. Equal values fix the width. Broader short strokes raise only the shortest width. controls sets overall speed_percent inside the saved limits. change_by.speed_percent adds a signed number to the current pace. A pace-only request preserves everything else. controls can also set anchor_percent, memory_cycles, pace_variation_percent or variation_mode.
layers patches named range, center or pace layers. amount_percent controls strength, period_cycles sets a 2..32 cycle period, period_change_cycles adds cycles, phase_percent offsets it and shape selects drift, alternate or wave. remove_layers removes only the named axes. evolve refreshes seeded detail when explicitly requested. When earlier scores are listed, recall can restore a whole identified score. Never emit raw points or device commands.`

// Translate code-owned instructions/examples only, before personalization or
// conversation is composed. This is a schema dialect, never a language rule.
// It is idempotent so state-specific live-action examples can be translated too.
func intermediateLayeredContract(contract string) string {
	base := contract == layeredContract
	contract = strings.ReplaceAll(contract, "controls.min_percent/max_percent", "range.min_percent/max_percent")
	contract = strings.ReplaceAll(contract, "controls min_percent/max_percent", "range min_percent/max_percent")
	contract = strings.ReplaceAll(contract, "controls: partial absolute min_percent/max_percent (outer band 0..100; deepest/shallowest endpoints), speed_percent", "range:{min_percent,max_percent}: outer travel endpoints; supply BOTH when changing either. New reach replaces the old band, not a safety limit. controls: partial absolute speed_percent")
	contract = strings.ReplaceAll(contract, "full_and_tip", "full_band_and_tip")
	contract = strings.ReplaceAll(contract, "full_and_base", "full_band_and_base")
	lines := strings.Split(contract, "\n")
	for i, line := range lines {
		start := strings.Index(line, `{"`)
		if start < 0 {
			continue
		}
		var root map[string]json.RawMessage
		if json.Unmarshal([]byte(line[start:]), &root) != nil {
			continue
		}
		var edits map[string]json.RawMessage
		if json.Unmarshal(root["edits"], &edits) != nil || edits == nil {
			continue
		}
		var controls map[string]json.RawMessage
		if json.Unmarshal(edits["controls"], &controls) == nil && controls["min_percent"] != nil && controls["max_percent"] != nil {
			edits["range"], _ = json.Marshal(map[string]json.RawMessage{"min_percent": controls["min_percent"], "max_percent": controls["max_percent"]})
			delete(controls, "min_percent")
			delete(controls, "max_percent")
			if len(controls) == 0 {
				delete(edits, "controls")
			} else {
				edits["controls"], _ = json.Marshal(controls)
			}
		}
		list := []map[string]json.RawMessage{}
		for _, key := range intermediateLayeredGroups {
			if value := edits[key]; value != nil {
				list = append(list, map[string]json.RawMessage{key: value})
			}
		}
		root["edits"], _ = json.Marshal(list)
		encoded, _ := json.Marshal(root)
		lines[i] = line[:start] + string(encoded)
	}
	contract = strings.Join(lines, "\n")
	contract = strings.ReplaceAll(contract, `"edits":{...}`, `"edits":[...]`)
	contract = strings.ReplaceAll(contract, "edits is {}", "edits is []")
	if base {
		contract = intermediateLayeredRangeGuide + "\n" + contract + "\n" + intermediateLayeredGeometryGuide + "\n" + intermediateLayeredListGuide
	}
	return contract
}
