package chat

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// continuousActionSchema constrains decisions using backend state, never words
// in a chat message. The model interprets intent; the engine enforces limits.
func continuousActionSchema(schema json.RawMessage, state MotionContext, allowStop bool) json.RawMessage {
	var root struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if json.Unmarshal(schema, &root) != nil || root.Properties == nil {
		return schema
	}
	actions := []string{MotionActionNone}
	if !state.Paused && !unavailableVibrationOnly(state) {
		if state.Running {
			actions = append(actions, MotionActionUpdate)
		} else {
			actions = append(actions, MotionActionStart)
		}
	}
	if allowStop && !unavailableVibrationOnly(state) {
		actions = append(actions, MotionActionStop)
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
		if action == MotionActionNone || action == MotionActionStop {
			var empty any = map[string]any{}
			var editShape struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(root.Properties["edits"], &editShape)
			if state.MotionMode == MotionModeCreativeV2 || editShape.Type == "array" {
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

// liveActionGuide is the live chat action section. Hosted advanced motion also
// reads requests that name a later cue (ADR 0066) and keeps a scene's reply and
// motion in step.
func liveActionGuide(capabilities Capabilities, current MotionContext) string {
	action := continuousActionGuideFor(current)
	if capabilities.AdvancedMotion && !unavailableVibrationOnly(current) {
		action += "\n" + advancedStopGuide + "\n" + hostedLiveIntentGuide + "\n" + hostedSceneGuide
	}
	return action
}

const advancedStopGuide = `Use action "stop" with empty edits to stop all playback, including Autopilot. It is also allowed while paused; it never resumes motion. Omit vibration or use null with stop. Easing off while continuing uses update instead. Claim a complete rest or stopped device only with stop, or when already stopped.`

// hostedSceneGuide keeps a roleplay's reply and motion in step. Hosted models
// read reactions as conversation: in synthetic persona scenes the motion stayed
// unchanged through invitations, requests for more and winding down, while
// replies described touch starting, quickening or resting.
const hostedSceneGuide = `In a scene, the reply and the motion are one act. The person's reactions are motion feedback even when they name no control: an invitation to begin, pleasure, asking for more, saying they are close, finishing, or wanting to rest. Answer them in character with the motion as well as the words. Praise usually means keep what works; once they finish or need to catch their breath, ease off or stop unless they ask for more. Holding them off, telling them to wait or not yet, means slower or lighter motion, not faster. Whenever the reply says the stroking starts, stops or rests, speeds up or slows down, goes deeper or shallower, or changes length, make that change in this turn's edits. Never describe resting, holding still or catching breath while the motion keeps its pace, and when you keep the motion as it is, do not describe it changing. Kissing or touching elsewhere is not the motion.`

// hostedLiveIntentGuide covers two readings a hosted model missed in a live
// session. Asked to build toward the edge now and to slow down on a later cue,
// GPT-6.1 Terra changed nothing or slowed down in half of eight replays: it
// treated the whole message as prospective, or read the edge as the tip.
const hostedLiveIntentGuide = `A message can pair something to do now with something to do on a later cue: encode the part for now in this turn, and let the cued part wait for its cue. Being close, or at the edge, describes how near the person is to climax, not a slider position; a request to bring them there asks you to build intensity now.`

// continuousKeepGuide is offered only while continuous Autopilot composes the
// motion, the one time a standing wish has any effect.
const continuousKeepGuide = `
AUTOPILOT WISH: After reply, set "stay_unchanged" from the HUMAN'S intent, not your reply. It controls FUTURE autonomous edits, independently of this turn's action: "none" can leave future variation enabled. Use false unless the person asks to freeze the exact score indefinitely, possibly after this edit. Enjoying motion or wanting it to continue does not freeze Autopilot. Preserve an existing user-requested hold until they ask for change, variety or surprise, or return the lead to you.`

func continuousActionGuideFor(state MotionContext) string {
	if unavailableVibrationOnly(state) {
		return "LIVE CHAT DECISION: Use action none and empty edits. The only selected output is a vibrator that is not connected/selected or has zero user intensity. Explain which setting needs attention; do not claim to have started it. The user must connect/select it and set a nonzero intensity in the app."
	}
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
	if response.continuousAction == MotionActionStop {
		return s.authorizeContinuousStop(response, changed)
	}
	if unavailableVibrationOnly(state) {
		return requireUnchangedContinuous(response, changed, "the selected vibrator is not ready or has zero intensity; motion edits cannot arm it")
	}
	if state.Paused {
		return requireUnchangedContinuous(response, changed, "continuous motion is paused; edits cannot resume it")
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

func requireUnchangedContinuous(response *AssistantResponse, changed []string, reason string) error {
	if len(changed) > 0 || (response.continuousAction != "" && response.continuousAction != MotionActionNone) {
		return errors.New(reason)
	}
	return nil
}

func (s Service) authorizeContinuousStop(response *AssistantResponse, changed []string) error {
	if !s.capabilities().AdvancedMotion || s.TrustedMotionInput {
		return errors.New("action stop is available only to Advanced live chat")
	}
	if len(changed) != 0 {
		return errors.New("action stop requires unchanged motion; use empty edits")
	}
	response.Motion = &MotionCommand{Action: MotionActionStop}
	return nil
}
