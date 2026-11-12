package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
)

type autoblowRoundTripFunc func(*http.Request) (*http.Response, error)

func (f autoblowRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVacuGlideQueuedValveStepCannotOutliveStop(t *testing.T) {
	f := newAutoblowFixture(t)
	f.options.HealthInterval = time.Hour
	v := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	generation := v.ControlGeneration()
	v.laneMu.Lock()
	done := make(chan error, 1)
	go func() { done <- v.StrokeStepAt(generation, true) }()
	// Hold the request lane so the admitted step cannot yet reach the cloud.
	time.Sleep(30 * time.Millisecond)
	wait := v.BeginEmergencyStop()
	waitFor(t, "Stop admission", func() bool { return v.ControlGeneration() != generation })
	v.laneMu.Unlock()
	if err := wait(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, errAutoblowSuperseded) {
		t.Errorf("queued valve step after Stop = %v, want superseded", err)
	}
	for _, request := range commandsTo(f.sim, "vacuglide", "valve/stroke-plus") {
		if request.Body["valveState"] == true {
			t.Fatal("the queued valve opened after Stop admission")
		}
	}
}

func TestVacuGlideStopClosesValvesWhenMotorStopFails(t *testing.T) {
	f := newAutoblowFixture(t)
	f.options.HealthInterval = time.Hour
	v := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	v.mu.Lock()
	client := v.client
	v.valveOpen = "stroke-plus"
	v.mu.Unlock()
	if _, err := client.Valve(context.Background(), "stroke-plus", true); err != nil {
		t.Fatal(err)
	}
	f.sim.FailNext("vacuglide", http.StatusInternalServerError, 1)
	if err := v.BeginEmergencyStop()(); err == nil {
		t.Fatal("failed motor Stop was reported as successful")
	}
	plus, minus, _ := f.sim.VacuGlideValves()
	if plus || minus {
		t.Fatalf("motor Stop failure skipped valve cleanup: plus=%v minus=%v", plus, minus)
	}
}

func TestVacuGlideUncertainValveOpenStillCloses(t *testing.T) {
	f := newAutoblowFixture(t)
	f.options.HealthInterval = time.Hour
	var loseReply atomic.Bool
	f.options.Client.HTTPClient = &http.Client{Transport: autoblowRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		response, err := http.DefaultTransport.RoundTrip(r)
		if err == nil && strings.HasSuffix(r.URL.Path, "/valve/stroke-plus") && loseReply.CompareAndSwap(true, false) {
			_ = response.Body.Close()
			return nil, context.DeadlineExceeded
		}
		return response, err
	})}
	v := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	loseReply.Store(true)
	if err := v.StrokeStepAt(v.ControlGeneration(), true); err == nil {
		t.Fatal("lost valve acknowledgment must be reported")
	}
	plus, minus, _ := f.sim.VacuGlideValves()
	if plus || minus {
		t.Fatalf("an applied valve command stayed open after its reply was lost: plus=%v minus=%v", plus, minus)
	}
}

func TestAutoblowReconnectDoesNotReplayLostStream(t *testing.T) {
	f := newAutoblowFixture(t)
	f.options.HealthInterval = 20 * time.Millisecond
	v := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	session, _ := v.MotionTransport()
	play(t, session, sweep(100, 400))
	waitFor(t, "initial motion", func() bool { return f.sim.VacuGlideSpeed() > 0 })
	f.sim.SetOnline("vacuglide", false)
	waitFor(t, "connection loss", func() bool { return !v.Connected() })
	f.sim.SetOnline("vacuglide", true)
	if _, err := v.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := len(commandsTo(f.sim, "vacuglide", "target-speed"))
	time.Sleep(150 * time.Millisecond)
	if got := len(commandsTo(f.sim, "vacuglide", "target-speed")); got != after || f.sim.VacuGlideSpeed() != 0 {
		t.Fatalf("reconnect replayed the old stream: commands %d -> %d; speed=%v", after, got, f.sim.VacuGlideSpeed())
	}
}

