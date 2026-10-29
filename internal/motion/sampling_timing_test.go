package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestBufferedQuantizationUsesCrossingsWithoutInventingSpeedPulses(t *testing.T) {
	var probes []MotionSample
	for at := int64(0); at <= 1000; at += 5 {
		probes = append(probes, MotionSample{TimeMillis: at, PositionPercent: float64(at) / 100})
	}
	samples := []MotionSample{{TimeMillis: 0}, {TimeMillis: 240, PositionPercent: 2.4},
		{TimeMillis: 540, PositionPercent: 5.4}, {TimeMillis: 760, PositionPercent: 7.6}, {TimeMillis: 1000, PositionPercent: 10}}
	fitted, err := fitQuantizedMotionTiming(samples, probes, map[int64]struct{}{0: {}, 1000: {}}, 1, 400, samplingTolerance{position: wireApproximationTolerance}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int64{0, 200, 500, 800, 1000} {
		if fitted[i].TimeMillis != want {
			t.Fatalf("point %d retained a speed pulse: at %d, want rounded-position crossing %d", i, fitted[i].TimeMillis, want)
		}
	}
	for i := 1; i < len(fitted); i++ {
		left, right := fitted[i-1], fitted[i]
		velocity := (math.Round(right.PositionPercent) - math.Round(left.PositionPercent)) * 1000 / float64(right.TimeMillis-left.TimeMillis)
		if math.Abs(velocity-10) > 1e-9 {
			t.Fatalf("constant-rate input pulsed at %f", velocity)
		}
	}
	// Authored anchors and paced transports must retain their timing.
	for _, floor := range []int64{1, 110} {
		mandatory := map[int64]struct{}{0: {}, 240: {}, 540: {}, 760: {}, 1000: {}}
		if floor > 1 {
			mandatory = map[int64]struct{}{0: {}, 1000: {}}
		}
		fixed, err := fitQuantizedMotionTiming(samples, probes, mandatory, 1, 400, samplingTolerance{position: wireApproximationTolerance}, floor)
		if err != nil {
			t.Fatal(err)
		}
		for i := range samples {
			if fixed[i].TimeMillis != samples[i].TimeMillis {
				t.Fatal("retimed a mandatory or paced point")
			}
		}
	}
}

func TestImmediateSamplingPreservesReversalAndTimingAcrossAppends(t *testing.T) {
	for _, floor := range []int64{30, 50, 100, 110} {
		settings := config.DefaultSettings().Motion
		plan := NewMotionPlan("immediate", MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 43}, settings, 0, 0, time.Unix(0, 0))
		owner := &timingCapabilityTransport{Fake: transport.NewFake(), minimumInterval: time.Duration(floor) * time.Millisecond}
		engine, err := NewEngine(EngineOptions{Transport: owner})
		if err != nil {
			t.Fatal(err)
		}
		engine.plan, engine.settings, engine.positionResolutionPercent = plan, settings, 1
		var all []MotionSample
		for engine.nextSampleMillis < 12000 {
			chunk, err := engine.nextMotionSamplesLocked()
			if err != nil {
				t.Fatalf("floor %d: %v", floor, err)
			}
			all = append(all, chunk...)
		}
		for i := 1; i < len(all); i++ {
			if dt := all[i].TimeMillis - all[i-1].TimeMillis; dt < floor {
				t.Fatalf("floor %d crossed append boundary: %dms", floor, dt)
			}
		}
		// A timing floor must preserve the full-width destination, rather than
		// missing the peak on the former 125ms grid.
		found := false
		for _, point := range all {
			if point.TimeMillis > 500 && point.TimeMillis < 700 && point.PositionPercent > 99.99 {
				found = true
			}
		}
		if !found {
			t.Fatalf("floor %d lost first exact peak", floor)
		}
	}
}

func TestSpacedSamplingRejectsUnrepresentableDirectionChanges(t *testing.T) {
	probes := []MotionSample{{TimeMillis: 0, PositionPercent: 0}, {TimeMillis: 20, PositionPercent: 100}, {TimeMillis: 40, PositionPercent: 0}, {TimeMillis: 100, PositionPercent: 100}}
	if _, _, err := fitSpacedMotionSamples(probes, 50); err == nil {
		t.Fatal("rapid reversals were silently aliased")
	}
}

