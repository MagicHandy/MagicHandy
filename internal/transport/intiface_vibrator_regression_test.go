package transport

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/vibration"
)

// Regression probes from the alpha.73 user report. All devices are an in-process fake server.
func TestVibratorStartupCoverage(t *testing.T) {
	for _, follow := range []string{vibration.FollowSpeed, vibration.FollowDepth, vibration.FollowSteady} {
		t.Run(follow, func(t *testing.T) {
			f := newVibratorFixture(t, nil)
			run := f.selectAndArm(0.4, follow)
			if _, err := run.AppendPoints(context.Background(), strokeCommand("review-start", 10_000)); err != nil {
				t.Fatal(err)
			}
			if _, err := run.Play(context.Background(), PlayCommand{StreamID: "review-start"}); err != nil {
				t.Fatal(err)
			}
			for elapsed := 100; elapsed <= 500; elapsed += 100 {
				f.pace(100 * time.Millisecond)
				state := f.vibrator.Status().PlaybackState
				t.Logf("wall=%dms stream=%dms profile=%s state=%s", elapsed, elapsed-120, follow, state)
				if state == "starving" || state == "starved" {
					t.Errorf("fully buffered startup reported %s at %dms; engine health checks will stop it", state, elapsed)
				}
			}
		})
	}
}

func TestVibratorRechecksLoweredLimitsAtWrite(t *testing.T) {
	for _, change := range []string{"intensity", "ceiling"} {
		t.Run(change, func(t *testing.T) {
			f := newVibratorFixture(t, nil)
			run := f.selectAndArm(0.4, vibration.FollowSteady)
			if _, err := run.AppendPoints(context.Background(), strokeCommand("review-limits", 10_000)); err != nil {
				t.Fatal(err)
			}
			if _, err := run.Play(context.Background(), PlayCommand{StreamID: "review-limits"}); err != nil {
				t.Fatal(err)
			}
			at := f.clock.Now().Add(3 * time.Second)
			f.vibrator.lastPace.Store(at.UnixNano())
			f.clock.Advance(3 * time.Second)
			f.vibrator.mu.Lock()
			send, ok := f.vibrator.composeLocked(at)
			f.vibrator.mu.Unlock()
			if !ok {
				t.Fatal("no composed level")
			}
			// This is the real race boundary: a command has been prepared,
			// but has not yet passed the session writer admission check.
			if change == "intensity" {
				if _, err := f.vibrator.SetIntensityAt(f.vibrator.ControlGeneration(), 0.1, false); err != nil {
					t.Fatal(err)
				}
			} else {
				f.vibrator.Configure(VibratorConfig{Profile: vibration.Profile{Follow: vibration.FollowSteady}, Ceiling: 0.1})
			}
			f.vibrator.send(f.hub.session.sessionCtx, send)
			waitForTest(t, time.Second, func() bool { f.vibrator.mu.Lock(); defer f.vibrator.mu.Unlock(); return !f.vibrator.inFlight })
			sent := lastScalars(t, f.server)
			t.Logf("lowered %s to 10%%; transmitted=%v", change, sent)
			for index, level := range sent {
				if level > 0.1 {
					t.Errorf("motor %d wrote %.2f after %s was lowered to .10", index, level, change)
				}
			}
		})
	}
}

func TestVibratorQuantizationRespectsLiveIntensity(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(0.03, vibration.FollowSteady)
	if _, err := run.AppendPoints(context.Background(), strokeCommand("review-quantize", 10_000)); err != nil {
		t.Fatal(err)
	}
	if _, err := run.Play(context.Background(), PlayCommand{StreamID: "review-quantize"}); err != nil {
		t.Fatal(err)
	}
	f.pace(3 * time.Second)
	sent := lastScalars(t, f.server)
	t.Logf("live intensity=3%%; transmitted=%v; status=%+v", sent, f.vibrator.Status())
	for index, level := range sent {
		if level > .03 {
			t.Errorf("motor %d level %.2f exceeds live intensity .03", index, level)
		}
	}
}

