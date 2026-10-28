package transport

import (
	"errors"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/vibration"
)

func TestVibratorTestDeadlineAllowsNextHealthyPacerTick(t *testing.T) {
	f := newVibratorFixture(t, nil)
	f.selectAndArm(.4, vibration.FollowSteady)
	if err := f.vibrator.Test(); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(1050 * time.Millisecond)
	f.vibrator.lastPace.Store(f.clock.Now().Add(-50 * time.Millisecond).UnixNano())
	f.vibrator.stopStalledOutput(t.Context())
	if f.vibrator.stalled.Load() || !f.vibrator.paceAlive(f.clock.Now()) {
		t.Fatal("ordinary test completion was marked as a stalled pacer")
	}
	f.pace(0)
	if f.vibrator.Status().Testing {
		t.Fatal("test did not end")
	}
}

func TestVibratorStoppedLevelCanOnlyFall(t *testing.T) {
	f := newVibratorFixture(t, nil)
	f.selectAndArm(.4, vibration.FollowSteady)
	if err := f.vibrator.SetModelLevel(.5, false); err != nil {
		t.Fatal(err)
	}
	if err := f.hub.EmergencyStop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.vibrator.SetModelLevel(.9, true); !errors.Is(err, errVibratorStopped) || f.vibrator.ModelLevel() != .5 {
		t.Fatalf("a stale model raise crossed Stop: %v level=%v", err, f.vibrator.ModelLevel())
	}
	if err := f.vibrator.SetModelLevel(.2, true); err != nil || f.vibrator.ModelLevel() != .2 {
		t.Fatalf("lowering after Stop: %v level=%v", err, f.vibrator.ModelLevel())
	}
}

func TestVibratorFailureSurvivesPolicyChangeButNotNextRun(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(.4, vibration.FollowSteady)
	_, _ = run.AppendPoints(t.Context(), strokeCommand("run", 10_000))
	_, _ = run.Play(t.Context(), PlayCommand{StreamID: "run"})
	f.clock.Advance(3 * time.Second)
	f.vibrator.mu.Lock()
	send, ok := f.vibrator.composeLocked(f.clock.Now())
	f.vibrator.mu.Unlock()
	if !ok {
		t.Fatal("missing prepared command")
	}
	_ = f.vibrator.SetModelLevel(.3, false)
	f.vibrator.failSend(send, errIntifaceResponseTime, true)
	if f.vibrator.Status().PlaybackState != "stale" {
		t.Fatal("settings edit hid a failed write from the active run")
	}
}

func TestVibratorEventsAreBoundedAndDetached(t *testing.T) {
	f := newVibratorFixture(t, nil)
	f.selectAndArm(.4, vibration.FollowSteady)
	f.vibrator.mu.Lock()
	for i := 0; i < 140; i++ {
		f.vibrator.addEventLocked("scalar_written", uint64(i), []int{1, 2})
	}
	f.vibrator.mu.Unlock()
	events, dropped := f.vibrator.Events()
	if len(events) != vibratorEventCapacity || dropped < 12 {
		t.Fatalf("unbounded events: %d/%d", len(events), dropped)
	}
	events[0].Levels[0].Percent = 999
	other, _ := f.vibrator.Events()
	if other[0].Levels[0].Percent == 999 {
		t.Fatal("export aliases live event state")
	}
}
