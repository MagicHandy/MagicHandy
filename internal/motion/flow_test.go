package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestFlowIndependentCarrierAndRuntimeEnvelope(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, speed := range []int{10, 25, 43, 85} {
		for _, anchor := range []int{0, 50, 100} {
			spec := DefaultFlowSpec()
			spec.SpeedPercent, spec.AnchorPercent = speed, anchor
			target, err := FlowTarget(spec, settings)
			if err != nil {
				t.Fatal(err)
			}
			if target.Dynamic != nil || target.Pattern != nil || target.prepared == nil {
				t.Fatal("flow reused a legacy authoring framework")
			}
			plan := NewMotionPlan("flow-test", target, settings, 0, 0, time.Unix(0, 0))
			factor := float64(plan.PeriodMillis) / float64(plan.curve.duration)
			if peak := plan.curve.maximumVelocityPerMillis() * 1000 / factor; peak > referenceTravelRateForSpeed(100, settings.HandyModel)*1.001 {
				t.Fatalf("velocity %f exceeds requested cap", peak)
			}
			if a := plan.curve.maximumAccelerationPerMillis2() * 1e6 / (factor * factor); a > flowAccelerationBudget*1.001 {
				t.Fatalf("acceleration %f", a)
			}
			if j := plan.curve.maximumJerkPerMillis3() * 1e9 / (factor * factor * factor); j > flowJerkBudget*1.001 {
				t.Fatalf("jerk %f", j)
			}
			if len(plan.curve.authoredKnots) < 120 {
				t.Fatalf("missing carrier reversals: %d", len(plan.curve.authoredKnots))
			}
			for at := int64(0); at < plan.PeriodMillis; at += 13 {
				x := plan.SampleAt(at).PositionPercent
				if x < float64(spec.MinPercent)-0.001 || x > float64(spec.MaxPercent)+0.001 {
					t.Fatalf("escaped requested band: %f", x)
				}
			}
			t.Logf("speed=%d anchor=%d period=%d mean=%.2f peak=%.2f jerk=%.1f variation=%.3f", speed, anchor, plan.PeriodMillis,
				plan.Perceptual.CommandedMeanTravelPerSecond, plan.Perceptual.CommandedPeakVelocityPerSecond,
				plan.curve.maximumJerkPerMillis3()*1e9/(factor*factor*factor), plan.Perceptual.StrokeLengthCV)
		}
	}
}

func TestFlowContinuousStatesAndRepeatableLayeredSequence(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	spec := DefaultFlowSpec()
	spec.Steps = []FlowStep{{MinPercent: 0, MaxPercent: 100, SpeedPercent: 25, Cycles: 4}, {MinPercent: 20, MaxPercent: 60, SpeedPercent: 35, Cycles: 3}}
	spec.Layers = []FlowLayer{{Axis: "range", AmountPercent: 40, PeriodCycles: 4}, {Axis: "pace", AmountPercent: 30, PeriodCycles: 7}}
	a, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	b, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	if a.prepared.id != b.prepared.id {
		t.Fatal("nonrepeatable identity")
	}
	curve := a.prepared.curve
	for index, left := range curve.quintics {
		right := curve.quintics[(index+1)%len(curve.quintics)]
		if math.Abs(left.position(1)-right.position(0)) > 1e-7 || math.Abs(left.velocity(1)-right.velocity(0)) > 1e-7 || math.Abs(left.acceleration(1)-right.acceleration(0)) > 1e-7 {
			t.Fatalf("kinematic seam at %d", index)
		}
	}
	plan := NewMotionPlan("sequence", a, settings, 0, 0, time.Unix(0, 0))
	settings.SpeedMaxPercent = 15
	relimited := NewMotionPlan("limited", a, settings, 0, 0, time.Unix(0, 0))
	if relimited.PeriodMillis < plan.PeriodMillis || relimited.Target.SpeedPercent != 15 {
		t.Fatal("live speed limit did not constrain prepared content")
	}
	if relimited.Perceptual.CommandedMeanTravelPerSecond > referenceTravelRateForSpeed(15, settings.HandyModel)*1.001 {
		t.Fatal("prepared content bypassed live speed cap")
	}
}

func TestFlowBufferedFramesPreserveTheContinuousPath(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, speed := range []int{10, 25, 43, 85} {
		spec := DefaultFlowSpec()
		spec.SpeedPercent = speed
		spec.Layers = []FlowLayer{{Axis: "range", AmountPercent: 50, PeriodCycles: 8}, {Axis: "center", AmountPercent: 30, PeriodCycles: 13}}
		target, err := FlowTarget(spec, settings)
		if err != nil {
			t.Fatal(err)
		}
		plan := NewMotionPlan("flow-wire", target, settings, 0, 0, time.Unix(0, 0))
		engine := &Engine{plan: plan, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
			preservePlanKnots: true, positionResolutionPercent: 1, maximumChunkPoints: maximumAdaptiveChunkPoints}
		for range 8 {
			previous := engine.lastSample
			samples, err := engine.nextMotionSamplesLocked()
			if err != nil {
				t.Fatal(err)
			}
			if previous != nil {
				samples = append([]MotionSample{*previous}, samples...)
			}
			assertNoStationaryWireEdges(t, "continuous flow", samples, 1)
			assertQuantizedSamplesTrackPath(t, "continuous flow", samples, 1,
				func(at int64) float64 { return plan.SampleAt(at).PositionPercent })
		}
	}
}
