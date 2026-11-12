package transport

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
	"github.com/mapledaemon/MagicHandy/internal/autoblowsim"
)

type autoblowFixture struct {
	sim     *autoblowsim.Server
	options AutoblowOptions
	mu      sync.Mutex
	pressed []string
}

func newAutoblowFixture(t *testing.T) *autoblowFixture {
	t.Helper()
	sim, err := autoblowsim.StartWith(autoblowsim.Options{Heartbeat: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &autoblowFixture{sim: sim}
	fixture.options = AutoblowOptions{
		Client:         autoblow.Options{LatencyURL: sim.URL(), ClusterURL: sim.ClusterURL},
		Simulated:      true,
		Tick:           10 * time.Millisecond,
		HealthInterval: 200 * time.Millisecond,
		OnButton: func(device, button string) {
			fixture.mu.Lock()
			fixture.pressed = append(fixture.pressed, device+":"+button)
			fixture.mu.Unlock()
		},
	}
	t.Cleanup(func() { _ = sim.Close() })
	return fixture
}

func (f *autoblowFixture) buttons() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.pressed...)
}

func connectedUltra(t *testing.T, f *autoblowFixture) *AutoblowUltra {
	t.Helper()
	ultra := NewAutoblowUltra(f.options)
	ultra.Configure(AutoblowConfig{Token: "ultra-token"})
	if _, err := ultra.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ultra.Close() })
	return ultra
}

func connectedVacuGlide(t *testing.T, f *autoblowFixture, config AutoblowConfig) *VacuGlide {
	t.Helper()
	device := NewVacuGlide(f.options)
	config.Token = "vacuglide-token"
	device.Configure(config)
	if _, err := device.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = device.Close() })
	return device
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func commandsTo(sim *autoblowsim.Server, family, path string) []autoblowsim.Request {
	var matched []autoblowsim.Request
	for _, request := range sim.Requests() {
		if request.Family == family && request.Path == path {
			matched = append(matched, request)
		}
	}
	return matched
}

// sweep builds strokes between 10 and 90 with legs of leg milliseconds.
func sweep(legs int, leg int64) []TimedPoint {
	points := make([]TimedPoint, 0, legs+1)
	for index := 0; index <= legs; index++ {
		position := 10.0
		if index%2 == 1 {
			position = 90
		}
		points = append(points, TimedPoint{TimeMillis: int64(index) * leg, PositionPercent: position})
	}
	return points
}

func play(t *testing.T, owner Transport, points []TimedPoint) {
	t.Helper()
	ctx := context.Background()
	if _, err := owner.AppendPoints(ctx, AppendPointsCommand{StreamID: "s1", Points: points}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "s1"}); err != nil {
		t.Fatal(err)
	}
}

