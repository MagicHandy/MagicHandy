package motion

import (
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// minimumFocusWidthPercent keeps a requested area focus wide enough to still
// be motion. Measured across the whole built-in catalog at slow speed with
// whole-percent device output, a 20-point window is stationary for 0.2% of
// playback; 10 points reaches 1.0% with 643 ms stalls, and 5 points reaches a
// four-second dead stop. Narrower than this, "move here" becomes "stop moving".
const minimumFocusWidthPercent = 20

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
	// PatternWaves identifies a retired swelling-amplitude pattern.
	PatternWaves PatternID = "waves"
	// PatternClimb identifies a retired ratcheting-build pattern.
	PatternClimb PatternID = "climb"
	// PatternFlutter is the shallow-flutter-with-sweep pattern.
	PatternFlutter PatternID = "flutter"
	// PatternSway identifies a retired asymmetric broad-arc pattern.
	PatternSway PatternID = "sway"
	// PatternDrift migrates a consistent stroke window across a cycle.
	PatternDrift PatternID = "drift"
	// PatternDoubleTap identifies a retired paired-accent pattern.
	PatternDoubleTap PatternID = "double-tap"
	// PatternCascade identifies a retired descending-peak pattern.
	PatternCascade PatternID = "cascade"
	// PatternPendulum identifies a retired alternating centered-arc pattern.
	PatternPendulum PatternID = "pendulum"
	// PatternCradle identifies the retired restrained centered-arcs pattern.
	PatternCradle PatternID = "cradle"
	// PatternSurge identifies a retired decaying-echo pattern.
	PatternSurge PatternID = "surge"
	// PatternRolling identifies a retired layered medium-and-deep pattern.
	PatternRolling PatternID = "rolling"
	// PatternSyncopate identifies a retired uneven-rhythm pattern.
	PatternSyncopate PatternID = "syncopate"
	// PatternFourLevelCircuit cycles full and partial strokes across both zones.
	PatternFourLevelCircuit PatternID = "four-level-circuit"
	// PatternHighLowBlocks groups upper and lower zone pulses.
	PatternHighLowBlocks PatternID = "high-low-blocks"
	// PatternDeepShallowSequence mixes deep and medium upper-anchored strokes.
	PatternDeepShallowSequence PatternID = "deep-shallow-sequence"
	// PatternShortMediumSteps identifies a retired lower-anchored step pattern.
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
	// PatternDeepMediumShortPairs identifies a retired paired reach-band pattern.
	PatternDeepMediumShortPairs PatternID = "deep-medium-short-pairs"
	// PatternFallingCrest identifies a retired lowering-reversal pattern.
	PatternFallingCrest PatternID = "falling-crest"
	// PatternThreeDeepOneShort identifies a retired grouped broad-stroke pattern.
	PatternThreeDeepOneShort PatternID = "three-deep-one-short"
	// PatternDescendingLadder identifies a retired stepped-endpoint pattern.
	PatternDescendingLadder PatternID = "descending-ladder"
	// PatternWanderingSwell identifies a retired migrating center-and-reach pattern.
	PatternWanderingSwell PatternID = "wandering-swell"
	// PatternRisingReach progressively extends alternating upper reversals.
	PatternRisingReach PatternID = "rising-reach"
	// PatternHardAndRegular is a promoted user-curated full-range rhythm.
	PatternHardAndRegular PatternID = "hard-and-regular"
	// PatternPlayfulJerk is a promoted user-curated staggered full-range rhythm.
	PatternPlayfulJerk PatternID = "playful-jerk"

	// The velocity-authored replacement catalog. Every entry below is generated
	// from a stroke velocity rather than a free-hand travel time, so no pattern
	// can collapse to a crawl the way the descending shapes above did.

	// PatternEasingDown steps the whole stroke window lower at a held pace.
	PatternEasingDown PatternID = "easing-down"
	// PatternBuildingUp steps the whole stroke window higher at a held pace.
	PatternBuildingUp PatternID = "building-up"
	// PatternBroadAndTight answers one wide sweep with tight centered strokes.
	PatternBroadAndTight PatternID = "broad-and-tight"
	// PatternUpperAccents keeps quick accents in the upper range.
	PatternUpperAccents PatternID = "upper-accents"
	// PatternLowerAccents keeps quick accents in the lower range.
	PatternLowerAccents PatternID = "lower-accents"
	// PatternSteadyDrift holds one pace while the window wanders.
	PatternSteadyDrift PatternID = "steady-drift"
	// PatternOffbeat breaks even strokes with one displaced deeper reach.
	PatternOffbeat PatternID = "offbeat"
	// PatternNarrowing converges stroke width toward the center.
	PatternNarrowing PatternID = "narrowing"
	// PatternOpeningUp diverges stroke width away from the center.
	PatternOpeningUp PatternID = "opening-up"
	// PatternLongReturn pairs a quick reach with an unhurried return.
	PatternLongReturn PatternID = "long-return"
	// PatternSwell carries the window up and back across one long arc.
	PatternSwell PatternID = "swell"
	// PatternRocking repeats one even mid-range stroke at a fixed pace.
	PatternRocking PatternID = "rocking"
	// PatternSurgeAndSettle follows one full sweep with a long settled run.
	PatternSurgeAndSettle PatternID = "surge-and-settle"
	// PatternThreeAndOne resolves tight upper strokes with one full plunge.
	PatternThreeAndOne PatternID = "three-and-one"
	// PatternCrosscut trades long blocks of broad and tight strokes.
	PatternCrosscut PatternID = "crosscut"
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
	Label                  string             `json:"label,omitempty"`
	Source                 string             `json:"source,omitempty"`
	PatternID              PatternID          `json:"pattern_id,omitempty"`
	PatternName            string             `json:"pattern_name,omitempty"`
	ProgramID              string             `json:"program_id,omitempty"`
	MediaID                string             `json:"media_id,omitempty"`
	SpeedPercent           int                `json:"speed_percent"`
	MediaSpeedLimitEnabled bool               `json:"media_speed_limit_enabled,omitempty"`
	AreaFocus              *AreaFocus         `json:"area_focus,omitempty"`
	SoftAnchor             *SoftAnchor        `json:"soft_anchor,omitempty"`
	Dynamic                *DynamicDefinition `json:"dynamic,omitempty"`

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
	if target.Dynamic != nil {
		dynamic := NormalizeDynamicDefinition(*target.Dynamic)
		target.Dynamic = &dynamic
		target.PatternID = ""
		target.PatternName = DynamicMotionName
		target.ProgramID = ""
		target.MediaID = ""
		target.Pattern = nil
		target.Program = nil
		target.Media = nil
		target.AreaFocus = nil
		target.SoftAnchor = nil
	}
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
	if target.Dynamic == nil && target.PatternID == "" && target.ProgramID == "" && target.MediaID == "" {
		target.PatternID = PatternStroke
	}
	if target.SpeedPercent == 0 {
		target.SpeedPercent = defaultSpeedPercent
	}
	target.SpeedPercent = clamp(target.SpeedPercent, settings.SpeedMinPercent, settings.SpeedMaxPercent)
	target.AreaFocus = resolveAreaFocus(target)
	target.SoftAnchor = normalizeSoftAnchor(target.SoftAnchor)
	return target
}

