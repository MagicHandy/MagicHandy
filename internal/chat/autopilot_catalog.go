package chat

import (
	"encoding/json"
)

// Catalog labels remain shared data; complete examples belong to the caller's
// response contract. Interactive reply/start examples cannot enter a motion-
// only Autopilot prompt that requires intent and permits only target/none.
func autopilotCatalog(patterns []PatternChoice, kind AutopilotKind) string {
	if len(patterns) == 0 {
		return "No enabled catalog entries. Hold motion or change only the current pace; omit pattern_id."
	}
	text := curationCatalog(patterns, false)
	response := map[string]any{"motion": map[string]any{"action": MotionActionTarget, "pattern_id": modelPatternID(patterns[0].ID), "speed_percent": 25}, "next": "normal", "variability": "normal"}
	if kind == AutopilotKindMotion {
		response["intent"] = "A different movement shape at the current pace."
	} else {
		response["reply"] = "Changing the movement shape."
	}
	encoded, _ := json.Marshal(response)
	return text + "\nComplete response example for this autonomous contract: " + string(encoded)
}

func autopilotPatternSchema(patterns []PatternChoice, capabilities Capabilities, state *MotionContext, kind AutopilotKind) json.RawMessage {
	base := PatternResponseSchema(patterns, capabilities, state)
	if len(base) == 0 {
		return nil
	}
	var schema map[string]any
	_ = json.Unmarshal(base, &schema)
	fields := schema["properties"].(map[string]any)
	delete(fields, "new_mood")
	fields["next"] = map[string]any{"type": "string", "enum": []string{"soon", "normal", "later"}}
	fields["variability"] = map[string]any{"type": "string", "enum": []string{"settled", "normal", "restless"}}
	if kind == AutopilotKindMotion {
		delete(fields, "reply")
		fields["intent"] = map[string]any{"type": "string"}
		schema["required"] = []string{"intent", "next", "variability"}
	} else {
		schema["required"] = []string{"reply", "next"}
	}
	motionProperties := fields["motion"].(map[string]any)["properties"].(map[string]any)
	motionProperties["action"] = map[string]any{"type": "string", "enum": []string{MotionActionNone, MotionActionTarget}}
	encoded, _ := json.Marshal(schema)
	return encoded
}
