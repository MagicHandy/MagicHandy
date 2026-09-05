package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func legacyPointCatalogForTest() []PatternDefinition {
	var definitions []PatternDefinition
	for _, definition := range BuiltinPatternDefinitions() {
		if definition.recipeID == "" {
			definitions = append(definitions, definition)
		}
	}
	return definitions
}

func TestContinuousCatalogUsesKinematicCompilerAndPreservesRuntimeLimits(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		for _, recipe := range ContinuousRecipes(25) {
			for _, speed := range []int{10, 25, 43, 85} {
				definition, ok := BuiltinPatternDefinition(recipe.ID)
				if !ok {
					t.Fatal("missing recipe", recipe.ID)
				}
				plan := NewMotionPlan("catalog-test", MotionTarget{PatternID: recipe.ID, Pattern: &definition, SpeedPercent: speed}, settings, 0, 0, time.Unix(0, 0))
				if plan.compileErr != nil || plan.Target.prepared == nil || plan.Target.Dynamic != nil || plan.Target.PatternID != recipe.ID {
					t.Fatalf("recipe bypassed kinematic compilation: %+v %v", plan.Target, plan.compileErr)
				}
				if acceleration := maximumPlanAcceleration(plan); acceleration > flowAccelerationBudget*1.001 {
					t.Fatalf("%s %d acceleration %.1f", recipe.ID, speed, acceleration)
				}
				if jerk := maximumPlanJerk(plan); jerk > flowJerkBudget*1.001 {
					t.Fatalf("%s %d jerk %.1f", recipe.ID, speed, jerk)
				}
				if plan.Perceptual.CommandedPeakVelocityPerSecond > referenceTravelRateForSpeed(100, model)*1.001 {
					t.Fatalf("%s exceeds device speed", recipe.ID)
				}
				for _, segment := range plan.curve.quintics {
					if math.IsNaN(segment.position(.5)) {
						t.Fatal("non-finite curve")
					}
				}
				limited := settings
				limited.SpeedMaxPercent = 15
				retargeted := NewMotionPlan("limited", plan.Target, limited, 0, 0, time.Unix(0, 0))
				if retargeted.Target.SpeedPercent > 15 || retargeted.Perceptual.CommandedMeanTravelPerSecond > referenceTravelRateForSpeed(15, model)*1.001 {
					t.Fatalf("%s escaped live limits", recipe.ID)
				}
			}
		}
	}
}

func TestContinuousCatalogNamesMatchGeometry(t *testing.T) {
	settings := config.DefaultSettings().Motion
	for _, test := range []struct {
		id     PatternID
		lo, hi float64
	}{
		{PatternFullSweeps, 0, 100}, {"flow-lower-strokes", 0, 40}, {"flow-middle-strokes", 30, 70}, {"flow-upper-strokes", 60, 100},
	} {
		plan := NewMotionPlan("geometry", MotionTarget{PatternID: test.id, SpeedPercent: 25}, settings, 0, 0, time.Unix(0, 0))
		if math.Abs(plan.Perceptual.PositionMinPercent-test.lo) > .01 || math.Abs(plan.Perceptual.PositionMaxPercent-test.hi) > .01 {
			t.Fatalf("%s does not reach its named band: %+v", test.id, plan.Perceptual)
		}
	}
	for _, recipe := range ContinuousRecipes(25) {
		plan := NewMotionPlan("wire", MotionTarget{PatternID: recipe.ID, SpeedPercent: 25}, settings, 0, 0, time.Unix(0, 0))
		engine := &Engine{plan: plan, settings: settings, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
			preservePlanKnots: true, positionResolutionPercent: 1, maximumChunkPoints: maximumAdaptiveChunkPoints}
		for range 6 {
			previous := engine.lastSample
			points, err := engine.nextMotionSamplesLocked()
			if err != nil {
				t.Fatal(recipe.ID, err)
			}
			if previous != nil {
				points = append([]MotionSample{*previous}, points...)
			}
			assertNoStationaryWireEdges(t, string(recipe.ID), points, 1)
			assertQuantizedSamplesTrackPath(t, string(recipe.ID), points, 1, func(at int64) float64 { return plan.SampleAt(at).PositionPercent })
		}
	}
}

func TestContinuousRecipeFocusPreservesShapeAndReportsTheZone(t *testing.T) {
	settings := config.DefaultSettings().Motion
	focus := &AreaFocus{MinPercent: 66, MaxPercent: 100}
	plan := NewMotionPlan("focus", MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 25, AreaFocus: focus}, settings, 0, 0, time.Unix(0, 0))
	if plan.Target.AreaFocus == nil || *plan.Target.AreaFocus != *focus || plan.Perceptual.PositionMinPercent < 65.99 || plan.Perceptual.PositionMaxPercent > 100.01 {
		t.Fatalf("focus was lost: %+v", plan)
	}
	if maximumPlanAcceleration(plan) > flowAccelerationBudget*1.001 || maximumPlanJerk(plan) > flowJerkBudget*1.001 {
		t.Fatal("focused recipe exceeded derivative budgets")
	}
}
