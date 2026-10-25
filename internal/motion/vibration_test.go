package motion

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestVibrationEnvelopeBoundsAndContinuity(t *testing.T) {
	for _, mode := range []string{"steady", "wave", "pulse", "off", "follow"} {
		for _, period := range []float64{0.5, 3, 30} {
			spec := VibrationSpec{Mode: mode, MinPercent: 10, MaxPercent: 85, PeriodSeconds: period}
			if mode == "steady" {
				spec.MinPercent, spec.PeriodSeconds = spec.MaxPercent, 0
			}
			if mode == "off" || mode == "follow" {
				spec.MinPercent, spec.MaxPercent, spec.PeriodSeconds = 0, 0, 0
			}
			if err := spec.Validate(); err != nil {
				t.Fatal(err)
			}
			plan := vibrationPlan{spec: spec, fromLevel: 20}
			previous := plan.LevelAt(0)
			for at := int64(1); at < 60_000; at++ {
				value := plan.LevelAt(at)
				if math.IsNaN(value) || value < 0 || value > 100 || math.Abs(value-previous) > 2 {
					t.Fatalf("%s period=%g at=%d value=%g previous=%g", mode, period, at, value, previous)
				}
				previous = value
			}
		}
	}
}

func TestCanceledVibrationEditDoesNotStopHealthyRun(t *testing.T) {
	linear, scalar := transport.NewFake(), transport.NewFake()
	owner := transport.NewLinkedSession([]transport.LinkedParticipant{{ID: "handy", Transport: linear}, {ID: "vibrator", Transport: scalar}}, false)
	e := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
	if _, err := e.Start(t.Context(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = e.Stop(context.Background(), "cleanup") })
	before := e.Snapshot()
	count := len(scalar.Commands())
	admission := e.AdmissionGeneration()
	e.commandMu.Lock()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := e.ApplyVibration(ctx, VibrationSpec{Mode: "off"}, admission); done <- err }()
	cancel()
	e.commandMu.Unlock()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	after := e.Snapshot()
	if !after.Running || after.Generation != before.Generation || after.Target.Vibration != nil || count != len(scalar.Commands()) {
		t.Fatal("canceled turn changed or stopped output")
	}
}

func TestLinearTargetPromotesVibrationOnlyThroughPreparedStart(t *testing.T) {
	linear, scalar := transport.NewFake(), transport.NewFake()
	owner := transport.NewLinkedSession([]transport.LinkedParticipant{{ID: "handy", Transport: linear}, {ID: "vibrator", Transport: scalar}}, false)
	e := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
	target := testTarget()
	target.VibrationOnly, target.Vibration = true, &VibrationSpec{Mode: "steady", MinPercent: 30, MaxPercent: 30}
	if _, err := e.Start(t.Context(), target, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = e.Stop(context.Background(), "cleanup") })
	after, err := e.ApplyTarget(t.Context(), testTarget(), "library_or_autopilot")
	if err != nil || !after.Running || after.Target.VibrationOnly || after.Target.Vibration == nil {
		t.Fatalf("promotion failed: %+v %v", after, err)
	}
	if countCommands(linear.Commands(), transport.CommandKindPointsPlay) != 1 || countCommands(linear.Commands(), transport.CommandKindStrokeWindow) != 1 {
		t.Fatal("Handy was not prepared and started")
	}
}

func TestRetiredVibratorCannotReportSuccessfulEditOrStopHealthyHandy(t *testing.T) {
	linear, scalar := transport.NewFake(), transport.NewFake()
	owner := transport.NewLinkedSession([]transport.LinkedParticipant{{ID: "handy", Transport: linear}, {ID: "vibrator", Transport: scalar}}, true)
	e := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
	if _, err := e.Start(t.Context(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = e.Stop(context.Background(), "cleanup") })
	// Retire through the real linked failure policy by making the scalar owner
	// unhealthy, then sample diagnostics as the engine health loop does.
	if _, err := scalar.Stop(t.Context(), transport.StopCommand{Reason: "lost"}); err != nil {
		t.Fatal(err)
	}
	_ = owner.Diagnostics()
	status := owner.(transport.LinkedSessionStatusProvider).LinkedSessionStatus()
	if len(status.RetiredDevices) != 1 || status.RetiredDevices[0] != "vibrator" {
		t.Fatalf("not retired: %+v", status)
	}
	count := len(linear.Commands())
	_, err := e.ApplyVibration(t.Context(), VibrationSpec{Mode: "steady", MinPercent: 40, MaxPercent: 40}, e.AdmissionGeneration())
	if !errors.Is(err, transport.ErrVibrationUnavailable) || !e.Snapshot().Running || len(linear.Commands()) != count || e.Snapshot().Target.Vibration != nil {
		t.Fatalf("retired output affected survivor: %v", err)
	}
}

