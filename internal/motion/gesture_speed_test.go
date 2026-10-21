package motion

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func fullSpeedGesture() FlowSpec {
	s := DefaultFlowSpec()
	g := DefaultGestureSpec()
	g.FocusMixPercent, g.VariationPercent = 0, 0
	s.MinPercent, s.MaxPercent, s.Gesture = 0, 100, &g
	return s
}

func roundedGestureReference(t *testing.T, s FlowSpec, model string) Curve {
	t.Helper()
	legs := gestureLegs(s)
	curves, err := fitGestureCurves(s, legs, model)
	if err != nil {
		t.Fatal(err)
	}
	curve, err := assembleLegCurve(legs, curves, "reference")
	if err != nil {
		t.Fatal(err)
	}
	return curve
}

func TestCreativeV2UsesRequestedBroadStrokePaceWithoutWeakeningLimits(t *testing.T) {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		limits.HandyModel = model
		s := fullSpeedGesture()
		s.SpeedPercent = 75
		target, err := FlowTarget(s, limits)
		if err != nil {
			t.Fatal(err)
		}
		plan := NewMotionPlan("fast-gesture", target, limits, 0, 0, time.Unix(0, 0))
		if plan.Perceptual.Pace.EffectivePercent < 74 || maximumPlanAcceleration(plan) > runtimeMaxAccelerationPercentPerSecond2*1.001 || maximumPlanJerk(plan) > runtimeMaxJerkPercentPerSecond3*1.001 || plan.Perceptual.CommandedPeakVelocityPerSecond > referenceTravelRateForSpeed(100, model)*1.001 {
			t.Fatalf("%s: missed requested pace or exceeded the envelope: %+v", model, plan.Perceptual.Pace)
		}
	}
}

func TestCreativeV2CruisePreservesGeometryRhythmAndSpeedOrder(t *testing.T) {
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		for _, shape := range []string{"full", "narrow", "varied", "directional"} {
			s := fullSpeedGesture()
			switch shape {
			case "narrow":
				s.MinPercent, s.MaxPercent, s.RangeFloorPercent, s.Gesture.FocusWidthPercent = 40, 60, 10, 10
			case "varied":
				*s.Gesture = DefaultGestureSpec()
			case "directional":
				s.Gesture.FasterDirection, s.Gesture.ContrastPercent, s.Gesture.InertiaPercent = "tip", 65, 70
			}
			previous := math.Inf(1)
			for _, speed := range []int{10, 25, 40, 50, 60, 65, 70, 75, 85, 100} {
				s.SpeedPercent = speed
				baseline := roundedGestureReference(t, s, model)
				curve, err := compileGestureCurve(s, model)
				if err != nil {
					t.Fatal(err)
				}
				if curve.duration > baseline.duration || float64(curve.duration) > previous*1.001 {
					t.Fatalf("%s %s at %d regressed pace: baseline=%d previous=%.0f fitted=%d", model, shape, speed, baseline.duration, previous, curve.duration)
				}
				previous = float64(curve.duration)
				if (shape == "full" && speed <= 50) || shape == "narrow" {
					if !reflect.DeepEqual(curve, baseline) {
						t.Fatalf("changed unconstrained or short stroke: %s %s %d", model, shape, speed)
					}
				}
				if len(curve.authoredKnots) != len(baseline.authoredKnots) {
					t.Fatal("changed authored turns")
				}
				for i, point := range curve.authoredKnots {
					if point.PositionPercent != baseline.authoredKnots[i].PositionPercent {
						t.Fatal("changed stroke reach")
					}
					if i > 0 && point.PositionPercent == curve.authoredKnots[i-1].PositionPercent && point.TimeMillis-curve.authoredKnots[i-1].TimeMillis != baseline.authoredKnots[i].TimeMillis-baseline.authoredKnots[i-1].TimeMillis {
						t.Fatal("shortened an intentional rest")
					}
				}
			}
		}
	}
}

func TestBufferedHighSpeedGestureKeepsWireFidelityAcrossPartitions(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		for _, oldCurve := range []bool{false, true} {
			s := fullSpeedGesture()
			s.SpeedPercent = 100
			if oldCurve {
				*s.Gesture = DefaultGestureSpec()
				s.SpeedPercent = 75
			}
			target, err := FlowTarget(s, settings)
			if err != nil {
				t.Fatal(err)
			}
			plan := NewMotionPlan("wire-fast", target, settings, 0, 0, time.Unix(0, 0))
			if oldCurve {
				plan.curve = roundedGestureReference(t, s, model)
				plan.PeriodMillis = plan.curve.duration
			}
			engine := &Engine{plan: plan, settings: settings, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval, preservePlanKnots: true, positionResolutionPercent: 1, maximumChunkPoints: 100}
			var previous *MotionSample
			for engine.nextSampleMillis < 20000 {
				points, err := engine.nextMotionSamplesLocked()
				if err != nil {
					t.Fatalf("%s old=%v: %v", model, oldCurve, err)
				}
				for _, point := range points {
					if previous != nil {
						assertGestureWireInterval(t, plan, *previous, point, model)
					}
					copyPoint := point
					previous = &copyPoint
				}
			}
		}
	}
}

func assertGestureWireInterval(t *testing.T, plan MotionPlan, left, right MotionSample, model string) {
	t.Helper()
	dt := right.TimeMillis - left.TimeMillis
	a, b := math.Round(left.PositionPercent), math.Round(right.PositionPercent)
	if dt <= 0 || math.Abs(b-a)*1000/float64(dt) > referenceTravelRateForSpeed(100, model)+1e-7 {
		t.Fatal("wire timestamps or velocity are invalid")
	}
	for at := left.TimeMillis; at <= right.TimeMillis; at++ {
		wire := a + (b-a)*float64(at-left.TimeMillis)/float64(dt)
		if deviation := math.Abs(wire - plan.SampleAt(at).PositionPercent); deviation > wireApproximationTolerance+0.5 {
			t.Errorf("%s wire error %.3f at %d between %d/%d", model, deviation, at, left.TimeMillis, right.TimeMillis)
			break
		}
	}
}

func BenchmarkCreativeV2Timing(b *testing.B) {
	for _, speed := range []int{25, 75} {
		for _, optimized := range []bool{false, true} {
			b.Run(fmt.Sprintf("speed-%d/optimized-%v", speed, optimized), func(b *testing.B) {
				s := fullSpeedGesture()
				s.SpeedPercent = speed
				b.ReportAllocs()
				for b.Loop() {
					if optimized {
						if _, err := compileGestureCurve(s, config.HandyModelOriginal); err != nil {
							b.Fatal(err)
						}
					} else {
						legs := gestureLegs(s)
						curves, err := fitGestureCurves(s, legs, config.HandyModelOriginal)
						if err != nil {
							b.Fatal(err)
						}
						if _, err := assembleLegCurve(legs, curves, "reference"); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}
