package config

import (
	"errors"
	"fmt"
)

const (
	// AutopilotSpeechOff disables autonomous spoken check-ins.
	AutopilotSpeechOff = "off"
	// AutopilotSpeechQuiet uses long gaps between autonomous lines.
	AutopilotSpeechQuiet = "quiet"
	// AutopilotSpeechNatural is the default conversational cadence.
	AutopilotSpeechNatural = "natural"
	// AutopilotSpeechTalkative uses shorter gaps between autonomous lines.
	AutopilotSpeechTalkative = "talkative"
	// AutopilotSpeechCustom uses the saved custom speech bounds.
	AutopilotSpeechCustom = "custom"

	// AutopilotMotionSteady changes semantic targets infrequently.
	AutopilotMotionSteady = "steady"
	// AutopilotMotionNatural is the default motion-evolution cadence.
	AutopilotMotionNatural = "natural"
	// AutopilotMotionDynamic changes semantic targets more frequently.
	AutopilotMotionDynamic = "dynamic"
	// AutopilotMotionCustom uses the saved custom motion bounds.
	AutopilotMotionCustom = "custom"

	// AutopilotSpeechMotionChatOnly prevents autonomous speech turns from
	// changing motion.
	AutopilotSpeechMotionChatOnly = "chat_only"
	// AutopilotSpeechMotionStyle lets autonomous speech adjust speed or area
	// while preserving the current pattern.
	AutopilotSpeechMotionStyle = "style_only"
	// AutopilotSpeechMotionFull lets autonomous speech select enabled patterns.
	AutopilotSpeechMotionFull = "full_motion"

	// AutopilotMinimumIntervalSeconds is the hard floor for either clock.
	AutopilotMinimumIntervalSeconds = 8
	// AutopilotMaximumSpeechSeconds caps custom spoken-check-in intervals.
	AutopilotMaximumSpeechSeconds = 600
	// AutopilotMaximumMotionSeconds caps custom motion-change intervals.
	AutopilotMaximumMotionSeconds = 300
	// AutopilotDefaultSpeechMinSeconds is the lower natural speech bound.
	AutopilotDefaultSpeechMinSeconds = 35
	// AutopilotDefaultSpeechMaxSeconds is the upper natural speech bound.
	AutopilotDefaultSpeechMaxSeconds = 120
	// AutopilotDefaultMotionMinSeconds is the lower natural motion bound.
	AutopilotDefaultMotionMinSeconds = 20
	// AutopilotDefaultMotionMaxSeconds is the upper natural motion bound.
	AutopilotDefaultMotionMaxSeconds = 60
)

// AutopilotSettings contains durable user preferences. Scheduler deadlines,
// pending model decisions, and playback acknowledgements are runtime state and
// never belong in the settings document.
type AutopilotSettings struct {
	SpeechCadence         string `json:"speech_cadence"`
	SpeechMinSeconds      int    `json:"speech_min_seconds"`
	SpeechMaxSeconds      int    `json:"speech_max_seconds"`
	MotionCadence         string `json:"motion_cadence"`
	MotionMinSeconds      int    `json:"motion_min_seconds"`
	MotionMaxSeconds      int    `json:"motion_max_seconds"`
	AdaptiveSpeechTiming  bool   `json:"adaptive_speech_timing"`
	AdaptiveMotionTiming  bool   `json:"adaptive_motion_timing"`
	SpeechMotionAuthority string `json:"speech_motion_authority"`
}

// DefaultAutopilotSettings returns the conservative first-run profile.
func DefaultAutopilotSettings() AutopilotSettings {
	return AutopilotSettings{
		SpeechCadence:         AutopilotSpeechNatural,
		SpeechMinSeconds:      AutopilotDefaultSpeechMinSeconds,
		SpeechMaxSeconds:      AutopilotDefaultSpeechMaxSeconds,
		MotionCadence:         AutopilotMotionNatural,
		MotionMinSeconds:      AutopilotDefaultMotionMinSeconds,
		MotionMaxSeconds:      AutopilotDefaultMotionMaxSeconds,
		AdaptiveSpeechTiming:  true,
		AdaptiveMotionTiming:  true,
		SpeechMotionAuthority: AutopilotSpeechMotionChatOnly,
	}
}