func TestImmediateMediaDispatchFitsDenseAuthoredPoints(t *testing.T) {
	settings := config.DefaultSettings().Motion
	timeline := MediaTimelineDefinition{ID: "dense-straight", Name: "Dense straight", DurationMillis: 2000, Points: []CurvePoint{
		{TimeMillis: 0, PositionPercent: 0}, {TimeMillis: 10, PositionPercent: 1},
		{TimeMillis: 20, PositionPercent: 2}, {TimeMillis: 1000, PositionPercent: 100},
		{TimeMillis: 2000, PositionPercent: 0},
	}}
	plan := NewMotionPlan("immediate-media", MotionTarget{Source: TargetSourceMedia, Media: &timeline}, settings, 0, 0, time.Unix(0, 0))
	if plan.compileErr != nil || plan.Target.Media == nil {
		t.Fatalf("media fixture did not compile: %v", plan.compileErr)
	}
	engine := &Engine{plan: plan, settings: settings, running: true, runEpoch: 1,
		chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval, preservePlanKnots: true,
		minimumPointIntervalMillis: 50, maximumChunkPoints: 128}
	_, points, _, err := engine.nextChunkThrough(1, 1500)
	if err != nil {
		t.Fatal(err)
	}
	foundPeak := false
	for index, point := range points {
		if index > 0 && point.TimeMillis-points[index-1].TimeMillis < 50 {
			t.Fatal("linear media bypassed the immediate command interval")
		}
		foundPeak = foundPeak || point.TimeMillis == 1000 && point.PositionPercent == 100
	}
	if !foundPeak {
		t.Fatalf("lost authored reversal: %+v", points)
	}
}

func TestImmediateCatalogKeepsSpacingWithoutLosingBufferedCoverage(t *testing.T) {
	settings := config.DefaultSettings().Motion
	for _, definition := range BuiltinPatternDefinitions() {
		if definition.recipeID == "" {
			continue
		}
		for _, speed := range []int{10, 25, 43} {
			plan := NewMotionPlan("immediate-catalog", MotionTarget{PatternID: definition.ID, Pattern: &definition, SpeedPercent: speed}, settings, 0, 0, time.Unix(0, 0))
			engine := &Engine{plan: plan, settings: settings, chunkSize: 8, sampleInterval: 125 * time.Millisecond, preservePlanKnots: true, minimumPointIntervalMillis: 50, positionResolutionPercent: 1, maximumChunkPoints: 128}
			var previous *MotionSample
			for range 12 {
				before := engine.nextSampleMillis
				points, err := engine.nextMotionSamplesLocked()
				if err != nil {
					t.Fatalf("%s speed %d: %v", definition.Name, speed, err)
				}
				if engine.nextSampleMillis <= before {
					t.Fatal("sampler failed to advance")
				}
				for _, point := range points {
					if previous != nil && point.TimeMillis-previous.TimeMillis < 50 {
						t.Fatalf("%s speed %d violates spacing", definition.Name, speed)
					}
					sampleCopy := point
					previous = &sampleCopy
				}
			}
		}
	}
}

