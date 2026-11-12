package transport

import (
	"context"
	"errors"
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

// playTempoLevel is the tempo play sends beside positions: 34 plays the
// VacuGlide at 37% across its full range.
const playTempoLevel = 34

func play(t *testing.T, owner Transport, points []TimedPoint) {
	t.Helper()
	tempo := steadyTempo(playTempoLevel, points[0].TimeMillis, points[len(points)-1].TimeMillis)
	playFrames(t, owner, points, tempo)
}

func playFrames(t *testing.T, owner Transport, points []TimedPoint, tempo *TempoFrame) {
	t.Helper()
	ctx := context.Background()
	if _, err := owner.AppendPoints(ctx, AppendPointsCommand{StreamID: "s1", Points: points, Tempo: tempo}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "s1"}); err != nil {
		t.Fatal(err)
	}
}

// steadyTempo is one engine tempo frame holding level over [from, to].
func steadyTempo(level float64, from, to int64) *TempoFrame {
	return tempoSteps(from, to, func(int64) float64 { return level })
}

func tempoSteps(from, to int64, level func(at int64) float64) *TempoFrame {
	frame := &TempoFrame{Revision: 1}
	for at := from; at <= to; at += 100 {
		frame.Points = append(frame.Points, TimedLevel{TimeMillis: at, LevelPercent: level(at)})
	}
	return frame
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

func TestVacuGlidePlaysTempoAcrossItsRangeAndStops(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	session, _ := device.MotionTransport()
	// Tempo 34 is a third of the way up the full range: 5 + 95 * 33/99 = 36.7.
	play(t, session, sweep(30, 400))
	waitFor(t, "the motor to reach 37 percent", func() bool { return f.sim.VacuGlideSpeed() == 37 })
	if status := device.Status(); status.CommandedPercent == nil || *status.CommandedPercent != 37 {
		t.Fatalf("commanded = %+v", status.CommandedPercent)
	}
	if _, err := session.Stop(context.Background(), StopCommand{Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	if speed := f.sim.VacuGlideSpeed(); speed != 0 {
		t.Fatalf("motor speed after Stop = %.0f", speed)
	}
}

func TestVacuGlideTempoRangeBoundsTheMotor(t *testing.T) {
	for _, test := range []struct {
		level, want float64
	}{{100, 60}, {1, 20}, {0.2, 0}} {
		f := newAutoblowFixture(t)
		device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 20, TempoMaxPercent: 60})
		session, _ := device.MotionTransport()
		playFrames(t, session, sweep(30, 400), steadyTempo(test.level, 0, 12000))
		if test.want == 0 {
			time.Sleep(300 * time.Millisecond)
			if speed := f.sim.VacuGlideSpeed(); speed != 0 {
				t.Fatalf("tempo %.1f ran the motor at %.0f", test.level, speed)
			}
			continue
		}
		waitFor(t, "the bounded speed", func() bool { return f.sim.VacuGlideSpeed() == test.want })
	}
}

func TestVacuGlideStopsForATempoPause(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	session, _ := device.MotionTransport()
	tempo := tempoSteps(0, 9000, func(at int64) float64 {
		if at < 1500 {
			return 40
		}
		return 0
	})
	playFrames(t, session, sweep(20, 400), tempo)
	waitFor(t, "the motor to start", func() bool { return f.sim.VacuGlideSpeed() > 0 })
	waitFor(t, "the motor to stop for the pause", func() bool { return f.sim.VacuGlideSpeed() == 0 })
	if !device.Status().Connected {
		t.Fatal("a pause must not end the connection")
	}
}

func TestVacuGlideKeepsTheNewestTempoRevision(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	session, _ := device.MotionTransport()
	playFrames(t, session, sweep(30, 400), steadyTempo(34, 0, 12000))
	waitFor(t, "the first tempo", func() bool { return f.sim.VacuGlideSpeed() == 37 })
	// A newer envelope replaces the tail; an older revision is ignored.
	newer := steadyTempo(100, 0, 12000)
	newer.Revision = 3
	if _, err := session.AppendPoints(context.Background(), AppendPointsCommand{StreamID: "s1", Tempo: newer}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the newer tempo", func() bool { return f.sim.VacuGlideSpeed() == 100 })
	older := steadyTempo(1, 0, 12000)
	older.Revision = 2
	if _, err := session.AppendPoints(context.Background(), AppendPointsCommand{StreamID: "s1", Tempo: older}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if speed := f.sim.VacuGlideSpeed(); speed != 100 {
		t.Fatalf("an older revision changed the motor to %.0f", speed)
	}
	bad := &TempoFrame{Revision: 4, Points: []TimedLevel{{TimeMillis: 10, LevelPercent: 120}}}
	if _, err := session.AppendPoints(context.Background(), AppendPointsCommand{StreamID: "s1", Tempo: bad}); err == nil {
		t.Fatal("a tempo above 100 was accepted")
	}
}

func TestVacuGlideMotionAirNeedsPermissionAndStaysBounded(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	var revision uint64
	air := func(steps int) error {
		revision++
		device.mu.Lock()
		id := device.stream.id
		device.mu.Unlock()
		_, err := device.AppendPoints(t.Context(), AppendPointsCommand{StreamID: id, Air: &AirAdjustment{Revision: revision, Steps: steps}})
		return err
	}
	if err := air(1); !errors.Is(err, errVacuGlideAirOff) {
		t.Fatalf("air without permission = %v", err)
	}
	allowed := device.config
	allowed.MotionAir = true
	device.Configure(allowed)
	if err := air(1); !errors.Is(err, errVacuGlideNotPlaying) {
		t.Fatalf("air while idle = %v", err)
	}
	session, _ := device.MotionTransport()
	play(t, session, sweep(30, 400))
	if err := air(2); err != nil {
		t.Fatal(err)
	}
	if err := air(2); !errors.Is(err, errVacuGlideAirLimit) {
		t.Fatalf("a fourth tightening step = %v", err)
	}
	if _, _, net := f.sim.VacuGlideValves(); net != 2 {
		t.Fatalf("rejected adjustment partially applied: net=%d", net)
	}
	if err := air(1); err != nil {
		t.Fatal(err)
	}
	if status := device.Status(); status.MotionAirSteps != 3 || status.StrokeSteps != 3 || status.ValveOpen != "" {
		t.Fatalf("status = %+v, want three motion steps and closed valves", status)
	}
	if _, _, net := f.sim.VacuGlideValves(); net != 3 {
		t.Fatalf("simulated net steps = %d", net)
	}
	// Manual steps stay the person's, past motion's bound.
	if err := device.StrokeStepAt(device.ControlGeneration(), true); err != nil {
		t.Fatal(err)
	}
	// A new session starts motion's count again from the person's air.
	_ = device.BeginEmergencyStop()()
	play(t, session, sweep(30, 400))
	if err := air(-1); err != nil {
		t.Fatal(err)
	}
	if status := device.Status(); status.MotionAirSteps != -1 {
		t.Fatalf("motion steps after a new session = %d", status.MotionAirSteps)
	}
}

func TestVacuGlideStrokeStepsPulseOneValveAndCount(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
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
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
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
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
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
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
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