// SpeechWindow returns the effective bounds and whether autonomous speech is
// enabled.
func (s AutopilotSettings) SpeechWindow() (minimum int, maximum int, enabled bool) {
	switch s.SpeechCadence {
	case AutopilotSpeechOff:
		return 0, 0, false
	case AutopilotSpeechQuiet:
		return 90, 240, true
	case AutopilotSpeechTalkative:
		return 15, 60, true
	case AutopilotSpeechCustom:
		return s.SpeechMinSeconds, s.SpeechMaxSeconds, true
	default:
		return 35, 120, true
	}
}

// MotionWindow returns the effective motion-evolution bounds.
func (s AutopilotSettings) MotionWindow() (minimum int, maximum int) {
	switch s.MotionCadence {
	case AutopilotMotionSteady:
		return 45, 120
	case AutopilotMotionDynamic:
		return 10, 35
	case AutopilotMotionCustom:
		return s.MotionMinSeconds, s.MotionMaxSeconds
	default:
		return 20, 60
	}
}

func validateAutopilotSettings(settings AutopilotSettings) error {
	if !oneOf(
		settings.SpeechCadence,
		AutopilotSpeechOff,
		AutopilotSpeechQuiet,
		AutopilotSpeechNatural,
		AutopilotSpeechTalkative,
		AutopilotSpeechCustom,
	) {
		return fmt.Errorf("unknown Autopilot speech cadence %q", settings.SpeechCadence)
	}
	if !oneOf(
		settings.MotionCadence,
		AutopilotMotionSteady,
		AutopilotMotionNatural,
		AutopilotMotionDynamic,
		AutopilotMotionCustom,
	) {
		return fmt.Errorf("unknown Autopilot motion cadence %q", settings.MotionCadence)
	}
	if !oneOf(
		settings.SpeechMotionAuthority,
		AutopilotSpeechMotionChatOnly,
		AutopilotSpeechMotionStyle,
		AutopilotSpeechMotionFull,
	) {
		return fmt.Errorf("unknown Autopilot speech motion authority %q", settings.SpeechMotionAuthority)
	}
	if err := validateAutopilotWindow(
		"speech",
		settings.SpeechMinSeconds,
		settings.SpeechMaxSeconds,
		AutopilotMaximumSpeechSeconds,
	); err != nil {
		return err
	}
	return validateAutopilotWindow(
		"motion",
		settings.MotionMinSeconds,
		settings.MotionMaxSeconds,
		AutopilotMaximumMotionSeconds,
	)
}

func validateAutopilotWindow(label string, minimum int, maximum int, ceiling int) error {
	if minimum < AutopilotMinimumIntervalSeconds || maximum > ceiling {
		return fmt.Errorf(
			"autopilot %s cadence bounds must be between %d and %d seconds",
			label,
			AutopilotMinimumIntervalSeconds,
			ceiling,
		)
	}
	if minimum > maximum {
		return errors.New("autopilot " + label + " cadence minimum cannot exceed maximum")
	}
	return nil
}

func applyMissingAutopilotDefaults(
	settings AutopilotSettings,
	defaults AutopilotSettings,
) AutopilotSettings {
	if settings.SpeechCadence == "" &&
		settings.MotionCadence == "" &&
		settings.SpeechMotionAuthority == "" {
		return defaults
	}
	if settings.SpeechCadence == "" {
		settings.SpeechCadence = defaults.SpeechCadence
	}
	if settings.MotionCadence == "" {
		settings.MotionCadence = defaults.MotionCadence
	}
	if settings.SpeechMotionAuthority == "" {
		settings.SpeechMotionAuthority = defaults.SpeechMotionAuthority
	}
	if settings.SpeechMinSeconds == 0 {
		settings.SpeechMinSeconds = defaults.SpeechMinSeconds
	}
	if settings.SpeechMaxSeconds == 0 {
		settings.SpeechMaxSeconds = defaults.SpeechMaxSeconds
	}
	if settings.MotionMinSeconds == 0 {
		settings.MotionMinSeconds = defaults.MotionMinSeconds
	}
	if settings.MotionMaxSeconds == 0 {
		settings.MotionMaxSeconds = defaults.MotionMaxSeconds
	}
	return settings
}
