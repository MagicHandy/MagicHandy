package motion

import (
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestConditionalTargetCannotOverwriteNewOwnerOrRestart(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, nil, time.Hour)
	initial, err := engine.Start(t.Context(), testTarget(), config.DefaultSettings().Motion)
	if err != nil {
		t.Fatal(err)
	}
	next := testTarget()
	next.Label, next.SpeedPercent = "new owner", 30
	if _, err := engine.ApplyTarget(t.Context(), next, "new_owner"); err != nil {
		t.Fatal(err)
	}
	commands := len(fake.Commands())
	if _, err := engine.ApplyTargetIfCurrent(t.Context(), testTarget(), "late_reply", initial.PlanID); err == nil {
		t.Fatal("late target replaced the newer plan")
	}
	if len(fake.Commands()) != commands || engine.Snapshot().Target.Label != "new owner" {
		t.Fatal("rejected target dispatched commands")
	}
	if _, err := engine.Stop(t.Context(), "test_stop"); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ApplyTargetIfCurrent(t.Context(), testTarget(), "late_after_stop", initial.PlanID); err == nil || engine.Snapshot().Running {
		t.Fatal("late target restarted motion")
	}
}
