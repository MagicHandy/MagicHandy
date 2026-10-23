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
Use "none" with empty edits for chat/questions or explicitly prospective/settings-only requests. Use "update" for edits or motion feedback while running. While stopped, a fresh motion request uses "start", even without saying "start" or after an earlier Stop. While paused, use "none"; only the app can resume.
Interpret the whole request: a question can accompany an edit, and preserving one control does not forbid changing another. Preserve unrequested controls. Stroke timing, layer period and overall pace are separate. Describe only emitted edits; words alone change nothing.`

// continuousKeepGuide is offered only while continuous Autopilot composes the
// motion, the one time a standing wish has any effect.
const continuousKeepGuide = `
AUTOPILOT WISH: After reply, set "stay_unchanged" from the HUMAN'S intent, not your reply. It controls FUTURE autonomous edits, independently of this turn's action: "none" can leave future variation enabled. Use false unless the person asks to freeze the exact score indefinitely, possibly after this edit. Enjoying motion or wanting it to continue does not freeze Autopilot. Preserve an existing user-requested hold until they ask for change, variety or surprise, or return the lead to you.`

func continuousActionGuideFor(state MotionContext) string {
	if state.Autopilot {
		guide := continuousActionGuide + continuousKeepGuide
		if state.StandingHold {
			return guide + ` Autopilot is ALREADY HELD at the person's request. Neutral feedback or continuing playback keeps stay_unchanged:true; only permission to change releases it.`
		}
		return guide + ` Autopilot is currently COMPOSING, with no standing hold. Positive feedback or "keep going" leaves stay_unchanged:false: continuing playback is not freezing future decisions.`
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
