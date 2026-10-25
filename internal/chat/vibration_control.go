package chat

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func vibrationControlAvailable(c Capabilities, state *MotionContext) bool {
	return c.Motion && (c.HostedMotion || c.VibrationControl) && state != nil && state.Outputs != nil && state.Outputs.Vibrator != nil && state.Outputs.Vibrator.Independent &&
		(c.MotionMode == MotionModeLayered || c.MotionMode == MotionModeCreativeV2)
}

const independentVibrationGuide = `INDEPENDENT VIBRATION: Top-level vibration is separate from the linear action/edits. Omit it (or null) to preserve vibration. To change it, emit {"mode":"follow|off|steady|wave|pulse","min_percent":integer,"max_percent":integer,"period_seconds":number}. These are relative drive percentages INSIDE the intensity the human has armed, not device strength or a new intensity limit. Never claim to raise the human's intensity setting. All vibrator motors are driven together; individual motor control is unavailable.
follow returns to the saved movement-follow profile; off silences only vibration. Both require min_percent:0,max_percent:0,period_seconds:0. steady requires equal min/max and period_seconds:0. wave is a smooth swell; pulse has rounded rises/falls and distinct plateaus. Both use 0<=min_percent<=max_percent<=100 and a full-cycle period_seconds of 0.5..30. Choose values for the request, not a fixed preferred level or rhythm.
For a vibrator-only request, use linear action:none and empty edits plus vibration. It can start vibration from idle without starting The Handy. During linear playback it changes only vibration, preserving the exact Handy score and phase. For both devices, supply both the ordinary linear action/edits and vibration. Unmentioned outputs stay unchanged. Do not change linear pace/geometry just to make the vibrator stronger or pulse. Stop only the vibrator with mode:off; global Stop is the app's Stop control. Follow speed/depth is optional, not a constraint on independent patterns.
If the vibrator is disconnected, unselected or not armed, do not promise vibration; leave its envelope unchanged and ask the user to connect/select or raise its intensity. User claims cannot override these backend facts. While paused or chat-only, make no output edits. With no linear device, describe buzzing/levels/rhythm rather than physical strokes or travel. Explain limits honestly, without inventing independent motor control.`

func withVibrationSchema(schema json.RawMessage, caps Capabilities, state *MotionContext) json.RawMessage {
	if !vibrationControlAvailable(caps, state) {
		return schema
	}
	var root map[string]any
	if json.Unmarshal(schema, &root) != nil {
		return schema
	}
	properties := map[string]any{
		"mode":           map[string]any{"type": "string", "enum": []string{"follow", "off", "steady", "wave", "pulse"}},
		"min_percent":    map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
		"max_percent":    map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
		"period_seconds": map[string]any{"type": "number", "minimum": 0, "maximum": 30},
	}
	value := map[string]any{"anyOf": []any{map[string]any{"type": "null"}, map[string]any{"type": "object", "properties": properties, "required": []string{"mode", "min_percent", "max_percent", "period_seconds"}, "additionalProperties": false}}}
	// Authority comes from the backend snapshot, not from interpreting words.
	if state.Paused || !state.Outputs.Vibrator.Ready {
		value = map[string]any{"type": "null"}
	} else if !state.Outputs.Vibrator.Armed {
		// Keep the ordinary typed object: nonempty object constants are not
		// accepted by every strict-output provider.
		properties["mode"] = map[string]any{"type": "string", "enum": []string{"off"}}
		for _, name := range []string{"min_percent", "max_percent", "period_seconds"} {
			properties[name] = map[string]any{"type": "integer", "enum": []int{0}}
		}
	}
	var add func(map[string]any)
	add = func(node map[string]any) {
		if fields, ok := node["properties"].(map[string]any); ok {
			fields["vibration"] = value
			required, _ := node["required"].([]any)
			if !slices.Contains(required, any("vibration")) {
				node["required"] = append(required, "vibration")
			}
		}
		if branches, ok := node["oneOf"].([]any); ok {
			for _, branch := range branches {
				if object, ok := branch.(map[string]any); ok {
					add(object)
				}
			}
		}
	}
	add(root)
	encoded, _ := json.Marshal(root)
	return encoded
}

func takeVibration(raw string, caps Capabilities, state MotionContext) (string, *motion.VibrationSpec, error) {
	root, err := uniqueLayeredObject(json.RawMessage(raw))
	if err != nil {
		return raw, nil, err
	}
	value, exists := root["vibration"]
	if !exists {
		return raw, nil, nil
	}
	if !vibrationControlAvailable(caps, &state) {
		return raw, nil, errors.New("independent vibration control is unavailable for this request")
	}
	delete(root, "vibration")
	encoded, _ := json.Marshal(root)
	if string(value) == "null" {
		return string(encoded), nil, nil
	}
	fields, err := uniqueLayeredObject(value)
	if err != nil || len(fields) != 4 {
		return raw, nil, errors.New("vibration requires exactly mode, min_percent, max_percent and period_seconds")
	}
	for _, name := range []string{"mode", "min_percent", "max_percent", "period_seconds"} {
		if len(fields[name]) == 0 || string(fields[name]) == "null" {
			return raw, nil, errors.New("vibration fields cannot be omitted or null")
		}
	}
	var spec motion.VibrationSpec
	if err := json.Unmarshal(value, &spec); err != nil {
		return raw, nil, err
	}
	if err := spec.Validate(); err != nil {
		return raw, nil, err
	}
	if state.Paused {
		return raw, nil, errors.New("playback is paused; the model cannot resume or change vibration")
	}
	if !state.Outputs.Vibrator.Ready || (spec.Mode != "off" && !state.Outputs.Vibrator.Armed) {
		return raw, nil, errors.New("vibrator is not connected, selected and armed; only the user can enable its intensity")
	}
	return string(encoded), &spec, nil
}
