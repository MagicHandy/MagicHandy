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
	// AutopilotMotionScaled uses the numbered 1-7 motion-change preference.
	AutopilotMotionScaled = "scaled"

	autopilotMinimumMotionChangeLevel = 1
	autopilotMaximumMotionChangeLevel = 7
	autopilotDefaultMotionChangeLevel = 4

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

	// AutopilotMinimumArcMinutes accepts any positive whole-minute buildup.
	// The model cadence remains independently bounded, so a short buildup does
	// not permit faster model calls or motion outside the user's limits.
	AutopilotMinimumArcMinutes = 1
	// AutopilotMaximumArcMinutes is only a time.Duration overflow guard, not a
	// product-level session limit. It is roughly 292 years.
	AutopilotMaximumArcMinutes = 153_722_867
	// AutopilotDefaultArcMinutes is the first-run buildup duration.
	AutopilotDefaultArcMinutes = 30
)

// AutopilotSettings contains durable user preferences. Scheduler deadlines,
// pending model decisions, and playback acknowledgements are runtime state and
// never belong in the settings document.
type AutopilotSettings struct {
	SpeechCadence         string `json:"speech_cadence"`
	SpeechMinSeconds      int    `json:"speech_min_seconds"`
	SpeechMaxSeconds      int    `json:"speech_max_seconds"`
	MotionCadence         string `json:"motion_cadence"`
	MotionChangeLevel     int    `json:"motion_change_level"`
	MotionMinSeconds      int    `json:"motion_min_seconds"`
	MotionMaxSeconds      int    `json:"motion_max_seconds"`
	AdaptiveSpeechTiming  bool   `json:"adaptive_speech_timing"`
	AdaptiveMotionTiming  bool   `json:"adaptive_motion_timing"`
	SpeechMotionAuthority string `json:"speech_motion_authority"`
	// SessionTracking lets the model see elapsed session time, how long the
	// current speed has held, and which way speed has been moving. Inert input:
	// it informs decisions and authorizes nothing. Off removes the facts from the
	// prompt entirely rather than sending zeros.
	SessionTracking bool `json:"session_tracking"`
	// SessionArc enables the visible elapsed-session fill bar supplied to the
	// model as pacing context. It is a separate switch from SessionTracking
	// because knowing how long a session has run and asking the model to pace
	// along a configured buildup are different choices.
	//
	// The buildup positions intent inside the user's existing speed band. It never
	// widens the band, the focus range, or any capability gate.
	SessionArc        bool `json:"session_arc"`
	SessionArcMinutes int  `json:"session_arc_minutes"`
}

// DefaultAutopilotSettings returns the conservative first-run profile.
func DefaultAutopilotSettings() AutopilotSettings {
	return AutopilotSettings{
		SpeechCadence:         AutopilotSpeechNatural,
		SpeechMinSeconds:      AutopilotDefaultSpeechMinSeconds,
		SpeechMaxSeconds:      AutopilotDefaultSpeechMaxSeconds,
		MotionCadence:         AutopilotMotionScaled,
		MotionChangeLevel:     autopilotDefaultMotionChangeLevel,
		MotionMinSeconds:      AutopilotDefaultMotionMinSeconds,
		MotionMaxSeconds:      AutopilotDefaultMotionMaxSeconds,
		AdaptiveSpeechTiming:  true,
		AdaptiveMotionTiming:  true,
		SpeechMotionAuthority: AutopilotSpeechMotionChatOnly,
		// Tracking defaults on: it is read-only context that makes cadence
		// decisions better informed. Buildup defaults off because it changes what
		// the model is encouraged to do, which is a choice the user should make.
		SessionTracking:   true,
		SessionArc:        false,
		SessionArcMinutes: AutopilotDefaultArcMinutes,
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
	if s.MotionCadence == AutopilotMotionScaled &&
		s.MotionChangeLevel >= autopilotMinimumMotionChangeLevel &&
		s.MotionChangeLevel <= autopilotMaximumMotionChangeLevel {
		return motionChangeLevelWindow(s.MotionChangeLevel)
	}
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

func motionChangeLevelWindow(level int) (minimum int, maximum int) {
	switch level {
	case 1:
		return 90, 240
	case 2:
		return 60, 150
	case 3:
		return 45, 120
	case 5:
		return 14, 45
	case 6:
		return 10, 35
	case 7:
		return 8, 24
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
		AutopilotMotionScaled,
	) {
		return fmt.Errorf("unknown Autopilot motion cadence %q", settings.MotionCadence)
	}
	if settings.MotionChangeLevel < autopilotMinimumMotionChangeLevel ||
		settings.MotionChangeLevel > autopilotMaximumMotionChangeLevel {
		return fmt.Errorf("autopilot motion change level must be between %d and %d",
			autopilotMinimumMotionChangeLevel, autopilotMaximumMotionChangeLevel)
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
	if err := validateAutopilotWindow(
		"motion",
		settings.MotionMinSeconds,
		settings.MotionMaxSeconds,
		AutopilotMaximumMotionSeconds,
	); err != nil {
		return err
	}
	if settings.SessionArcMinutes < AutopilotMinimumArcMinutes ||
		settings.SessionArcMinutes > AutopilotMaximumArcMinutes {
		return fmt.Errorf(
			"autopilot session buildup duration must be between %d and %d minutes",
			AutopilotMinimumArcMinutes,
			AutopilotMaximumArcMinutes,
		)
	}
	// The buildup is a reading of session progress, so it cannot exist without the
	// tracking that produces it. Rejecting the combination keeps the settings
	// document from expressing a state the runtime would have to silently ignore.
	if settings.SessionArc && !settings.SessionTracking {
		return errors.New("autopilot session buildup requires session tracking")
	}
	return nil
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
	if settings.MotionChangeLevel == 0 {
		settings.MotionChangeLevel = legacyMotionChangeLevel(settings)
		settings.MotionCadence = AutopilotMotionScaled
	}
	// A bool cannot distinguish "absent" from "explicitly false", so the new buildup
	// length doubles as the presence marker for this field group: it is zero only
	// in a document written before the group existed. Without this, a document
	// saved between the cadence release and this one would silently run with
	// tracking off while the documented default is on. Once the group has been
	// saved once, the marker is non-zero and an explicit false is preserved.
	if settings.SessionArcMinutes == 0 {
		settings.SessionArcMinutes = defaults.SessionArcMinutes
		settings.SessionTracking = defaults.SessionTracking
	}
	return settings
}

func legacyMotionChangeLevel(settings AutopilotSettings) int {
	switch settings.MotionCadence {
	case AutopilotMotionSteady:
		return 3
	case AutopilotMotionDynamic:
		return 6
	case AutopilotMotionCustom:
		midpoint := (settings.MotionMinSeconds + settings.MotionMaxSeconds) / 2
		bestLevel, bestDistance := autopilotDefaultMotionChangeLevel, int(^uint(0)>>1)
		for level := autopilotMinimumMotionChangeLevel; level <= autopilotMaximumMotionChangeLevel; level++ {
			minimum, maximum := motionChangeLevelWindow(level)
			distance := midpoint - (minimum+maximum)/2
			if distance < 0 {
				distance = -distance
			}
			if distance < bestDistance {
				bestLevel, bestDistance = level, distance
			}
		}
		return bestLevel
	default:
		return autopilotDefaultMotionChangeLevel
	}
}
