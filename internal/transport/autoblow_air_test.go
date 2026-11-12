package transport

import (
	"context"
	"errors"
	"testing"
)

func TestVacuGlideAirIsNotReplayedOrAppliedAfterStop(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100, MotionAir: true})
	session, _ := device.MotionTransport()
	play(t, session, sweep(30, 400))
	command := AppendPointsCommand{StreamID: "s1", Air: &AirAdjustment{Revision: 3, Steps: 1}}
	for range 2 {
		if _, err := session.AppendPoints(t.Context(), command); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, net := f.sim.VacuGlideValves(); net != 1 {
		t.Fatalf("duplicate command applied %d steps", net)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	command.Air.Revision++
	if _, err := session.AppendPoints(ctx, command); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled air = %v", err)
	}
	if err := device.BeginEmergencyStop()(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendPoints(t.Context(), command); err == nil {
		t.Fatal("old air was admitted after Stop")
	}
	if _, _, net := f.sim.VacuGlideValves(); net != 1 {
		t.Fatalf("stopped/canceled air applied: net=%d", net)
	}
}

func TestVacuGlideAirBudgetSurvivesPauseResume(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100, MotionAir: true})
	session, _ := device.MotionTransport()
	play(t, session, sweep(30, 400))
	for i := uint64(1); i <= 3; i++ {
		if _, err := session.AppendPoints(t.Context(), AppendPointsCommand{StreamID: "s1", Air: &AirAdjustment{Revision: i, Steps: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := session.Stop(t.Context(), StopCommand{Reason: "pause"}); err != nil {
		t.Fatal(err)
	}
	points := &TempoFrame{Revision: 1, Points: []TimedLevel{{TimeMillis: 0, LevelPercent: 30}, {TimeMillis: 10_000, LevelPercent: 30}}}
	if _, err := session.AppendPoints(t.Context(), AppendPointsCommand{StreamID: "resumed", Tempo: points}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Play(t.Context(), PlayCommand{StreamID: "resumed", Resume: true}); err != nil {
		t.Fatal(err)
	}
	if got := device.Status().MotionAirSteps; got != 3 {
		t.Fatalf("resume reset air budget to %d", got)
	}
	if _, err := session.AppendPoints(t.Context(), AppendPointsCommand{StreamID: "resumed", Air: &AirAdjustment{Revision: 1, Steps: 1}}); !errors.Is(err, ErrAirRejected) {
		t.Fatalf("resume bypassed air bound: %v", err)
	}
	if _, err := session.Stop(t.Context(), StopCommand{Reason: "stop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendPoints(t.Context(), AppendPointsCommand{StreamID: "new", Tempo: points}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Play(t.Context(), PlayCommand{StreamID: "new"}); err != nil {
		t.Fatal(err)
	}
	if got := device.Status().MotionAirSteps; got != 0 {
		t.Fatalf("new run kept old air budget %d", got)
	}
}

func TestVacuGlideQueuedAirCannotCrossReconnect(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100, MotionAir: true})
	session, _ := device.MotionTransport()
	play(t, session, sweep(30, 400))
	device.valveMu.Lock()
	locked := true
	t.Cleanup(func() {
		if locked {
			device.valveMu.Unlock()
		}
	})
	done := make(chan error, 1)
	go func() {
		_, err := session.AppendPoints(context.Background(), AppendPointsCommand{StreamID: "s1", Air: &AirAdjustment{Revision: 1, Steps: 1}})
		done <- err
	}()
	if _, err := device.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	device.valveMu.Unlock()
	locked = false
	if err := <-done; !errors.Is(err, errAutoblowSessionEnded) {
		t.Fatalf("queued old air = %v", err)
	}
	if plus, minus, net := f.sim.VacuGlideValves(); plus || minus || net != 0 {
		t.Fatalf("old air reached new connection: %v/%v/%d", plus, minus, net)
	}
}

func TestAirPermissionRejectionDoesNotRetireHealthyDevice(t *testing.T) {
	f := newAutoblowFixture(t)
	device := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
	session, _ := device.MotionTransport()
	linked := NewLinkedSession([]LinkedParticipant{{ID: "vacuglide", Label: "VacuGlide", Transport: session}}, true)
	play(t, linked, sweep(30, 400))
	_, err := linked.AppendPoints(t.Context(), AppendPointsCommand{StreamID: "s1", Air: &AirAdjustment{Revision: 1, Steps: 1}})
	if !errors.Is(err, ErrAirRejected) {
		t.Fatalf("permission error = %v", err)
	}
	if state := linked.Diagnostics(); !state.Connected || state.PlaybackState == "stale" {
		t.Fatalf("rejection retired healthy output: %+v", state)
	}
	if status := device.Status(); !status.Connected || status.PlaybackState != "playing" {
		t.Fatalf("rejected air stopped playback: %+v", status)
	}
	if _, _, net := f.sim.VacuGlideValves(); net != 0 {
		t.Fatalf("unpermitted air steps = %d", net)
	}
}
