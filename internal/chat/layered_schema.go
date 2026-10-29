package chat

import (
	"encoding/json"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// LayeredResponseSchema shares one compact contract between live chat and Labs.
func LayeredResponseSchema(limits config.MotionSettings, mood bool) json.RawMessage {
	return layeredResponseSchema(limits, mood, false)
}

func layeredResponseSchema(limits config.MotionSettings, mood, operations bool) json.RawMessage {
	integer := func(lo, hi int) map[string]any {
		return map[string]any{"type": "integer", "minimum": lo, "maximum": hi}
	}
	object := func(properties map[string]any, required []string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	axis := map[string]any{"type": "string", "enum": []string{"range", "center", "pace"}}
	controls := map[string]any{
		"min_percent": integer(0, 90), "max_percent": integer(10, 100), "speed_percent": integer(limits.SpeedMinPercent, limits.SpeedMaxPercent),
		"anchor_percent": integer(0, 100), "memory_cycles": integer(2, 32), "pace_variation_percent": integer(0, 40),
		"variation_mode": map[string]any{"type": "string", "enum": []string{"drift", "waves"}},
	}
	deltas := map[string]any{}
	for _, name := range layeredControlNames {
		if name != "variation_mode" {
			deltas[name] = integer(-100, 100)
		}
	}
	layer := object(map[string]any{"axis": axis, "amount_percent": integer(0, 100), "period_cycles": integer(2, 32), "phase_percent": integer(0, 100), "shape": map[string]any{"type": "string", "enum": []string{"drift", "alternate", "wave"}}}, []string{"axis"})
	layer["properties"].(map[string]any)["period_change_cycles"] = integer(-30, 30)
	if operations {
		for name, absolute := range controls {
			controls[name] = layeredOperationSchema(absolute, deltas[name])
		}
		periods := layer["properties"].(map[string]any)
		periods["period_cycles"] = layeredOperationSchema(periods["period_cycles"], periods["period_change_cycles"])
		delete(periods, "period_change_cycles")
	}
	edits := map[string]any{"controls": object(controls, []string{}), "change_by": object(deltas, []string{}), "layers": map[string]any{"type": "array", "items": layer, "maxItems": 3}, "remove_layers": map[string]any{"type": "array", "items": axis, "maxItems": 3}, "evolve": map[string]any{"type": "boolean"}}
	if operations {
		delete(edits, "change_by")
	}
	edits["stroke_width"] = object(map[string]any{"min_percent": integer(10, 100), "max_percent": integer(10, 100)}, []string{"min_percent", "max_percent"})
	edits["geometry"] = map[string]any{"type": "string", "enum": layeredGeometries}
	properties := map[string]any{"edits": object(edits, []string{}), "reply": map[string]any{"type": "string"}}
	if operations {
		edits["stroke_width"] = object(map[string]any{"shortest_percent": integer(10, 100), "widest_percent": integer(10, 100)}, []string{"shortest_percent", "widest_percent"})
		edits["layers"] = hostedLayeredLayersSchema(layer)
		delete(edits, "remove_layers")
		properties["edits"] = hostedLayeredEditSchema(edits)
	}
	if mood {
		properties["new_mood"] = map[string]any{"type": "string", "enum": Moods()}
	}
	encoded, _ := json.Marshal(object(properties, []string{"edits", "reply"}))
	return encoded
}

// Disjoint op constants make the strict provider's anyOf conversion exclusive.
// Absolute and relative values retain their separate bounds and types.
func layeredOperationSchema(absolute, relative any) map[string]any {
	branch := func(op string, value any) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false,
			"properties": map[string]any{"op": map[string]any{"const": op}, "value": value},
			"required":   []string{"op", "value"}}
	}
	if relative == nil {
		return branch("set", absolute)
	}
	return map[string]any{"oneOf": []any{branch("set", absolute), branch("adjust", relative)}}
}
