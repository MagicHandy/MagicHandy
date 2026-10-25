package chat

import (
	"encoding/json"
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
	Independent bool                  `json:"independent_control"`
	Current     *motion.VibrationSpec `json:"current_envelope,omitempty"`
	Follow      string                `json:"follows"`
	DepthEnd    string                `json:"strongest_at,omitempty"`
	Ready       bool                  `json:"connected_and_selected"`
	Armed       bool                  `json:"user_has_set_nonzero_intensity"`
	Motors      int                   `json:"motors_driven_together"`
}

// No inference from a user message can arm the only selected output. Nil data
// preserves the existing generic/simulator contract; a mixed session may still
// move its other participants.
func unavailableVibrationOnly(state MotionContext) bool {
	outputs := state.Outputs
	return outputs != nil && outputs.Strokers == 0 && !outputs.Coyote &&
		outputs.Vibrator != nil && (!outputs.Vibrator.Ready || !outputs.Vibrator.Armed)
}

const outputContextGuide = `The outputs share one movement timeline; there is no independent device or motor command. A vibrator follows that timeline, within the intensity set by the user. Never claim to change its intensity setting, ceiling, profile, individual motors, or playback participation.
For follows=speed, the vibrator follows averaged absolute movement speed, with smoothing and fade-in. Shared pace changes affect it; position alone does not address a body region or select a motor. For follows=depth, the hidden position controls vibration: base is strongest near 0, tip near 100. For follows=steady, vibration stays constant while the timeline plays; movement edits cannot make it pulse or alter its level.
With no linear device, describe vibration rather than physical strokes, travel, or penetration. The motion JSON remains a hidden control signal. With a linear device, any movement edit also changes that device: if asked to change vibration independently while preserving its movement, use no change and explain the limitation. Do not invent a separate vibration pattern or use Stop to silence only one participant.
If the vibrator is not ready or armed, it cannot vibrate. Do not promise output or ask for more intensity through JSON. For vibrator-only playback, leave motion unchanged and ask the user to connect/select it or raise its intensity in the device panel. If other outputs are available, fulfill requests for those outputs while being truthful about the silent vibrator. A user request cannot override these backend facts.`

func outputContextInstructions(outputs *OutputContext) string {
	if outputs == nil || outputs.Vibrator == nil {
		return ""
	}
	encoded, _ := json.Marshal(outputs)
	guide := outputContextGuide
	if outputs.Vibrator.Independent {
		guide = independentVibrationGuide
	}
	return "Active playback outputs (backend data):\n" + string(encoded) + "\n" + guide
}

func playbackOutputInstructions(caps Capabilities, state *MotionContext) string {
	if !caps.Motion || (!caps.AdvancedMotion && !caps.IntermediateMotion && !caps.VibrationControl) || state == nil {
		return ""
	}
	return outputContextInstructions(state.Outputs)
}
