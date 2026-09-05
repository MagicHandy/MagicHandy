package chat

import "encoding/json"

// PatternResponseSchema constrains the compact library action vocabulary.
// Intent, lifecycle authority and actual motion still pass normal validation.
// It never supplies a missing action or selects a pattern.
func PatternResponseSchema(patterns []PatternChoice, capabilities Capabilities, state *MotionContext) json.RawMessage {
	if !capabilities.Motion || capabilities.MotionMode == MotionModeDynamic || !capabilities.Patterns {
		return nil
	}
	minimum, maximum := 1, 100
	if state != nil && state.SpeedMinPercent > 0 && state.SpeedMaxPercent >= state.SpeedMinPercent {
		minimum, maximum = state.SpeedMinPercent, state.SpeedMaxPercent
	}
	object := func(properties map[string]any, required []string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	motionFields := map[string]any{
		"action":        map[string]any{"type": "string", "enum": []string{MotionActionNone, MotionActionStart, MotionActionTarget, MotionActionStop}},
		"speed_percent": map[string]any{"type": "integer", "minimum": minimum, "maximum": maximum},
	}
	ids := []string{}
	for _, pattern := range patterns {
		if id := modelPatternID(pattern.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		motionFields["pattern_id"] = map[string]any{"type": "string", "enum": ids}
	}
	if capabilities.AreaFocus {
		motionFields["area"] = map[string]any{"type": "string", "enum": AreaZones()}
	}
	fields := map[string]any{"reply": map[string]any{"type": "string"}, "motion": object(motionFields, []string{"action"})}
	if capabilities.MoodTracking {
		fields["new_mood"] = map[string]any{"type": "string", "enum": Moods()}
	}
	encoded, _ := json.Marshal(object(fields, []string{"reply"}))
	return encoded
}
