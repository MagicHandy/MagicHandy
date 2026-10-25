package chat

import (
	"encoding/json"
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

// VibrationContext describes the selected output and its relative drive capability.
type VibrationContext struct {
	ResumeOnMotionRequest bool                  `json:"manual_request_can_resume_previous_intensity,omitempty"`
	Independent           bool                  `json:"independent_control"`
	Current               *motion.VibrationSpec `json:"current_envelope,omitempty"`
	Follow                string                `json:"follows"`
	DepthEnd              string                `json:"strongest_at,omitempty"`
	Ready                 bool                  `json:"connected_and_selected"`
	Armed                 bool                  `json:"user_has_set_nonzero_intensity"`
	Motors                int                   `json:"motors_driven_together"`
}

// Only backend permission can restore a previously human-selected intensity.
// Nil data preserves the generic/simulator contract; mixed sessions may still
// move their other participants when vibration is unavailable.
func unavailableVibrationOnly(state MotionContext) bool {
	outputs := state.Outputs
	return outputs != nil && outputs.Strokers == 0 && !outputs.Coyote &&
		outputs.Vibrator != nil && (!outputs.Vibrator.Ready || (!outputs.Vibrator.Armed && !outputs.Vibrator.ResumeOnMotionRequest))
}

const outputContextGuide = `The outputs share one movement timeline; there is no independent device or motor command. A vibrator follows that timeline, within the intensity set by the user. Never claim to change its intensity setting, ceiling, profile, individual motors, or playback participation.
For follows=speed, the vibrator follows averaged absolute movement speed, with smoothing and fade-in. Shared pace changes affect it; position alone does not address a body region or select a motor. For follows=depth, the hidden position controls vibration: base is strongest near 0, tip near 100. For follows=steady, vibration stays constant while the timeline plays; movement edits cannot make it pulse or alter its level.
With no linear device, describe vibration rather than physical strokes, travel, or penetration. The motion JSON remains a hidden control signal. With a linear device, any movement edit also changes that device: if asked to change vibration independently while preserving its movement, use no change and explain the limitation. Do not invent a separate vibration pattern or use Stop to silence only one participant.`

const vibrationAvailabilityGuide = `If the vibrator is not ready or armed, it cannot vibrate. Do not promise output or ask for more intensity through JSON. For vibrator-only playback, leave motion unchanged and ask the user to connect/select it or raise its intensity in the device panel. If other outputs are available, fulfill requests for those outputs while being truthful about the silent vibrator. A user request cannot override these backend facts.`

func outputContextInstructions(outputs *OutputContext) string {
	if outputs == nil || outputs.Vibrator == nil {
		return ""
	}
	encoded, _ := json.Marshal(outputs)
	guide := outputContextGuide
	if outputs.Vibrator.Independent {
		guide = independentVibrationGuide
	}
	if outputs.Vibrator.ResumeOnMotionRequest {
		guide += "\nMANUAL RESTART PERMISSION: The vibrator is stopped now. Use the refreshed running/paused values for this turn; earlier chat and a saved current_score do not mean playback is still running. This fresh human request may resume the previously human-selected intensity after Stop, even though the current intensity is zero. If the person requests motion, issue the normal start or independent vibration change; the app restores that previous level only when applying valid motion. Questions and chat-only replies leave it stopped. You still cannot choose or raise the intensity, bypass connection/ceiling limits, or revive an older request."
	} else {
		guide += "\n" + vibrationAvailabilityGuide
	}
	return "Active playback outputs (backend data):\n" + string(encoded) + "\n" + guide
}

func playbackOutputInstructions(caps Capabilities, state *MotionContext) string {
	if !caps.Motion || (!caps.AdvancedMotion && !caps.IntermediateMotion && !caps.VibrationControl) || state == nil {
		return ""
	}
	return outputContextInstructions(state.Outputs)
}

// The 12B model can read old "playing" speech as the current state despite a
// correct large system prompt. A short backend fact beside this fresh request
// resolves that ambiguity without classifying or rewriting the person's words.
// Keep ordinary turns, Simple, Advanced and autonomous budgets unchanged.
func withLocalManualResumeState(messages []llm.Message, caps Capabilities, state MotionContext) []llm.Message {
	if !caps.IntermediateMotion || caps.AdvancedMotion || state.Running || state.Paused ||
		state.Outputs == nil || state.Outputs.Vibrator == nil || !state.Outputs.Vibrator.ResumeOnMotionRequest || len(messages) == 0 {
		return messages
	}
	last := len(messages) - 1
	result := append([]llm.Message{}, messages[:last]...)
	return append(result, llm.Message{Role: "system", Content: "Current app state for this turn: playback is stopped, not paused. Previous chat describes earlier runs. A new motion request may start the saved or revised rhythm; a question leaves playback stopped. The app can restore the previous human-selected vibrator intensity for this request."}, messages[last])
}
