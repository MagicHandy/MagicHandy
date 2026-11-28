package modes

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type observedEdgeEngine struct {
	edgeTestEngine
	delaySnapshot atomic.Bool
	applying      chan struct{}
	apply         chan struct{}
	observing     chan struct{}
	observe       chan struct{}
}

func (e *observedEdgeEngine) ApplyTargetIfCurrent(ctx context.Context, target motion.MotionTarget, reason, id string) (motion.ActiveMotionState, error) {
	if reason == "edge_hold" {
		close(e.applying)
		<-e.apply
	}
	return e.edgeTestEngine.ApplyTargetIfCurrent(ctx, target, reason, id)
}

func (e *observedEdgeEngine) Snapshot() motion.ActiveMotionState {
	state := e.edgeTestEngine.Snapshot()
	if e.delaySnapshot.Swap(false) {
		close(e.observing)
		<-e.observe
	}
	return state
}

func TestStatusDuringEdgeInstallationPreservesReturn(t *testing.T) {
	m, base, _ := edgeManager(t)
	engine := &observedEdgeEngine{edgeTestEngine: base, applying: make(chan struct{}), apply: make(chan struct{}),
		observing: make(chan struct{}), observe: make(chan struct{})}
	m.options.Current = func() Engine { return engine }
	started := make(chan error, 1)
	go func() {
		_, err := m.ActivateEdge(t.Context())
		started <- err
	}()
	<-engine.applying
	engine.delaySnapshot.Store(true)
	observed := make(chan struct{})
	go func() {
		m.Status()
		close(observed)
	}()
	// A read of the prior engine state must not erase a hold installed
	// before that read finishes. An applying hold can also be reported directly.
	select {
	case <-engine.observing:
	case <-observed:
		engine.delaySnapshot.Store(false)
	}
	close(engine.apply)
	err := <-started
	close(engine.observe)
	<-observed
	if err != nil {
		t.Fatal(err)
	}
	if state := m.Status().EdgeHold; state == nil || !state.Active || base.Snapshot().Target.Source != "edge" {
		t.Fatal("the published hold lost its Return control")
	}
	if _, err = m.ResumeEdge(t.Context()); err != nil {
		t.Fatal(err)
	}
}

type failedEdgeApply struct {
	edgeTestEngine
	outcome string
}

func (e failedEdgeApply) ApplyTargetIfCurrent(ctx context.Context, target motion.MotionTarget, reason, id string) (motion.ActiveMotionState, error) {
	if reason != "edge_hold" {
		return e.edgeTestEngine.ApplyTargetIfCurrent(ctx, target, reason, id)
	}
	switch e.outcome {
	case "installed":
		if _, err := e.edgeTestEngine.ApplyTargetIfCurrent(ctx, target, reason, id); err != nil {
			return e.Snapshot(), err
		}
	case "stopped":
		e.setState(false, false)
	case "admission changed":
		return motion.ActiveMotionState{}, motion.ErrEdgeUnavailable
	}
	return e.Snapshot(), errors.New("simulated target application failure")
}

func TestEdgeApplyFailureReflectsEngineOutcome(t *testing.T) {
	for _, outcome := range []string{"unconfirmed while running", "admission changed", "installed", "stopped"} {
		t.Run(outcome, func(t *testing.T) {
			m, base, _ := edgeManager(t)
			m.options.Current = func() Engine { return failedEdgeApply{base, outcome} }
			m.loop.mode = ModeChat
			target := base.Snapshot().Target
			m.chat.target, m.chat.keepalive = &target, true
			if _, err := m.ActivateEdge(t.Context()); err == nil {
				t.Fatal("missing application error")
			}
			if outcome == "installed" {
				hold := m.Status().EdgeHold
				if hold == nil || !hold.Active || hold.Automatic {
					t.Fatalf("installed target needs a manual Return: %+v", hold)
				}
				if _, err := m.ActivateEdge(t.Context()); err != nil {
					t.Fatal("a second press replaced the installed hold", err)
				}
				if _, err := m.ResumeEdge(t.Context()); err != nil || base.Snapshot().Target.SpeedPercent != target.SpeedPercent {
					t.Fatal("lost original target", err)
				}
				return
			}
			if m.tickEdge(t.Context()) || m.Status().EdgeHold != nil {
				t.Fatal("failed entry blocks subsequent mode ticks")
			}
			if m.user.stopped != (outcome != "admission changed") || m.chat.keepalive != (outcome == "admission changed") {
				t.Fatal("failed entry restored the wrong playback ownership")
			}
		})
	}
}
