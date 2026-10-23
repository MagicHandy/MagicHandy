package modes

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type edgeTestEngine struct{ *fakeEngine }

func (e edgeTestEngine) snapshotLocked() motion.ActiveMotionState {
	return motion.ActiveMotionState{Running: e.running, Paused: e.paused, Target: e.target, PlanID: fmt.Sprint(len(e.targets)), CurrentSample: &motion.MotionSample{PositionPercent: 50}}
}
func (e edgeTestEngine) Snapshot() motion.ActiveMotionState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshotLocked()
}
func (e edgeTestEngine) ApplyTargetIfCurrent(ctx context.Context, target motion.MotionTarget, reason, planID string) (motion.ActiveMotionState, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ctx.Err() != nil {
		return e.snapshotLocked(), ctx.Err()
	}
	if !e.running || e.paused || planID != fmt.Sprint(len(e.targets)) {
		return e.snapshotLocked(), errors.New("stale or stopped motion")
	}
	if e.targetErr != nil {
		return e.snapshotLocked(), e.targetErr
	}
	e.target = target
	e.targets = append(e.targets, target)
	e.reasons = append(e.reasons, reason)
	return e.snapshotLocked(), nil
}

func edgeManager(t *testing.T) (*Manager, edgeTestEngine, *fakeClock) {
	t.Helper()
	clock := &fakeClock{now: time.Now()}
	engine := edgeTestEngine{&fakeEngine{running: true, target: motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 50}}}
	m, err := NewManager(Options{Ensure: func(context.Context) (Engine, error) { return engine, nil }, Current: func() Engine { return engine }, Settings: func() config.MotionSettings {
		s := config.DefaultSettings().Motion
		s.SpeedMinPercent, s.SpeedMaxPercent = 5, 95
		return s
	}, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Shutdown)
	return m, engine, clock
}

func TestEdgeImmediateHoldAndManualReturn(t *testing.T) {
	m, e, _ := edgeManager(t)
	m.options.DecideEdge = func(context.Context, EdgeInput) (EdgeDecision, error) {
		t.Fatal("manual hold called a model")
		return EdgeDecision{}, nil
	}
	status, err := m.ActivateEdge(t.Context())
	if err != nil || status.EdgeHold == nil || status.EdgeHold.Automatic {
		t.Fatalf("%+v %v", status, err)
	}
	if got := e.Snapshot(); got.Target.SpeedPercent != 20 || got.Target.Flow == nil || got.Target.Flow.PaceVariationPercent != 0 {
		t.Fatalf("not a gentle semantic hold: %+v", got.Target)
	}
	if !m.tickEdge(t.Context()) {
		t.Fatal("hold allowed the normal planner")
	}
	if _, err = m.ActivateEdge(t.Context()); err != nil {
		t.Fatal("duplicate activation must keep holding")
	}
	status, err = m.ResumeEdge(t.Context())
	if err != nil || status.EdgeHold != nil || e.Snapshot().Target.PatternID != motion.PatternStroke || e.Snapshot().Target.SpeedPercent != 50 {
		t.Fatalf("return: %+v %v", status, err)
	}
	starts, targets := e.counts()
	if starts != 0 || targets != 2 {
		t.Fatalf("unexpected starts=%d targets=%d", starts, targets)
	}
}

func TestEdgeAutopilotChoosesDurationAndReturn(t *testing.T) {
	for _, style := range []string{"previous", "gentle"} {
		t.Run(style, func(t *testing.T) {
			m, e, clock := edgeManager(t)
			m.loop.mode = ModeAutopilot
			calls := 0
			m.options.DecideEdge = func(context.Context, EdgeInput) (EdgeDecision, error) {
				calls++
				return EdgeDecision{HoldSeconds: 12, ResumeSpeed: 25, ResumeStyle: style}, nil
			}
			if _, err := m.ActivateEdge(t.Context()); err != nil {
				t.Fatal(err)
			}
			if calls != 0 {
				t.Fatal("activation waited for inference")
			}
			m.tickEdge(t.Context())
			clock.Advance(11 * time.Second)
			m.tickEdge(t.Context())
			if len(e.targets) != 1 || calls != 1 || m.Status().EdgeHold.ResumeAt == "" {
				t.Fatal("incorrect planned deadline")
			}
			clock.Advance(time.Second)
			m.tickEdge(t.Context())
			if m.Status().EdgeHold != nil || len(e.targets) != 2 || e.Snapshot().Target.SpeedPercent != 25 {
				t.Fatal("planned return not applied")
			}
			if (e.Snapshot().Target.Flow != nil) != (style == "gentle") {
				t.Fatal("return style ignored")
			}
			if e.Snapshot().Target.Source != ModeAutopilot {
				t.Fatal("return lost Autopilot ownership")
			}
		})
	}
}

