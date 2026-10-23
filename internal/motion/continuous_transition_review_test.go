//go:build magichandy_labs

package motion

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Inert comparison of the release crossfade and the candidate turn join.
func TestContinuousTurnHandoffReview(t *testing.T) {
	output := os.Getenv("MAGICHANDY_TURN_HANDOFF_OUTPUT")
	if output == "" {
		t.Skip("set inert comparison output")
	}
	var rows []map[string]any
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, profile := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = profile
		for _, kind := range []string{"new_phrase", "creative_pace", "layered_geometry"} {
			for _, speed := range []int{15, 35, 94} {
				a := phraseFixture(0, 35, 5, speed)
				a.PaceVariationPercent = 0
				b := phraseFixture(100, 72, 3, max(7, speed-10))
				b.PaceVariationPercent = 0
				if kind == "creative_pace" {
					g := DefaultGestureSpec()
					a = DefaultFlowSpec()
					a.Gesture = &g
					a.SpeedPercent = speed
					b = *CloneFlowSpec(&a)
					b.SpeedPercent = max(7, speed-10)
				}
				if kind == "layered_geometry" {
					a = DefaultFlowSpec()
					a.SpeedPercent = speed
					b = a
					b.RangeCeilingPercent = 35
					b.AnchorPercent = 100
				}
				ta, err := FlowTarget(a, settings)
				if err != nil {
					t.Fatal(err)
				}
				tb, err := FlowTarget(b, settings)
				if err != nil {
					t.Fatal(err)
				}
				old := NewMotionPlan("old", ta, settings, 0, 0, time.Unix(0, 0))
				for _, fraction := range []float64{.17, .43, .71, .93} {
					at := int64(float64(old.PeriodMillis) * fraction)
					for _, variant := range []string{"release", "turn_join"} {
						next := old.Retarget("next", tb, settings, at, time.Unix(0, 0))
						bridge := layeredPaceTransition(old, nil, &next, at)
						if variant == "turn_join" {
							bridge = layeredTurnTransition(old, nil, &next, at)
						}
						spliced := bridge != nil
						if bridge == nil {
							bridge = newPlanTransition(old, nil, at)
						}
						var path [][3]float64
						var tinyTurns int
						var peakA, previousV float64
						lastTurn, lastDirection, seen := old.SampleAt(at).PositionPercent, 0, false
						for now := at - 250; now <= bridge.endMillis+750; now += 5 {
							x := sampleMotionPath(next, bridge, now).PositionPercent
							v := (sampleMotionPath(next, bridge, now+1).PositionPercent - sampleMotionPath(next, bridge, now-1).PositionPercent) * 500
							path = append(path, [3]float64{float64(now-at) / 1000, x, v})
							if now > at && now < bridge.endMillis {
								peakA = max(peakA, math.Abs(v-previousV)*200)
								direction := curveDirection(v)
								if direction != 0 && lastDirection != 0 && direction != lastDirection {
									if seen && math.Abs(x-lastTurn) < 1 {
										tinyTurns++
									}
									lastTurn, seen = x, true
								}
								if direction != 0 {
									lastDirection = direction
								}
							}
							previousV = v
						}
						rows = append(rows, map[string]any{"name": fmt.Sprintf("%s/%s/%d/%.2f/%s", profile, kind, speed, fraction, variant), "variant": variant, "spliced": spliced, "tiny_turns": tinyTurns, "peak_acceleration": peakA, "join_start": float64(bridge.strokeStartMillis-at) / 1000, "join_end": float64(bridge.endMillis-at) / 1000, "path": path})
					}
				}
			}
		}
	}
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, b, 0600); err != nil {
		t.Fatal(err)
	}
}