func TestUltraSendsOneGotoPerLegAndNothingAfterStop(t *testing.T) {
	f := newAutoblowFixture(t)
	ultra := connectedUltra(t, f)
	session, err := ultra.MotionTransport()
	if err != nil {
		t.Fatal(err)
	}
	play(t, session, sweep(8, 400))
	waitFor(t, "four legs", func() bool { return len(commandsTo(f.sim, "autoblow", "goto")) >= 5 })
	gotos := commandsTo(f.sim, "autoblow", "goto")
	// The startup move goes to the first point gently, then each leg targets
	// its turning point at the speed that covers it in time: 80 units in
	// 400 ms is 200 units per second, half of the Ultra's 400.
	if gotos[0].Body["position"] != 10.0 || gotos[0].Body["speed"] != ultraStartupSpeed {
		t.Fatalf("startup move = %+v", gotos[0].Body)
	}
	for _, request := range gotos[1:4] {
		position, speed := request.Body["position"].(float64), request.Body["speed"].(float64)
		if (position != 90 && position != 10) || speed < 40 || speed > 70 {
			t.Fatalf("leg = %+v, want a turning point at about 50%% speed", request.Body)
		}
	}
	if _, err := session.Stop(context.Background(), StopCommand{Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	stopped := len(f.sim.Requests())
	if stops := commandsTo(f.sim, "autoblow", "oscillate/stop"); len(stops) < 2 {
		t.Fatalf("stop requests = %d, want the connect stop and this one", len(stops))
	}
	time.Sleep(150 * time.Millisecond)
	for _, request := range f.sim.Requests()[stopped:] {
		if request.Path == "goto" {
			t.Fatalf("a goto followed Stop: %+v", request)
		}
	}
	if status := ultra.Status(); status.PlaybackState != "idle" || !status.Connected {
		t.Fatalf("status after Stop = %+v", status)
	}
}

func TestUltraProjectsTheStrokeWindowOnce(t *testing.T) {
	f := newAutoblowFixture(t)
	ultra := connectedUltra(t, f)
	session, _ := ultra.MotionTransport()
	if _, err := session.SetStrokeWindow(context.Background(), StrokeWindowCommand{MinPercent: 20, MaxPercent: 80, ReverseDirection: true}); err != nil {
		t.Fatal(err)
	}
	play(t, session, sweep(4, 400))
	waitFor(t, "a leg", func() bool { return len(commandsTo(f.sim, "autoblow", "goto")) >= 2 })
	gotos := commandsTo(f.sim, "autoblow", "goto")
	// Reversed 10 is 90 of the semantic span; 20 + 0.9 * 60 = 74. Reversed 90 is 26.
	if gotos[0].Body["position"] != 74.0 || gotos[1].Body["position"] != 26.0 {
		t.Fatalf("projected positions = %v then %v, want 74 then 26", gotos[0].Body["position"], gotos[1].Body["position"])
	}
}

func TestUltraReportsStarvationWhenTheFrameRunsOut(t *testing.T) {
	f := newAutoblowFixture(t)
	ultra := connectedUltra(t, f)
	session, _ := ultra.MotionTransport()
	play(t, session, sweep(2, 200))
	waitFor(t, "starvation", func() bool { return session.Diagnostics().PlaybackState == "starved" })
}

func TestVacuGlideFollowsPaceAndStopsTheMotor(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{SpeedScale: 1})
	session, _ := device.MotionTransport()
	// 80 units every 400 ms is 200 per second: a third of full motor speed.
	play(t, session, sweep(30, 400))
	waitFor(t, "the motor to settle near a third", func() bool { return math.Abs(f.sim.VacuGlideSpeed()-33) <= 3 })
	if status := device.Status(); status.CommandedPercent == nil || math.Abs(*status.CommandedPercent-33) > 3 {
		t.Fatalf("commanded = %+v", status.CommandedPercent)
	}
	if _, err := session.Stop(context.Background(), StopCommand{Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	if speed := f.sim.VacuGlideSpeed(); speed != 0 {
		t.Fatalf("motor speed after Stop = %.0f", speed)
	}
}

func TestVacuGlideStopsWhenTheFrameHolds(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{SpeedScale: 1})
	session, _ := device.MotionTransport()
	points := append(sweep(4, 300), TimedPoint{TimeMillis: 4000, PositionPercent: 10}, TimedPoint{TimeMillis: 9000, PositionPercent: 10})
	play(t, session, points)
	waitFor(t, "the motor to start", func() bool { return f.sim.VacuGlideSpeed() > 0 })
	waitFor(t, "the motor to stop for the hold", func() bool { return f.sim.VacuGlideSpeed() == 0 })
}

func TestVacuGlideStrokeStepsPulseOneValveAndCount(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{SpeedScale: 1})
	if err := device.StrokeStepAt(device.ControlGeneration(), true); err != nil {
		t.Fatal(err)
	}
	plusOpen, minusOpen, net := f.sim.VacuGlideValves()
	if plusOpen || minusOpen || net != 1 {
		t.Fatalf("valves after a step: plus %v minus %v net %d; want both closed after one plus", plusOpen, minusOpen, net)
	}
	if err := device.StrokeStepAt(0, false); err != nil {
		t.Fatal(err)
	}
	if status := device.Status(); status.StrokeSteps != 0 || status.ValveOpen != "" {
		t.Fatalf("status = %+v", status)
	}
	// A longer step requested before a Stop does not run after it.
	generation := device.ControlGeneration()
	_ = device.BeginEmergencyStop()()
	if err := device.StrokeStepAt(generation, true); !errors.Is(err, errAutoblowSuperseded) {
		t.Fatalf("a stale longer step = %v", err)
	}
}

func TestAutoblowSessionsEndWithTheirConnection(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{SpeedScale: 1})
	old, _ := device.MotionTransport()
	if _, err := device.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := old.AppendPoints(context.Background(), AppendPointsCommand{StreamID: "s", Points: sweep(2, 300)}); !errors.Is(err, errAutoblowSessionEnded) {
		t.Fatalf("an old engine appended through a new connection: %v", err)
	}
	if result, err := old.Stop(context.Background(), StopCommand{}); err != nil || result.Status != "connection_retired" {
		t.Fatalf("old Stop = %+v, %v", result, err)
	}
	if old.Diagnostics().PlaybackState != "stale" {
		t.Fatal("an old session must report stale")
	}
}

