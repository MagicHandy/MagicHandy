package chat

import (
	"bytes"
	"encoding/json"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Local models do better when each choice has only the fields it uses. In
// particular, choosing a named routine must not also require inventing bounds
// and a period for an unrelated wave. Advanced providers retain their tested
// strict, nullable-field contract.
func localTempoSchema(tier tempoTier, state *MotionContext) map[string]any {
	integer := func(lo, hi int) map[string]any {
		return map[string]any{"type": "integer", "minimum": lo, "maximum": hi}
	}
	choice := func(mode string, fields map[string]any) map[string]any {
		properties := map[string]any{"mode": map[string]any{"type": "string", "enum": []string{mode}}}
		required := []string{"mode"}
		for _, name := range []string{"routine", "level_percent", "min_percent", "max_percent", "period_seconds"} {
			if field, ok := fields[name]; ok {
				properties[name] = field
				required = append(required, name)
			}
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	var routines []string
	for _, routine := range motion.TempoRoutines() {
		routines = append(routines, routine.ID)
	}
	choices := []any{
		map[string]any{"type": "null"},
		choice("steady", map[string]any{"level_percent": integer(1, 100)}),
		choice("pause", nil),
		choice("routine", map[string]any{"routine": map[string]any{"type": "string", "enum": routines}}),
	}
	if tier >= tempoIntermediate {
		choices = append(choices, choice("ramp", map[string]any{"level_percent": integer(0, 100), "period_seconds": integer(1, 600)}))
		for _, mode := range []string{"wave", "pulse"} {
			choices = append(choices, choice(mode, map[string]any{"min_percent": integer(0, 99), "max_percent": integer(1, 100), "period_seconds": integer(2, 120)}))
		}
	}
	if resumableTempo(state) != nil {
		choices = append(choices, choice("resume", nil))
	}
	return map[string]any{"anyOf": choices}
}

// Constrained local decoders follow property order. The discriminator must
// precede numeric fields: otherwise choosing "mode" first can make steady,
// ramp and wave branches unreachable while pause/routine remain valid.
func orderedTempoSchema(schema json.RawMessage, tier tempoTier) json.RawMessage {
	if tier == tempoAdvanced {
		return schema
	}
	var root any
	_ = json.Unmarshal(schema, &root)
	var order func(any) any
	order = func(value any) any {
		switch object := value.(type) {
		case []any:
			for i := range object {
				object[i] = order(object[i])
			}
		case map[string]any:
			for key, item := range object {
				object[key] = order(item)
			}
			properties, ok := object["properties"].(map[string]any)
			required, allRequired := object["required"].([]any)
			if ok && allRequired && len(properties) == len(required) {
				var out bytes.Buffer
				out.WriteByte('{')
				for i, name := range required {
					if i > 0 {
						out.WriteByte(',')
					}
					key, _ := json.Marshal(name)
					item, _ := json.Marshal(properties[name.(string)])
					out.Write(key)
					out.WriteByte(':')
					out.Write(item)
				}
				out.WriteByte('}')
				object["properties"] = json.RawMessage(out.Bytes())
			}
		}
		return value
	}
	encoded, _ := json.Marshal(order(root))
	return encoded
}
