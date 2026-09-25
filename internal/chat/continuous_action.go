package chat

import (
	"encoding/json"
	"errors"
	"slices"

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
	required := append(slices.Clone(root.Required), "action")
	if state.Autopilot {
		// Required, and named to follow the reply: as an optional field the
		// model usually skipped it, and asked before the reply it confused
		// wanting the motion to go on with wanting it frozen.
		required = append(required, "stay_unchanged")
	}
	branches := make([]any, 0, len(actions))
	for _, action := range actions {
		fields := make(map[string]json.RawMessage, len(root.Properties)+2)
		for key, value := range root.Properties {
			fields[key] = value
		}
		fields["action"], _ = json.Marshal(map[string]any{"const": action})
		if state.Autopilot {
			fields["stay_unchanged"] = json.RawMessage(`{"type":"boolean"}`)
		}
		if action == MotionActionNone {
			var empty any = map[string]any{}
			if state.MotionMode == MotionModeCreativeV2 {
				empty = []any{}
			}
			fields["edits"], _ = json.Marshal(map[string]any{"const": empty})
		}
		branches = append(branches, map[string]any{"type": "object", "properties": fields, "required": required, "additionalProperties": false})
	}
	encoded, _ := json.Marshal(map[string]any{"oneOf": branches})
	return encoded
}

const continuousActionGuide = `LIVE CHAT DECISION: Include top-level "action" before "edits" and "reply".
Choose "none" for conversation, questions, feedback, or keeping the current motion; emit empty edits. Choose "update" to carry out a motion edit while running, including a change you decide on in answer to how the motion feels or how close they say they are, such as slowing down when it is too much; a reply that promises a change without the edit changes nothing. Choose "start" only when the user asks to begin movement from stopped, including starting with the unchanged current settings. Adjusting settings while stopped does not authorize starting. While paused, choose "none" and explain that motion must first be resumed using the app.
Interpret the whole request in context. A request can both change a control and ask for an explanation. A restriction on one control does not cancel an edit to another. Preserve controls the user wants kept; change only what they ask for. Timing within a stroke and the speed of a variation layer are separate from overall pace. Describe the edit actually emitted; do not claim an edit when action is none.`

// continuousKeepGuide is offered only while continuous Autopilot composes the
// motion, the one time a standing wish has any effect.
const continuousKeepGuide = `
AUTOPILOT WISH: "autopilot" in the current state means Autopilot keeps composing the motion between chat turns. After the reply, set "stay_unchanged" true only when the user wants the motion to stay exactly as it is from now on, with no further changes; it can follow an edit, such as slowing down and then staying there. Otherwise set it false: enjoying the motion or wanting it to go on is not a request to stop varying it. While "autopilot" says it is holding at the user's request, answer true until the user's latest words ask for change, variety or surprise, or hand the lead back to you.`

func continuousActionGuideFor(state MotionContext) string {
	if state.Autopilot {
		return continuousActionGuide + continuousKeepGuide
	}
	return continuousActionGuide
}

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
