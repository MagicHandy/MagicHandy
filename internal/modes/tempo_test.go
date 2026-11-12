package modes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestTempoAutopilotStartsWithoutAPositionSegment(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	spec := motion.TempoSpec{Mode: motion.TempoModeRoutine, Routine: "swells", Seed: 42}
	decider := &fakeDecider{decisions: []Decision{{Hold: true, Tempo: &spec, Next: TimingLater}}}
	manager := newAutopilotManager(t, engine, clock, decider, &announceLog{})
	manager.options.MotionGenerationMode = func() string { return config.LLMMotionModeLayered }
	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, func() bool {
		starts, _ := engine.counts()
		return starts > 0 || manager.Status().LastEvent == "start_waiting_for_model"
	})
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if len(engine.starts) != 1 {
		t.Fatalf("tempo decision did not start: starts=%d", len(engine.starts))
	}
	target := engine.starts[0]
	if !target.TempoOnly || target.Tempo == nil || !target.Tempo.Equal(spec) {
		t.Fatalf("tempo decision lost its native target: %+v", target)
	}
	if target.PatternID != "" || target.Dynamic != nil || target.Flow != nil {
		t.Fatalf("tempo startup invented position content: %+v", target)
	}
}

func TestTempoPlannerFailureNeverStartsALibraryFallback(t *testing.T) {
	engine := &fakeEngine{}
	manager := newAutopilotManager(t, engine, &fakeClock{now: time.Unix(0, 0)}, &fakeDecider{}, &announceLog{})
	manager.options.MotionGenerationMode = func() string { return config.LLMMotionModePattern }
	manager.options.TempoLeads = func() bool { return true }
	choice := manager.runDecision(t.Context(), func(context.Context, DecisionInput) (Decision, error) {
		return Decision{}, errors.New("invalid native response")
	}, true)
	if choice.source != "hold" || choice.segment.hasContent() {
		t.Fatalf("native error invented fallback: %+v", choice)
	}
	starts, _ := engine.counts()
	if starts != 0 {
		t.Fatal("native error started playback")
	}
}
