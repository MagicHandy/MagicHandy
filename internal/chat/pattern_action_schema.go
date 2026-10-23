package chat

// The interactive library contract follows the same lifecycle as continuous chat.
// A none/stop branch cannot accidentally carry a pattern selection or speed.
// Autonomous callers keep their separate target-selection grammar.
func interactivePatternActionSchema(targetFields map[string]any, state *MotionContext) map[string]any {
	choices := []any{}
	for _, action := range []string{MotionActionNone, MotionActionStop} {
		choices = append(choices, map[string]any{
			"type": "object", "properties": map[string]any{"action": map[string]any{"const": action}},
			"required": []string{"action"}, "additionalProperties": false,
		})
	}
	if state != nil && !state.Paused {
		action, required := MotionActionStart, []string{"action", "speed_percent"}
		if state.Running {
			action, required = MotionActionTarget, []string{"action"}
		}
		fields := make(map[string]any, len(targetFields))
		for key, value := range targetFields {
			fields[key] = value
		}
		fields["action"] = map[string]any{"const": action}
		choices = append(choices, map[string]any{
			"type": "object", "properties": fields, "required": required, "additionalProperties": false,
		})
	}
	return map[string]any{"oneOf": choices}
}
