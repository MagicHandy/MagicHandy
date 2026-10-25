package chat

import (
	"encoding/json"
	"strings"
)

const intermediateLayeredRangeGuide = "Outer range edits replace the previous endpoints. When the user specifies endpoint numbers, use those numbers for both endpoints; the current range is not a saved limit."
const intermediateLayeredListGuide = "Edits is a list of independent changes, exactly one group per item. Emit all requested groups, not just the first. Never repeat a group. For a compound range and geometry change use two items. Questions and exact holds use an empty list."
const intermediateLayeredGeometryGuide = "Absolute outer reach uses edits.range with both min_percent and max_percent; never put endpoints in controls. Changing only the shorter strokes preserves the widest strokes: keep stroke_width.max_percent at the current outer width.\nGeometry full_slider_and_tip/full_slider_and_base mix full-DEVICE 0..100 sweeps with shorter strokes at that end, replacing an old region. Use these for full-device mixtures. full_band_and_tip/full_band_and_base instead preserve the current outer endpoints. All preserve pace and allow stroke_width overrides for custom coverage."

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
