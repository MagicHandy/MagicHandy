package config

import (
	"errors"
	"fmt"
)

const (
	// FreestyleFeelGentle is slow, long, calm strokes.
	FreestyleFeelGentle = "gentle"
	// FreestyleFeelBalanced is the default mixed feel.
	FreestyleFeelBalanced = "balanced"
	// FreestyleFeelIntense is quick, lively strokes.
	FreestyleFeelIntense = "intense"
	// FreestyleFeelCustom keeps the user's own control values.
	FreestyleFeelCustom = "custom"

	// FreestyleAccentEven times both directions alike.
	FreestyleAccentEven = "even"
	// FreestyleAccentTip makes strokes toward the tip quicker.
	FreestyleAccentTip = "tip"
	// FreestyleAccentBase makes strokes toward the base quicker.
	FreestyleAccentBase = "base"

	// FreestyleShapeSteady keeps the chosen pace for the whole session.
	FreestyleShapeSteady = "steady"
	// FreestyleShapeBuild starts slow and builds to the chosen pace over the
	// shape duration, then holds it.
	FreestyleShapeBuild = "build"
	// FreestyleShapeWaves rises and falls in slow waves of a few minutes.
	FreestyleShapeWaves = "waves"
	// FreestyleShapeEdge builds toward the chosen pace, backs off into
	// unhurried teasing, and builds again.
	FreestyleShapeEdge = "edge"
	// FreestyleShapeCooldown eases down over the shape duration and then
	// comes to rest, ending Freestyle.
	FreestyleShapeCooldown = "cooldown"

	// FreestyleMinimumShapeMinutes bounds Slow build and Cooldown.
	FreestyleMinimumShapeMinutes = 1
	// FreestyleMaximumShapeMinutes bounds Slow build and Cooldown.
	FreestyleMaximumShapeMinutes = 240
	// FreestyleDefaultShapeMinutes is the first-run shape duration.
	FreestyleDefaultShapeMinutes = 15
)

// FreestyleSettings are Freestyle's durable preferences. The running stream
// reads them live; its session clock, seed and control ramps are runtime state
// and never belong in the settings document. Every value shapes motion inside
// the saved speed and stroke limits; none of them can widen those limits.
type FreestyleSettings struct {
	// Feel names a preset for the control values below, or custom.
	Feel           string `json:"feel"`
	PacePercent    int    `json:"pace_percent"`
	LengthPercent  int    `json:"length_percent"`
	FocusPercent   int    `json:"focus_percent"`
	RoamingPercent int    `json:"roaming_percent"`
	VarietyPercent int    `json:"variety_percent"`
	Accent         string `json:"accent"`
	// Shape is the visible, user-chosen session arc.
	Shape        string `json:"shape"`
	ShapeMinutes int    `json:"shape_minutes"`
}

// FreestyleFeelPreset returns the control values a named feel stands for. The
// shape is not part of a feel. Values sit on the UI's quarter steps.
func FreestyleFeelPreset(feel string) (FreestyleSettings, bool) {
	switch feel {
	case FreestyleFeelGentle:
		return FreestyleSettings{Feel: feel, PacePercent: 25, LengthPercent: 75, FocusPercent: 50,
			RoamingPercent: 25, VarietyPercent: 25, Accent: FreestyleAccentEven}, true
	case FreestyleFeelBalanced:
		return FreestyleSettings{Feel: feel, PacePercent: 50, LengthPercent: 75, FocusPercent: 50,
			RoamingPercent: 50, VarietyPercent: 50, Accent: FreestyleAccentEven}, true
	case FreestyleFeelIntense:
		return FreestyleSettings{Feel: feel, PacePercent: 75, LengthPercent: 50, FocusPercent: 50,
			RoamingPercent: 50, VarietyPercent: 75, Accent: FreestyleAccentEven}, true
	}
	return FreestyleSettings{}, false
}

// DefaultFreestyleSettings returns the first-run Freestyle preferences.
func DefaultFreestyleSettings() FreestyleSettings {
	settings, _ := FreestyleFeelPreset(FreestyleFeelBalanced)
	settings.Shape = FreestyleShapeSteady
	settings.ShapeMinutes = FreestyleDefaultShapeMinutes
	return settings
}

// normalizeFreestyleSettings fills a section saved before it existed and
// makes a named feel authoritative over its control values, so the document
// can never show one feel while playing another.
func normalizeFreestyleSettings(settings FreestyleSettings) FreestyleSettings {
	if settings.Feel == "" {
		return DefaultFreestyleSettings()
	}
	if preset, ok := FreestyleFeelPreset(settings.Feel); ok {
		preset.Shape, preset.ShapeMinutes = settings.Shape, settings.ShapeMinutes
		settings = preset
	}
	if settings.Shape == "" {
		settings.Shape = FreestyleShapeSteady
	}
	if settings.ShapeMinutes == 0 {
		settings.ShapeMinutes = FreestyleDefaultShapeMinutes
	}
	return settings
}

// Existing installs used Motion.Style for Freestyle. Preserve that choice
// when introducing its separate preferences; new installs start balanced.
func applyMissingFreestyleDefaults(settings FreestyleSettings, legacyStyle string) FreestyleSettings {
	if settings.Feel == "" {
		settings = DefaultFreestyleSettings()
		if preset, ok := FreestyleFeelPreset(legacyStyle); ok {
			preset.Shape, preset.ShapeMinutes = settings.Shape, settings.ShapeMinutes
			settings = preset
		}
	}
	return normalizeFreestyleSettings(settings)
}

func validateFreestyleSettings(settings FreestyleSettings) error {
	if !oneOf(settings.Feel, FreestyleFeelGentle, FreestyleFeelBalanced, FreestyleFeelIntense, FreestyleFeelCustom) {
		return fmt.Errorf("unknown Freestyle feel %q", settings.Feel)
	}
	for _, value := range []int{settings.PacePercent, settings.LengthPercent, settings.FocusPercent,
		settings.RoamingPercent, settings.VarietyPercent} {
		if value < 0 || value > 100 {
			return errors.New("freestyle controls must be between 0 and 100")
		}
	}
	if !oneOf(settings.Accent, FreestyleAccentEven, FreestyleAccentTip, FreestyleAccentBase) {
		return fmt.Errorf("unknown Freestyle accent %q", settings.Accent)
	}
	if !oneOf(settings.Shape, FreestyleShapeSteady, FreestyleShapeBuild, FreestyleShapeWaves,
		FreestyleShapeEdge, FreestyleShapeCooldown) {
		return fmt.Errorf("unknown Freestyle shape %q", settings.Shape)
	}
	if settings.ShapeMinutes < FreestyleMinimumShapeMinutes || settings.ShapeMinutes > FreestyleMaximumShapeMinutes {
		return fmt.Errorf("freestyle shape duration must be between %d and %d minutes",
			FreestyleMinimumShapeMinutes, FreestyleMaximumShapeMinutes)
	}
	return nil
}
