package motion

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestDefaultLoopPlaybackDoesNotIntroducePerceptibleDwells(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	const (
		velocityFloor    = 45.0
		maximumDipMillis = int64(250)
		sampleMillis     = int64(5)
		speedPercent     = 40
	)

	for _, definition := range BuiltinPatternDefinitions() {
		if slices.Contains(definition.Tags, TagExperimental) ||
			slices.Contains(definition.Tags, "midpoint-holds") {
			continue
		}
		plan := NewMotionPlan("runtime-dwell", MotionTarget{
			PatternID: definition.ID, Pattern: &definition, SpeedPercent: speedPercent,
		}, settings, 0, 0, time.Unix(0, 0))
		longestDip := longestLowVelocityInterval(
			plan.PeriodMillis,
			sampleMillis,
			velocityFloor,
			func(at int64) float64 { return plan.SampleAt(at).PositionPercent },
		)
		if longestDip <= maximumDipMillis {
			continue
		}
		t.Errorf(
			"%s at %d%% spends %dms continuously below %.0f%% travel/s; want at most %dms",
			definition.Name, speedPercent, longestDip, velocityFloor, maximumDipMillis,
		)
	}
}

func TestHardAndRegularCloudFrameKeepsMovingAfterTheUpstroke(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	definition, found := BuiltinPatternDefinition(PatternHardAndRegular)
	if !found {
		t.Fatal("Hard and Regular pattern is missing")
	}
	const sampleMillis = int64(5)
	for _, speedPercent := range []int{35, 40} {
		plan := NewMotionPlan("hard-regular-wire", MotionTarget{
			PatternID: definition.ID, Pattern: &definition, SpeedPercent: speedPercent,
		}, settings, 0, 0, time.Unix(0, 0))
		samples := catalogSamples(t, plan, 1)
		longestDip := longestLowVelocityInterval(
			plan.PeriodMillis,
			sampleMillis,
			45,
			func(at int64) float64 { return interpolateMotionSamples(samples, at) },
		)
		if longestDip > 250 {
			t.Fatalf("Cloud wire frame at %d%% spends %dms continuously below 45%% travel/s; want at most 250ms", speedPercent, longestDip)
		}
	}
}

func longestLowVelocityInterval(
	durationMillis int64,
	sampleMillis int64,
	velocityFloor float64,
	positionAt func(int64) float64,
) int64 {
	longestDip, currentDip := int64(0), int64(0)
	// Two cycles make a low-velocity interval crossing the loop seam visible as
	// one run instead of two shorter edge fragments.
	for at := sampleMillis; at <= durationMillis*2; at += sampleMillis {
		before := positionAt(at - sampleMillis)
		after := positionAt(at)
		velocity := math.Abs(after-before) * 1000 / float64(sampleMillis)
		if velocity < velocityFloor {
			currentDip += sampleMillis
			longestDip = max(longestDip, currentDip)
			continue
		}
		currentDip = 0
	}
	return longestDip
}
