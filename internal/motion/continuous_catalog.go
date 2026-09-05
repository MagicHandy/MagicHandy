package motion

import (
	"fmt"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

const (
	// PatternFullSweeps is the default continuous full-band carrier.
	PatternFullSweeps PatternID = "flow-full-sweeps"
	// PatternBaseVariation varies reach while retaining the lower return.
	PatternBaseVariation PatternID = "flow-base-anchored"
	// PatternPaceWave varies pace without changing full-band geometry.
	PatternPaceWave PatternID = "flow-pace-wave"
	// TagDeprecated keeps legacy content playable but out of model selection.
	TagDeprecated = "deprecated"
)

// ContinuousRecipe is a distinct movement behavior, independent of pace.
// IDs and descriptions are also the compact model-facing action vocabulary.
type ContinuousRecipe struct {
	ID          PatternID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Spec        FlowSpec  `json:"spec"`
}

// ContinuousRecipes returns fresh scores so callers cannot mutate the catalog.
func ContinuousRecipes(speed int) []ContinuousRecipe {
	base := DefaultFlowSpec()
	base.MinPercent, base.MaxPercent, base.RangeFloorPercent = 0, 100, 100
	base.SpeedPercent, base.PaceVariationPercent, base.MemoryCycles = speed, 0, 8
	base.LoopCycles = 8
	fixed := func(lo, hi int) FlowSpec {
		spec := base
		spec.MinPercent, spec.MaxPercent, spec.RangeFloorPercent = lo, hi, hi-lo
		spec.LoopCycles = 12
		return spec
	}
	variable := func(anchor int) FlowSpec {
		spec := base
		spec.AnchorPercent, spec.RangeFloorPercent = anchor, 25
		spec.LoopCycles = 16
		return spec
	}
	travel := base
	travel.RangeFloorPercent, travel.RangeCeilingPercent, travel.AnchorPercent = 40, 40, 50
	travel.LoopCycles = 16
	travel.Layers = []FlowLayer{{Axis: "center", AmountPercent: 100, PeriodCycles: 16, PhasePercent: 50}}
	contrast := base
	contrast.Steps = []FlowStep{{0, 100, speed, 4}, {30, 70, speed, 4}}
	pace := base
	pace.Layers = []FlowLayer{{Axis: "pace", AmountPercent: 70, PeriodCycles: 8}}
	return []ContinuousRecipe{
		{PatternFullSweeps, "Full sweeps", "Repeat smooth full-length strokes from base 0 to tip 100. Fixed reach and even pace.", base},
		{"flow-lower-strokes", "Lower strokes", "Repeat smooth fixed-width strokes only in the lower/base region, 0–40. Even pace.", fixed(0, 40)},
		{"flow-middle-strokes", "Middle strokes", "Repeat smooth fixed-width strokes only around the middle, 30–70. Even pace.", fixed(30, 70)},
		{"flow-upper-strokes", "Upper strokes", "Repeat smooth fixed-width strokes only in the upper/tip region, 60–100. Even pace.", fixed(60, 100)},
		{PatternBaseVariation, "Base-anchored variety", "Return to base 0 every stroke while reach gradually varies between short and broad. Pace stays independent.", variable(0)},
		{"flow-tip-anchored", "Tip-anchored variety", "Return to tip 100 every stroke while reach gradually varies between short and broad. Pace stays independent.", variable(100)},
		{"flow-centered-variety", "Centered variety", "Gradually vary stroke width symmetrically around the middle. Both endpoints move; neither endpoint is anchored.", variable(50)},
		{"flow-traveling-window", "Traveling window", "Keep a 40-wide stroke window and gradually move it from lower to middle to upper and back. Preserve stroke width.", travel},
		{"flow-wide-narrow", "Wide then narrow", "Repeat four full-width cycles, then four middle-width cycles. Blend between the two sections; keep the pace setting.", contrast},
		{PatternPaceWave, "Pace wave", "Keep every stroke full-length while pace gradually slows and rises over eight cycles. Reach does not change.", pace},
	}
}

// ContinuousRecipeByID resolves only the reviewed canonical action IDs.
func ContinuousRecipeByID(id PatternID, speed int) (ContinuousRecipe, bool) {
	if !strings.HasPrefix(string(id), "flow-") {
		return ContinuousRecipe{}, false
	}
	for _, recipe := range ContinuousRecipes(speed) {
		if recipe.ID == id {
			return recipe, true
		}
	}
	return ContinuousRecipe{}, false
}

func buildContinuousPatternDefinitions() []PatternDefinition {
	definitions := make([]PatternDefinition, 0, 10)
	for _, recipe := range ContinuousRecipes(25) {
		curve, err := compileFlowCurve(recipe.Spec, config.HandyModelOriginal)
		if err != nil {
			panic(fmt.Sprintf("continuous recipe %s: %v", recipe.ID, err))
		}
		// Stored guides describe complete strokes. Playback resolves the recipe;
		// the library exports a dense bake sampled from that actual plan.
		points := append([]CurvePoint(nil), curve.authoredKnots...)
		definitions = append(definitions, PatternDefinition{ID: recipe.ID, Name: recipe.Name,
			Description: recipe.Description, Kind: PatternKindRoutine, CycleMillis: curve.duration,
			Points: points, Tags: []string{"continuous", "smooth"}, recipeID: recipe.ID})
	}
	return definitions
}

// ContinuousPatternDefinition supplies canonical content to the durable library.
func ContinuousPatternDefinition(id PatternID) (PatternDefinition, bool) {
	if _, ok := ContinuousRecipeByID(id, 25); !ok {
		return PatternDefinition{}, false
	}
	return BuiltinPatternDefinition(id)
}

func prepareContinuousRecipe(target MotionTarget, settings config.MotionSettings) (MotionTarget, error) {
	recipe, ok := ContinuousRecipeByID(target.PatternID, target.SpeedPercent)
	if !ok || target.Dynamic != nil || target.Program != nil || target.Media != nil {
		return target, nil
	}
	if target.Pattern != nil && target.Pattern.recipeID == "" {
		return target, nil
	}
	compiled, err := FlowTarget(recipe.Spec, settings)
	if err != nil {
		return target, err
	}
	compiled.prepared.id, compiled.prepared.name = string(recipe.ID), recipe.Name
	compiled.prepared.libraryID = recipe.ID
	target.prepared = compiled.prepared
	target.Pattern, target.Dynamic = nil, nil
	target.PatternName = recipe.Name
	return target, nil
}

// ContinuousPatternPreview samples the real playback plan at a declared
// reference pace; the library must not show a different PCHIP reconstruction.
func ContinuousPatternPreview(id PatternID) ([]CurvePoint, int64, bool) {
	if _, ok := ContinuousRecipeByID(id, 25); !ok {
		return nil, 0, false
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	plan := NewMotionPlan("library-preview", MotionTarget{PatternID: id, SpeedPercent: 25}, settings, 0, 0, time.Unix(0, 0))
	points := make([]CurvePoint, 0, 258)
	for at := int64(0); at < plan.PeriodMillis; at += max(int64(10), plan.PeriodMillis/256) {
		points = append(points, CurvePoint{TimeMillis: at, PositionPercent: plan.SampleAt(at).PositionPercent})
	}
	points = append(points, CurvePoint{TimeMillis: plan.PeriodMillis, PositionPercent: plan.SampleAt(plan.PeriodMillis).PositionPercent})
	return points, plan.PeriodMillis, true
}
