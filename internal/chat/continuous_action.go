package chat

import (
	"encoding/json"
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// continuousActionSchema constrains decisions using backend state, never words
// in a chat message. The model interprets intent; the engine enforces limits.
func continuousActionSchema(schema json.RawMessage, state MotionContext) json.RawMessage {
	var root struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if json.Unmarshal(schema, &root) != nil || root.Properties == nil {
		return schema
	}
	actions := []string{MotionActionNone}
	if !state.Paused {
		if state.Running {
			actions = append(actions, MotionActionUpdate)
		} else {
			actions = append(actions, MotionActionStart)
		}
	}
	branches := make([]any, 0, len(actions))
	for _, action := range actions {
		fields := make(map[string]json.RawMessage, len(root.Properties)+1)
		for key, value := range root.Properties {
			fields[key] = value
		}
		fields["action"], _ = json.Marshal(map[string]any{"const": action})
		if action == MotionActionNone {
			var empty any = map[string]any{}
			if state.MotionMode == MotionModeCreativeV2 {
				empty = []any{}
			}
			fields["edits"], _ = json.Marshal(map[string]any{"const": empty})
		}
		branches = append(branches, map[string]any{"type": "object", "properties": fields, "required": append(root.Required, "action"), "additionalProperties": false})
	}
	encoded, _ := json.Marshal(map[string]any{"oneOf": branches})
	return encoded
}

const continuousActionGuide = `LIVE CHAT DECISION: Include top-level "action" before "edits" and "reply".
Choose "none" for conversation, questions, feedback, or keeping the current motion; emit empty edits. Choose "update" to carry out a motion edit while running. Choose "start" only when the user asks to begin movement from stopped, including starting with the unchanged current settings. Adjusting settings while stopped does not authorize starting. While paused, choose "none" and explain that motion must first be resumed using the app.
Interpret the whole request in context. A request can both change a control and ask for an explanation. A restriction on one control does not cancel an edit to another. Preserve controls the user wants kept; change only what they ask for. Timing within a stroke and the speed of a variation layer are separate from overall pace. Describe the edit actually emitted; do not claim an edit when action is none.`

func (s Service) authorizeLayeredReply(response *AssistantResponse, after motion.FlowSpec, changed []string, state MotionContext) error {
	if !s.capabilities().Motion {
		return nil
	}
	if state.Paused {
		if len(changed) > 0 || (response.continuousAction != "" && response.continuousAction != MotionActionNone) {
			return errors.New("continuous motion is paused; edits cannot resume it")
		}
		return nil
	}
	action := response.continuousAction
	// Scheduled decisions already have explicit authority from the scheduler.
	// Keep their compact edit contract; live user chat must declare its action.
	if s.TrustedMotionInput && action == "" {
		action = MotionActionUpdate
		if !state.Running {
			action = MotionActionStart
		}
	}
	switch action {
	case MotionActionNone:
		if len(changed) != 0 {
			return errors.New("action none requires unchanged motion")
		}
		return nil
	case MotionActionStart:
		if state.Running {
			return errors.New("motion is already running; use update or none")
		}
	case MotionActionUpdate:
		if !state.Running {
			return errors.New("motion is stopped; an update cannot start it")
		}
		if len(changed) == 0 {
			return nil
		}
	default:
		return errors.New("continuous chat requires action none, start or update")
	}
	response.Motion = &MotionCommand{Action: action, Layered: motion.CloneFlowSpec(&after)}
	return nil
}
