//go:build magichandy_labs

package motion

import (
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// This opt-in review samples the same prepared motion with each transport's
// capabilities. It never creates a transport or connects hardware. Its output
// records commanded trajectories, not measured carriage movement.
func TestTransportParityReview(t *testing.T) {
	path := os.Getenv("MAGICHANDY_TRANSPORT_PARITY_REPORT")
	if path == "" {
		t.Skip("set MAGICHANDY_TRANSPORT_PARITY_REPORT for an inert comparison")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	var entries []Review
	var metrics []map[string]any
	for _, speed := range []int{10, 20, 25, 35, 43, 75} {
		for _, name := range []string{"full-sweeps", "soft-turns", "pace-wave", "creative", "layered", "v2-full", "v2-varied", "v2-directional"} {
			spec := DefaultFlowSpec()
			spec.MinPercent, spec.MaxPercent, spec.SpeedPercent = 0, 100, speed
			g := DefaultGestureSpec()
			g.FocusMixPercent, g.VariationPercent = 0, 0
			spec.Gesture = &g
			switch name {
			case "v2-varied":
				g = DefaultGestureSpec()
			case "v2-directional":
				g.FasterDirection, g.ContrastPercent, g.InertiaPercent = "tip", 65, 70
			case "layered":
				spec.Gesture = nil
			}
			target, err := FlowTarget(spec, settings)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "full-sweeps", "soft-turns", "pace-wave":
				ids := map[string]PatternID{"full-sweeps": PatternFullSweeps, "soft-turns": "flow-soft-sweeps", "pace-wave": "flow-pace-wave"}
				target = MotionTarget{PatternID: ids[name], SpeedPercent: speed}
			case "creative":
				d := NormalizeDynamicDefinition(DynamicDefinition{CenterPercent: 50, SpanPercent: 100, SpanMinPercent: 100, SpanProfile: "steady"})
				target = MotionTarget{Dynamic: &d, SpeedPercent: speed}
			}
			plan := NewMotionPlan("parity", target, settings, 0, 0, time.Unix(0, 0))
			if err := plan.compilationError(); err != nil {
				t.Fatal(err)
			}
			for _, owner := range []string{"cloud", "intiface", "intiface-handy-100ms"} {
				entry := ReviewMotionOutput(target, settings)
				entry.ID, entry.Name, entry.Group = fmt.Sprintf("%s-%s-%d", owner, name, speed), name, owner
				entry.Wire = nil
				engine := &Engine{plan: plan, settings: settings, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
					preservePlanKnots: true, positionResolutionPercent: 1, maximumChunkPoints: maximumAdaptiveChunkPoints}
				if owner != "cloud" {
					engine.minimumPointIntervalMillis = 50
					engine.resolutionAfterStrokeWindow = true
					if owner == "intiface-handy-100ms" {
						// Central 3.2.2 reports 100ms for the real Handy. The
						// transport honors that value plus its 10ms margin.
						engine.minimumPointIntervalMillis = 110
						engine.preferredPointIntervalMillis = 180
						entry.Description = "Handy on Intiface Central 3.2.2: advertised 100ms, hard 110ms floor, preferred 180ms spacing on generated curves."
					}
				}
				for engine.nextSampleMillis < 12000 {
					points, err := engine.nextMotionSamplesLocked()
					if err != nil {
						entry.Error = err.Error()
						break
					}
					for _, p := range points {
						entry.Wire = append(entry.Wire, CurvePoint{TimeMillis: p.TimeMillis, PositionPercent: math.Round(p.PositionPercent)})
					}
				}
				// Natural append boundaries differ. Compare the same 12 seconds,
				// rather than letting the final partial stroke bias mean speed.
				for i := 1; i < len(entry.Wire); i++ {
					if entry.Wire[i].TimeMillis >= 12000 {
						l, r := entry.Wire[i-1], entry.Wire[i]
						entry.Wire[i] = CurvePoint{TimeMillis: 12000, PositionPercent: l.PositionPercent + (r.PositionPercent-l.PositionPercent)*float64(12000-l.TimeMillis)/float64(r.TimeMillis-l.TimeMillis)}
						entry.Wire = entry.Wire[:i+1]
						break
					}
				}
				var distance, peak, rms, maxError float64
				var samples int
				for i := 1; i < len(entry.Wire); i++ {
					l, r := entry.Wire[i-1], entry.Wire[i]
					distance += math.Abs(r.PositionPercent - l.PositionPercent)
					peak = math.Max(peak, math.Abs(r.PositionPercent-l.PositionPercent)*1000/float64(r.TimeMillis-l.TimeMillis))
					for at := l.TimeMillis; at < r.TimeMillis; at += 5 {
						x := l.PositionPercent + (r.PositionPercent-l.PositionPercent)*float64(at-l.TimeMillis)/float64(r.TimeMillis-l.TimeMillis)
						deviation := math.Abs(x - sampleMotionPath(plan, nil, at).PositionPercent)
						rms += deviation * deviation
						maxError = math.Max(maxError, deviation)
						samples++
					}
				}
				rate := 0.0
				if len(entry.Wire) > 1 {
					rate = distance * 1000 / float64(entry.Wire[len(entry.Wire)-1].TimeMillis-entry.Wire[0].TimeMillis)
				}
				row := map[string]any{"id": entry.ID, "mean_travel_per_second": rate, "peak_velocity": peak, "rms_position_error": math.Sqrt(rms / float64(max(1, samples))), "max_position_error": maxError, "points": len(entry.Wire), "error": entry.Error}
				entries, metrics = append(entries, entry), append(metrics, row)
				t.Logf("%s: mean=%.2f peak=%.2f max_error=%.3f error=%s", entry.ID, rate, peak, maxError, entry.Error)
			}
		}
	}
	writeSpeedReviewReport(t, path, map[string]any{"schema": "magichandy.motion-atlas.v1", "entries": entries, "metrics": metrics})
}
