package modes

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// slowDecider answers after a simulated model latency on the fake clock and
// records what each decision was shown and when it was asked.
type slowDecider struct {
	mu sync.Mutex
	// latencies are consumed one per call; the last repeats.
	latencies []time.Duration
	clock     *fakeClock
	inputs    []DecisionInput
	askedAt   []time.Time
	library   *motion.PatternDefinition
}

func (d *slowDecider) decide(_ context.Context, input DecisionInput) (Decision, error) {
	d.mu.Lock()
	d.inputs = append(d.inputs, input)
	d.askedAt = append(d.askedAt, d.clock.Now())
	speed := 30 + 5*len(d.inputs)
	latency := d.latencies[min(len(d.inputs), len(d.latencies))-1]
	d.mu.Unlock()
	d.clock.Advance(latency)
	// Settled keeps intra-stretch sway out of the engine's retarget count.
	return Decision{Segment: Segment{PatternID: motion.PatternID(d.library.ID), SpeedPercent: speed}, Pattern: d.library,
		NextCheck: 40 * time.Second, Variability: VariabilitySettled}, nil
}

func (d *slowDecider) snapshot() ([]DecisionInput, []time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]DecisionInput(nil), d.inputs...), append([]time.Time(nil), d.askedAt...)
}

func newPreparingManager(t *testing.T, prepare bool, latencies ...time.Duration) (*Manager, *fakeEngine, *fakeClock, *slowDecider) {
	t.Helper()
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	decider := &slowDecider{clock: clock, latencies: latencies, library: &motion.PatternDefinition{ID: "custom-wave", Name: "Custom wave"}}
	manager := newAutopilotManager(t, engine, clock, nil, &announceLog{})
	manager.options.Decide = decider.decide
	manager.options.PrepareNextDecision = func() bool { return prepare }
	return manager, engine, clock, decider
}

func motionDeadline(manager *Manager) time.Time {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.motion.deadline
}

func stretchStarts(manager *Manager) []time.Time {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	starts := make([]time.Time, len(manager.history.recentSpeeds))
	for i, mark := range manager.history.recentSpeeds {
		starts[i] = mark.at
	}
	return starts
}

// A prepared decision is asked as soon as its stretch begins, sees the facts
// of the stretch's end and lands exactly on the boundary, even when the
// external model answers far more slowly than last time.
func TestPreparedDecisionLandsOnTheBoundary(t *testing.T) {
	manager, engine, clock, decider := newPreparingManager(t, true, time.Second, 6*time.Second)
	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForAutonomousStart(t, manager, engine)
	deadline := motionDeadline(manager)
	waitFor(t, time.Second, func() bool { return manager.Status().MotionPlanned })

	inputs, asked := decider.snapshot()
	if len(inputs) != 2 || inputs[0].Lookahead != 0 {
		t.Fatalf("decisions before the boundary = %d, first lookahead %s", len(inputs), inputs[0].Lookahead)
	}
	if lookahead := deadline.Sub(asked[1]); inputs[1].Lookahead != lookahead || lookahead < 30*time.Second {
		t.Fatalf("prepared lookahead = %s, want the %s left in the stretch", inputs[1].Lookahead, lookahead)
	}
	if got, want := inputs[1].SecondsAtCurrentSpeed, int(deadline.Sub(stretchStarts(manager)[0])/time.Second); got != want {
		t.Fatalf("prepared speed age = %ds, want %ds at the boundary", got, want)
	}
	if _, retargets := engine.counts(); retargets != 0 {
		t.Fatal("a prepared decision changed motion before its boundary")
	}

	clock.Advance(deadline.Sub(clock.Now()))
	waitFor(t, time.Second, func() bool { return len(stretchStarts(manager)) == 2 })
	if starts := stretchStarts(manager); !starts[1].Equal(deadline) {
		t.Fatalf("second stretch began at %v, want the boundary %v", starts, deadline)
	}
	// The next decision is prepared at once for the new stretch.
	waitFor(t, time.Second, func() bool { inputs, _ := decider.snapshot(); return len(inputs) == 3 })
}

// Without preparation a slower-than-last answer pushes the boundary late:
// planning starts one measured latency before it, and this answer took longer.
func TestJustInTimeDecisionSlipsWhenTheModelIsSlow(t *testing.T) {
	manager, engine, clock, decider := newPreparingManager(t, false, time.Second, 6*time.Second)
	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForAutonomousStart(t, manager, engine)
	deadline := motionDeadline(manager)
	manager.mu.Lock()
	planAt := manager.motion.planAt
	manager.mu.Unlock()
	if lead := deadline.Sub(planAt); lead != 1500*time.Millisecond {
		t.Fatalf("just-in-time planning %s before the boundary, want the last latency plus 0.5s", lead)
	}
	clock.Advance(planAt.Sub(clock.Now()))
	waitFor(t, time.Second, func() bool { return len(stretchStarts(manager)) == 2 })
	inputs, _ := decider.snapshot()
	if inputs[1].Lookahead != 0 {
		t.Fatalf("just-in-time lookahead = %s", inputs[1].Lookahead)
	}
	if starts := stretchStarts(manager); starts[1].Sub(deadline) != 4500*time.Millisecond {
		t.Fatalf("second stretch began %s after the boundary, want 4.5s late", starts[1].Sub(deadline))
	}
}

// Anything the person says discards a prepared decision; Autopilot prepares
// again with their words once chat has finished.
func TestChatDiscardsAPreparedDecision(t *testing.T) {
	manager, engine, clock, decider := newPreparingManager(t, true, time.Second)
	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForAutonomousStart(t, manager, engine)
	waitFor(t, time.Second, func() bool { return manager.Status().MotionPlanned })

	activity := manager.NotifyChatActivity()
	if manager.Status().MotionPlanned {
		t.Fatal("chat activity kept the prepared decision")
	}
	time.Sleep(20 * time.Millisecond)
	if inputs, _ := decider.snapshot(); len(inputs) != 2 {
		t.Fatalf("Autopilot planned during chat: %d decisions", len(inputs))
	}
	clock.Advance(2 * time.Second)
	manager.NotifyChatActivityComplete(activity)
	waitFor(t, time.Second, func() bool {
		inputs, _ := decider.snapshot()
		return len(inputs) == 3 && manager.Status().MotionPlanned
	})
	if _, retargets := engine.counts(); retargets != 0 {
		t.Fatal("the discarded or replacement decision changed motion before the boundary")
	}
	found := false
	for _, row := range manager.options.Traces.Rows() {
		if row.Planner != nil && strings.Contains(row.Planner.Note, "prepared=") {
			found = true
		}
	}
	if !found {
		t.Fatal("no trace records a prepared decision")
	}
}