func TestEdgeLatePlanCannotBeatNewerIntent(t *testing.T) {
	for _, action := range []string{"stop", "pause", "chat", "settings"} {
		t.Run(action, func(t *testing.T) {
			m, e, clock := edgeManager(t)
			m.loop.mode = ModeAutopilot
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			m.options.DecideEdge = func(context.Context, EdgeInput) (EdgeDecision, error) {
				close(entered)
				<-release
				return EdgeDecision{HoldSeconds: 8, ResumeSpeed: 50, ResumeStyle: "previous"}, nil
			}
			if _, err := m.ActivateEdge(t.Context()); err != nil {
				t.Fatal(err)
			}
			go func() { defer close(done); m.tickEdge(t.Context()) }()
			<-entered
			switch action {
			case "stop":
				m.NotifyUserStop()
				e.setState(false, false)
			case "pause":
				finish, _ := m.BeginUserPause()
				e.setState(true, true)
				finish(true)
			case "chat":
				id := m.NotifyChatActivity()
				m.NotifyChatActivityComplete(id)
			case "settings":
				m.NotifyAutopilotSettingsChanged()
			}
			close(release)
			<-done
			clock.Advance(time.Minute)
			m.tickEdge(t.Context())
			if _, targets := e.counts(); targets != 1 {
				t.Fatal("stale model result changed motion")
			}
			if status := m.Status().EdgeHold; status != nil && status.Automatic {
				t.Fatal("stale automatic return still armed")
			}
		})
	}
}

func TestEdgeFailureNeverRestartsOrEscalates(t *testing.T) {
	for _, failure := range []string{"model", "invalid", "disconnected", "new_plan"} {
		t.Run(failure, func(t *testing.T) {
			m, e, clock := edgeManager(t)
			m.loop.mode = ModeAutopilot
			m.options.DecideEdge = func(context.Context, EdgeInput) (EdgeDecision, error) {
				if failure == "model" {
					return EdgeDecision{}, errors.New("refused")
				}
				if failure == "invalid" {
					return EdgeDecision{HoldSeconds: 8, ResumeSpeed: 100, ResumeStyle: "previous"}, nil
				}
				return EdgeDecision{HoldSeconds: 8, ResumeSpeed: 25, ResumeStyle: "previous"}, nil
			}
			if _, err := m.ActivateEdge(t.Context()); err != nil {
				t.Fatal(err)
			}
			m.tickEdge(t.Context())
			if failure == "disconnected" {
				e.setState(false, false)
			}
			if failure == "new_plan" {
				e.mu.Lock()
				e.targets = append(e.targets, e.target)
				e.mu.Unlock()
			}
			clock.Advance(time.Minute)
			m.tickEdge(t.Context())
			if status := m.Status().EdgeHold; (status == nil && failure != "disconnected") || (status != nil && status.Automatic) {
				t.Fatalf("unsafe state %+v", status)
			}
			if starts, _ := e.counts(); starts != 0 {
				t.Fatal("hold recovered a stopped device")
			}
		})
	}
}

func TestEdgeUnavailableForStoppedPausedAndFiniteMotion(t *testing.T) {
	for _, kind := range []string{"stopped", "paused", "media", "program"} {
		t.Run(kind, func(t *testing.T) {
			m, e, _ := edgeManager(t)
			switch kind {
			case "stopped":
				e.setState(false, false)
			case "paused":
				e.setState(true, true)
			case "media":
				e.target.MediaID = "video"
			case "program":
				e.target.ProgramID = "finite"
			}
			if _, err := m.ActivateEdge(t.Context()); err == nil {
				t.Fatal("unsupported activation admitted")
			}
			if starts, targets := e.counts(); starts+targets != 0 {
				t.Fatal("unsupported activation touched motion")
			}
		})
	}
}
