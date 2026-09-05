package chat

import (
	"encoding/json"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// LLMLabSchema constrains syntax, field names and numeric ranges, not intent.
// The same strict semantic parser still runs for schema-guided and plain JSON.
func LLMLabSchema(method string, limits config.MotionSettings) json.RawMessage {
	if method == "creative_v2" {
		return CreativeV2ResponseSchema(limits, false)
	}
	if method == "layered" {
		return LayeredResponseSchema(limits, false)
	}
	if strings.HasPrefix(method, "library") {
		return libraryLabSchema(method, limits)
	}
	integer := func(minimum, maximum int) map[string]any {
		return map[string]any{"type": "integer", "minimum": minimum, "maximum": maximum}
	}
	object := func(properties map[string]any, required []string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	speed := integer(limits.SpeedMinPercent, limits.SpeedMaxPercent)
	controls := object(map[string]any{
		"min_percent": integer(0, 90), "max_percent": integer(10, 100), "speed_percent": speed,
		"range_floor_percent": integer(10, 100), "range_ceiling_percent": integer(0, 100), "anchor_percent": integer(0, 100), "memory_cycles": integer(2, 32), "pace_variation_percent": integer(0, 40),
		"variation_mode":        map[string]any{"type": "string", "enum": []string{"waves", "drift"}},
		"turn_softness_percent": integer(0, 100), "cadence_hold_percent": integer(0, 100),
	}, []string{})
	// Some local grammar runtimes reject large bounded-string productions.
	// String length stays strictly bounded by the parser, not the grammar.
	properties := map[string]any{"reply": map[string]any{"type": "string"}, "controls": controls}
	if method == "edits" {
		adjust := map[string]any{}
		for _, key := range []string{"min_percent", "max_percent", "speed_percent", "range_floor_percent", "range_ceiling_percent", "anchor_percent", "memory_cycles", "pace_variation_percent", "turn_softness_percent", "cadence_hold_percent"} {
			adjust[key] = integer(-100, 100)
		}
		properties["change_by"] = object(adjust, []string{})
		properties["remove_layers"] = map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"range", "center", "pace"}}, "maxItems": 3}
	}
	if method == "sequence" {
		step := object(map[string]any{"min_percent": integer(0, 90), "max_percent": integer(10, 100), "speed_percent": speed, "cycles": integer(2, 12)}, []string{"min_percent", "max_percent", "speed_percent", "cycles"})
		properties["steps"] = map[string]any{"type": "array", "items": step, "maxItems": 4}
	}
	if method == "layers" || method == "edits" {
		layer := object(map[string]any{"axis": map[string]any{"type": "string", "enum": []string{"range", "center", "pace"}}, "amount_percent": integer(0, 100), "period_cycles": integer(2, 32), "phase_percent": integer(0, 100)}, []string{"axis", "amount_percent", "period_cycles", "phase_percent"})
		properties["layers"] = map[string]any{"type": "array", "items": layer, "maxItems": 3}
	}
	encoded, _ := json.Marshal(object(properties, []string{"reply"}))
	return encoded
}
