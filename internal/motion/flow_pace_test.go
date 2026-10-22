package motion

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestFlowPaceFloorKeepsLegacyClockAndGeometry(t *testing.T) {
	for _, shape := range []string{"wave", "alternate", "drift"} {
		for _, speed := range []int{9, 47, 89} {
			spec := DefaultFlowSpec()
			spec.SpeedPercent = speed
			spec.Layers = []FlowLayer{{Axis: "pace", AmountPercent: 73, PeriodCycles: 13, PhasePercent: 19, Shape: shape}}
			explicit := *CloneFlowSpec(&spec)
			explicit.Layers[0].LowRatePercent = 50
			for u := 0.; u < spec.cycleCount(); u += .07 {
				a, spanA, clockA := spec.flowState(u, config.HandyModelOriginal)
				b, spanB, clockB := explicit.flowState(u, config.HandyModelOriginal)
				if a != b || spanA != spanB || clockA != clockB {
					t.Fatal("omitting the pace floor changed the historical rate envelope")
				}
			}
			encoded, _ := json.Marshal(spec)
			var restored FlowSpec
			if err := json.Unmarshal(encoded, &restored); err != nil || restored.Layers[0].LowRatePercent != 0 {
				t.Fatal("legacy score gained a rate floor")
			}
		}
	}
}

func TestFlowPaceFloorCreatesContrastWithoutMovingTheBand(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, profile := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = profile
		for _, speed := range []int{9, 47, 89} {
			for _, floor := range []int{1, 17, 100} {
				t.Run(fmt.Sprintf("%s/%d/%d", profile, speed, floor), func(t *testing.T) {
					spec := DefaultFlowSpec()
					spec.SpeedPercent, spec.PaceVariationPercent = speed, 0
					spec.RangeFloorPercent, spec.RangeCeilingPercent = 20, 20
					spec.Layers = []FlowLayer{{Axis: "pace", AmountPercent: 100, PeriodCycles: 8, Shape: "wave", LowRatePercent: floor}}
					_, _, fastClock := spec.flowState(0, profile)
					_, _, slowClock := spec.flowState(4, profile)
					if floor <= 17 && slowClock < fastClock*1.4 || floor == 100 && slowClock != fastClock {
						t.Fatalf("pace contrast not represented: %.1f / %.1f", slowClock, fastClock)
					}
					target, err := FlowTarget(spec, settings)
					if err != nil {
						t.Fatal(err)
					}
					plan := NewMotionPlan("pace-floor", target, settings, 0, 0, time.Unix(0, 0))
					factor := float64(plan.PeriodMillis) / float64(plan.curve.duration)
					if plan.curve.maximumAccelerationPerMillis2()*1e6/(factor*factor) > flowAccelerationBudget*1.001 || plan.curve.maximumJerkPerMillis3()*1e9/(factor*factor*factor) > flowJerkBudget*1.001 {
						t.Fatal("rate floor bypassed kinematic limits")
					}
					for at := int64(0); at < plan.PeriodMillis; at += max(1, plan.PeriodMillis/1000) {
						p := plan.SampleAt(at).PositionPercent
						if p < 5-1e-5 || p > 25+1e-5 || math.IsNaN(p) {
							t.Fatal("rate floor changed the stroke band")
						}
					}
					settingsLow := settings
					settingsLow.SpeedMaxPercent = 5
					limited := NewMotionPlan("relimited", target, settingsLow, 0, 0, time.Unix(0, 0))
					if limited.Target.SpeedPercent != 5 || limited.PeriodMillis < plan.PeriodMillis {
						t.Fatal("rate floor bypassed live speed limits")
					}
				})
			}
		}
	}
}

func TestFlowPaceFloorRejectsInvalidAxesAndBounds(t *testing.T) {
	for _, layer := range []FlowLayer{
		{Axis: "pace", LowRatePercent: -1}, {Axis: "pace", LowRatePercent: 101},
		{Axis: "center", LowRatePercent: 15}, {Axis: "range", LowRatePercent: 15},
	} {
		spec := DefaultFlowSpec()
		layer.AmountPercent, layer.PeriodCycles = 100, 8
		spec.Layers = []FlowLayer{layer}
		if spec.Validate(config.DefaultSettings().Motion) == nil {
			t.Fatal("invalid rate floor accepted")
		}
	}
}

func TestFlowEnvelopeInversionPreservesUnequalDwells(t *testing.T) {
	spec := DefaultFlowSpec()
	for _, shape := range []string{"wave", "alternate", "drift"} {
		a := FlowLayer{Axis: "pace", AmountPercent: 100, PeriodCycles: 13, PhasePercent: 31, Shape: shape}
		b := a
		b.Inverted = true
		for u := 0.; u < spec.cycleCount(); u += .07 {
			if math.Abs(spec.layerEnvelope(u, a)+spec.layerEnvelope(u, b)-1) > 1e-12 {
				t.Fatal("inverted envelope did not oppose the source exactly")
			}
		}
	}
}
