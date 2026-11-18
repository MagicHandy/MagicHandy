package deviceapp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type resumeWindowGate struct {
	*transport.Fake
	block   atomic.Bool
	entered chan struct{}
}

func (t *resumeWindowGate) SetStrokeWindow(ctx context.Context, window transport.StrokeWindowCommand) (transport.CommandResult, error) {
	if t.block.Load() {
		close(t.entered)
		<-ctx.Done()
		return transport.CommandResult{}, ctx.Err()
	}
	return t.Fake.SetStrokeWindow(ctx, window)
}

func TestStopCanInterruptResumeAndRetireOwner(t *testing.T) {
	var life Lifecycle
	owner := &resumeWindowGate{Fake: transport.NewFake(), entered: make(chan struct{})}
	build := func() (*motion.Engine, error) { return motion.NewEngine(motion.EngineOptions{Transport: owner}) }
	engine, admission, err := life.Admit(func() (string, bool, error) { return "first", false, nil }, build)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultSettings().Motion
	if _, err = engine.StartAtGeneration(t.Context(), motion.MotionTarget{PatternID: motion.PatternFullSweeps, SpeedPercent: 35}, settings, admission); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	if _, err = engine.Pause(t.Context(), "pause"); err != nil {
		t.Fatal(err)
	}
	owner.block.Store(true)
	done := make(chan error, 1)
	go func() {
		_, e := life.Resume(t.Context(), engine, func() config.MotionSettings { return settings })
		done <- e
	}()
	select {
	case <-owner.entered:
	case <-time.After(time.Second):
		t.Fatal("Resume did not reach owner")
	}
	release := life.Hold()
	outcome, err := life.Stop(t.Context(), "interrupt", func(string) error { return nil }, func() (transport.CommandResult, error) {
		t.Error("fallback reached with engine")
		return transport.CommandResult{}, nil
	})
	release()
	if err != nil || !outcome.Engine || engine.Observation().Running {
		t.Fatal("Stop did not invalidate the admitted engine")
	}
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("interrupted Resume succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("Stop waited for settings serialization")
	}
	next, _, err := life.Admit(func() (string, bool, error) { return "replacement", false, nil }, build)
	if err != nil || next == engine {
		t.Fatal("replacement inherited the old owner's engine")
	}
	if _, err = engine.StartAtGeneration(t.Context(), motion.MotionTarget{}, settings, admission); err == nil {
		t.Fatal("old admission could command the retired engine")
	}
}
