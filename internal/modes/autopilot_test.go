package modes

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type fakeDecider struct {
	mu        sync.Mutex
	decisions []Decision
	errs      []error
	inputs    []DecisionInput
	calls     int
}

func (d *fakeDecider) decide(_ context.Context, input DecisionInput) (Decision, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.inputs = append(d.inputs, input)
	index := d.calls
	d.calls++
	if index < len(d.errs) && d.errs[index] != nil {
		return Decision{}, d.errs[index]
	}
	if index < len(d.decisions) {
		return d.decisions[index], nil
	}
	if len(d.decisions) > 0 {
		return d.decisions[len(d.decisions)-1], nil
	}
	return Decision{}, errors.New("no scripted decision")
}

func (d *fakeDecider) callCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

type announceLog struct {
	mu   sync.Mutex
	says []string
}

func (a *announceLog) announce(_ context.Context, say string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.says = append(a.says, say)
}

func (a *announceLog) all() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.says...)
}

func newAutopilotManager(t *testing.T, engine *fakeEngine, clock *fakeClock, decider *fakeDecider, announcer *announceLog) *Manager {
	t.Helper()
	options := Options{
		Ensure:   func(context.Context) (Engine, error) { return engine, nil },
		Current:  func() Engine { return engine },
		Settings: func() config.MotionSettings { return config.DefaultSettings().Motion },
		Traces:   diagnostics.NewTraceRing(256),
		Now:      clock.Now,
		Tick:     2 * time.Millisecond,
		Seed:     42,
	}
	if decider != nil {
		options.Decide = decider.decide
	}
	if announcer != nil {
		options.Announce = announcer.announce
	}
	manager, err := NewManager(options)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Shutdown)
	return manager
}

func TestAutopilotRequiresDecisionStep(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	manager := newAutopilotManager(t, engine, clock, nil, nil)

	if _, err := manager.Start(context.Background(), ModeAutopilot); err == nil {
		t.Fatal("expected autopilot start to fail without a decision step")
	}
}

func TestAutopilotAppliesModelDecisionAndAnnouncesAfterArming(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	announcer := &announceLog{}
	library := &motion.PatternDefinition{ID: "warmup-wave", Name: "Warmup wave"}
	decider := &fakeDecider{decisions: []Decision{{
		Segment: Segment{PatternID: "warmup-wave", SpeedPercent: 35},
		Pattern: library,
		Say:     "Settling into a slow wave.",
	}}}
	manager := newAutopilotManager(t, engine, clock, decider, announcer)

	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, time.Second, func() bool { starts, _ := engine.counts(); return starts >= 1 })

	engine.mu.Lock()
	target := engine.starts[0]
	engine.mu.Unlock()
	if target.PatternID != "warmup-wave" || target.Pattern != library {
		t.Fatalf("model decision target = %+v, want curated library pattern attached", target)
	}
	if target.Source != ModeAutopilot || target.Label != "Autopilot" {
		t.Fatalf("target label/source = %q/%q, want Autopilot/autopilot", target.Label, target.Source)
	}
	waitFor(t, time.Second, func() bool { return len(announcer.all()) == 1 })
	if announcer.all()[0] != "Settling into a slow wave." {
		t.Fatalf("announced %q", announcer.all()[0])
	}

	status := manager.Status()
	if status.Mode != ModeAutopilot || status.DecisionSource != "model" || status.LastSay == "" {
		t.Fatalf("status = %+v, want autopilot model decision with say", status)
	}
	if status.SegmentEndsMs <= 0 {
		t.Fatal("model decision without duration must receive a deterministic bounded duration")
	}
}

