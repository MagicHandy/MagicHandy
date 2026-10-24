package modes

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func remainingSeconds(manager *Manager) float64 {
	return float64(manager.Status().SegmentEndsMs) / 1000
}

// Under auto timing a hosted planner's own horizon sets the boundary, varied
// slightly, and a hold keeps the horizon it asked for. The planner sees its
// recent choices, and a decision without one falls back to the sampled window.
func TestAutopilotModelHorizonSetsTheBoundary(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	library := &motion.PatternDefinition{ID: "custom-wave", Name: "Custom wave"}
	decider := &fakeDecider{decisions: []Decision{
		{Segment: Segment{PatternID: library.ID, SpeedPercent: 40}, Pattern: library, NextCheck: 150 * time.Second},
		{Hold: true, NextCheck: 20 * time.Second},
		{Hold: true},
	}}
	manager := newAutopilotManager(t, engine, clock, decider, &announceLog{})
	secondDecided, releaseSecond := blockSecondHorizonDecision(manager, decider)
	defer close(releaseSecond)
	if _, err := manager.Start(context.Background(), ModeAutopilot); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForAutonomousStart(t, manager, engine)
	if remaining := remainingSeconds(manager); remaining < 132 || remaining > 168 {
		t.Fatalf("first boundary in %.1fs, want 150s ±12%%", remaining)
	}

	clock.Advance(170 * time.Second)
	select {
	case <-secondDecided:
	case <-time.After(time.Second):
		t.Fatal("second decision did not begin")
	}
	// A decider call is not an applied segment. While its result is pending,
	// the old deadline has expired and legitimately reports zero remaining.
	if status := manager.Status(); status.SegmentIndex != 1 || status.SegmentEndsMs != 0 {
		t.Fatalf("pending decision changed the applied segment: %+v", status)
	}
	releaseSecond <- struct{}{}
	waitFor(t, time.Second, func() bool { return manager.Status().SegmentIndex >= 2 })
	if remaining := remainingSeconds(manager); remaining < 17 || remaining > 23 {
		t.Fatalf("held boundary in %.1fs, want 20s ±12%%", remaining)
	}
	if _, retargets := engine.counts(); retargets != 0 {
		t.Fatalf("a timed hold retargeted the engine %d times", retargets)
	}

	clock.Advance(25 * time.Second)
	waitFor(t, time.Second, func() bool { return manager.Status().SegmentIndex >= 3 })
	// Level 4 with adaptive timing samples the middle of 20-60 seconds.
	waitFor(t, time.Second, func() bool { remaining := remainingSeconds(manager); return remaining >= 28 && remaining <= 52 })

	decider.mu.Lock()
	inputs := append([]DecisionInput(nil), decider.inputs...)
	decider.mu.Unlock()
	if len(inputs[0].RecentCheckSeconds) != 0 || !slices.Equal(inputs[1].RecentCheckSeconds, []int{150}) ||
		!slices.Equal(inputs[2].RecentCheckSeconds, []int{150, 20}) {
		t.Fatalf("recent checks = %v, %v, %v", inputs[0].RecentCheckSeconds, inputs[1].RecentCheckSeconds, inputs[2].RecentCheckSeconds)
	}
	found := false
	for _, row := range manager.options.Traces.Rows() {
		if row.Planner != nil && row.Planner.Event == "autopilot_start" && strings.Contains(row.Planner.Note, "check=150s") {
			found = true
		}
	}
	if !found {
		t.Fatal("the start trace does not record the model horizon")
	}
}

func blockSecondHorizonDecision(manager *Manager, decider *fakeDecider) (<-chan struct{}, chan struct{}) {
	decided, release := make(chan struct{}), make(chan struct{}, 1)
	manager.options.Decide = func(ctx context.Context, input DecisionInput) (Decision, error) {
		decision, err := decider.decide(ctx, input)
		if input.SegmentIndex == 1 {
			close(decided)
			select {
			case <-release:
			case <-ctx.Done():
				return Decision{}, ctx.Err()
			}
		}
		return decision, err
	}
	return decided, release
}

// The model chooses the length, but the bounds always hold, the variation is
// real, and the recent list stays short.
func TestModelHorizonIsBoundedVariedAndRemembered(t *testing.T) {
	manager := newAutopilotManager(t, &fakeEngine{}, &fakeClock{now: time.Unix(0, 0)}, &fakeDecider{}, nil)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if got := manager.modelHorizonLocked(2 * time.Second); got != 8*time.Second {
		t.Fatalf("short horizon = %s, want the 8s floor", got)
	}
	if got := manager.modelHorizonLocked(20 * time.Minute); got != 300*time.Second {
		t.Fatalf("long horizon = %s, want the 300s ceiling", got)
	}
	seen := map[time.Duration]bool{}
	for range 200 {
		got := manager.modelHorizonLocked(60 * time.Second)
		if got < 53*time.Second || got > 67*time.Second {
			t.Fatalf("60s horizon became %s", got)
		}
		seen[got] = true
	}
	if len(seen) < 8 {
		t.Fatalf("60s horizon took only %d values", len(seen))
	}
	for _, requested := range []time.Duration{2 * time.Second, 30 * time.Second, 45 * time.Second, 60 * time.Second, 90 * time.Second, 120 * time.Second, 20 * time.Minute} {
		manager.rememberCheckLocked(requested)
	}
	if !slices.Equal(manager.history.recentChecks, []int{30, 45, 60, 90, 120, 300}) {
		t.Fatalf("recent checks = %v", manager.history.recentChecks)
	}
}
