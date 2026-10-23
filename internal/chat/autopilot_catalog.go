package chat

import (
	"encoding/json"
)

// Catalog labels remain shared data; complete examples belong to the caller's
// response contract. Interactive reply/start examples cannot enter a motion-
// only Autopilot prompt that requires intent and permits only target/none.
func autopilotCatalog(patterns []PatternChoice, kind AutopilotKind, autoTiming bool) string {
	if len(patterns) == 0 {
		return "No enabled catalog entries. Hold motion or change only the current pace; omit pattern_id."
	}
	text := curationCatalog(patterns, false)
	response := map[string]any{"motion": map[string]any{"action": MotionActionTarget, "pattern_id": modelPatternID(patterns[0].ID), "speed_percent": 25}, "next": "normal", "variability": "normal"}
	if autoTiming && kind == AutopilotKindMotion {
		// A number here would become the model's default horizon; the contract
		// and the strict schema already require next_check_seconds.
		delete(response, "next")
	}
	if kind == AutopilotKindMotion {
		response["intent"] = "A different movement shape at the current pace."
	} else {
		response["intent"] = "Describe the new movement shape"
		response["reply"] = "Changing the movement shape."
	}
	encoded, _ := json.Marshal(response)
	return text + "\nComplete response example for this autonomous contract: " + string(encoded)
}

func autopilotPatternSchema(patterns []PatternChoice, capabilities Capabilities, state *MotionContext, kind AutopilotKind) json.RawMessage {
	// Autonomous target selection keeps its separate contract and authority;
	// it must not inherit live chat's state-specific start/none branches.
	base := patternResponseSchema(patterns, capabilities, state, false)
	if len(base) == 0 {
		return nil
	}
	var schema map[string]any
	_ = json.Unmarshal(base, &schema)
	fields := schema["properties"].(map[string]any)
	delete(fields, "new_mood")
	fields["next"] = map[string]any{"type": "string", "enum": []string{"soon", "normal", "later"}}
	if capabilities.Motion || kind == AutopilotKindMotion {
		fields["variability"] = map[string]any{"type": "string", "enum": []string{"settled", "normal", "restless"}}
	}
	if kind == AutopilotKindMotion && capabilities.AutoTiming {
		delete(fields, "reply")
		delete(fields, "next")
		fields["intent"] = map[string]any{"type": "string"}
		fields[nextCheckField] = nextCheckSchema()
		schema["required"] = []string{"intent", nextCheckField, "variability"}
	} else if kind == AutopilotKindMotion {
		delete(fields, "reply")
		fields["intent"] = map[string]any{"type": "string"}
		schema["required"] = []string{"intent", "next", "variability"}
	} else {
		fields["intent"] = map[string]any{"type": "string"}
		schema["required"] = []string{"intent", "reply", "next"}
	}
	if capabilities.Motion {
		motionProperties := fields["motion"].(map[string]any)["properties"].(map[string]any)
		motionProperties["action"] = map[string]any{"type": "string", "enum": []string{MotionActionNone, MotionActionTarget}}
	}
	encoded, _ := json.Marshal(schema)
	return encoded
}