func TestAutopilotFallsBackToPlannerOnDecisionFailure(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	decider := &fakeDecider{errs: []error{errors.New("model unavailable")}}
	manager := newAutopilotManager(t, engine, clock, decider, &announceLog{})

	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, time.Second, func() bool { starts, _ := engine.counts(); return starts >= 1 })

	engine.mu.Lock()
	target := engine.starts[0]
	engine.mu.Unlock()
	builtin := map[motion.PatternID]bool{motion.PatternStroke: true, motion.PatternPulse: true, motion.PatternTease: true}
	if !builtin[target.PatternID] {
		t.Fatalf("fallback target pattern = %q, want a deterministic builtin", target.PatternID)
	}
	if status := manager.Status(); status.DecisionSource != "fallback" {
		t.Fatalf("decision source = %q, want fallback", status.DecisionSource)
	}
}

func TestAutopilotHoldKeepsCurrentSegmentWithoutDrift(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	library := &motion.PatternDefinition{ID: "custom-wave", Name: "Custom wave"}
	decider := &fakeDecider{decisions: []Decision{
		{Segment: Segment{PatternID: library.ID, SpeedPercent: 40, DurationMillis: 4000}, Pattern: library},
		{Hold: true, Say: "Staying right here."},
	}}
	announcer := &announceLog{}
	manager := newAutopilotManager(t, engine, clock, decider, announcer)

	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, time.Second, func() bool { starts, _ := engine.counts(); return starts >= 1 })

	// Cross the first segment boundary so the hold decision runs.
	clock.Advance(6 * time.Second)
	waitFor(t, time.Second, func() bool { _, targets := engine.counts(); return targets >= 1 })

	engine.mu.Lock()
	held := engine.targets[0]
	engine.mu.Unlock()
	if held.PatternID != library.ID || held.Pattern == nil || held.Pattern.ID != library.ID || held.SpeedPercent != 40 {
		t.Fatalf("hold target = %+v, want same pattern and speed", held)
	}
	if status := manager.Status(); status.DecisionSource != "hold" {
		t.Fatalf("decision source = %q, want hold", status.DecisionSource)
	}
	waitFor(t, time.Second, func() bool { return len(announcer.all()) >= 1 })
	decider.mu.Lock()
	if len(decider.inputs) < 2 {
		decider.mu.Unlock()
		t.Fatal("hold decision did not receive a second input")
	}
	input := decider.inputs[1]
	decider.mu.Unlock()
	if input.CurrentPatternID != library.ID || input.CurrentSpeed != 40 {
		t.Fatalf("hold input current motion = %+v, want the live custom wave at 40%%", input)
	}
}

func TestInteractiveChatTargetSuspendsAndReplacesAutopilotState(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	decider := &fakeDecider{decisions: []Decision{
		{Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 30, DurationMillis: 4000}},
		{Hold: true},
	}}
	manager := newAutopilotManager(t, engine, clock, decider, nil)
	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, time.Second, func() bool { starts, _ := engine.counts(); return starts == 1 })

	handoffGeneration := manager.PrepareChatTarget()
	clock.Advance(150 * time.Second)
	time.Sleep(20 * time.Millisecond)
	if calls := decider.callCount(); calls != 1 {
		t.Fatalf("Autopilot decided while interactive target was pending: %d calls", calls)
	}

	definition, _ := motion.BuiltinPatternDefinition(motion.PatternPulse)
	focus := &motion.AreaFocus{MinPercent: 0, MaxPercent: 34}
	interactive := motion.MotionTarget{
		Source: "chat", PatternID: motion.PatternPulse, Pattern: &definition,
		SpeedPercent: 28, AreaFocus: focus,
	}
	if _, err := engine.ApplyTarget(t.Context(), interactive, "chat_target"); err != nil {
		t.Fatalf("interactive ApplyTarget: %v", err)
	}
	if !manager.NotifyChatTarget(handoffGeneration, interactive) {
		t.Fatal("interactive target handoff was unexpectedly stale")
	}
	if status := manager.Status(); status.DecisionSource != "interactive" {
		t.Fatalf("status after chat handoff = %+v, want interactive source", status)
	}
	manager.mu.Lock()
	dwell := manager.deadline.Sub(clock.Now())
	manager.mu.Unlock()
	if want := time.Duration(styleProfiles[config.MotionStyleBalanced].minDurationMillis) * time.Millisecond; dwell != want {
		t.Fatalf("interactive target dwell = %s, want shortest balanced dwell %s", dwell, want)
	}

	clock.Advance(150 * time.Second)
	waitFor(t, time.Second, func() bool { _, targets := engine.counts(); return targets >= 2 })
	engine.mu.Lock()
	held := engine.targets[len(engine.targets)-1]
	engine.mu.Unlock()
	if held.PatternID != motion.PatternPulse || held.SpeedPercent != 28 || held.AreaFocus == nil || *held.AreaFocus != *focus {
		t.Fatalf("Autopilot Hold reverted the chat target: %+v", held)
	}

	decider.mu.Lock()
	lastInput := decider.inputs[len(decider.inputs)-1]
	decider.mu.Unlock()
	if lastInput.CurrentPatternID != motion.PatternPulse || lastInput.CurrentSpeed != 28 ||
		lastInput.CurrentAreaFocus == nil || *lastInput.CurrentAreaFocus != *focus {
		t.Fatalf("post-chat decision input = %+v, want the interactive target", lastInput)
	}
}