func BenchmarkQuantizedMotionFrame(b *testing.B) {
	settings := config.DefaultSettings().Motion
	plan := NewMotionPlan("benchmark", MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 43}, settings, 0, 0, time.Unix(0, 0))
	b.ReportAllocs()
	for b.Loop() {
		engine := &Engine{plan: plan, settings: settings, chunkSize: 8, sampleInterval: 125 * time.Millisecond, preservePlanKnots: true, positionResolutionPercent: 1, maximumChunkPoints: 128}
		if _, err := engine.nextMotionSamplesLocked(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestCrossingTimingDoesNotSearchAcrossAnUnpinnedReversal(t *testing.T) {
	probes := []MotionSample{{TimeMillis: 0, PositionPercent: 0}, {TimeMillis: 250, PositionPercent: 5}, {TimeMillis: 500, PositionPercent: 0}, {TimeMillis: 750, PositionPercent: 5}, {TimeMillis: 1000, PositionPercent: 0}}
	samples := []MotionSample{probes[0], {TimeMillis: 240, PositionPercent: 4.8}, {TimeMillis: 760, PositionPercent: 4.8}, probes[4]}
	fitted, err := fitQuantizedMotionTiming(samples, probes, nil, 1, 320, samplingTolerance{position: wireApproximationTolerance}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range samples {
		if fitted[i] != samples[i] {
			t.Fatal("aligned a point to a crossing in another stroke")
		}
	}
}

func TestBufferedTimingResumesAtTheActualBlendEnd(t *testing.T) {
	settings := config.DefaultSettings().Motion
	engine := &Engine{settings: settings, transition: &planTransition{startMillis: 0, endMillis: 1000}}
	var probes []MotionSample
	for at := int64(0); at <= 2000; at += 10 {
		probes = append(probes, MotionSample{TimeMillis: at, PositionPercent: float64(at) / 100})
	}
	samples := []MotionSample{{TimeMillis: 0}, {TimeMillis: 240, PositionPercent: 2.4}, {TimeMillis: 1000, PositionPercent: 10}, {TimeMillis: 1240, PositionPercent: 12.4}, {TimeMillis: 1540, PositionPercent: 15.4}, {TimeMillis: 2000, PositionPercent: 20}}
	fitted, err := engine.retimeBufferedSamples(samples, probes, map[int64]struct{}{1000: {}}, 1, wireApproximationTolerance, false)
	if err != nil {
		t.Fatal(err)
	}
	if fitted[1] != samples[1] || fitted[2] != samples[2] {
		t.Fatal("changed the blend or its boundary")
	}
	if fitted[3].TimeMillis != 1200 || fitted[4].TimeMillis != 1500 {
		t.Fatal("ordinary strokes after the blend lost crossing alignment")
	}
}

func TestQuantizedCreativeSamplingHonorsVelocityCeiling(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.HandyModel = config.HandyModel2Standard
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	dynamic := NormalizeDynamicDefinition(DynamicDefinition{CenterPercent: 50, SpanPercent: 90, SpanMinPercent: 25, SpanProfile: "wander", VariationPercent: 65, PhraseSeed: 17})
	spec := DefaultFlowSpec()
	gesture := DefaultGestureSpec()
	gesture.FocusRoamPercent, gesture.FocusPercent, gesture.VariationPercent, gesture.FocusMixPercent = 0, 50, 75, 55
	spec.Gesture, spec.RangeFloorPercent, spec.SpeedPercent = &gesture, 10, 85
	native, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []MotionTarget{{Dynamic: &dynamic, SpeedPercent: 85}, native} {
		plan := NewMotionPlan("rounded-limit", target, settings, 0, 0, time.Unix(0, 0))
		engine := &Engine{plan: plan, settings: settings, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval, preservePlanKnots: true, positionResolutionPercent: 1, maximumChunkPoints: 128}
		var previous *MotionSample
		for engine.nextSampleMillis < 12000 {
			points, err := engine.nextMotionSamplesLocked()
			if err != nil {
				t.Fatal(err)
			}
			for _, point := range points {
				if previous != nil {
					velocity := math.Abs(math.Round(point.PositionPercent)-math.Round(previous.PositionPercent)) * 1000 / float64(point.TimeMillis-previous.TimeMillis)
					if velocity > 320+1e-9 {
						t.Fatalf("rounded command velocity %g exceeds model ceiling at %dms", velocity, point.TimeMillis)
					}
				}
				sampleCopy := point
				previous = &sampleCopy
			}
		}
	}
}

func TestQuantizedTimingRetainsCommittedTailAndExtrema(t *testing.T) {
	points := []MotionSample{{TimeMillis: 0, PositionPercent: 0}, {TimeMillis: 31, PositionPercent: 9.49}, {TimeMillis: 100, PositionPercent: 20}}
	probes := []MotionSample{{TimeMillis: 0, PositionPercent: 0}, {TimeMillis: 30, PositionPercent: 9}, {TimeMillis: 31, PositionPercent: 9.49}, {TimeMillis: 100, PositionPercent: 20}}
	fit, err := fitQuantizedMotionTiming(points, probes, map[int64]struct{}{100: {}}, 1, 320, samplingTolerance{position: wireApproximationTolerance}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if fit[0] != points[0] || fit[len(fit)-1] != points[len(points)-1] {
		t.Fatal("immutable append tail or extremum moved")
	}
}
