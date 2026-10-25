package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Local Layered uses small, independent edit groups. The semantic score and
// validator are shared with hosted/legacy replies; no user text is interpreted
// here and no model-selected values are repaired or filled from a preset.
var intermediateLayeredGroups = []string{"range", "controls", "change_by", "geometry", "stroke_width", "layers", "remove_layers", "evolve", "recall"}

func normalizeIntermediateLayeredReply(raw string) (string, error) {
	root, err := uniqueLayeredObject(json.RawMessage(raw))
	if err != nil {
		return raw, err
	}
	value := root["edits"]
	if len(value) == 0 {
		return raw, nil
	}
	edits, err := intermediateEditGroups(value)
	if err != nil {
		return raw, err
	}
	if err := normalizeIntermediateReach(edits); err != nil {
		return raw, err
	}
	root["edits"], _ = json.Marshal(edits)
	encoded, _ := json.Marshal(root)
	return string(encoded), nil
}

func intermediateEditGroups(value json.RawMessage) (map[string]json.RawMessage, error) {
	edits := map[string]json.RawMessage{}
	if strings.HasPrefix(strings.TrimSpace(string(value)), "[") {
		var items []json.RawMessage
		if json.Unmarshal(value, &items) != nil || len(items) > len(intermediateLayeredGroups) {
			return nil, errors.New("layered edits must be a bounded list of independent groups")
		}
		for _, item := range items {
			fields, err := uniqueLayeredObject(item)
			if err != nil || len(fields) != 1 {
				return nil, errors.New("each Layered edit must name exactly one group")
			}
			for key, field := range fields {
				if !slices.Contains(intermediateLayeredGroups, key) || edits[key] != nil {
					return nil, fmt.Errorf("unsupported or repeated Layered edit group: %s", key)
				}
				edits[key] = field
			}
		}
		return edits, nil
	}
	return uniqueLayeredObject(value)
}

func normalizeIntermediateReach(edits map[string]json.RawMessage) error {
	controls := map[string]json.RawMessage{}
	var err error
	if edits["controls"] != nil {
		controls, err = uniqueLayeredObject(edits["controls"])
		if err != nil {
			return err
		}
	}
	if edits["range"] != nil {
		reach, err := uniqueLayeredObject(edits["range"])
		if err != nil || len(reach) != 2 {
			return errors.New("range requires both min_percent and max_percent")
		}
		for _, key := range []string{"min_percent", "max_percent"} {
			var number int
			if reach[key] == nil || string(reach[key]) == "null" || json.Unmarshal(reach[key], &number) != nil || controls[key] != nil {
				return errors.New("range requires two integer endpoints, without duplicate controls")
			}
			controls[key] = reach[key]
		}
		delete(edits, "range")
	}
	if err := normalizeIntermediateGeometry(edits, controls); err != nil {
		return err
	}
	if len(controls) > 0 {
		edits["controls"], _ = json.Marshal(controls)
	}
	return nil
}

func normalizeIntermediateGeometry(edits, controls map[string]json.RawMessage) error {
	var geometry string
	_ = json.Unmarshal(edits["geometry"], &geometry)
	switch geometry {
	case "full_slider_and_tip", "full_slider_and_base":
		var changes map[string]json.RawMessage
		_ = json.Unmarshal(edits["change_by"], &changes)
		for key, expected := range map[string]int{"min_percent": 0, "max_percent": 100} {
			if changes[key] != nil {
				return errors.New("full-slider geometry conflicts with relative endpoints")
			}
			if existing := controls[key]; existing != nil {
				var number int
				if string(existing) == "null" || json.Unmarshal(existing, &number) != nil || number != expected {
					return errors.New("outer range conflicts with full-slider geometry")
				}
			}
			controls[key], _ = json.Marshal(expected)
		}
		edits["geometry"], _ = json.Marshal(strings.Replace(geometry, "full_slider_", "full_", 1))
	case "full_band_and_tip", "full_band_and_base":
		edits["geometry"], _ = json.Marshal(strings.Replace(geometry, "full_band_", "full_", 1))
	}
	return nil
}

func intermediateLayeredScoreContext(score motion.FlowSpec) map[string]any {
	state := layeredScoreContext(score)
	controls := state["controls"].(map[string]any)
	state["range"] = map[string]any{"min_percent": controls["min_percent"], "max_percent": controls["max_percent"]}
	delete(controls, "min_percent")
	delete(controls, "max_percent")
	return state
}

// Convert the local grammar only. Hosted operations and their strict nullable
// schema stay unchanged. Repeated groups are rejected by the shared parser.
func intermediateLayeredSchema(schema json.RawMessage) json.RawMessage {
	var root map[string]any
	if json.Unmarshal(schema, &root) != nil {
		return schema
	}
	properties, _ := root["properties"].(map[string]any)
	edits, _ := properties["edits"].(map[string]any)
	fields, _ := edits["properties"].(map[string]any)
	controls, _ := fields["controls"].(map[string]any)
	controlFields, _ := controls["properties"].(map[string]any)
	if controlFields["min_percent"] == nil {
		return schema
	}
	fields["range"] = map[string]any{"type": "object", "properties": map[string]any{"min_percent": controlFields["min_percent"], "max_percent": controlFields["max_percent"]}, "required": []string{"min_percent", "max_percent"}, "additionalProperties": false}
	delete(controlFields, "min_percent")
	delete(controlFields, "max_percent")
	geometries := slices.Clone(layeredGeometries)
	for i, value := range geometries {
		if value == "full_and_tip" {
			geometries[i] = "full_band_and_tip"
		}
		if value == "full_and_base" {
			geometries[i] = "full_band_and_base"
		}
	}
	fields["geometry"] = map[string]any{"type": "string", "enum": append(geometries, "full_slider_and_tip", "full_slider_and_base")}
	choices := []any{}
	for _, name := range intermediateLayeredGroups {
		if field := fields[name]; field != nil {
			choices = append(choices, map[string]any{"type": "object", "properties": map[string]any{name: field}, "required": []string{name}, "additionalProperties": false})
		}
	}
	properties["edits"] = map[string]any{"type": "array", "minItems": 0, "maxItems": len(choices), "items": map[string]any{"anyOf": choices}}
	encoded, _ := json.Marshal(root)
	return encoded
}
