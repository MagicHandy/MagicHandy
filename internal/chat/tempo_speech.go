package chat

import (
	"encoding/json"
	"errors"
	"strings"
)

// Spoken check-ins keep their independent conversation/quiet decision while
// sharing the native output vocabulary. They must never fall through to the
// position contract just because their primary output is speech.
func (s AutopilotService) tempoSpeechContract(tier tempoTier) (string, json.RawMessage) {
	guide := tempoDeviceGuide(tier, s.MotionContext)
	if !s.Capabilities.Motion {
		guide = "The VacuGlide makes its own strokes. Its motor tempo controls speed, and the air sets stroke length and tightness. This check-in cannot change motion or air: tempo and air_steps must both be null. Do not claim a change."
	}
	contract := `RESPONSE CONTRACT: Reply with one JSON object: {"intent":string,"reply":string,"tempo":object or null,"air_steps":integer or null,"next":"soon"|"normal"|"later"}.
` + autopilotSpeechDecisionContract + "\n" + guide + "\nAn empty reply requires tempo null, air_steps null and next later."
	var root map[string]any
	_ = json.Unmarshal(tempoAutopilotSchema(tier, s.MotionContext, false, 0), &root)
	properties := root["properties"].(map[string]any)
	properties["reply"] = map[string]any{"type": "string"}
	if !s.Capabilities.Motion {
		properties["tempo"], properties["air_steps"] = map[string]any{"type": "null"}, map[string]any{"type": "null"}
	}
	root["required"] = []string{"intent", "reply", "tempo", "air_steps", "next"}
	schema, _ := json.Marshal(root)
	return contract, orderedTempoSchema(schema, tier)
}

func parseTempoSpeech(raw string, tier tempoTier, state *MotionContext, motionAllowed bool) (AutopilotResponse, error) {
	response, err := parseTempoAutopilot(raw, tier, state)
	if err != nil {
		return response, err
	}
	root, _ := uniqueLayeredObject(json.RawMessage(strings.TrimSpace(raw)))
	var reply *string
	if err := json.Unmarshal(root["reply"], &reply); err != nil || reply == nil {
		return response, errors.New("the spoken check-in needs reply, or an empty reply to stay quiet")
	}
	response.Reply = strings.TrimSpace(*reply)
	if response.Reply == "" && (*reply != "" || strings.TrimSpace(response.Intent) == "" || response.Motion != nil || response.Next != AutopilotTimingLater) {
		return response, errors.New("a quiet check-in requires empty reply, later timing and no motion")
	}
	if !motionAllowed && response.Motion != nil {
		return response, errors.New("this check-in cannot change motion; tempo and air_steps must be null")
	}
	return response, nil
}
