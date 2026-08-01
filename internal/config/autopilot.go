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

	// AutopilotArcHold keeps the session buildup where it is.
	AutopilotArcHold = "hold"
	// AutopilotArcAdvance asks the backend to move the buildup forward one bounded
	// step. The model can never write the value itself.
	AutopilotArcAdvance = "advance"
	// AutopilotArcEase asks the backend to move the buildup back one bounded step, so
	// the model can wind down as well as build.
	AutopilotArcEase = "ease"

	// AutopilotMinimumArcMinutes accepts any positive whole-minute buildup.
	// The model cadence remains independently bounded, so a short buildup does
	// not permit faster model calls or motion outside the user's limits.
	AutopilotMinimumArcMinutes = 1
	// AutopilotMaximumArcMinutes is only a time.Duration overflow guard, not a
	// product-level session limit. It is roughly 292 years.
	AutopilotMaximumArcMinutes = 153_722_867
	// AutopilotDefaultArcMinutes is the first-run buildup duration.
	AutopilotDefaultArcMinutes = 30
	// AutopilotArcNudgePercent is the most one model turn may move the buildup. A
	// clamp is what keeps an eager model from sprinting the bar to full.
	AutopilotArcNudgePercent = 6
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
	// SessionTracking lets the model see elapsed session time, how long the
	// current speed has held, and which way speed has been moving. Inert input:
	// it informs decisions and authorizes nothing. Off removes the facts from the
	// prompt entirely rather than sending zeros.
	SessionTracking bool `json:"session_tracking"`
	// SessionArc enables the visible fill bar the model is encouraged to build
	// along. It is a separate switch from SessionTracking because knowing how
	// long a session has run and being encouraged to escalate through it are
	// different things, and a user may want the first without the second.
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
		MotionCadence:         AutopilotMotionNatural,
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

// ValidAutopilotArcIntent reports whether a model-supplied buildup nudge is one this
// build honors. Anything else resolves to hold.
func ValidAutopilotArcIntent(intent string) bool {
	return oneOf(intent, AutopilotArcHold, AutopilotArcAdvance, AutopilotArcEase)
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
