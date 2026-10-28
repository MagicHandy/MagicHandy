package config

import (
	"fmt"
	"slices"
)

// DeviceVibrator is a Buttplug vibrator reached through Intiface Central. It
// follows the shared motion frame through a conversion profile, at a level the
// model may choose under the user's Max (ADR 0061).
const DeviceVibrator = "vibrator"

// Vibration conversion profiles: what the vibration level follows.
const (
	VibrationFollowSpeed  = "speed"
	VibrationFollowDepth  = "depth"
	VibrationFollowSteady = "steady"
)

// Vibration depth ends, in the frame's semantic coordinates: base is
// position 0, tip is position 100.
const (
	VibrationDepthBase = "base"
	VibrationDepthTip  = "tip"
)

// Who sets the vibration level under Max: the chat model, or nobody (the
// vibrator then follows movement at Max).
const (
	VibrationLevelModel  = "model"
	VibrationLevelManual = "manual"
)

// Vibration setting ranges and defaults.
const (
	VibrationDefaultCeilingPercent = 40
	VibrationMaxTimingOffsetMs     = 300
)

// VibrationSettings configures vibrator output. The ceiling is the user's Max:
// it caps every output, including the level the model chooses. The minimum is
// the floor of movement-following vibration.
type VibrationSettings struct {
	Follow             string `json:"follow"`
	DepthEnd           string `json:"depth_end"`
	MinimumPercent     int    `json:"minimum_percent"`
	CeilingPercent     int    `json:"ceiling_percent"`
	TimingOffsetMillis int    `json:"timing_offset_ms"`
	LevelControl       string `json:"level_control"`
}

// DefaultVibrationSettings starts low, follows stroke speed and lets the model
// choose the level under Max.
func DefaultVibrationSettings() VibrationSettings {
	return VibrationSettings{
		Follow:         VibrationFollowSpeed,
		DepthEnd:       VibrationDepthBase,
		CeilingPercent: VibrationDefaultCeilingPercent,
		LevelControl:   VibrationLevelModel,
	}
}

// ModelSetsLevel reports whether the chat model chooses the level under Max.
func (s VibrationSettings) ModelSetsLevel() bool {
	return s.LevelControl != VibrationLevelManual
}

// applyMissingVibrationDefaults fills a block that older settings lack. A
// stored block keeps its values, including a deliberate 0% ceiling.
func applyMissingVibrationDefaults(settings VibrationSettings) VibrationSettings {
	if settings.Follow == "" {
		return DefaultVibrationSettings()
	}
	if settings.DepthEnd == "" {
		settings.DepthEnd = VibrationDepthBase
	}
	if settings.LevelControl == "" {
		settings.LevelControl = VibrationLevelModel
	}
	return settings
}

// validateCompanionSettings checks the settings of outputs that follow the
// shared frame through a conversion profile.
func validateCompanionSettings(settings Settings) error {
	if err := validateEstimSettings(settings.Estim); err != nil {
		return err
	}
	return validateVibrationSettings(settings.Vibration)
}

func validateVibrationSettings(settings VibrationSettings) error {
	if !slices.Contains([]string{VibrationFollowSpeed, VibrationFollowDepth, VibrationFollowSteady}, settings.Follow) {
		return fmt.Errorf("unknown vibration profile %q", settings.Follow)
	}
	if settings.DepthEnd != VibrationDepthBase && settings.DepthEnd != VibrationDepthTip {
		return fmt.Errorf("unknown vibration depth end %q", settings.DepthEnd)
	}
	if settings.LevelControl != VibrationLevelModel && settings.LevelControl != VibrationLevelManual {
		return fmt.Errorf("unknown vibration level control %q", settings.LevelControl)
	}
	if settings.CeilingPercent < 0 || settings.CeilingPercent > 100 || settings.MinimumPercent < 0 || settings.MinimumPercent > settings.CeilingPercent {
		return fmt.Errorf("the vibration range must satisfy 0 <= minimum <= ceiling <= 100")
	}
	if settings.TimingOffsetMillis < -VibrationMaxTimingOffsetMs || settings.TimingOffsetMillis > VibrationMaxTimingOffsetMs {
		return fmt.Errorf("the vibration timing offset must be between -%d and %d ms", VibrationMaxTimingOffsetMs, VibrationMaxTimingOffsetMs)
	}
	return nil
}