func TestAutoblowHealthErrorsEndSteadyPlayback(t *testing.T) {
	f := newAutoblowFixture(t)
	f.options.HealthInterval = 20 * time.Millisecond
	var broken atomic.Bool
	f.options.Client.HTTPClient = &http.Client{Transport: autoblowRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if broken.Load() && strings.HasSuffix(r.URL.Path, "/state") {
			return nil, errors.New("simulated lost state response")
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	v := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	session, _ := v.MotionTransport()
	play(t, session, sweep(100, 400))
	waitFor(t, "initial motion", func() bool { return f.sim.VacuGlideSpeed() > 0 })
	broken.Store(true)
	waitFor(t, "health loss to retire the connection", func() bool { return !v.Connected() })
	if session.Diagnostics().PlaybackState != "stale" || f.sim.VacuGlideSpeed() != 0 {
		t.Fatalf("health loss did not stop and retire playback: %+v, motor=%v", session.Diagnostics(), f.sim.VacuGlideSpeed())
	}
}

func TestVacuGlideConcurrentValveStepsRespectTheLimit(t *testing.T) {
	f := newAutoblowFixture(t)
	f.options.HealthInterval = time.Hour
	v := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	v.mu.Lock()
	v.steps = vacuglideMaxSteps - 1
	v.mu.Unlock()
	generation := v.ControlGeneration()
	start, results := make(chan struct{}), make(chan error, 3)
	var pending sync.WaitGroup
	for range 3 {
		pending.Add(1)
		go func() { defer pending.Done(); <-start; results <- v.StrokeStepAt(generation, true) }()
	}
	close(start)
	pending.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, errVacuGlideStrokeLimit) {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || v.Status().StrokeSteps != vacuglideMaxSteps {
		t.Fatalf("concurrent step limit: successful=%d, steps=%d", succeeded, v.Status().StrokeSteps)
	}
}

func TestAutoblowDelayedStopDoesNotStopFreshPlayback(t *testing.T) {
	f := newAutoblowFixture(t)
	f.options.HealthInterval = time.Hour
	v := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	entered, completed := make(chan struct{}), make(chan error, 1)
	go func() {
		completed <- v.command(v.ControlGeneration(), 3*time.Second, func(ctx context.Context, _ *autoblow.Client) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-entered
	if err := v.BeginEmergencyStop()(); err != nil {
		t.Fatal(err)
	}
	if err := <-completed; !errors.Is(err, context.Canceled) {
		t.Fatalf("in-flight command=%v", err)
	}
	stops := len(commandsTo(f.sim, "vacuglide", "target-speed/stop"))
	play(t, v, sweep(20, 400))
	waitFor(t, "fresh playback", func() bool { return f.sim.VacuGlideSpeed() > 0 })
	time.Sleep(autoblowStopConfirm + 100*time.Millisecond)
	if got := len(commandsTo(f.sim, "vacuglide", "target-speed/stop")); got != stops || f.sim.VacuGlideSpeed() == 0 {
		t.Fatalf("old cleanup interrupted new playback: stops=%d -> %d", stops, got)
	}
}

func TestVacuGlideUncertainTestStartStillStops(t *testing.T) {
	f := newAutoblowFixture(t)
	f.options.HealthInterval = time.Hour
	var loseReply atomic.Bool
	f.options.Client.HTTPClient = &http.Client{Transport: autoblowRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		response, err := http.DefaultTransport.RoundTrip(r)
		if err == nil && strings.HasSuffix(r.URL.Path, "/target-speed") && loseReply.CompareAndSwap(true, false) {
			_ = response.Body.Close()
			return nil, context.DeadlineExceeded
		}
		return response, err
	})}
	v := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	loseReply.Store(true)
	if err := v.TestAt(v.ControlGeneration()); err == nil {
		t.Fatal("lost start acknowledgment was accepted")
	}
	if speed := f.sim.VacuGlideSpeed(); speed != 0 {
		t.Fatalf("test remained running after its reply was lost: %v", speed)
	}
}
