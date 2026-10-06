package motion

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// TestFreestyleStreamDump exports every profile/pace/feel/seed in the safety
// review matrix from shared MotionPlans. It creates no transport or timers.
// Set MAGICHANDY_FREESTYLE_STREAM_DUMP to an ignored JSON path, then render
// with scripts/render-freestyle-streams.py. It is opt-in development tooling.
func TestFreestyleStreamDump(t *testing.T) {
	path := os.Getenv("MAGICHANDY_FREESTYLE_STREAM_DUMP")
	if path == "" {
		t.Skip("set MAGICHANDY_FREESTYLE_STREAM_DUMP to an ignored output path")
	}
	report := map[string]any{}
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		for _, speed := range []int{10, 43, 100} {
			for _, seed := range []uint32{1, 17, 0x5eed} {
				for _, feel := range []string{config.FreestyleFeelGentle, config.FreestyleFeelBalanced, config.FreestyleFeelIntense} {
					preset, _ := config.FreestyleFeelPreset(feel)
					controls := FreestyleControls{PacePercent: preset.PacePercent, LengthPercent: preset.LengthPercent,
						FocusPercent: preset.FocusPercent, RoamingPercent: preset.RoamingPercent,
						VarietyPercent: preset.VarietyPercent, EnergyPercent: 100}
					settings := config.DefaultSettings().Motion
					settings.HandyModel, settings.SpeedMinPercent, settings.SpeedMaxPercent = model, 1, speed
					spec := NewFreestyleFlow(seed, speed, FreestyleSpec{Strokes: 128, MinSpeedPercent: 1, Keyframes: []FreestyleKeyframe{{Controls: controls}}})
					name := fmt.Sprintf("%s/%d/%d/%s", model, speed, seed, feel)
					report[name] = renderFreestyleStream(t, spec, settings)
				}
			}
		}
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- Explicit opt-in local test artifact path, never an HTTP request or runtime setting.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func renderFreestyleStream(t *testing.T, spec FlowSpec, settings config.MotionSettings) map[string]any {
	t.Helper()
	started := time.Now()
	target, err := FlowTarget(spec, settings)
	if err != nil {
		t.Fatal(err)
	}
	compile := time.Since(started)
	plan := NewMotionPlan("render", target, settings, 0, 0, time.Unix(0, 0))
	samples, velocities := make([]float64, 0, 6000), make([]float64, 0, 6000)
	retargets, blends := 0, 0
	for at := int64(0); at < 120_000; at += 20 {
		progress := plan.freestyleProgress(at, at)
		if progress.RemainingMillis < 40_000 {
			nextSpec := spec
			nextSpec.Freestyle = cloneFreestyleSpec(spec.Freestyle)
			nextSpec.Freestyle.StartStroke = progress.PlayingStroke
			next := plan.retargetFromState("next", MotionTarget{Label: "Freestyle", Flow: &nextSpec}, settings, at,
				plan.SampleAt(at).PositionPercent, plan.DirectionAt(at), plan.VelocityAt(at), time.Unix(0, 0))
			if transitionRequired(plan, nil, next, at) {
				blends++
			}
			plan = next
			retargets++
		}
		samples = append(samples, plan.SampleAt(at).PositionPercent)
		velocities = append(velocities, plan.VelocityAt(at))
	}
	if blends != 0 {
		t.Fatalf("rendered stream needed %d blends", blends)
	}
	return map[string]any{"samples": samples, "velocities": velocities, "rate_hz": 50, "compile_ms": float64(compile.Microseconds()) / 1000,
		"points": len(target.prepared.curve.points), "window_ms": target.prepared.curve.duration, "retargets": retargets, "blends": blends,
		"peak_velocity":     target.prepared.curve.maximumVelocityPerMillis() * 1000,
		"peak_acceleration": target.prepared.curve.maximumAccelerationPerMillis2() * 1e6,
		"peak_jerk":         target.prepared.curve.maximumJerkPerMillis3() * 1e9, "limits": settings, "controls": spec.Freestyle.Keyframes[0].Controls}
}
