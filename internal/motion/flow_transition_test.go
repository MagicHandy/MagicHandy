package motion

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestLayeredPaceSplicesKeepPositionVelocityAccelerationAndLimits(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, profile := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = profile
		for _, speeds := range [][2]int{{7, 85}, {65, 22}, {22, 35}, {35, 7}, {85, 45}} {
			for _, fraction := range []float64{.01, .17, .43, .71, .93, .999} {
				t.Run(fmt.Sprintf("%s/%d-%d/%.3f", profile, speeds[0], speeds[1], fraction), func(t *testing.T) {
					spec := DefaultFlowSpec()
					spec.SpeedPercent = speeds[0]
					spec.VariationMode = "drift"
					before, err := FlowTarget(spec, settings)
					if err != nil {
						t.Fatal(err)
					}
					previous := NewMotionPlan("before", before, settings, 0, 0, time.Unix(0, 0))
					spec.SpeedPercent = speeds[1]
					after, err := FlowTarget(spec, settings)
					if err != nil {
						t.Fatal(err)
					}
					at := int64(float64(previous.PeriodMillis) * fraction)
					next := previous.Retarget("after", after, settings, at, time.Unix(0, 0))
					bridge := layeredPaceTransition(previous, nil, &next, at)
					if bridge == nil {
						t.Fatal("representable pace edit fell back to oscillator crossfade")
					}
					assertLayeredSplice(t, previous, next, bridge, settings)
				})
			}
		}
	}
}

func assertLayeredSplice(t *testing.T, previous, next MotionPlan, bridge *planTransition, settings config.MotionSettings) {
	t.Helper()
	stroke := bridge.stroke
	v, a := layeredPlanDerivatives(previous, bridge.strokeStartMillis)
	if math.Abs(stroke.position(0)-previous.SampleAt(bridge.strokeStartMillis).PositionPercent) > 1e-7 ||
		math.Abs(stroke.velocity(0)-v) > 1e-7 || math.Abs(stroke.acceleration(0)-a) > 1e-7 {
		t.Fatal("splice lost incoming kinematic state")
	}
	v, a = layeredPlanDerivatives(next, bridge.endMillis)
	if math.Abs(stroke.position(1)-next.SampleAt(bridge.endMillis).PositionPercent) > 1e-7 ||
		math.Abs(stroke.velocity(1)-v) > 1e-7 || math.Abs(stroke.acceleration(1)-a) > 1e-7 {
		t.Fatal("splice lost outgoing kinematic state")
	}
	ceilingAcceleration, ceilingJerk := joinEnvelope(previous, next)
	if stroke.maximumVelocity()*1000 > referenceTravelRateForSpeed(100, settings.HandyModel)*1.00001 ||
		stroke.maximumAcceleration()*1e6 > ceilingAcceleration*1.00001 || stroke.maximumJerk()*1e9 > ceilingJerk*1.00001 {
		t.Fatal("splice exceeded existing envelope")
	}
	for at := bridge.startMillis; at < bridge.strokeStartMillis; at += 5 {
		if sampleMotionPath(next, bridge, at).PositionPercent != previous.SampleAt(at).PositionPercent {
			t.Fatal("deferred splice changed the unfinished stroke")
		}
	}
	engine := &Engine{plan: next, settings: settings, transition: bridge, nextSampleMillis: bridge.startMillis,
		chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval, preservePlanKnots: true, positionResolutionPercent: 1, maximumChunkPoints: 100}
	var previousPoint *MotionSample
	for engine.nextSampleMillis <= bridge.endMillis+1000 {
		points, err := engine.nextMotionSamplesLocked()
		if err != nil {
			t.Fatal(err)
		}
		for _, point := range points {
			if previousPoint != nil {
				velocity := math.Abs(math.Round(point.PositionPercent)-math.Round(previousPoint.PositionPercent)) * 1000 / float64(point.TimeMillis-previousPoint.TimeMillis)
				if velocity > referenceTravelRateForSpeed(100, settings.HandyModel)+1e-8 {
					t.Fatalf("wire splice exceeded velocity: %f", velocity)
				}
			}
			p := point
			previousPoint = &p
		}
	}
}

func TestLayeredPaceSpliceRunsOnOneStreamAndStopCancelsIt(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(64), time.Hour)
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	spec := DefaultFlowSpec()
	spec.SpeedPercent = 65
	target, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Start(t.Context(), target, settings); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	spec.SpeedPercent = 22
	target, err = FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ApplyTarget(t.Context(), target, "pace edit"); err != nil {
		t.Fatal(err)
	}
	engine.mu.Lock()
	hasSplice := engine.transition != nil && engine.transition.stroke != nil
	engine.mu.Unlock()
	if !hasSplice {
		t.Fatal("engine did not select the bounded Layered splice")
	}
	if countCommands(fake.Commands(), transport.CommandKindPointsPlay) != 1 {
		t.Fatal("pace edit restarted playback")
	}
	if _, err := engine.Stop(t.Context(), "review stop"); err != nil {
		t.Fatal(err)
	}
	count := len(fake.Commands())
	if _, err := engine.RefreshSettings(t.Context(), settings, "after stop"); err != nil {
		t.Fatal(err)
	}
	if engine.Snapshot().Running || len(fake.Commands()) != count {
		t.Fatal("stopped splice produced more device work")
	}
}

func TestRetargetWindowKeepsOrdinaryKnotsAfterTheBlend(t *testing.T) {
	settings := config.DefaultSettings().Motion
	plan := NewMotionPlan("knots", MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 43}, settings, 0, 0, time.Unix(0, 0))
	transition := newPlanTransition(plan, nil, 200)
	_, mandatory, _ := motionProbeTimes(plan, transition, true, 0, 3000, 5, 100)
	checked := 0
	for _, at := range plan.knotTimesBetween(transition.endMillis, 3000) {
		if _, ok := mandatory[at]; !ok {
			t.Fatalf("ordinary reversal at %dms was discarded after the blend", at)
		}
		checked++
	}
	if checked < 2 {
		t.Fatal("fixture did not span ordinary strokes")
	}
}
