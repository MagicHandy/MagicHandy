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
	stopped, err := engine.Stop(t.Context(), "end run")
	if err != nil || stopped.PreviousFlowSpeedPercent != 0 {
		t.Fatalf("stopped run exposed a prior pace: %d, %v", stopped.PreviousFlowSpeedPercent, err)
	}
	fresh, err := engine.Start(t.Context(), target(65), limits)
	if err != nil || fresh.PreviousFlowSpeedPercent != 0 {
		t.Fatalf("new run inherited a prior pace: %d, %v", fresh.PreviousFlowSpeedPercent, err)
	}
}
