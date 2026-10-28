package chat

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func vibrationControlAvailable(c Capabilities, state *MotionContext) bool {
	return c.Motion && (c.AdvancedMotion || c.VibrationControl) && state != nil && state.Outputs != nil && state.Outputs.Vibrator != nil && state.Outputs.Vibrator.Independent &&
		(c.MotionMode == MotionModeLayered || c.MotionMode == MotionModeCreativeV2)
}

const independentVibrationGuide = `INDEPENDENT VIBRATION: Top-level vibration is separate from the linear action/edits. Omit it (or null) to preserve vibration. To change it, emit {"mode":"follow|off|steady|wave|pulse","min_percent":integer,"max_percent":integer,"period_seconds":number}. Percentages are of the user's Max, the strongest the user allows; you choose the strength within it, and 100 is that Max. Changes apply exactly as written. Starting linear movement requires vibration for the start, usually follow with max_percent as its strength. Never claim to change the user's Max. All vibrator motors are driven together; individual motor control is unavailable.
follow tracks the movement through the saved profile, from min_percent while moving up to max_percent at its strongest, with period_seconds:0; all zeros follow at level_percent_of_max. off silences only vibration and requires all zeros. steady requires equal min/max and period_seconds:0. wave is a smooth swell; pulse has rounded rises/falls and distinct plateaus. Both use 0<=min_percent<=max_percent<=100 and a full-cycle period_seconds of 0.5..30. Choose values for the request, not a fixed preferred level or rhythm.
For a vibrator-only request, use linear action:none and empty edits plus vibration. It can start vibration from idle without starting The Handy. During linear playback it changes only vibration, preserving the exact Handy score and phase. For both devices, supply both the ordinary linear action/edits and vibration. Unmentioned outputs stay unchanged. Do not change linear pace/geometry just to make the vibrator stronger or pulse. Stop only the vibrator with mode:off; global Stop is the app's Stop control. Follow speed/depth is optional, not a constraint on independent patterns.
From idle, only steady/wave/pulse start independent vibration with action:none. follow needs a playing movement timeline: to resume its saved rhythm use action:start and empty edits, including when the vibrator is the only playback device. previous_envelope is the last stopped vibration pattern, not active output; reuse it when asked to continue unchanged. A positive level or follow object alone does not start an idle timeline.
While paused or chat-only, make no output edits. With no linear device, describe buzzing/levels/rhythm rather than physical strokes or travel. Explain limits honestly, without inventing independent motor control.`

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
	} else if !state.Outputs.Vibrator.Available() {
		// Keep the ordinary typed object: nonempty object constants are not
		// accepted by every strict-output provider.
		properties["mode"] = map[string]any{"type": "string", "enum": []string{"off"}}
		for _, name := range []string{"min_percent", "max_percent", "period_seconds"} {
			properties[name] = map[string]any{"type": "integer", "enum": []int{0}}
		}
	}
	// The model chooses the starting level: a start cannot omit vibration.
	addTopLevelProperty(root, "vibration", value, startValue(value))
	encoded, _ := json.Marshal(root)
	return encoded
}

// startValue drops null from a nullable field for the start branch, so the
// model chooses the starting level (ADR 0061). A null-only field stays null.
func startValue(value map[string]any) any {
	choices, ok := value["anyOf"].([]any)
	if !ok || len(choices) != 2 {
		return value
	}
	return choices[1]
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
	delete(root, "vibration")
	encoded, _ := json.Marshal(root)
	if string(value) == "null" {
		return string(encoded), nil, nil
	}
	// An explicit no-change is harmless even if the output disappeared or its
	// control was disabled. Non-null edits still require current authority.
	if !vibrationControlAvailable(caps, &state) {
		return raw, nil, errors.New("independent vibration control is unavailable for this request")
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
	if !state.Outputs.Vibrator.Ready || (spec.Mode != "off" && !state.Outputs.Vibrator.Available()) {
		return raw, nil, errors.New("the vibrator is not connected and selected, or the user's Stop paused it; only the user can resume it")
	}
	return string(encoded), &spec, nil
}

func vibrationLevelAvailable(c Capabilities, state *MotionContext) bool {
	// Simple Creative v2 is excluded by evaluation (ADR 0061), as in ModelFeatures.
	simpleCreative := simpleContinuous(c) && c.MotionMode == MotionModeCreativeV2
	return c.Motion && !c.AdvancedMotion && !simpleCreative && state != nil && state.Outputs != nil && state.Outputs.Vibrator != nil &&
		state.Outputs.Vibrator.LevelControl && state.Outputs.Vibrator.Ready &&
		(c.MotionMode == MotionModeLayered || c.MotionMode == MotionModeCreativeV2)
}

// withVibrationLevelSchema adds the Simple/Intermediate level beside the reply.
// It is nullable and required, like the Advanced envelope, so strict decoders
// accept it and a constrained model always decides it.
func withVibrationLevelSchema(schema json.RawMessage, caps Capabilities, state *MotionContext) json.RawMessage {
	if !vibrationLevelAvailable(caps, state) {
		return schema
	}
	var root map[string]any
	if json.Unmarshal(schema, &root) != nil {
		return schema
	}
	value := map[string]any{"anyOf": []any{map[string]any{"type": "null"}, map[string]any{"type": "integer", "minimum": 0, "maximum": 100}}}
	if state.Paused || !state.Outputs.Vibrator.Available() {
		value = map[string]any{"type": "null"}
	}
	addTopLevelProperty(root, "vibration_level", value, startValue(value))
	encoded, _ := json.Marshal(root)
	return encoded
}

// addTopLevelProperty requires a field on the object and on each action
// branch; the start branch takes start, which may require a choice.
func addTopLevelProperty(node map[string]any, name string, value, start any) {
	if fields, ok := node["properties"].(map[string]any); ok {
		chosen := value
		if action, ok := fields["action"].(map[string]any); ok && action["const"] == MotionActionStart {
			chosen = start
		}
		fields[name] = chosen
		required, _ := node["required"].([]any)
		if !slices.Contains(required, any(name)) {
			node["required"] = append(required, name)
		}
	}
	if branches, ok := node["oneOf"].([]any); ok {
		for _, branch := range branches {
			if object, ok := branch.(map[string]any); ok {
				addTopLevelProperty(object, name, value, start)
			}
		}
	}
}

// takeVibrationLevel removes the level from a reply before the motion parser
// sees it. A level the backend cannot apply is dropped rather than failing the
// whole reply: it never moves anything, and the prompt states the limit.
func takeVibrationLevel(raw string, caps Capabilities, state MotionContext) (string, *int, error) {
	root, err := uniqueLayeredObject(json.RawMessage(raw))
	if err != nil {
		return raw, nil, err
	}
	value, exists := root["vibration_level"]
	if !exists {
		return raw, nil, nil
	}
	delete(root, "vibration_level")
	encoded, _ := json.Marshal(root)
	if string(value) == "null" || !vibrationLevelAvailable(caps, &state) {
		return string(encoded), nil, nil
	}
	var level int
	if err := json.Unmarshal(value, &level); err != nil || level < 0 || level > 100 {
		return raw, nil, errors.New("vibration_level must be an integer from 0 to 100, or null")
	}
	if state.Paused {
		return raw, nil, errors.New("playback is paused; the model cannot change the vibration level")
	}
	if level > 0 && !state.Outputs.Vibrator.Available() {
		return string(encoded), nil, nil
	}
	return string(encoded), &level, nil
}
