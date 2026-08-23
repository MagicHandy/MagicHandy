package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestPerceptualSummaryDescribesCompiledCreativeOutput(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	definition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 42, SpanMinPercent: 20,
		SpanProfile: DynamicSpanProfileBreathe, VariationPercent: 68,
	})
	plan := NewMotionPlan("perceptual", MotionTarget{
		Dynamic: &definition, SpeedPercent: 58,
	}, settings, 0, 0, time.Unix(0, 0))
	summary := plan.Perceptual
	if summary.CommandedMeanTravelPerSecond <= 0 || summary.CommandedPeakVelocityPerSecond <= 0 {
		t.Fatalf("compiled summary omitted pace: %+v", summary)
	}
	requested := referenceTravelRateForSpeed(58, settings.HandyModel)
	if math.Abs(summary.CommandedPeakVelocityPerSecond-requested) > requested*0.01 {
		t.Fatalf("compiled peak %.1f does not represent selected %.1f", summary.CommandedPeakVelocityPerSecond, requested)
	}
	if summary.MeanStrokePercent <= 0 || summary.MinimumLocalStrokeCV < 0.05 ||
		summary.MinimumLocalStrokeRange < 6 || summary.SpanProfile != DynamicSpanProfileBreathe {
		t.Fatalf("compiled summary omitted local length diversity: %+v", summary)
	}
}

func TestPerceptualDifferenceAccumulatesSmallEditsButIgnoresPace(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	baseDefinition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 42, SpanMinPercent: 20,
		SpanProfile: DynamicSpanProfileWander, VariationPercent: 68,
	})
	plan := func(definition DynamicDefinition, speed int) MotionPlan {
		return NewMotionPlan("perceptual-difference", MotionTarget{
			Dynamic: &definition, SpeedPercent: speed,
		}, settings, 0, 0, time.Unix(0, 0))
	}
	base := plan(baseDefinition, 42).Perceptual
	faster := plan(baseDefinition, 72).Perceptual
	if base.MateriallyDifferent(faster) {
		t.Fatalf("pace-only change became a new phrase: base=%+v faster=%+v", base, faster)
	}

	smallDefinition := baseDefinition
	smallDefinition.CenterPercent += 2
	smallDefinition.SpanPercent += 2
	smallDefinition.PhraseSeed = 0
	small := plan(NormalizeDynamicDefinition(smallDefinition), 42).Perceptual
	if base.MateriallyDifferent(small) {
		t.Fatalf("small scalar edit reset perceptual phrase age: base=%+v small=%+v", base, small)
	}

	largeDefinition := baseDefinition
	largeDefinition.CenterPercent = 64
	largeDefinition.SpanPercent = 68
	largeDefinition.SpanMinPercent = 24
	largeDefinition.SpanProfile = DynamicSpanProfileContrast
	largeDefinition.PhraseSeed = 0
	large := plan(NormalizeDynamicDefinition(largeDefinition), 42).Perceptual
	if !base.MateriallyDifferent(large) {
		t.Fatalf("macro reshape did not reset perceptual phrase age: base=%+v large=%+v", base, large)
	}

	wideDefinition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 42, SpanPercent: 80, SpanMinPercent: 30,
		SpanProfile: DynamicSpanProfileContrast, VariationPercent: 75,
	})
	nearFullDefinition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 94, SpanMinPercent: 36,
		SpanProfile: DynamicSpanProfileWander, VariationPercent: 82,
	})
	wide := plan(wideDefinition, 65).Perceptual
	nearFull := plan(nearFullDefinition, 65).Perceptual
	if !wide.MateriallyDifferent(nearFull) {
		t.Fatalf("near-full combined expansion remained cosmetic: wide=%+v near_full=%+v", wide, nearFull)
	}
}