func TestVibrationChangePreservesLinearPlanAndStopFence(t *testing.T) {
	linear, scalar := transport.NewFake(), transport.NewFake()
	owner := transport.NewLinkedSession([]transport.LinkedParticipant{{ID: "handy", Label: "Handy", Transport: linear}, {ID: "vibrator", Label: "Vibrator", Transport: scalar}}, false)
	engine := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
	if _, err := engine.Start(t.Context(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	before := engine.Snapshot()
	linearBefore := len(linear.Commands())
	admission := engine.AdmissionGeneration()
	spec := VibrationSpec{Mode: "wave", MinPercent: 15, MaxPercent: 70, PeriodSeconds: 4}
	after, err := engine.ApplyVibration(t.Context(), spec, admission)
	if err != nil {
		t.Fatal(err)
	}
	if after.PlanID != before.PlanID || after.NextSampleMillis != before.NextSampleMillis || after.Target.SpeedPercent != before.Target.SpeedPercent || !reflect.DeepEqual(after.Target.Flow, before.Target.Flow) || len(linear.Commands()) != linearBefore {
		t.Fatal("vibration edit touched the Handy plan or dispatch")
	}
	last := scalar.Commands()[len(scalar.Commands())-1].PointsAdd
	if last == nil || len(last.Points) != 0 || last.Vibration == nil || len(last.Vibration.Points) < 2 {
		t.Fatalf("missing independent frame: %+v", last)
	}
	countBeforeRepair := len(scalar.Commands())
	if _, err := engine.ApplyVibrationIfCurrent(t.Context(), spec, admission, before.Generation); err == nil || len(scalar.Commands()) != countBeforeRepair {
		t.Fatal("stale repair overwrote a newer vibration change")
	}
	if _, err := engine.Stop(t.Context(), "stop"); err != nil {
		t.Fatal(err)
	}
	count := len(scalar.Commands())
	if _, err := engine.ApplyVibration(t.Context(), spec, admission); err == nil || len(scalar.Commands()) != count {
		t.Fatal("late change crossed Stop")
	}
}

func TestVibrationOnlyStartDoesNotDispatchToIdleHandy(t *testing.T) {
	linear, scalar := transport.NewFake(), transport.NewFake()
	owner := transport.NewLinkedSession([]transport.LinkedParticipant{{ID: "handy", Label: "Handy", Transport: linear}, {ID: "vibrator", Label: "Vibrator", Transport: scalar}}, false)
	engine := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
	target := testTarget()
	target.VibrationOnly, target.Vibration = true, &VibrationSpec{Mode: "steady", MinPercent: 40, MaxPercent: 40}
	state, err := engine.Start(t.Context(), target, config.DefaultSettings().Motion)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	if !state.Running || len(linear.Commands()) != 0 || countCommands(scalar.Commands(), transport.CommandKindPointsPlay) != 1 {
		t.Fatal("vibration-only start reached idle Handy")
	}
	if owner.Diagnostics().PlaybackState == "stale" {
		t.Fatal("idle Handy was treated as failed participant")
	}
	if _, err := engine.Stop(t.Context(), "global_stop"); err != nil {
		t.Fatal(err)
	}
	if countCommands(linear.Commands(), transport.CommandKindStop) != 1 || countCommands(scalar.Commands(), transport.CommandKindStop) != 1 {
		t.Fatal("global Stop missed an output")
	}
}

func TestVibrationOnlyPauseResumeAndNewStart(t *testing.T) {
	linear, scalar := transport.NewFake(), transport.NewFake()
	owner := transport.NewLinkedSession([]transport.LinkedParticipant{{ID: "handy", Label: "Handy", Transport: linear}, {ID: "vibrator", Label: "Vibrator", Transport: scalar}}, false)
	engine := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	target := testTarget()
	spec := VibrationSpec{Mode: "pulse", MinPercent: 10, MaxPercent: 60, PeriodSeconds: 3}
	target.VibrationOnly, target.Vibration = true, &spec
	if _, err := engine.Start(t.Context(), target, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Pause(t.Context(), "pause"); err != nil {
		t.Fatal(err)
	}
	count := len(scalar.Commands())
	if _, err := engine.ApplyVibration(t.Context(), spec, engine.AdmissionGeneration()); err == nil || count != len(scalar.Commands()) {
		t.Fatal("model changed paused output")
	}
	resumed, err := engine.Resume(t.Context(), "resume")
	if err != nil || !resumed.Target.VibrationOnly || resumed.Target.Vibration == nil || *resumed.Target.Vibration != spec {
		t.Fatalf("resume lost envelope: %+v %v", resumed, err)
	}
	if countCommands(linear.Commands(), transport.CommandKindPointsPlay) != 0 {
		t.Fatal("resume started idle Handy")
	}
	if _, err := engine.Stop(t.Context(), "stop"); err != nil {
		t.Fatal(err)
	}
	started, err := engine.Start(t.Context(), testTarget(), config.DefaultSettings().Motion)
	if err != nil || started.Target.Vibration != nil || started.Target.VibrationOnly {
		t.Fatalf("new run inherited override: %+v %v", started, err)
	}
}
