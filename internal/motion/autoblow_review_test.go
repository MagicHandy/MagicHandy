//go:build magichandy_labs

package motion

import (
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// An inert review of the production sampler with each new owner's actual
// capabilities. No connection, playback worker or device request is created.
func TestAutoblowMotionReview(t *testing.T) {
	path := os.Getenv("MAGICHANDY_AUTOBLOW_REVIEW")
	if path == "" {
		t.Skip("set MAGICHANDY_AUTOBLOW_REVIEW to an ignored atlas path")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	var entries []Review
	motors := map[string][][2]float64{}
	for _, speed := range []int{5, 40, 100} {
		fixtures := motionPathFixtures(speed)
		for _, name := range []string{"full", "short", "compound", "regional", "down-fast", "creative-v2"} {
			target, err := FlowTarget(fixtures[name], settings)
			if err != nil {
				t.Fatal(err)
			}
			for _, owner := range []string{"ultra", "vacuglide"} {
				var device transport.Transport = &transport.AutoblowUltra{}
				if owner == "vacuglide" {
					device = &transport.VacuGlide{}
				}
				prototype, err := NewEngine(EngineOptions{Transport: device})
				if err != nil {
					t.Fatal(err)
				}
				entry := reviewMotionOutput(target, settings, func(e *Engine) {
					e.sampleInterval = prototype.sampleInterval
					e.minimumPointIntervalMillis = prototype.minimumPointIntervalMillis
					e.preferredPointIntervalMillis = prototype.preferredPointIntervalMillis
					e.preferSparseGeneratedMotion = prototype.preferSparseGeneratedMotion
					e.positionResolutionPercent = prototype.positionResolutionPercent
					e.resolutionAfterStrokeWindow = prototype.resolutionAfterStrokeWindow
					e.maximumChunkPoints = prototype.maximumChunkPoints
				})
				entry.ID = fmt.Sprintf("%s-%s-%03d", owner, name, speed)
				entry.Name, entry.Group = entry.ID, owner
				entry.Description = "Shared-engine points entering the cloud owner; not physical telemetry. Ultra sends leg targets; VacuGlide plays the engine's follow tempo (ADR 0071)."
				entries = append(entries, entry)
				if entry.Error != "" {
					t.Errorf("%s: %s", entry.ID, entry.Error)
					continue
				}
				for i, point := range entry.Wire {
					if math.IsNaN(point.PositionPercent) || point.PositionPercent < 0 || point.PositionPercent > 100 {
						t.Errorf("%s: invalid point %+v", entry.ID, point)
					}
					if i > 0 && point.TimeMillis <= entry.Wire[i-1].TimeMillis {
						t.Errorf("%s: time not increasing", entry.ID)
					}
				}
				if owner == "vacuglide" && len(entry.Wire) > 0 {
					// The engine derives follow tempo from its plan; the owner
					// maps it onto the default full Tempo range.
					follower := &Engine{plan: NewMotionPlan("visual-review", target, settings, 0, 0, time.Unix(0, 0)), settings: settings}
					end := min(entry.Wire[len(entry.Wire)-1].TimeMillis, 12000)
					for _, level := range follower.followTempoLevelsLocked(0, end, tempoFollowSampleMillis) {
						motor := 0.0
						if level.LevelPercent >= 0.5 {
							motor = 5 + 95*(math.Max(1, level.LevelPercent)-1)/99
						}
						if math.IsNaN(motor) || motor < 0 || motor > 100 {
							t.Errorf("%s: motor=%v", entry.ID, motor)
						}
						motors[entry.ID] = append(motors[entry.ID], [2]float64{float64(level.TimeMillis) / 1000, motor})
					}
				}
			}
		}
	}
	writeSpeedReviewReport(t, path, map[string]any{"schema": "magichandy.motion-atlas.v1", "entries": entries, "motors": motors})
	t.Logf("retained %d trajectories", len(entries))
}
