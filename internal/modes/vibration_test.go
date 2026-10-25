package modes

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type scalarTestEngine struct {
	fakeEngine
	vibration []motion.VibrationSpec
}

func (e *scalarTestEngine) AdmissionGeneration() uint64 { return 7 }
func (e *scalarTestEngine) ApplyVibration(ctx context.Context, spec motion.VibrationSpec, _ uint64) (motion.ActiveMotionState, error) {
	if err := ctx.Err(); err != nil {
		return motion.ActiveMotionState{}, err
	}
	e.vibration = append(e.vibration, spec)
	return e.Snapshot(), nil
}

func TestAutopilotLinearHoldAppliesOnlyVibration(t *testing.T) {
	e := &scalarTestEngine{}
	m, err := NewManager(Options{Ensure: func(context.Context) (Engine, error) { return e, nil }, Current: func() Engine { return e }, Settings: func() config.MotionSettings { return config.DefaultSettings().Motion }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Shutdown)
	m.loop.mode, m.loop.generation = ModeAutopilot, 1
	spec := motion.VibrationSpec{Mode: "steady", MinPercent: 30, MaxPercent: 30}
	choice := segmentChoice{source: "hold", vibration: &spec, segment: Segment{SpeedPercent: 40, DurationMillis: int64(time.Minute / time.Millisecond)}}
	m.applyAutopilotChoice(t.Context(), e, choice, 1, "test")
	if len(e.vibration) != 1 || e.vibration[0] != spec || len(e.targets) != 0 || len(e.starts) != 0 {
		t.Fatal("scalar hold retargeted linear playback or lost vibration")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := applyHeldVibration(ctx, e, &spec); err == nil || len(e.vibration) != 1 {
		t.Fatal("canceled planner edit applied")
	}
}