func TestVibratorCoarseMotorRespectsTestCap(t *testing.T) {
	f := newVibratorFixture(t, func(server *fakeButtplugServer) {
		server.extraDevices = []map[string]any{fakeScalarDevice(3, "Coarse vibrator", 3, "Vibrate")}
	})
	f.selectAndArm(.4, vibration.FollowSteady)
	if err := f.vibrator.Test(); err == nil {
		t.Fatal("coarse motor test should explain that it cannot vibrate within the cap")
	}
	f.pace(0)
	sent := lastScalars(t, f.server)
	t.Logf("25%% test cap; transmitted=%v", sent)
	for index, level := range sent {
		if level > .25 {
			t.Errorf("motor %d test level %.4f exceeds .25 test cap", index, level)
		}
	}
}

func TestVibratorLateFailureDoesNotStopNewRun(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(.4, vibration.FollowSteady)
	ctx := context.Background()
	_, _ = run.AppendPoints(ctx, strokeCommand("old-run", 10_000))
	_, _ = run.Play(ctx, PlayCommand{StreamID: "old-run"})
	at := f.clock.Now().Add(3 * time.Second)
	f.vibrator.lastPace.Store(at.UnixNano())
	f.clock.Advance(3 * time.Second)
	f.vibrator.mu.Lock()
	oldSend, ok := f.vibrator.composeLocked(at)
	f.vibrator.mu.Unlock()
	if !ok {
		t.Fatal("no old send")
	}
	// An old command was written but its acknowledgement can arrive after
	// Pause/Stop and the next run. Deliver that timeout at the handler seam.
	if _, err := run.Stop(ctx, StopCommand{Reason: "pause"}); err != nil {
		t.Fatal(err)
	}
	_, _ = run.AppendPoints(ctx, strokeCommand("new-run", 10_000))
	_, _ = run.Play(ctx, PlayCommand{StreamID: "new-run"})
	f.vibrator.failSend(oldSend, errIntifaceResponseTime, true)
	state := f.vibrator.Status().PlaybackState
	t.Logf("after old timeout, new run state=%s", state)
	if state != "playing" {
		t.Errorf("old generation timeout stopped new run: %s", state)
	}
}

func TestVibratorPendingWriteCannotFollowWatchdogStop(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(.4, vibration.FollowSteady)
	_, _ = run.AppendPoints(context.Background(), strokeCommand("watchdog", 10_000))
	_, _ = run.Play(context.Background(), PlayCommand{StreamID: "watchdog"})
	at := f.clock.Now().Add(3 * time.Second)
	f.vibrator.lastPace.Store(at.UnixNano())
	f.clock.Advance(3 * time.Second)
	f.vibrator.mu.Lock()
	prepared, ok := f.vibrator.composeLocked(at)
	f.vibrator.mu.Unlock()
	if !ok {
		t.Fatal("no composed output")
	}
	stops := countKind(f.server, "StopDeviceCmd")
	f.clock.Advance(2 * time.Second)
	waitForTest(t, time.Second, func() bool { return countKind(f.server, "StopDeviceCmd") > stops })
	f.vibrator.send(f.hub.session.sessionCtx, prepared)
	waitForTest(t, time.Second, func() bool { f.vibrator.mu.Lock(); defer f.vibrator.mu.Unlock(); return !f.vibrator.inFlight })
	sent := lastScalars(t, f.server)
	t.Logf("stalled=%v output after watchdog Stop=%v", f.vibrator.stalled.Load(), sent)
	for index, level := range sent {
		if level > 0 {
			t.Errorf("motor %d wrote %.2f after watchdog Stop", index, level)
		}
	}
}

func TestVibratorWatchdogCoversOneSecondTest(t *testing.T) {
	f := newVibratorFixture(t, nil)
	f.selectAndArm(.4, vibration.FollowSteady)
	if err := f.vibrator.Test(); err != nil {
		t.Fatal(err)
	}
	f.pace(0)
	stops := countKind(f.server, "StopDeviceCmd")
	f.clock.Advance(2 * time.Second)
	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	alive := f.hub.session.ownersAlive(f.clock.Now())
	t.Logf("one-second test after stalled 2s: ownersAlive=%v stop delta=%d output=%v", alive, countKind(f.server, "StopDeviceCmd")-stops, lastScalars(t, f.server))
	if alive && countKind(f.server, "StopDeviceCmd") == stops {
		t.Error("stalled Test is exempt from both watchdog and ping liveness")
	}
}
