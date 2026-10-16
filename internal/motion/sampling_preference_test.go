package motion

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func preferredSamples(t *testing.T, target MotionTarget, floor, preference time.Duration) []MotionSample {
	t.Helper()
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	owner := &timingCapabilityTransport{Fake: transport.NewFake(), minimumInterval: floor, preferredInterval: preference}
	engine, err := NewEngine(EngineOptions{Transport: owner})
	if err != nil {
		t.Fatal(err)
	}
	engine.plan = NewMotionPlan("preference", target, settings, 0, 0, time.Unix(0, 0))
	engine.settings, engine.positionResolutionPercent = settings, 1
	var points []MotionSample
	for engine.nextSampleMillis < 12000 {
		chunk, err := engine.nextMotionSamplesLocked()
		if err != nil {
			t.Fatal(err)
		}
		points = append(points, chunk...)
	}
	for i := 1; i < len(points); i++ {
		left, right := points[i-1], points[i]
		duration := right.TimeMillis - left.TimeMillis
		if duration < max(int64(1), floor.Milliseconds()) {
			t.Fatalf("preference violated hard spacing across appends: %dms", duration)
		}
		velocity := math.Abs(math.Round(right.PositionPercent)-math.Round(left.PositionPercent)) * 1000 / float64(duration)
		if velocity > referenceTravelRateForSpeed(100, settings.HandyModel)+1e-8 {
			t.Fatalf("encoded velocity exceeds the shared ceiling: %v", velocity)
		}
	}
	return points
}

func TestSamplingPreferenceReducesUpdatesWithoutLosingReversals(t *testing.T) {
	target := MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 35}
	baseline := preferredSamples(t, target, 110*time.Millisecond, 0)
	preferred := preferredSamples(t, target, 110*time.Millisecond, 180*time.Millisecond)
	if len(preferred) >= len(baseline) {
		t.Fatalf("preferred fit did not reduce updates: %d >= %d", len(preferred), len(baseline))
	}
	for _, index := range motionSampleReversalAnchors(baseline) {
		point := baseline[index]
		if point.TimeMillis == 0 || point.TimeMillis >= 12000 {
			continue
		}
		found := false
		for _, candidate := range preferred {
			found = found || (candidate.TimeMillis == point.TimeMillis && candidate.PositionPercent == point.PositionPercent)
		}
		if !found {
			t.Fatalf("lost reversal %+v", point)
		}
	}
}

func TestSamplingPreferenceAllowsFastCreativeTurns(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	spec := DefaultFlowSpec()
	spec.MinPercent, spec.MaxPercent, spec.SpeedPercent = 0, 100, 75
	gesture := DefaultGestureSpec()
	spec.Gesture = &gesture
	target, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	points := preferredSamples(t, target, 110*time.Millisecond, 180*time.Millisecond)
	foundShort := false
	for index := 1; index < len(points); index++ {
		foundShort = foundShort || points[index].TimeMillis-points[index-1].TimeMillis < 180
	}
	if !foundShort {
		t.Fatal("fast Creative v2 turns were stretched to the preferred interval")
	}
	probes := []MotionSample{{TimeMillis: 0}, {TimeMillis: 100, PositionPercent: 50}, {TimeMillis: 300}}
	if _, _, err := fitSpacedMotionSamplesPreferred(probes, 110, 180); err == nil {
		t.Fatal("preference bypassed an unrepresentable reversal")
	}
}

func TestSamplingPreferenceLeavesBufferedAndImportedOutputUnchanged(t *testing.T) {
	target := MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 35}
	baseline := preferredSamples(t, target, 0, 0)
	preferred := preferredSamples(t, target, 0, 180*time.Millisecond)
	if !reflect.DeepEqual(baseline, preferred) {
		t.Fatal("preference changed buffered output")
	}
	timeline := MediaTimelineDefinition{ID: "authored", Name: "Authored", DurationMillis: 20000, Points: []CurvePoint{
		{TimeMillis: 0}, {TimeMillis: 400, PositionPercent: 50}, {TimeMillis: 2000, PositionPercent: 100},
		{TimeMillis: 6000}, {TimeMillis: 12000, PositionPercent: 100}, {TimeMillis: 20000},
	}}
	target = MotionTarget{Source: TargetSourceMedia, Media: &timeline}
	baseline = preferredSamples(t, target, 110*time.Millisecond, 0)
	preferred = preferredSamples(t, target, 110*time.Millisecond, 180*time.Millisecond)
	if !reflect.DeepEqual(baseline, preferred) {
		t.Fatal("preference changed the authored media fit")
	}
}
