package motion

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestFlowLayerAlternationReachesExtremesWithoutWidthLocationConfusion(t *testing.T) {
	spec := DefaultFlowSpec()
	spec.AnchorPercent = 100
	spec.RangeFloorPercent = 20
	spec.RangeCeilingPercent = 90
	layer := FlowLayer{Axis: "range", AmountPercent: 100, PeriodCycles: 8, Shape: "alternate"}
	spec.Layers = []FlowLayer{layer}
	lows, highs := 0, 0
	for u := 0.; u < spec.cycleCount(); u += 0.01 {
		envelope := spec.layerEnvelope(u, layer)
		if envelope < 0 || envelope > 1 {
			t.Fatal("alternation escaped envelope")
		}
		if envelope == 0 {
			lows++
		}
		if envelope == 1 {
			highs++
		}
	}
	if lows < 100 || highs < 100 {
		t.Fatal("alternation does not dwell at both extrema")
	}
	// At each full carrier reversal, range alternation keeps the tip fixed.
	for u := 0.; u < spec.cycleCount(); u++ {
		p, _ := spec.signal(u+0.5, config.HandyModelOriginal)
		if math.Abs(p-95) > 1e-7 {
			t.Fatalf("tip anchor drifted: %.3f", p)
		}
	}
	clone := *CloneFlowSpec(&spec)
	clone.Seed += 0x9e3779b9
	difference := 0.
	for u := 0.; u < spec.cycleCount(); u += .17 {
		a, _ := spec.signal(u, config.HandyModelOriginal)
		b, _ := clone.signal(u, config.HandyModelOriginal)
		difference += math.Abs(a - b)
	}
	if difference < 100 {
		t.Fatal("evolution did not produce a fresh realization")
	}
}

func TestFlowShapedLayersUseSharedBoundedPlannerAcrossProfiles(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, profile := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = profile
		for _, shape := range []string{"drift", "alternate", "wave"} {
			for _, speed := range []int{10, 45, 85} {
				t.Run(fmt.Sprintf("%s/%s/%d", profile, shape, speed), func(t *testing.T) {
					spec := DefaultFlowSpec()
					spec.SpeedPercent = speed
					spec.VariationMode = "drift"
					spec.AnchorPercent = 100
					spec.RangeFloorPercent = 20
					spec.RangeCeilingPercent = 90
					spec.Layers = []FlowLayer{{Axis: "range", AmountPercent: 100, PeriodCycles: 8, Shape: shape}, {Axis: "pace", AmountPercent: 25, PeriodCycles: 19, Shape: "drift"}}
					target, err := FlowTarget(spec, settings)
					if err != nil {
						t.Fatal(err)
					}
					// A serialized semantic target must compile to the same engine curve.
					encoded, _ := json.Marshal(target)
					var restored MotionTarget
					if err := json.Unmarshal(encoded, &restored); err != nil {
						t.Fatal(err)
					}
					a := NewMotionPlan("original", target, settings, 0, 0, time.Unix(0, 0))
					b := NewMotionPlan("restored", restored, settings, 0, 0, time.Unix(0, 0))
					if a.PeriodMillis != b.PeriodMillis || !reflect.DeepEqual(a.Target.Flow, b.Target.Flow) {
						t.Fatal("semantic score changed on roundtrip")
					}
					factor := float64(a.PeriodMillis) / float64(a.curve.duration)
					if a.curve.maximumAccelerationPerMillis2()*1e6/(factor*factor) > flowAccelerationBudget*1.001 || a.curve.maximumJerkPerMillis3()*1e9/(factor*factor*factor) > flowJerkBudget*1.001 {
						t.Fatal("derivative budget exceeded")
					}
					if a.Perceptual.CommandedPeakVelocityPerSecond > referenceTravelRateForSpeed(100, profile)*1.001 {
						t.Fatal("profile velocity exceeded")
					}
					for at := int64(0); at < a.PeriodMillis; at += 17 {
						x, y := a.SampleAt(at).PositionPercent, b.SampleAt(at).PositionPercent
						if x < 5-1e-5 || x > 95+1e-5 || math.Abs(x-y) > 1e-7 {
							t.Fatal("bounds/roundtrip mismatch")
						}
					}
					limited := settings
					limited.SpeedMaxPercent = 5
					c := NewMotionPlan("limited", target, limited, 0, 0, time.Unix(0, 0))
					if c.Target.Flow.SpeedPercent != 5 || c.PeriodMillis < a.PeriodMillis {
						t.Fatal("flow did not obey lower live limit")
					}
				})
			}
		}
	}
}
