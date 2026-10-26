package modes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestPlanningFailureStatusTracksOutcomesWithoutCountingSpeechOrStaleWork(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	manager := newAutopilotManager(t, engine, clock, nil, nil)
	manager.mu.Lock()
	manager.loop.mode = ModeAutopilot
	manager.loop.generation = 7
	manager.mu.Unlock()
	manager.options.MotionGenerationMode = func() string { return config.LLMMotionModeLayered }
	fail := func(context.Context, DecisionInput) (Decision, error) {
		return Decision{}, errors.New("private provider failure")
	}
	for want := 1; want <= 2; want++ {
		choice := manager.runDecision(t.Context(), fail, true)
		if !choice.failed || choice.source != "hold" {
			t.Fatalf("failure = %+v", choice)
		}
		manager.recordPlanningOutcome(choice, 7)
		if manager.Status().PlanningFailures != want {
			t.Fatal("failure counter not published")
		}
	}
	manager.recordPlanningOutcome(segmentChoice{}, 6)
	if manager.Status().PlanningFailures != 2 {
		t.Fatal("stale outcome cleared failure")
	}
	// Check-ins run the same planner but do not record a motion outcome.
	source := SpeechSource{Provider: "chatgpt", Model: "gpt-6-sol"}
	speech := manager.runDecision(t.Context(), func(context.Context, DecisionInput) (Decision, error) {
		return Decision{Hold: true, Say: "A new line.", SpeechSource: source}, nil
	}, false)
	if speech.speechSource != source || manager.Status().PlanningFailures != 2 {
		t.Fatal("speech lost provenance or hid planning failure")
	}
	hold := manager.runDecision(t.Context(), func(context.Context, DecisionInput) (Decision, error) {
		return Decision{Hold: true, Abstain: true}, nil
	}, true)
	manager.recordPlanningOutcome(hold, 7)
	if manager.Status().PlanningFailures != 0 {
		t.Fatal("valid model hold did not clear failure")
	}
	manager.recordPlanningOutcome(segmentChoice{failed: true}, 7)
	manager.mu.Lock()
	manager.resetForModeStartLocked(ModeAutopilot)
	manager.mu.Unlock()
	if manager.Status().PlanningFailures != 0 {
		t.Fatal("new run kept old failures")
	}
}

func TestContinuousSchedulerPublishesFailuresAndClearsOnValidHold(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	decider := &fakeDecider{errs: []error{nil, errors.New("unavailable"), errors.New("unavailable")}, decisions: []Decision{
		{Segment: Segment{PatternID: motion.PatternStroke, SpeedPercent: 30, DurationMillis: 4000}}, {}, {}, {Hold: true, Abstain: true},
	}}
	manager := newAutopilotManager(t, engine, clock, decider, nil)
	manager.options.MotionGenerationMode = func() string { return config.LLMMotionModeLayered }
	if _, err := manager.Start(t.Context(), ModeAutopilot); err != nil {
		t.Fatal(err)
	}
	waitForAutonomousStart(t, manager, engine)
	for _, want := range []int{1, 2, 0} {
		clock.Advance(150 * time.Second)
		waitFor(t, time.Second, func() bool { return manager.Status().PlanningFailures == want })
	}
	_, retargets := engine.counts()
	if retargets != 0 {
		t.Fatalf("error/valid holds retargeted the engine %d times", retargets)
	}
}
