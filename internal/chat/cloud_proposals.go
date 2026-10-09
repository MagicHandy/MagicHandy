package chat

import (
	"encoding/json"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// TechnicalAutopilotSchema shares the existing semantic vocabulary without
// composing a persona, memories, or conversation into the cloud request.
func TechnicalAutopilotSchema(patterns []PatternChoice, capabilities Capabilities, state *MotionContext) json.RawMessage {
	if capabilities.MotionMode == MotionModeDynamic {
		return technicalDynamicSchema(state)
	}
	return autopilotPatternSchema(patterns, capabilities, state, AutopilotKindMotion)
}

func technicalDynamicSchema(state *MotionContext) json.RawMessage {
	integer := func(lo, hi int) any { return map[string]any{"type": "integer", "minimum": lo, "maximum": hi} }
	object := func(properties map[string]any, required []string) any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	minimum, maximum := 1, 100
	if state != nil {
		minimum, maximum = state.SpeedMinPercent, state.SpeedMaxPercent
	}
	fields := map[string]any{
		"action": map[string]any{"type": "string", "enum": []string{"update", "none"}}, "speed_percent": integer(minimum, maximum),
		"center_percent": integer(0, 100), "span_percent": integer(10, 100), "span_min_percent": integer(10, 100),
		"span_profile": map[string]any{"type": "string", "enum": DynamicSpanProfiles()}, "variation_percent": integer(0, 100), "segment_seconds": integer(4, 120),
		"anchors": map[string]any{"type": "array", "minItems": 2, "maxItems": 6, "items": map[string]any{"type": "string", "enum": DynamicAnchorNames()}},
	}
	schema := object(map[string]any{"intent": map[string]any{"type": "string"}, "motion": object(fields, []string{"action"}), "next": map[string]any{"type": "string", "enum": []string{"normal"}}, "variability": map[string]any{"type": "string", "enum": []string{"settled"}}}, []string{"intent", "motion", "next", "variability"})
	encoded, _ := json.Marshal(schema)
	return encoded
}

// ParseTechnicalAutopilot uses production validation, without repair/fallback.
func ParseTechnicalAutopilot(raw string, patterns []PatternChoice, capabilities Capabilities, state *MotionContext) (AutopilotResponse, error) {
	service := AutopilotService{Patterns: patterns, Capabilities: capabilities, MotionContext: state}
	response, err := service.parse(raw, AutopilotKindMotion)
	if err != nil {
		return AutopilotResponse{}, &llm.CloudError{Kind: "incomplete"}
	}
	return response, nil
}
