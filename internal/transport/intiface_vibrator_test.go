package transport

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/vibration"
)

// fakeComboDevice is a stroker with a built-in vibration motor.
func fakeComboDevice(index uint32) map[string]any {
	device := fakeLinearDevice(index, "Test Combo")
	device["DeviceMessages"].(map[string]any)["ScalarCmd"] = []map[string]any{{"FeatureDescriptor": "Vibe", "ActuatorType": "Vibrate", "StepCount": 20}}
	return device
}

type vibratorFixture struct {
	t        *testing.T
	server   *fakeButtplugServer
	hub      *IntifaceHub
	vibrator *IntifaceVibrator
	clock    *manualClock
}

// newVibratorFixture connects a hub whose vibrator pacer only runs when the
// test calls pace, on a manual clock.
func newVibratorFixture(t *testing.T, configure func(*fakeButtplugServer)) *vibratorFixture {
	t.Helper()
	server := newFakeButtplugServer(t, 0)
	server.extraDevices = []map[string]any{fakeScalarDevice(3, "Test Vibrator", 20, "Vibrate", "Rotate", "Vibrate")}
	if configure != nil {
		configure(server)
	}
	t.Cleanup(server.Close)
	hub, err := NewIntifaceHub(IntifaceOptions{Address: server.URL()})
	if err != nil {
		t.Fatal(err)
	}
	clock := &manualClock{now: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	hub.vibrator.clock = clock.Now
	hub.vibrator.manualPace = true
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := hub.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	return &vibratorFixture{t: t, server: server, hub: hub, vibrator: hub.vibrator, clock: clock}
}

func (f *vibratorFixture) selectAndArm(intensity float64, follow string) Transport {
	f.t.Helper()
	if err := f.vibrator.Select(context.Background(), 3); err != nil {
		f.t.Fatal(err)
	}
	f.vibrator.Configure(VibratorConfig{Profile: vibration.Profile{Follow: follow, DepthEnd: vibration.DepthBase}, Ceiling: 0.4})
	f.vibrator.SetIntensity(intensity)
	run, err := f.vibrator.MotionTransport()
	if err != nil {
		f.t.Fatal(err)
	}
	return run
}

// pace runs one tick after advancing the clock and waits for its send, if
// any, to be acknowledged or dropped.
func (f *vibratorFixture) pace(advance time.Duration) {
	f.t.Helper()
	f.clock.Advance(advance)
	f.vibrator.paceOnce(f.hub.session.sessionCtx)
	waitForTest(f.t, time.Second, func() bool {
		f.vibrator.mu.Lock()
		defer f.vibrator.mu.Unlock()
		return !f.vibrator.inFlight
	})
}

// lastScalars returns the scalar of each actuator index in the latest ScalarCmd.
func lastScalars(t *testing.T, server *fakeButtplugServer) map[int]float64 {
	t.Helper()
	history := server.historySnapshot()
	for index := len(history) - 1; index >= 0; index-- {
		if history[index].kind != "ScalarCmd" {
			continue
		}
		scalars := map[int]float64{}
		for _, raw := range history[index].fields["Scalars"].([]any) {
			scalar := raw.(map[string]any)
			scalars[int(scalar["Index"].(float64))] = scalar["Scalar"].(float64)
		}
		return scalars
	}
	return nil
}

func countKind(server *fakeButtplugServer, kind string) int {
	count := 0
	for _, message := range server.historySnapshot() {
		if message.kind == kind {
			count++
		}
	}
	return count
}

func TestVibratorSelectionUsesOnlyVibrateMotorsAndStartsAtZero(t *testing.T) {
	f := newVibratorFixture(t, nil)
	if err := f.vibrator.Select(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if index, _ := f.server.waitForKind(t, "StopDeviceCmd", time.Second).uint32("DeviceIndex"); index != 3 {
		t.Fatalf("Select stopped device %d", index)
	}
	status := f.vibrator.Status()
	if !status.Selected || len(status.Motors) != 2 || status.Motors[0].Index != 0 || status.Motors[1].Index != 2 || status.IntensityPercent != 0 {
		t.Fatalf("status = %+v", status)
	}
	if err := f.vibrator.Select(context.Background(), 7); !errors.Is(err, errVibratorNoMotors) {
		t.Fatalf("a linear-only device was accepted: %v", err)
	}
}

func TestVibratorCannotTakeTheStrokersDevice(t *testing.T) {
	f := newVibratorFixture(t, func(server *fakeButtplugServer) {
		server.extraDevices = append(server.extraDevices, fakeComboDevice(5))
	})
	if err := f.hub.Linear().Select(context.Background(), 5, 0); err != nil {
		t.Fatal(err)
	}
	if err := f.vibrator.Select(context.Background(), 5); !errors.Is(err, errIntifaceDeviceClaimed) {
		t.Fatalf("the vibrator claimed the stroker's device: %v", err)
	}
}

func TestVibratorFollowsTheFrameAndSendsOnlyStepChanges(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(0.4, vibration.FollowSteady)
	ctx := context.Background()
	if _, err := run.AppendPoints(ctx, strokeCommand("s", 10_000)); err != nil {
		t.Fatal(err)
	}
	if _, err := run.Play(ctx, PlayCommand{StreamID: "s"}); err != nil {
		t.Fatal(err)
	}
	// Fade-in: the level rises over two seconds to the live intensity.
	f.pace(1120 * time.Millisecond)
	half := lastScalars(t, f.server)
	if math.Abs(half[0]-0.2) > 0.051 || half[2] != half[0] || half[1] != 0 {
		t.Fatalf("mid fade-in scalars = %v", half)
	}
	f.pace(2 * time.Second)
	full := lastScalars(t, f.server)
	if full[0] != 0.4 || full[2] != 0.4 {
		t.Fatalf("settled scalars = %v, want the 40%% intensity", full)
	}
	sent := countKind(f.server, "ScalarCmd")
	f.pace(100 * time.Millisecond)
	if countKind(f.server, "ScalarCmd") != sent {
		t.Fatal("an unchanged level was sent again")
	}
	// Lowering the intensity takes effect on the next send.
	f.vibrator.SetIntensity(0.1)
	f.pace(0)
	if lowered := lastScalars(t, f.server); lowered[0] != 0.1 {
		t.Fatalf("lowered scalars = %v", lowered)
	}
}

func TestVibratorStopSilencesAndFencesTheOldGeneration(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(0.4, vibration.FollowSteady)
	ctx := context.Background()
	_, _ = run.AppendPoints(ctx, strokeCommand("s", 10_000))
	_, _ = run.Play(ctx, PlayCommand{StreamID: "s"})
	f.pace(3 * time.Second)
	stops := countKind(f.server, "StopDeviceCmd")
	if _, err := run.Stop(ctx, StopCommand{Reason: "pause"}); err != nil {
		t.Fatal(err)
	}
	if lastScalars(t, f.server)[0] != 0 || countKind(f.server, "StopDeviceCmd") != stops+1 {
		t.Fatal("Stop must send a zero level and StopDeviceCmd")
	}
	sent := countKind(f.server, "ScalarCmd")
	f.pace(time.Second)
	if countKind(f.server, "ScalarCmd") != sent {
		t.Fatal("a level followed Stop")
	}
}

func TestVibratorMissingAcknowledgementStopsTheDevice(t *testing.T) {
	f := newVibratorFixture(t, func(server *fakeButtplugServer) { server.dropScalar = true })
	f.hub.session.responseTimeout = 50 * time.Millisecond
	run := f.selectAndArm(0.4, vibration.FollowSteady)
	ctx := context.Background()
	_, _ = run.AppendPoints(ctx, strokeCommand("s", 10_000))
	_, _ = run.Play(ctx, PlayCommand{StreamID: "s"})
	stops := countKind(f.server, "StopDeviceCmd")
	f.pace(3 * time.Second)
	waitForTest(t, time.Second, func() bool { return countKind(f.server, "StopDeviceCmd") > stops })
	if state := run.Diagnostics().PlaybackState; state != "stale" {
		t.Fatalf("state after a missing acknowledgement = %q", state)
	}
	if status := f.vibrator.Status(); status.TimeoutCount != 1 {
		t.Fatalf("status = %+v", status)
	}
}

func TestVibratorEmergencyStopZeroesTheIntensity(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(0.3, vibration.FollowSteady)
	ctx := context.Background()
	_, _ = run.AppendPoints(ctx, strokeCommand("s", 10_000))
	_, _ = run.Play(ctx, PlayCommand{StreamID: "s"})
	if err := f.hub.EmergencyStop(ctx); err != nil {
		t.Fatal(err)
	}
	if status := f.vibrator.Status(); status.IntensityPercent != 0 || status.PlaybackState != "idle" {
		t.Fatalf("after Emergency Stop: %+v", status)
	}
}

func TestVibratorRunIsFencedAfterReselection(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(0.3, vibration.FollowSteady)
	if err := f.vibrator.Select(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if _, err := run.AppendPoints(context.Background(), strokeCommand("s", 1000)); !errors.Is(err, errVibratorStale) {
		t.Fatalf("an old run appended to the reselected vibrator: %v", err)
	}
	if result, err := run.Stop(context.Background(), StopCommand{}); err != nil || result.Status != "connection_retired" {
		t.Fatalf("old run Stop = %+v, %v", result, err)
	}
}

func TestVibratorTestStaysBelowTheTestCeilingAndEnds(t *testing.T) {
	f := newVibratorFixture(t, nil)
	f.selectAndArm(0.4, vibration.FollowSteady)
	if err := f.vibrator.Test(); err != nil {
		t.Fatal(err)
	}
	f.pace(0)
	if scalars := lastScalars(t, f.server); scalars[0] != 0.25 {
		t.Fatalf("test scalars = %v, want 25%%", scalars)
	}
	f.pace(vibratorTestDuration)
	if scalars := lastScalars(t, f.server); scalars[0] != 0 {
		t.Fatalf("after the test: %v", scalars)
	}
	f.vibrator.SetIntensity(0)
	if err := f.vibrator.Test(); !errors.Is(err, errVibratorSilent) {
		t.Fatalf("a silent test was accepted: %v", err)
	}
}

func TestVibratorStalledPacerStopsTheDeviceAndThePing(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(0.3, vibration.FollowSteady)
	ctx := context.Background()
	_, _ = run.AppendPoints(ctx, strokeCommand("s", 10_000))
	_, _ = run.Play(ctx, PlayCommand{StreamID: "s"})
	stops := countKind(f.server, "StopDeviceCmd")
	// The pacer never runs while the clock moves on.
	f.clock.Advance(2 * time.Second)
	if f.hub.session.ownersAlive(f.clock.Now()) {
		t.Fatal("a stalled pacer must stop the keepalive")
	}
	waitForTest(t, 2*time.Second, func() bool { return countKind(f.server, "StopDeviceCmd") > stops })
	if state := run.Diagnostics().PlaybackState; state != "stale" {
		t.Fatalf("state after a stall = %q", state)
	}
}

func TestLinkedSessionStartsTheVibratorOnTheStrokersClock(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(0.3, vibration.FollowSteady)
	start := f.clock.Now().Add(400 * time.Millisecond)
	stroker := &clockedOwner{failingLinkedOwner: &failingLinkedOwner{Fake: NewFake()}, start: start}
	session := NewLinkedSession([]LinkedParticipant{
		{ID: "handy", Label: "Handy", Transport: stroker},
		{ID: "vibrator", Label: "Vibrator", Transport: run},
	}, false)
	ctx := context.Background()
	if _, err := session.AppendPoints(ctx, strokeCommand("s", 10_000)); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Play(ctx, PlayCommand{StreamID: "s"}); err != nil {
		t.Fatal(err)
	}
	if got := f.vibrator.PlaybackStartTime(); !got.Equal(start) {
		t.Fatalf("vibrator origin %v, want the stroker's %v", got, start)
	}
}
