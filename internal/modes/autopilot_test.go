package modes

import (
	"context"
	"errors"
	"reflect"
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

func (a *announceLog) announce(_ context.Context, say string) Announcement {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.says = append(a.says, say)
	return Announcement{Published: true}
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

func TestAutopilotAppliesModelDecisionWithoutCoupledSpeech(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	announcer := &announceLog{}
	library := &motion.PatternDefinition{ID: "warmup-wave", Name: "Warmup wave"}
	decider := &fakeDecider{decisions: []Decision{{
		Segment: Segment{PatternID: "warmup-wave", SpeedPercent: 35},
		Pattern: library,
		Say:     "This motion-turn line must be ignored.",
	}}}
	manager := newAutopilotManager(t, engine, clock, decider, announcer)

	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForAutonomousStart(t, manager, engine)

	engine.mu.Lock()
	target := engine.starts[0]
	engine.mu.Unlock()
	if target.PatternID != "warmup-wave" || target.Pattern != library {
		t.Fatalf("model decision target = %+v, want curated library pattern attached", target)
	}
	if target.Source != ModeAutopilot || target.Label != "Autopilot" {
		t.Fatalf("target label/source = %q/%q, want Autopilot/autopilot", target.Label, target.Source)
	}
	if announced := announcer.all(); len(announced) != 0 {
		t.Fatalf("motion decision unexpectedly announced %v", announced)
	}

	status := manager.Status()
	if status.Mode != ModeAutopilot || status.DecisionSource != "model" || status.LastSay != "" {
		t.Fatalf("status = %+v, want motion-only model decision", status)
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
	waitForAutonomousStart(t, manager, engine)

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

func TestDynamicAutopilotHoldAtStartupNeverFallsBackToPattern(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	decider := &fakeDecider{decisions: []Decision{{
		Hold: true, Next: TimingNormal, Variability: VariabilityNormal,
	}}}
	manager := newAutopilotManager(t, engine, clock, decider, &announceLog{})
	manager.options.MotionGenerationMode = func() string { return config.LLMMotionModeDynamic }

	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, time.Second, func() bool { return decider.callCount() >= 1 })

	starts, retargets := engine.counts()
	if starts != 0 || retargets != 0 {
		t.Fatalf("Dynamic startup hold dispatched legacy motion: starts=%d retargets=%d", starts, retargets)
	}
	foundWaiting := false
	for _, row := range manager.options.Traces.Rows() {
		if row.Reason == "start_waiting_for_model" {
			foundWaiting = true
		}
		if row.Planner != nil && row.Planner.PatternIdentifier != "" &&
			row.Planner.PatternIdentifier != "dynamic" {
			t.Fatalf("Dynamic startup trace contains catalog pattern: %+v", row.Planner)
		}
	}
	if !foundWaiting {
		t.Fatal("Dynamic startup hold did not enter the model-waiting retry path")
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
	waitForAutonomousStart(t, manager, engine)

	// Cross the first segment boundary so the hold decision runs.
	clock.Advance(150 * time.Second)
	waitFor(t, time.Second, func() bool {
		return decider.callCount() >= 2 && manager.Status().DecisionSource == "hold"
	})
	_, retargets := engine.counts()
	if retargets != 0 {
		t.Fatalf("scheduler-only hold produced %d engine retargets", retargets)
	}
	if announced := announcer.all(); len(announced) != 0 {
		t.Fatalf("motion hold unexpectedly announced %v", announced)
	}
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
	waitForAutonomousStart(t, manager, engine)

	// Make the superseded segment's texture deterministically due during the
	// handoff. The new target must never inherit this speed waypoint.
	manager.mu.Lock()
	manager.swayPoints = []swayPoint{{
		generation:   manager.generation,
		at:           clock.Now().Add(time.Second),
		speedPercent: 36,
	}}
	manager.mu.Unlock()
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
	if dwell < 20*time.Second || dwell > 60*time.Second {
		t.Fatalf("interactive target dwell = %s, want bounded natural motion cadence", dwell)
	}

	clock.Advance(150 * time.Second)
	waitFor(t, time.Second, func() bool { return decider.callCount() >= 2 })
	engine.mu.Lock()
	held := engine.targets[len(engine.targets)-1]
	targetCount := len(engine.targets)
	engine.mu.Unlock()
	if held.PatternID != motion.PatternPulse || held.SpeedPercent != 28 || held.AreaFocus == nil || *held.AreaFocus != *focus {
		t.Fatalf("Autopilot Hold reverted the chat target: %+v", held)
	}
	if targetCount != 1 {
		t.Fatalf("Autopilot Hold produced %d post-start retargets, want only the interactive target", targetCount)
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
		Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 30},
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
		DecideSpeech: func(context.Context, DecisionInput) (Decision, error) {
			return Decision{Hold: true, Say: "This line is in flight.", Next: TimingNormal}, nil
		},
		AutopilotSettings: func() config.AutopilotSettings {
			settings := config.DefaultAutopilotSettings()
			settings.SpeechCadence = config.AutopilotSpeechCustom
			settings.SpeechMinSeconds = 8
			settings.SpeechMaxSeconds = 8
			return settings
		},
		Announce: func(ctx context.Context, _ string) Announcement {
			close(entered)
			<-ctx.Done()
			close(canceled)
			return Announcement{}
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
	waitForAutonomousStart(t, manager, engine)
	clock.Advance(9 * time.Second)

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

func TestAutopilotSpeechClockIsIndependentAndPlaybackAware(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	motionDecider := &fakeDecider{decisions: []Decision{{
		Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 30},
		Next:    TimingLater,
	}}}
	var speechMu sync.Mutex
	speechCalls := 0
	manager, err := NewManager(Options{
		Ensure:   func(context.Context) (Engine, error) { return engine, nil },
		Current:  func() Engine { return engine },
		Settings: func() config.MotionSettings { return config.DefaultSettings().Motion },
		AutopilotSettings: func() config.AutopilotSettings {
			settings := config.DefaultAutopilotSettings()
			settings.MotionCadence = config.AutopilotMotionCustom
			settings.MotionMinSeconds = 60
			settings.MotionMaxSeconds = 60
			settings.AdaptiveMotionTiming = false
			settings.SpeechCadence = config.AutopilotSpeechCustom
			settings.SpeechMinSeconds = 8
			settings.SpeechMaxSeconds = 8
			settings.AdaptiveSpeechTiming = false
			return settings
		},
		Now:    clock.Now,
		Tick:   2 * time.Millisecond,
		Seed:   42,
		Decide: motionDecider.decide,
		DecideSpeech: func(context.Context, DecisionInput) (Decision, error) {
			speechMu.Lock()
			speechCalls++
			speechMu.Unlock()
			return Decision{Hold: true, Say: "An independent line.", Next: TimingSoon}, nil
		},
		Announce: func(context.Context, string) Announcement {
			return Announcement{Published: true, RequestID: "speech-1", AwaitPlayback: true}
		},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Start(t.Context(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForAutonomousStart(t, manager, engine)

	clock.Advance(9 * time.Second)
	waitFor(t, time.Second, func() bool {
		speechMu.Lock()
		defer speechMu.Unlock()
		return speechCalls == 1 && manager.Status().SpeechWaitingPlayback
	})
	if calls := motionDecider.callCount(); calls != 1 {
		t.Fatalf("speech deadline triggered %d motion decisions, want first start only", calls)
	}
	if status := manager.Status(); !status.SpeechWaitingPlayback || status.SpeechMs != 0 {
		t.Fatalf("waiting status = %+v", status)
	}
	if !manager.NotifySpeechPlaybackComplete("speech-1") {
		t.Fatal("playback acknowledgement was not accepted")
	}
	status := manager.Status()
	if status.SpeechWaitingPlayback || status.SpeechMs < 7_000 || status.SpeechMs > 8_000 {
		t.Fatalf("post-playback speech clock = %+v", status)
	}
	if manager.NotifySpeechPlaybackComplete("speech-1") {
		t.Fatal("duplicate playback acknowledgement was accepted")
	}
}

func TestAutopilotSpeechMotionPreservesSpeechTimingChoice(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	motionDecider := &fakeDecider{decisions: []Decision{{
		Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 30},
		Next:    TimingLater,
	}}}
	manager, err := NewManager(Options{
		Ensure:   func(context.Context) (Engine, error) { return engine, nil },
		Current:  func() Engine { return engine },
		Settings: func() config.MotionSettings { return config.DefaultSettings().Motion },
		AutopilotSettings: func() config.AutopilotSettings {
			settings := config.DefaultAutopilotSettings()
			settings.MotionCadence = config.AutopilotMotionCustom
			settings.MotionMinSeconds = 60
			settings.MotionMaxSeconds = 60
			settings.AdaptiveMotionTiming = false
			settings.SpeechCadence = config.AutopilotSpeechCustom
			settings.SpeechMinSeconds = 8
			settings.SpeechMaxSeconds = 24
			return settings
		},
		Now:    clock.Now,
		Tick:   2 * time.Millisecond,
		Seed:   42,
		Decide: motionDecider.decide,
		DecideSpeech: func(context.Context, DecisionInput) (Decision, error) {
			return Decision{
				Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 34},
				Say:     "A timed motion line.",
				Next:    TimingLater,
			}, nil
		},
		Announce: func(context.Context, string) Announcement {
			return Announcement{Published: true, RequestID: "speech-motion-1", AwaitPlayback: true}
		},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Start(t.Context(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForAutonomousStart(t, manager, engine)

	clock.Advance(121 * time.Second)
	waitFor(t, time.Second, func() bool { return manager.Status().SpeechWaitingPlayback })
	manager.mu.Lock()
	timing := manager.speechNextTiming
	manager.mu.Unlock()
	if timing != TimingLater {
		t.Fatalf("speech timing = %q, want later after speech-owned motion", timing)
	}
	if !manager.NotifySpeechPlaybackComplete("speech-motion-1") {
		t.Fatal("playback acknowledgement was not accepted")
	}
	if remaining := manager.Status().SpeechMs; remaining < 15_000 || remaining > 24_000 {
		t.Fatalf("later speech delay = %d ms, want 15-24 seconds", remaining)
	}
}

func TestAutopilotSpeechPublicationFailureDoesNotApplyMotion(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	motionDecider := &fakeDecider{decisions: []Decision{{
		Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 30},
		Next:    TimingLater,
	}}}
	announcementAttempted := make(chan struct{})
	var announcementOnce sync.Once
	manager, err := NewManager(Options{
		Ensure:   func(context.Context) (Engine, error) { return engine, nil },
		Current:  func() Engine { return engine },
		Settings: func() config.MotionSettings { return config.DefaultSettings().Motion },
		AutopilotSettings: func() config.AutopilotSettings {
			settings := config.DefaultAutopilotSettings()
			settings.MotionCadence = config.AutopilotMotionCustom
			settings.MotionMinSeconds = 60
			settings.MotionMaxSeconds = 60
			settings.AdaptiveMotionTiming = false
			settings.SpeechCadence = config.AutopilotSpeechCustom
			settings.SpeechMinSeconds = 8
			settings.SpeechMaxSeconds = 8
			settings.AdaptiveSpeechTiming = false
			return settings
		},
		Now:    clock.Now,
		Tick:   2 * time.Millisecond,
		Seed:   42,
		Decide: motionDecider.decide,
		DecideSpeech: func(context.Context, DecisionInput) (Decision, error) {
			return Decision{
				Segment:     Segment{PatternID: motion.PatternStroke, SpeedPercent: 40},
				Say:         "A line that cannot be published.",
				Next:        TimingNormal,
				Variability: VariabilityRestless,
			}, nil
		},
		Announce: func(context.Context, string) Announcement {
			announcementOnce.Do(func() { close(announcementAttempted) })
			return Announcement{}
		},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Start(t.Context(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForAutonomousStart(t, manager, engine)

	clock.Advance(9 * time.Second)
	select {
	case <-announcementAttempted:
	case <-time.After(time.Second):
		t.Fatal("speech publication was not attempted")
	}
	engine.mu.Lock()
	reasons := append([]string(nil), engine.reasons...)
	engine.mu.Unlock()
	for _, reason := range reasons {
		if reason == "autopilot_speech" {
			t.Fatalf("failed speech publication applied coupled motion: reasons = %v", reasons)
		}
	}
}

func TestAutopilotChatActivityBlocksAutonomousWorkUntilComplete(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	decider := &fakeDecider{decisions: []Decision{
		{Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 30}},
		{Hold: true, Next: TimingLater},
	}}
	manager := newAutopilotManager(t, engine, clock, decider, nil)
	if _, err := manager.Start(t.Context(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForAutonomousStart(t, manager, engine)

	manager.NotifyChatActivity()
	clock.Advance(150 * time.Second)
	time.Sleep(20 * time.Millisecond)
	if calls := decider.callCount(); calls != 1 {
		t.Fatalf("Autopilot made %d decisions during interactive chat, want first start only", calls)
	}
	manager.NotifyChatActivityComplete()
	waitFor(t, time.Second, func() bool { return decider.callCount() >= 2 })
}

func TestAutopilotCadenceRandomnessDoesNotPerturbPatternPlanner(t *testing.T) {
	newManager := func() *Manager {
		engine := &fakeEngine{}
		manager, err := NewManager(Options{
			Ensure:   func(context.Context) (Engine, error) { return engine, nil },
			Current:  func() Engine { return engine },
			Settings: func() config.MotionSettings { return config.DefaultSettings().Motion },
			Seed:     42,
		})
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		return manager
	}
	withCadence := newManager()
	withoutCadence := newManager()

	firstWith, _ := withCadence.nextPlannedSegment()
	firstWithout, _ := withoutCadence.nextPlannedSegment()
	withCadence.mu.Lock()
	for range 5 {
		withCadence.sampleMotionDelayLocked(TimingNormal)
		withCadence.sampleSpeechDelayLocked(TimingNormal)
	}
	withCadence.mu.Unlock()
	secondWith, _ := withCadence.nextPlannedSegment()
	secondWithout, _ := withoutCadence.nextPlannedSegment()

	if !reflect.DeepEqual(firstWith, firstWithout) || !reflect.DeepEqual(secondWith, secondWithout) {
		t.Fatalf(
			"cadence sampling changed pattern sequence: with=%+v/%+v without=%+v/%+v",
			firstWith,
			secondWith,
			firstWithout,
			secondWithout,
		)
	}
}

func TestAutopilotUserStopEndsModeWithoutRestart(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	decider := &fakeDecider{decisions: []Decision{{Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 30, DurationMillis: 8000}}}}
	manager := newAutopilotManager(t, engine, clock, decider, &announceLog{})

	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForAutonomousStart(t, manager, engine)

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
