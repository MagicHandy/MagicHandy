package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestFlowExperimentsStayBoundedAndContinuous(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		for _, speed := range []int{10, 25, 43} {
			base := DefaultFlowSpec()
			base.SpeedPercent = speed
			for _, experiment := range FlowExperiments(base) {
				target, err := FlowTarget(experiment.Spec, settings)
				if err != nil {
					t.Fatal(err)
				}
				plan := NewMotionPlan(experiment.ID, target, settings, 0, 0, time.Unix(0, 0))
				assertExperimentPlan(t, plan, experiment.Spec, settings)
			}
		}
	}
	// Stress all optional controls at their limits with sections and layers.
	base := DefaultFlowSpec()
	base.VariationMode, base.MemoryCycles, base.TurnSoftnessPercent, base.CadenceHoldPercent = "drift", 2, 100, 100
	base.Steps = []FlowStep{{MinPercent: 0, MaxPercent: 100, SpeedPercent: 85, Cycles: 2}, {MinPercent: 40, MaxPercent: 60, SpeedPercent: 15, Cycles: 3}}
	base.Layers = []FlowLayer{{Axis: "range", AmountPercent: 100, PeriodCycles: 2}, {Axis: "center", AmountPercent: 100, PeriodCycles: 3}, {Axis: "pace", AmountPercent: 100, PeriodCycles: 2}}
	target, err := FlowTarget(base, settings)
	if err != nil {
		t.Fatal(err)
	}
	assertExperimentPlan(t, NewMotionPlan("extreme", target, settings, 0, 0, time.Unix(0, 0)), base, settings)
}

func assertExperimentPlan(t *testing.T, plan MotionPlan, spec FlowSpec, settings config.MotionSettings) {
	t.Helper()
	factor := float64(plan.PeriodMillis) / float64(plan.curve.duration)
	if peak := plan.curve.maximumVelocityPerMillis() * 1000 / factor; peak > referenceTravelRateForSpeed(100, settings.HandyModel)*1.001 {
		t.Fatalf("%s velocity %f", plan.ID, peak)
	}
	if a := plan.curve.maximumAccelerationPerMillis2() * 1e6 / (factor * factor); a > flowAccelerationBudget*1.001 {
		t.Fatalf("%s acceleration %f", plan.ID, a)
	}
	if j := plan.curve.maximumJerkPerMillis3() * 1e9 / (factor * factor * factor); j > flowJerkBudget*1.001 {
		t.Fatalf("%s jerk %f", plan.ID, j)
	}
	for index, left := range plan.curve.quintics {
		right := plan.curve.quintics[(index+1)%len(plan.curve.quintics)]
		if math.Abs(left.position(1)-right.position(0)) > 1e-7 || math.Abs(left.velocity(1)-right.velocity(0)) > 1e-7 || math.Abs(left.acceleration(1)-right.acceleration(0)) > 1e-7 {
			t.Fatalf("%s discontinuity at knot %d", plan.ID, index)
		}
	}
	lo, hi := float64(spec.MinPercent), float64(spec.MaxPercent)
	if len(spec.Steps) > 0 {
		lo, hi = 0, 100
	}
	for at := int64(0); at < plan.PeriodMillis; at += 17 {
		x := plan.SampleAt(at).PositionPercent
		if math.IsNaN(x) || x < lo-.001 || x > hi+.001 {
			t.Fatalf("%s escaped range: %f", plan.ID, x)
		}
	}
	engine := &Engine{plan: plan, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval, preservePlanKnots: true, positionResolutionPercent: 1, maximumChunkPoints: maximumAdaptiveChunkPoints}
	for range 4 {
		previous := engine.lastSample
		samples, err := engine.nextMotionSamplesLocked()
		if err != nil {
			t.Fatal(err)
		}
		if previous != nil {
			samples = append([]MotionSample{*previous}, samples...)
		}
		assertNoStationaryWireEdges(t, plan.ID, samples, 1)
		assertQuantizedSamplesTrackPath(t, plan.ID, samples, 1, func(at int64) float64 { return plan.SampleAt(at).PositionPercent })
	}
}

func TestFlowDriftIsSeededPeriodicAndCorrelated(t *testing.T) {
	spec := DefaultFlowSpec()
	spec.VariationMode = "drift"
	other := spec
	other.Seed++
	different := false
	for i := 0; i < 6400; i++ {
		u := float64(i) / 100
		value := spec.field(u, 17)
		if value < 0 || value > 1 || value != spec.field(u, 17) || math.Abs(value-spec.field(u+spec.cycleCount(), 17)) > 1e-10 {
			t.Fatal("unbounded or nonrepeatable drift")
		}
		if math.Abs(value-spec.field(u+.01, 17)) > .025 {
			t.Fatal("drift contains point noise")
		}
		if math.Abs(value-other.field(u, 17)) > .05 {
			different = true
		}
	}
	if !different {
		t.Fatal("seed did not change drift")
	}
}

func TestFlowTurnSoftnessIsSymmetricAndCadenceHoldReducesTimingVariation(t *testing.T) {
	spec := DefaultFlowSpec()
	spec.TurnSoftnessPercent = 100
	for i := 0; i <= 100; i++ {
		u := float64(i) / 100
		if math.Abs(spec.carrier(u)-spec.carrier(1-u)) > 1e-12 {
			t.Fatal("turn softness introduced directional bias")
		}
	}
	plain := DefaultFlowSpec()
	if spec.carrier(.03) >= plain.carrier(.03) {
		t.Fatal("turn softness did not lengthen the turnaround")
	}
	plain.PaceVariationPercent = 0
	plain.VariationMode = "drift"
	held := plain
	held.CadenceHoldPercent = 100
	spread := func(s FlowSpec) float64 {
		minimum, maximum := math.Inf(1), 0.0
		for i := 0; i < 640; i++ {
			_, dt := s.signal(float64(i)/10, config.HandyModelOriginal)
			minimum = math.Min(minimum, dt)
			maximum = math.Max(maximum, dt)
		}
		return (maximum - minimum) / minimum
	}
	if spread(held) >= spread(plain)*.2 {
		t.Fatalf("cadence spread: held %.4f vs plain %.4f", spread(held), spread(plain))
	}
}
