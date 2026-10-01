package motion

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestFreestyleWindowsFitAllDeviceProfilesAndPaces(t *testing.T) {
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		for _, speed := range []int{10, 43, 100} {
			t.Run(fmt.Sprintf("%s/%d", model, speed), func(t *testing.T) {
				settings := config.DefaultSettings().Motion
				settings.HandyModel, settings.SpeedMinPercent, settings.SpeedMaxPercent = model, 1, speed
				for _, seed := range []uint32{1, 17, 0x5eed} {
					for _, feel := range []string{config.FreestyleFeelGentle, config.FreestyleFeelBalanced, config.FreestyleFeelIntense} {
						preset, _ := config.FreestyleFeelPreset(feel)
						controls := FreestyleControls{PacePercent: preset.PacePercent, LengthPercent: preset.LengthPercent,
							FocusPercent: preset.FocusPercent, RoamingPercent: preset.RoamingPercent,
							VarietyPercent: preset.VarietyPercent, EnergyPercent: 100}
						assertSafeFreestyleWindows(t, settings, seed, controls)
					}
				}
			})
		}
	}
}

func assertSafeFreestyleWindows(t *testing.T, settings config.MotionSettings, seed uint32, controls FreestyleControls) {
	t.Helper()
	compile := func(start int) *preparedMotion {
		target, err := FlowTarget(NewFreestyleFlow(seed, settings.SpeedMaxPercent,
			FreestyleSpec{StartStroke: start, Strokes: 64, MinSpeedPercent: settings.SpeedMinPercent,
				Keyframes: []FreestyleKeyframe{{Controls: controls}}}), settings)
		if err != nil {
			t.Fatalf("seed %d controls %+v: %v", seed, controls, err)
		}
		plan := NewMotionPlan("safety", target, settings, 0, 0, time.Unix(0, 0))
		if err := plan.compilationError(); err != nil {
			t.Fatal(err)
		}
		if plan.PeriodMillis != target.prepared.curve.duration {
			t.Fatalf("seed %d controls %+v: the shared plan retimed the stream", seed, controls)
		}
		for at := int64(0); at <= plan.PeriodMillis; at += 19 {
			position := plan.SampleAt(at).PositionPercent
			if math.IsNaN(position) || position < 0 || position > 100 {
				t.Fatalf("unsafe sample %v at %d ms", position, at)
			}
		}
		return target.prepared
	}
	sameStrokes(t, compile(0), compile(29), 29, 64)
}

func TestFreestyleCompactionKeepsEventAndTimingHistory(t *testing.T) {
	for _, seed := range []uint32{1, 7, 17, 91, 0x5eed} {
		for _, start := range []int{23, 29, 37, 55} {
			initial, changed := freestyleControls(), freestyleControls()
			initial.PacePercent, initial.VarietyPercent = 0, 100
			changed.PacePercent, changed.LengthPercent, changed.VarietyPercent = 100, 0, 0
			keyframes := []FreestyleKeyframe{{Controls: initial}, {Stroke: 12, RampStrokes: 10, Controls: changed}}
			full := freestyleWindow(start, 64, keyframes...)
			full.Seed = seed
			compacted := full
			compacted.Freestyle = cloneFreestyleSpec(full.Freestyle)
			compacted.Freestyle.Keyframes = CompactFreestyleKeyframes(keyframes, start)
			_, before := compileFreestyleWindow(t, full)
			_, after := compileFreestyleWindow(t, compacted)
			sameStrokes(t, before, after, start, start+64)
		}
	}
}
