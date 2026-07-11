package motion

import (
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

const defaultSpeedPercent = 50

// PatternID identifies a repeatable semantic motion pattern.
type PatternID string

const (
	// PatternStroke is the default full-stroke triangle pattern.
	PatternStroke PatternID = "stroke"
	// PatternPulse is a fixed double-peak pattern.
	PatternPulse PatternID = "pulse"
	// PatternTease is a fixed shallow-to-deep pattern.
	PatternTease PatternID = "tease"
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
	Label        string      `json:"label,omitempty"`
	Source       string      `json:"source,omitempty"`
	PatternID    PatternID   `json:"pattern_id,omitempty"`
	ProgramID    string      `json:"program_id,omitempty"`
	SpeedPercent int         `json:"speed_percent"`
	AreaFocus    *AreaFocus  `json:"area_focus,omitempty"`
	SoftAnchor   *SoftAnchor `json:"soft_anchor,omitempty"`

	// Resolved content is backend-owned and never serialized to clients. The
	// public IDs above remain the authoritative snapshot vocabulary.
	Pattern *PatternDefinition `json:"-"`
	Program *ProgramDefinition `json:"-"`
}

// NormalizeTarget clamps semantic intent without applying physical stroke settings.
func NormalizeTarget(target MotionTarget, settings config.MotionSettings) MotionTarget {
	target.Label = strings.TrimSpace(target.Label)
	target.Source = strings.TrimSpace(target.Source)
	if target.Source == "" {
		target.Source = "motion"
	}
	target.ProgramID = strings.TrimSpace(target.ProgramID)
	if target.Program != nil {
		target.ProgramID = strings.TrimSpace(target.Program.ID)
		target.PatternID = ""
	}
	if target.Pattern != nil {
		target.PatternID = target.Pattern.ID
		target.ProgramID = ""
		target.Program = nil
	}
	if target.PatternID == "" && target.ProgramID == "" {
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
