package motion

import (
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

const defaultSpeedPercent = 50

// TargetSourceManualUI identifies motion explicitly started by the diagnostic
// manual-test controls. Autonomous modes use their mode identifier as source.
const TargetSourceManualUI = "manual_ui"

// TargetSourceMedia identifies a clock-locked paired-funscript run.
const TargetSourceMedia = "media"

// PatternID identifies a repeatable semantic motion pattern.
type PatternID string

const (
	// PatternStroke is the default full-stroke triangle pattern.
	PatternStroke PatternID = "stroke"
	// PatternPulse is a fixed double-peak pattern.
	PatternPulse PatternID = "pulse"
	// PatternTease is a fixed shallow-to-deep pattern.
	PatternTease PatternID = "tease"
	// PatternWaves is the swelling-amplitude pattern.
	PatternWaves PatternID = "waves"
	// PatternClimb is the ratcheting-build pattern.
	PatternClimb PatternID = "climb"
	// PatternFlutter is the shallow-flutter-with-sweep pattern.
	PatternFlutter PatternID = "flutter"
	// PatternSway is the asymmetric broad-arc pattern.
	PatternSway PatternID = "sway"
	// PatternDrift migrates a consistent stroke window across a cycle.
	PatternDrift PatternID = "drift"
	// PatternDoubleTap groups paired accents around deeper sweeps.
	PatternDoubleTap PatternID = "double-tap"
	// PatternCascade steps peak depth downward before resetting.
	PatternCascade PatternID = "cascade"
	// PatternPendulum alternates long and short centered arcs.
	PatternPendulum PatternID = "pendulum"
	// PatternCradle uses restrained centered arcs with changing width.
	PatternCradle PatternID = "cradle"
	// PatternSurge follows one full sweep with decaying echoes.
	PatternSurge PatternID = "surge"
	// PatternRolling layers offset medium and deep strokes.
	PatternRolling PatternID = "rolling"
	// PatternSyncopate uses an intentionally uneven complete rhythm.
	PatternSyncopate PatternID = "syncopate"
	// PatternFourLevelCircuit cycles full and partial strokes across both zones.
	PatternFourLevelCircuit PatternID = "four-level-circuit"
	// PatternHighLowBlocks groups upper and lower zone pulses.
	PatternHighLowBlocks PatternID = "high-low-blocks"
	// PatternDeepShallowSequence mixes deep and medium upper-anchored strokes.
	PatternDeepShallowSequence PatternID = "deep-shallow-sequence"
	// PatternShortMediumSteps repeats short and medium lower-anchored strokes.
	PatternShortMediumSteps PatternID = "short-medium-steps"
	// PatternTopAnchoredDepths identifies a retired upper-return catalog pattern.
	PatternTopAnchoredDepths PatternID = "top-anchored-depths"
	// PatternDeepBookends identifies a retired lower-return catalog pattern.
	PatternDeepBookends PatternID = "deep-bookends"
	// PatternOneDeepThreeShallow identifies a retired shallow-pulse catalog pattern.
	PatternOneDeepThreeShallow PatternID = "one-deep-three-shallow"
	// PatternLowerMidrangeMix identifies a retired lower-midrange catalog pattern.
	PatternLowerMidrangeMix PatternID = "lower-midrange-mix"
	// PatternMidTopSwitch identifies a retired upper-pulse catalog pattern.
	PatternMidTopSwitch PatternID = "mid-top-switch"
	// PatternSlowFastFull changes from slow full strokes to fast full strokes.
	PatternSlowFastFull PatternID = "slow-fast-full"
	// PatternMidrangeFullFinish identifies a retired repeated-midrange pattern.
	PatternMidrangeFullFinish PatternID = "midrange-full-finish"
	// PatternDeepPartialSequence mixes full and partial lower-anchored strokes.
	PatternDeepPartialSequence PatternID = "deep-partial-sequence"
	// PatternDeepMediumShortPairs moves through paired reach bands.
	PatternDeepMediumShortPairs PatternID = "deep-medium-short-pairs"
	// PatternFallingCrest lowers successive upper reversals across broad strokes.
	PatternFallingCrest PatternID = "falling-crest"
	// PatternThreeDeepOneShort resolves grouped broad strokes with a shorter phrase.
	PatternThreeDeepOneShort PatternID = "three-deep-one-short"
	// PatternDescendingLadder steps both endpoints downward before rebounding.
	PatternDescendingLadder PatternID = "descending-ladder"
	// PatternWanderingSwell changes both center and reach before a full sweep.
	PatternWanderingSwell PatternID = "wandering-swell"
	// PatternRisingReach progressively extends alternating upper reversals.
	PatternRisingReach PatternID = "rising-reach"
	// PatternHardAndRegular is a promoted user-curated full-range rhythm.
	PatternHardAndRegular PatternID = "hard-and-regular"
	// PatternPlayfulJerk is a promoted user-curated staggered full-range rhythm.
	PatternPlayfulJerk PatternID = "playful-jerk"
)

// AreaFocus constrains semantic sampling to a focus region.
type AreaFocus struct {
	MinPercent int `json:"min_percent"`
	MaxPercent int `json:"max_percent"`
}

// SoftAnchor gently biases sampled positions toward one semantic point.
type SoftAnchor struct {
	PositionPercent int `json:"position_percent"`
	WeightPercent   int `json:"weight_percent"`
}

// MotionTarget is the app-level semantic motion intent.
//
//revive:disable-next-line:exported -- Phase 6 explicitly names this contract.
type MotionTarget struct {
	Label                  string      `json:"label,omitempty"`
	Source                 string      `json:"source,omitempty"`
	PatternID              PatternID   `json:"pattern_id,omitempty"`
	PatternName            string      `json:"pattern_name,omitempty"`
	ProgramID              string      `json:"program_id,omitempty"`
	MediaID                string      `json:"media_id,omitempty"`
	SpeedPercent           int         `json:"speed_percent"`
	MediaSpeedLimitEnabled bool        `json:"media_speed_limit_enabled,omitempty"`
	AreaFocus              *AreaFocus  `json:"area_focus,omitempty"`
	SoftAnchor             *SoftAnchor `json:"soft_anchor,omitempty"`

	// Resolved content is backend-owned and never serialized to clients. The
	// public IDs above remain the authoritative snapshot vocabulary.
	Pattern *PatternDefinition       `json:"-"`
	Program *ProgramDefinition       `json:"-"`
	Media   *MediaTimelineDefinition `json:"-"`
}

// NormalizeTarget clamps semantic intent without applying physical stroke settings.
func NormalizeTarget(target MotionTarget, settings config.MotionSettings) MotionTarget {
	target.Label = strings.TrimSpace(target.Label)
	target.Source = strings.TrimSpace(target.Source)
	target.PatternName = strings.TrimSpace(target.PatternName)
	if target.Source == "" {
		target.Source = "motion"
	}
	target.ProgramID = strings.TrimSpace(target.ProgramID)
	target.MediaID = strings.TrimSpace(target.MediaID)
	target.MediaSpeedLimitEnabled = false
	if target.Media != nil {
		target.MediaID = strings.TrimSpace(target.Media.ID)
		target.PatternID = ""
		target.PatternName = ""
		target.ProgramID = ""
		target.Pattern = nil
		target.Program = nil
		// The maximum remains available for safe startup positioning, but changes
		// the authored timeline only when the user explicitly enables that policy.
		// Video timestamps always remain locked to the media clock.
		target.SpeedPercent = settings.SpeedMaxPercent
		target.MediaSpeedLimitEnabled = settings.ApplyVideoSpeedLimit
	}
	if target.Program != nil {
		target.ProgramID = strings.TrimSpace(target.Program.ID)
		target.PatternID = ""
		target.PatternName = ""
		target.MediaID = ""
	}
	if target.Pattern != nil {
		target.PatternID = target.Pattern.ID
		target.ProgramID = ""
		target.MediaID = ""
		target.Program = nil
	}
	if target.PatternID == "" && target.ProgramID == "" && target.MediaID == "" {
		target.PatternID = PatternStroke
	}
	if target.SpeedPercent == 0 {
		target.SpeedPercent = defaultSpeedPercent
	}
	target.SpeedPercent = clamp(target.SpeedPercent, settings.SpeedMinPercent, settings.SpeedMaxPercent)
	target.AreaFocus = normalizeAreaFocus(target.AreaFocus)
	target.SoftAnchor = normalizeSoftAnchor(target.SoftAnchor)
	return target
}

func normalizeAreaFocus(focus *AreaFocus) *AreaFocus {
	if focus == nil {
		return nil
	}
	normalized := AreaFocus{
		MinPercent: clamp(focus.MinPercent, 0, 100),
		MaxPercent: clamp(focus.MaxPercent, 0, 100),
	}
	if normalized.MinPercent >= normalized.MaxPercent {
		center := clamp((normalized.MinPercent+normalized.MaxPercent)/2, 0, 100)
		normalized.MinPercent = clamp(center-5, 0, 99)
		normalized.MaxPercent = clamp(center+5, normalized.MinPercent+1, 100)
	}
	return &normalized
}

func normalizeSoftAnchor(anchor *SoftAnchor) *SoftAnchor {
	if anchor == nil {
		return nil
	}
	normalized := SoftAnchor{
		PositionPercent: clamp(anchor.PositionPercent, 0, 100),
		WeightPercent:   clamp(anchor.WeightPercent, 0, 100),
	}
	if normalized.WeightPercent == 0 {
		return nil
	}
	return &normalized
}

func normalizeMotionSettings(settings config.MotionSettings) config.MotionSettings {
	defaults := config.DefaultSettings().Motion
	if settings.SpeedMinPercent == 0 {
		settings.SpeedMinPercent = defaults.SpeedMinPercent
	}
	if settings.SpeedMaxPercent == 0 {
		settings.SpeedMaxPercent = defaults.SpeedMaxPercent
	}
	if settings.StrokeMaxPercent == 0 {
		settings.StrokeMaxPercent = defaults.StrokeMaxPercent
	}
	settings.SpeedMinPercent = clamp(settings.SpeedMinPercent, 1, 100)
	settings.SpeedMaxPercent = clamp(settings.SpeedMaxPercent, settings.SpeedMinPercent, 100)
	settings.StrokeMinPercent = clamp(settings.StrokeMinPercent, 0, 99)
	settings.StrokeMaxPercent = clamp(settings.StrokeMaxPercent, settings.StrokeMinPercent+1, 100)
	return settings
}

func clamp(value int, minimum int, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
