package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestReferenceTravelRateCalibration(t *testing.T) {
	tests := []struct {
		model        string
		speed        int
		wantPhysical float64
	}{
		{model: config.HandyModelOriginal, speed: 1, wantPhysical: 32},
		{model: config.HandyModelOriginal, speed: 73, wantPhysical: 32 + (400-32)*72.0/99.0},
		{model: config.HandyModelOriginal, speed: 100, wantPhysical: 400},
		{model: config.HandyModel2Standard, speed: 1, wantPhysical: 32},
		{model: config.HandyModel2Standard, speed: 100, wantPhysical: 400},
		{model: config.HandyModel2Pro, speed: 1, wantPhysical: 32},
		{model: config.HandyModel2Pro, speed: 100, wantPhysical: 450},
	}
	for _, test := range tests {
		profile := handySpeedProfileFor(test.model)
		gotPhysical := referenceTravelRateForSpeed(test.speed, test.model) * profile.strokeMillimeters / 100
		if math.Abs(gotPhysical-test.wantPhysical) > 1e-9 {
			t.Errorf("model %s speed %d = %.12fmm/s, want %.12f", test.model, test.speed, gotPhysical, test.wantPhysical)
		}
	}
}

func TestCalibratedStrokeAt73PercentRequestsReferenceRate(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	definition, _ := BuiltinPatternDefinition(PatternStroke)
	plan := NewMotionPlan("calibration", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 73,
	}, settings, 0, 0, time.Unix(0, 0))
	got := totalCurveTravel(definition.Points) * 1000 / float64(plan.PeriodMillis)
	if math.Abs(got-referenceTravelRateForSpeed(73, settings.HandyModel)) > 0.1 {
		t.Fatalf("73%% stroke rate = %.2f%% travel/s, want calibrated %.2f",
			got, referenceTravelRateForSpeed(73, settings.HandyModel))
	}
}

func TestHandyModelChangesSemanticRateToPreservePhysicalCalibration(t *testing.T) {
	original := handySpeedProfileFor(config.HandyModelOriginal)
	standard := handySpeedProfileFor(config.HandyModel2Standard)
	for _, speed := range []int{1, 20, 73, 100} {
		originalPhysical := referenceTravelRateForSpeed(speed, config.HandyModelOriginal) *
			original.strokeMillimeters / 100
		standardPhysical := referenceTravelRateForSpeed(speed, config.HandyModel2Standard) *
			standard.strokeMillimeters / 100
		if math.Abs(originalPhysical-standardPhysical) > 1e-9 {
			t.Fatalf("speed %d physical rate differs: original %.3fmm/s standard %.3fmm/s",
				speed, originalPhysical, standardPhysical)
		}
	}
}

func TestMotionPlanUsesSelectedHandyModelProfile(t *testing.T) {
	definition, _ := BuiltinPatternDefinition(PatternStroke)
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings := config.DefaultSettings().Motion
		settings.SpeedMaxPercent = 100
		settings.HandyModel = model
		plan := NewMotionPlan("profile", MotionTarget{
			PatternID: definition.ID, Pattern: &definition, SpeedPercent: 73,
		}, settings, 0, 0, time.Unix(0, 0))
		profile := handySpeedProfileFor(model)
		semanticRate := totalCurveTravel(definition.Points) * 1000 / float64(plan.PeriodMillis)
		physicalRate := semanticRate * profile.strokeMillimeters / 100
		wantPhysical := 32 + (profile.maximumMMPerSecond-32)*72.0/99.0
		if math.Abs(physicalRate-wantPhysical) > 0.2 {
			t.Errorf("model %s 73%% plan = %.2fmm/s, want %.2f", model, physicalRate, wantPhysical)
		}
	}
}

func TestFinitePlanSamplingPreservesFractionalAuthoredTime(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	definition := ProgramDefinition{
		ID: "slow", Name: "Slow", DurationMillis: 1000,
		Points: []CurvePoint{{TimeMillis: 0, PositionPercent: 0}, {TimeMillis: 1000, PositionPercent: 100}},
	}
	plan := NewMotionPlan("slow", MotionTarget{
		ProgramID: definition.ID, Program: &definition, SpeedPercent: 1,
	}, settings, 0, 0, time.Unix(0, 0))
	first := plan.SampleAt(10).PositionPercent
	second := plan.SampleAt(20).PositionPercent
	if !(first > 0 && second > first) {
		t.Fatalf("fractional authored samples = %.9f then %.9f, want continuous progress", first, second)
	}
}
