package motion

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestPreviousFlowPaceOnlyDescribesTheCurrentRun(t *testing.T) {
	engine := newTestEngine(t, transport.NewFake(), diagnostics.NewTraceRing(32), time.Hour)
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	limits := config.DefaultSettings().Motion
	target := func(pace int) MotionTarget {
		spec := DefaultFlowSpec()
		spec.SpeedPercent = pace
		result, err := FlowTarget(spec, limits)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first, err := engine.Start(t.Context(), target(48), limits)
	if err != nil || first.PreviousFlowSpeedPercent != 0 {
		t.Fatalf("unexpected initial pace history: %d, %v", first.PreviousFlowSpeedPercent, err)
	}
	changed, err := engine.ApplyTarget(t.Context(), target(25), "pace edit")
	if err != nil || changed.PreviousFlowSpeedPercent != 48 {
		t.Fatalf("previous commanded pace = %d, %v", changed.PreviousFlowSpeedPercent, err)
	}
	if _, err := engine.ApplyTargetIfCurrent(t.Context(), target(63), "late edit", first.PlanID); err == nil {
		t.Fatal("accepted an edit based on an obsolete plan")
	}
	if state := engine.Snapshot(); state.PreviousFlowSpeedPercent != 48 || state.Target.SpeedPercent != 25 {
		t.Fatal("a rejected edit entered pace history")
	}
	paused, err := engine.Pause(t.Context(), "pause")
	if err != nil || paused.PreviousFlowSpeedPercent != 0 {
		t.Fatalf("paused run exposed a previous pace: %d, %v", paused.PreviousFlowSpeedPercent, err)
	}
	resumed, err := engine.Resume(t.Context(), "resume")
	if err != nil || resumed.StreamID == changed.StreamID || resumed.PreviousFlowSpeedPercent != 0 {
		t.Fatalf("resumed stream inherited a previous pace: %d, %v", resumed.PreviousFlowSpeedPercent, err)
	}
	stopped, err := engine.Stop(t.Context(), "end run")
	if err != nil || stopped.PreviousFlowSpeedPercent != 0 {
		t.Fatalf("stopped run exposed a prior pace: %d, %v", stopped.PreviousFlowSpeedPercent, err)
	}
	fresh, err := engine.Start(t.Context(), target(65), limits)
	if err != nil || fresh.PreviousFlowSpeedPercent != 0 {
		t.Fatalf("new run inherited a prior pace: %d, %v", fresh.PreviousFlowSpeedPercent, err)
	}
}

func TestReplacedUnsentFlowPaceDoesNotEnterHistory(t *testing.T) {
	limits := config.DefaultSettings().Motion
	now := time.Unix(0, 0)
	plan := func(id string, pace int) MotionPlan {
		spec := DefaultFlowSpec()
		spec.SpeedPercent = pace
		target, err := FlowTarget(spec, limits)
		if err != nil {
			t.Fatal(err)
		}
		return NewMotionPlan(id, target, limits, 0, 0, now)
	}
	original, queued, replacement := plan("original", 42), plan("queued", 57), plan("replacement", 23)
	engine := Engine{plan: original, nextSampleMillis: 1_000}
	engine.installEditLocked(original, nil, &queued, 1_000, 0)
	before, transition := engine.editPathLocked(0)
	engine.installEditLocked(before, transition, &replacement, 1_000, 0)
	if replacement.previousFlowSpeed != 42 {
		t.Fatalf("replacement remembered an unsent speed: %d", replacement.previousFlowSpeed)
	}
	next := plan("same pace", 23)
	engine.installEditLocked(replacement, nil, &next, 2_000, 1_000)
	if next.previousFlowSpeed != 42 {
		t.Fatalf("an equal-speed edit displaced the previous distinct pace: %d", next.previousFlowSpeed)
	}
}