func TestAutoblowDeviceLeavingOnlineModeIsLost(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{SpeedScale: 1})
	session, _ := device.MotionTransport()
	play(t, session, sweep(30, 400))
	waitFor(t, "the motor to start", func() bool { return f.sim.VacuGlideSpeed() > 0 })
	f.sim.SetOnline("vacuglide", false)
	waitFor(t, "the loss to be noticed", func() bool { return !device.Connected() })
	if status := device.Status(); status.PlaybackState != "stale" || status.LastError == "" {
		t.Fatalf("status = %+v, want stale with a reason", status)
	}
	if _, err := session.Stop(context.Background(), StopCommand{}); !errors.Is(err, errAutoblowStale) {
		t.Fatalf("Stop on a lost connection = %v, want unconfirmed", err)
	}
}

func TestAutoblowButtonsReachTheCallback(t *testing.T) {
	f := newAutoblowFixture(t)
	ultra := connectedUltra(t, f)
	waitFor(t, "the event stream", func() bool { return ultra.Status().ButtonEvents })
	f.sim.Press("autoblow", autoblow.ButtonUltraPause)
	waitFor(t, "the button", func() bool { return len(f.buttons()) == 1 })
	if got := f.buttons()[0]; got != "autoblow:"+autoblow.ButtonUltraPause {
		t.Fatalf("button = %q", got)
	}
}

func TestAutoblowTestRunsBrieflyAndStopsItself(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{SpeedScale: 1})
	if err := device.TestAt(device.ControlGeneration()); err != nil {
		t.Fatal(err)
	}
	if speed := f.sim.VacuGlideSpeed(); speed != vacuglideTestSpeed {
		t.Fatalf("test speed = %.0f", speed)
	}
	waitFor(t, "the test to end", func() bool { return f.sim.VacuGlideSpeed() == 0 && !device.Status().Testing })
}

func TestAutoblowConnectFailsCleanlyWhenOffline(t *testing.T) {
	f := newAutoblowFixture(t)
	f.sim.SetOnline("autoblow", false)
	ultra := NewAutoblowUltra(f.options)
	ultra.Configure(AutoblowConfig{Token: "ultra-token"})
	defer func() { _ = ultra.Close() }()
	if _, err := ultra.Connect(context.Background()); !errors.Is(err, autoblow.ErrOffline) {
		t.Fatalf("offline connect = %v", err)
	}
	if status := ultra.Status(); status.Connected || status.Connecting || status.LastError == "" {
		t.Fatalf("status = %+v", status)
	}
	if _, err := ultra.MotionTransport(); err == nil {
		t.Fatal("an engine was admitted without a connection")
	}
}