func TestAutopilotAnnouncementContextCancelsWithStop(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	decider := &fakeDecider{decisions: []Decision{{
		Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 30, DurationMillis: 8000},
		Say:     "This line is in flight.",
	}}}
	entered := make(chan struct{})
	canceled := make(chan struct{})
	options := Options{
		Ensure:   func(context.Context) (Engine, error) { return engine, nil },
		Current:  func() Engine { return engine },
		Settings: func() config.MotionSettings { return config.DefaultSettings().Motion },
		Now:      clock.Now,
		Tick:     2 * time.Millisecond,
		Decide:   decider.decide,
		Announce: func(ctx context.Context, _ string) {
			close(entered)
			<-ctx.Done()
			close(canceled)
		},
	}
	manager, err := NewManager(options)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("announcement did not start")
	}
	finishStop := manager.BeginUserStop()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		finishStop()
		t.Fatal("announcement context was not canceled by Stop")
	}
	finishStop()
}

func TestAutopilotUserStopEndsModeWithoutRestart(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	decider := &fakeDecider{decisions: []Decision{{Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 30, DurationMillis: 8000}}}}
	manager := newAutopilotManager(t, engine, clock, decider, &announceLog{})

	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, time.Second, func() bool { starts, _ := engine.counts(); return starts >= 1 })

	manager.NotifyUserStop()
	engine.setState(false, false)
	callsAfterStop := decider.callCount()
	clock.Advance(30 * time.Second)
	time.Sleep(20 * time.Millisecond)

	if status := manager.Status(); status.Active {
		t.Fatalf("autopilot still active after user stop: %+v", status)
	}
	starts, _ := engine.counts()
	if starts != 1 {
		t.Fatalf("engine restarted after user stop: %d starts", starts)
	}
	if decider.callCount() != callsAfterStop {
		t.Fatal("decision step ran again after user stop")
	}
}

func TestAutopilotTraceRecordsDecisionSource(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	traces := diagnostics.NewTraceRing(64)
	decider := &fakeDecider{decisions: []Decision{{
		Segment: Segment{PatternID: motion.PatternTease, SpeedPercent: 25, DurationMillis: 5000},
		Say:     "hello",
	}}}
	options := Options{
		Ensure:   func(context.Context) (Engine, error) { return engine, nil },
		Current:  func() Engine { return engine },
		Settings: func() config.MotionSettings { return config.DefaultSettings().Motion },
		Traces:   traces,
		Now:      clock.Now,
		Tick:     2 * time.Millisecond,
		Seed:     42,
		Decide:   decider.decide,
	}
	manager, err := NewManager(options)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Shutdown)

	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, time.Second, func() bool {
		for _, row := range traces.Rows() {
			if row.Planner != nil && row.Planner.Event == "autopilot_start" &&
				row.Planner.SegmentIndex == 1 && strings.Contains(row.Planner.Note, "model") && strings.Contains(row.Planner.Note, "say") {
				return true
			}
		}
		return false
	})
}
