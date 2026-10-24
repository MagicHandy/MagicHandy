package motion

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func sparseReviewEngine(t testing.TB, spec FlowSpec, enabled bool) *Engine {
	t.Helper()
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	target, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	return &Engine{plan: NewMotionPlan("sparse-review", target, settings, 0, 0, time.Unix(0, 0)), settings: settings,
		preservePlanKnots: true, preferSparseGeneratedMotion: enabled, positionResolutionPercent: 1,
		sampleInterval: defaultSampleInterval, chunkSize: defaultChunkSize, maximumChunkPoints: 100}
}

func BenchmarkSparseGeneratedFrame(b *testing.B) {
	for _, enabled := range []bool{false, true} {
		b.Run(fmt.Sprintf("sparse=%t", enabled), func(b *testing.B) {
			spec := DefaultFlowSpec()
			spec.MinPercent, spec.MaxPercent, spec.SpeedPercent = 0, 100, 58
			spec.RangeFloorPercent, spec.RangeCeilingPercent, spec.PaceVariationPercent = 28, 100, 0
			spec.Phrase = &FlowPhrase{Strokes: []PhraseStroke{
				{BottomPercent: 72, TopPercent: 100, Count: 2, RatePercent: 25},
				{BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: 25},
			}}
			engine := sparseReviewEngine(b, spec, enabled)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				engine.nextSampleMillis, engine.lastSample = 0, nil
				if _, err := engine.nextMotionSamplesLocked(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func sparseReviewSamples(t *testing.T, engine *Engine) []MotionSample {
	t.Helper()
	var result []MotionSample
	for engine.nextSampleMillis < min(int64(20000), engine.plan.PeriodMillis+1000) {
		points, err := engine.nextMotionSamplesLocked()
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, points...)
	}
	return result
}

func TestSparseGeneratedSamplingPreservesTurnsClockAndBoundedPath(t *testing.T) {
	for _, speed := range []int{10, 58, 80} {
		for _, width := range []int{10, 28, 100} {
			for _, rate := range []int{12, 25, 100} {
				t.Run(fmt.Sprintf("speed%d-width%d-rate%d", speed, width, rate), func(t *testing.T) {
					spec := DefaultFlowSpec()
					spec.MinPercent, spec.MaxPercent, spec.SpeedPercent = 0, 100, speed
					spec.RangeFloorPercent, spec.RangeCeilingPercent, spec.PaceVariationPercent = width, 100, 0
					spec.Phrase = &FlowPhrase{Strokes: []PhraseStroke{
						{BottomPercent: 100 - width, TopPercent: 100, Count: 2, RatePercent: rate},
						{BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: rate},
					}}
					engine := sparseReviewEngine(t, spec, true)
					before := engine.plan
					points := sparseReviewSamples(t, engine)
					if !reflect.DeepEqual(before, engine.plan) {
						t.Fatal("sampling changed the planned geometry or clock")
					}
					checkSparseWireBounds(t, engine, before, points, width)
					if width == 28 && rate == 25 && speed == 58 {
						precise := sparseReviewSamples(t, sparseReviewEngine(t, spec, false))
						if len(points)*2 >= len(precise) {
							t.Fatal("accepted compound case did not retain the substantial point reduction", len(points), len(precise))
						}
					}
				})
			}
		}
	}
}

func TestSparsePreferenceLeavesUnacceptedPathsUnchanged(t *testing.T) {
	for _, mode := range []string{"off", "immediate", "media", "transition", "handy2"} {
		t.Run(mode, func(t *testing.T) {
			spec := DefaultFlowSpec()
			engine := sparseReviewEngine(t, spec, true)
			transition := false
			switch mode {
			case "off":
				engine.preferSparseGeneratedMotion = false
			case "immediate":
				engine.minimumPointIntervalMillis = 110
			case "media":
				engine.plan.Target.Media = &MediaTimelineDefinition{}
			case "transition":
				transition = true
			case "handy2":
				engine.settings.HandyModel = config.HandyModel2Pro
			}
			if got := engine.generatedPositionTolerance(transition); got != wireApproximationTolerance {
				t.Fatal("changed an unaccepted fitting path", got)
			}
		})
	}
}

func TestSparsePreferenceIsImportedFromTransport(t *testing.T) {
	owner := &startupResolutionTransport{startupStateTransport: &startupStateTransport{Fake: transport.NewFake()}, capabilities: (&transport.CloudRESTTransport{}).MotionSamplingCapabilities()}
	engine, err := NewEngine(EngineOptions{Transport: owner})
	if err != nil {
		t.Fatal(err)
	}
	if !engine.preferSparseGeneratedMotion {
		t.Fatal("Cloud fitting preference did not reach the shared engine")
	}
}

func TestSparsePreferenceDoesNotAlterProtectedOutputFrames(t *testing.T) {
	for _, mode := range []string{"immediate", "transition", "handy2"} {
		t.Run(mode, func(t *testing.T) {
			var outputs [2][]MotionSample
			for i, enabled := range []bool{false, true} {
				engine := sparseReviewEngine(t, DefaultFlowSpec(), enabled)
				switch mode {
				case "immediate":
					engine.minimumPointIntervalMillis, engine.preferredPointIntervalMillis = 50, 110
				case "transition":
					engine.nextSampleMillis = 1000
					engine.transition = newPlanTransition(engine.plan, nil, 1000)
				case "handy2":
					engine.settings.HandyModel = config.HandyModel2Pro
				}
				var err error
				outputs[i], err = engine.nextMotionSamplesLocked()
				if err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(outputs[0], outputs[1]) {
				t.Fatal("preference altered a protected output frame")
			}
		})
	}
}

func checkSparseWireBounds(t *testing.T, engine *Engine, before MotionPlan, points []MotionSample, width int) {
	t.Helper()
	limit := math.Min(3, float64(width)*.12) + .5
	byTime := make(map[int64]MotionSample, len(points))
	for _, point := range points {
		byTime[point.TimeMillis] = point
		if point.PositionPercent < 0 || point.PositionPercent > 100 || point.PlanID != before.ID {
			t.Fatal("point lost bounds or plan identity", point)
		}
	}
	for _, at := range before.knotTimesBetween(0, points[len(points)-1].TimeMillis) {
		point, ok := byTime[at]
		if !ok || math.Abs(point.PositionPercent-before.SampleAt(at).PositionPercent) > .5 {
			t.Fatal("sampling moved or omitted an authored turn", at)
		}
	}
	for i := 1; i < len(points); i++ {
		left, right := points[i-1], points[i]
		from, to := math.Round(left.PositionPercent), math.Round(right.PositionPercent)
		duration := right.TimeMillis - left.TimeMillis
		if duration <= 0 || math.Abs(to-from)*1000/float64(duration) > referenceTravelRateForSpeed(100, engine.settings.HandyModel)+1e-9 {
			t.Fatal("encoded timing or velocity left its bound", left, right)
		}
		for at := left.TimeMillis; at <= right.TimeMillis; at++ {
			encoded := from + (to-from)*float64(at-left.TimeMillis)/float64(duration)
			if math.Abs(encoded-before.SampleAt(at).PositionPercent) > limit+1e-9 {
				t.Fatal("encoded path exceeds the selected approximation budget", at, limit)
			}
		}
	}
}
