package chat

import (
	"encoding/json"
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// OutputContext contains only backend-confirmed effect/participation facts.
// It cannot identify a physical device or authorize changing its limits.
type OutputContext struct {
	Strokers int               `json:"linear_devices"`
	Coyote   bool              `json:"estim_follows_motion"`
	Vibrator *VibrationContext `json:"vibrator,omitempty"`
}

// VibrationContext describes the selected vibrator and the model's control of
// it. Levels are percentages of the user's Max, which the model never changes
// (ADR 0061). Independent is the Advanced envelope; LevelControl is the
// Simple/Intermediate level.
type VibrationContext struct {
	ResumeOnMotionRequest bool `json:"manual_request_can_resume_after_stop,omitempty"`
	Independent           bool `json:"independent_control"`
	LevelControl          bool `json:"level_control,omitempty"`
	LevelPercent          int  `json:"level_percent_of_max"`
	// LevelFollowsHandy says no model chose a level yet, so vibration
	// follows The Handy's intensity.
	LevelFollowsHandy    bool                  `json:"level_follows_handy,omitempty"`
	PreviousLevelPercent int                   `json:"previous_level_percent,omitempty"`
	Current              *motion.VibrationSpec `json:"current_envelope,omitempty"`
	Previous             *motion.VibrationSpec `json:"previous_envelope,omitempty"`
	Follow               string                `json:"follows"`
	DepthEnd             string                `json:"strongest_at,omitempty"`
	Ready                bool                  `json:"connected_and_selected"`
	Stopped              bool                  `json:"paused_by_user_stop,omitempty"`
	Motors               int                   `json:"motors_driven_together"`
}

// Available reports whether the vibrator can sound for this request: it is
// connected and selected, and no Stop suspension stands against it.
func (v VibrationContext) Available() bool {
	return v.Ready && (!v.Stopped || v.ResumeOnMotionRequest)
}

// Only backend permission can lift a Stop suspension. Nil data preserves the
// generic/simulator contract; mixed sessions may still move their other
// participants when vibration is unavailable.
func unavailableVibrationOnly(state MotionContext) bool {
	outputs := state.Outputs
	return outputs != nil && outputs.Strokers == 0 && !outputs.Coyote &&
		outputs.Vibrator != nil && !outputs.Vibrator.Available()
}

const outputContextGuide = `The outputs share one movement timeline; there is no independent device or motor command. A vibrator follows that timeline, within the strength set by the user. Never claim to change its strength, the user's Max, profile, individual motors, or playback participation.
` + outputFollowGuide + `
With no linear device, describe vibration rather than physical strokes, travel, or penetration. The motion JSON remains a hidden control signal. With a linear device, any movement edit also changes that device: if asked to change vibration independently while preserving its movement, use no change and explain the limitation. Do not invent a separate vibration pattern or use Stop to silence only one participant.`

// levelOutputContextGuide replaces the shared guide when the model sets the
// level (ADR 0061): the strength is then the model's, and vibration needs
// movement.
const levelOutputContextGuide = `The outputs share one movement timeline; there is no separate vibration pattern or motor command. A vibrator plays only while that timeline plays, at the vibration_level you choose under the user's Max, so vibration from idle means starting movement. Never claim to change the user's Max, profile, individual motors, or playback participation.
` + outputFollowGuide + `
With no linear device, describe vibration rather than physical strokes, travel, or penetration. The motion JSON remains a hidden control signal. Do not use Stop to silence only one participant.`

const outputFollowGuide = `For follows=speed, the vibrator follows averaged absolute movement speed, with smoothing and fade-in. Shared pace changes affect it; position alone does not address a body region or select a motor. For follows=depth, the hidden position controls vibration: base is strongest near 0, tip near 100. For follows=steady, vibration stays constant while the timeline plays; movement edits cannot make it pulse or alter its level.`

const vibrationAvailabilityGuide = `If the vibrator is not connected and selected, its Max is 0, or the user's Stop paused it, it cannot vibrate. Do not promise output or try to restore it through JSON. For vibrator-only playback, leave motion unchanged and ask the user to connect/select it, or to choose Resume now in the device panel. If other outputs are available, fulfill requests for those outputs while being truthful about the silent vibrator. A user request cannot override these backend facts.`

// The Simple/Intermediate level vocabulary. Changes glide in the app, so a
// model chooses a value and never ramps it itself. Small models follow the
// example far better than a rule about leaving the linear edits empty.
const vibrationLevelGuide = `VIBRATION LEVEL: vibration_level and movement are separate controls. Top-level vibration_level is the vibrator's strength as a percentage of the user's Max; level_percent_of_max is the current value, and null keeps it. Starting movement requires a vibration_level for the start: choose it for the request, lower for a soft or gentle start; level_follows_handy means no level was chosen yet. A request only about vibration (stronger, gentler, off, back on) changes vibration_level alone: action none and edits %s, with no speed, layer or geometry change; do not speed up the strokes to strengthen vibration. Example: {"action":"none","edits":%s,"reply":"Stronger now.","vibration_level":75}. Change movement only when movement is asked for. For a combined pace and vibration request, use one pace edit: an absolute controls.speed_percent OR a relative change_by.speed_percent, never both. Put all absolute control changes in one controls group. vibration_level is separate from these edits. Choose a higher number for stronger, a lower one for gentler, 100 for the user's Max, 0 to stop only the vibrator, and a level suited to the request to turn it back on (previous_level_percent when shown); the app changes it smoothly. A separate vibration rhythm and individual motors are unavailable; say so honestly.`

// simpleVibrationGuide keeps Simple's 16K allocation: no backend JSON, only
// the facts the level needs.
const simpleVibrationGuide = `VIBRATOR: Vibration follows the movement. Top-level vibration_level is its strength from 0 to 100 percent of the user's maximum; %s. When you start movement, choose vibration_level for the start, lower for a gentle start. Use null to keep it; only a request about vibration changes it. Use a higher number when asked for stronger vibration, a lower one for gentler, 0 to turn off only the vibrator, and a real level such as 50 to turn it back on. A vibration-only request keeps the strokes: {"action":"none","edits":%s,"reply":"Stronger now.","vibration_level":75}`

func outputContextInstructions(outputs *OutputContext) string {
	return outputContextInstructionsWith(outputs, "{}")
}

// outputContextInstructionsWith writes the level example with this mode's
// empty edits.
func outputContextInstructionsWith(outputs *OutputContext, emptyEdits string) string {
	if outputs == nil || outputs.Vibrator == nil {
		return ""
	}
	encoded, _ := json.Marshal(outputs)
	guide := outputContextGuide
	if outputs.Vibrator.Independent {
		guide = independentVibrationGuide
	} else if outputs.Vibrator.LevelControl {
		guide = levelOutputContextGuide + "\n" + fmt.Sprintf(vibrationLevelGuide, emptyEdits, emptyEdits)
	}
	if outputs.Vibrator.ResumeOnMotionRequest {
		guide += "\n" + manualRestartGuide
	} else if !outputs.Vibrator.Available() {
		guide += "\n" + vibrationAvailabilityGuide
	}
	return "Active playback outputs (backend data):\n" + string(encoded) + "\n" + guide
}

const manualRestartGuide = "MANUAL RESTART PERMISSION: The user's Stop paused the vibrator and playback is stopped now. Use the refreshed running/paused values for this turn; earlier chat and a saved current_score do not mean playback is still running. This fresh human request may restart it: if the person asks for motion or vibration, issue the normal start or vibration change, and the app resumes the vibrator only when applying it. Questions and chat-only replies leave it stopped. You still cannot change the user's Max, bypass connection limits, or revive an older request."

func playbackOutputInstructions(caps Capabilities, state *MotionContext) string {
	if !caps.Motion || state == nil {
		return ""
	}
	if simpleContinuous(caps) && !caps.VibrationControl {
		if !vibrationLevelAvailable(caps, state) {
			return ""
		}
		return simpleVibrationInstructions(state.Outputs, continuousEmptyEdits(caps))
	}
	if !caps.AdvancedMotion && !caps.IntermediateMotion && !caps.VibrationControl {
		return ""
	}
	return outputContextInstructionsWith(state.Outputs, continuousEmptyEdits(caps))
}

// withoutLiveVibrationLevel keeps Autopilot on the level live chat chose: the
// Simple/Intermediate level is a live-chat vocabulary (ADR 0061).
func withoutLiveVibrationLevel(state *MotionContext) *MotionContext {
	if state == nil || state.Outputs == nil || state.Outputs.Vibrator == nil || !state.Outputs.Vibrator.LevelControl {
		return state
	}
	copied := *state
	outputs := *state.Outputs
	vibrator := *state.Outputs.Vibrator
	vibrator.LevelControl = false
	outputs.Vibrator = &vibrator
	copied.Outputs = &outputs
	return &copied
}

// Simple gets the level and nothing else; its stopped state reaches it through
// the local manual resume message instead.
func simpleVibrationInstructions(outputs *OutputContext, emptyEdits string) string {
	if outputs == nil || outputs.Vibrator == nil || !outputs.Vibrator.LevelControl || !outputs.Vibrator.Available() {
		return ""
	}
	current := fmt.Sprintf("it is %d now", outputs.Vibrator.LevelPercent)
	if outputs.Vibrator.LevelFollowsHandy {
		current = "no level is chosen yet, so it follows the strokes"
	}
	return fmt.Sprintf(simpleVibrationGuide, current, emptyEdits)
}

// Small models can read old "playing" speech as the current state despite a
// correct large system prompt. A short backend fact beside this fresh request
// resolves that ambiguity without classifying or rewriting the person's words.
// Keep ordinary turns, Advanced and autonomous budgets unchanged.
func withLocalManualResumeState(messages []llm.Message, caps Capabilities, state MotionContext) []llm.Message {
	available := state.ManualRestartAvailable || (state.Outputs != nil && state.Outputs.Vibrator != nil && state.Outputs.Vibrator.ResumeOnMotionRequest)
	if (!caps.IntermediateMotion && !caps.SimpleMotion) || caps.AdvancedMotion || state.Running || state.Paused ||
		!available || len(messages) == 0 {
		return messages
	}
	last := len(messages) - 1
	result := append([]llm.Message{}, messages[:last]...)
	content := "Current app state for this turn: playback is stopped, not paused. Previous chat describes earlier runs. A new motion request may start the saved or revised rhythm; a question leaves playback stopped. For a request to continue unchanged, start the saved current_score with empty edits instead of selecting new speed or geometry."
	if state.Outputs != nil && state.Outputs.Vibrator != nil && state.Outputs.Vibrator.ResumeOnMotionRequest {
		content += " The vibrator resumes with a motion request in this turn."
	}
	return append(result, llm.Message{Role: "system", Content: content}, messages[last])
}
