//go:build magichandy_labs

package motion

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestLayeredHandoffReview(t *testing.T) {
	output := os.Getenv("MAGICHANDY_LAYERED_HANDOFF_OUTPUT")
	if output == "" {
		t.Skip("set the inert handoff review output path")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	var rows []map[string]any
	for _, kind := range []string{"pace-up", "pace-down", "region-transfer", "geometry-edit"} {
		spec := DefaultFlowSpec()
		spec.SpeedPercent, spec.VariationMode = 22, "drift"
		spec.Layers = []FlowLayer{{Axis: "center", AmountPercent: 30, PeriodCycles: 13, Shape: "drift"}}
		if kind == "region-transfer" {
			spec = layeredTransferFixture()
		}
		if kind == "pace-down" {
			spec.SpeedPercent = 65
		}
		oldTarget, err := FlowTarget(spec, settings)
		if err != nil {
			t.Fatal(err)
		}
		old := NewMotionPlan("old", oldTarget, settings, 0, 0, time.Unix(0, 0))
		switch kind {
		case "pace-up":
			spec.SpeedPercent = 35
		case "pace-down":
			spec.SpeedPercent = 22
		case "region-transfer":
			spec.SpeedPercent = 45
		case "geometry-edit":
			spec.RangeCeilingPercent = 45
			spec.AnchorPercent = 100
		}
		target, err := FlowTarget(spec, settings)
		if err != nil {
			t.Fatal(err)
		}
		for _, fraction := range []float64{.17, .43, .71, .93} {
			at := int64(float64(old.PeriodMillis) * fraction)
			oldSlot := handoffReviewSlot(old, at)
			for _, variant := range []string{"global", "nearby", "splice"} {
				phase := chooseNearestPhase(target, settings, old.SampleAt(at).PositionPercent, old.DirectionAt(at), old.VelocityAt(at))
				if variant != "global" {
					phase = chooseNearbyStrokePhase(old, target, settings, at, old.SampleAt(at).PositionPercent, old.DirectionAt(at), old.VelocityAt(at))
				}
				next := NewMotionPlan("next", target, settings, phase, at, time.Unix(0, 0))
				bridge := newPlanTransition(old, nil, at)
				if variant == "splice" {
					if fitted := layeredPaceTransition(old, nil, &next, at); fitted != nil {
						bridge = fitted
					}
				}
				var path [][3]float64
				var accelerationMax, previousV float64
				var tinyTurns int
				lastExtreme, lastDirection := sampleMotionPath(next, bridge, at).PositionPercent, 0
				seenTurn := false
				for offset := int64(-250); offset <= 1750; offset += 5 {
					now := at + offset
					x := sampleMotionPath(next, bridge, now).PositionPercent
					v := (sampleMotionPath(next, bridge, now+1).PositionPercent - sampleMotionPath(next, bridge, now-1).PositionPercent) * 500
					path = append(path, [3]float64{float64(offset) / 1000, x, v})
					if offset > 0 && offset <= retargetTransitionMillis {
						accelerationMax = math.Max(accelerationMax, math.Abs(v-previousV)*200)
						direction := curveDirection(v)
						if direction != 0 && lastDirection != 0 && direction != lastDirection {
							if seenTurn && math.Abs(x-lastExtreme) < 1 {
								tinyTurns++
							}
							lastExtreme = x
							seenTurn = true
						}
						if direction != 0 {
							lastDirection = direction
						}
					}
					previousV = v
				}
				engine := &Engine{plan: next, settings: settings, transition: bridge, nextSampleMillis: at,
					chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval, preservePlanKnots: true,
					positionResolutionPercent: 1, maximumChunkPoints: 100}
				var wire []MotionSample
				for engine.nextSampleMillis < at+1750 {
					points, err := engine.nextMotionSamplesLocked()
					if err != nil {
						t.Fatal(kind, variant, err)
					}
					wire = append(wire, points...)
				}
				rows = append(rows, map[string]any{"name": fmt.Sprintf("%s/%.2f", kind, fraction), "variant": variant,
					"old_slot": oldSlot, "new_slot": handoffReviewSlot(next, at), "tiny_turns": tinyTurns,
					"handoff_ms": at, "bridge_ms": bridge.endMillis - bridge.startMillis, "splice": bridge.stroke != nil,
					"acceleration_max": accelerationMax, "path": path, "wire": wire})
			}
		}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func handoffReviewSlot(plan MotionPlan, at int64) int {
	clock := plan.PhaseAt(at) * float64(plan.curve.duration)
	return max(0, sort.Search(len(plan.curve.authoredKnots), func(i int) bool { return float64(plan.curve.authoredKnots[i].TimeMillis) > clock })-1)
}