// resolveAreaFocus keeps a requested zone in full-stroke coordinates so that
// normalizing a target twice is the same as normalizing it once. Clock-locked
// media is never focused — a video follows authored positions.
func resolveAreaFocus(target MotionTarget) *AreaFocus {
	if target.Media != nil {
		return nil
	}
	return normalizeAreaFocus(target.AreaFocus, 0, 100)
}

// effectiveAreaFocus returns the requested semantic area. Pattern projection
// later expands the pattern's own authored span to fill this area automatically.
func effectiveAreaFocus(target MotionTarget) *AreaFocus {
	if target.Media != nil {
		return nil
	}
	return normalizeAreaFocus(target.AreaFocus, 0, 100)
}

func normalizeAreaFocus(focus *AreaFocus, outerMin, outerMax int) *AreaFocus {
	if focus == nil {
		return nil
	}
	outerMin = clamp(outerMin, 0, 100)
	outerMax = clamp(outerMax, outerMin, 100)
	normalized := AreaFocus{
		MinPercent: clamp(focus.MinPercent, outerMin, outerMax),
		MaxPercent: clamp(focus.MaxPercent, outerMin, outerMax),
	}
	if normalized.MaxPercent < normalized.MinPercent {
		normalized.MinPercent, normalized.MaxPercent = normalized.MaxPercent, normalized.MinPercent
	}
	// A window covering the whole stroke is not a focus. Saying so here keeps
	// one rule true everywhere: unfocused content keeps its authored amplitude,
	// and only a deliberately narrowed region re-expands to fill itself.
	if normalized.MinPercent <= 0 && normalized.MaxPercent >= 100 {
		return nil
	}
	width := min(minimumFocusWidthPercent, outerMax-outerMin)
	if normalized.MaxPercent-normalized.MinPercent < width {
		center := (normalized.MinPercent + normalized.MaxPercent) / 2
		normalized.MinPercent = clamp(center-width/2, outerMin, outerMax-width)
		normalized.MaxPercent = normalized.MinPercent + width
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
