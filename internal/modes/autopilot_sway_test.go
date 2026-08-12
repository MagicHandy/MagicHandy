package modes

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func swayTestManager(t *testing.T, autopilot config.AutopilotSettings) *Manager {
	t.Helper()
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	manager, err := NewManager(Options{
		Ensure:            func(context.Context) (Engine, error) { return engine, nil },
		Current:           func() Engine { return engine },
		Settings:          func() config.MotionSettings { return config.DefaultSettings().Motion },
		AutopilotSettings: func() config.AutopilotSettings { return autopilot },
		Traces:            diagnostics.NewTraceRing(64),
		Now:               clock.Now,
		Tick:              2 * time.Millisecond,
		Seed:              7,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Shutdown)
	return manager
}

func swayChoice(variability VariabilityPreference) segmentChoice {
	return segmentChoice{
		segment: Segment{
			PatternID:    motion.PatternID("steady"),
			SpeedPercent: 30,
		},
		source:      "model",
		timing:      TimingNormal,
		variability: variability,
	}
}

func planSway(t *testing.T, manager *Manager, duration time.Duration, variability VariabilityPreference) []swayPoint {
	t.Helper()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.planSwayLocked(time.Unix(0, 0), duration, swayChoice(variability), manager.generation)
}

// Settled is the model's way of asking for a flat stretch, so it must produce no
// engine traffic at all — not a smaller wander.
func TestSettledVariabilityPlansNoSway(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	if points := planSway(t, manager, 60*time.Second, VariabilitySettled); len(points) != 0 {
		t.Fatalf("settled planned %d waypoints, want none", len(points))
	}
}

// The manager remains defensive for planner/fallback and direct callers even
// though model motion turns validate this required category at the chat edge.
func TestUnknownVariabilityBehavesLikeNormal(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	// Compare the earned allowance rather than two sampled schedules: planning
	// twice advances the shared RNG, so the second call legitimately drops a
	// different number of same-speed waypoints and the counts need not match.
	manager.mu.Lock()
	defer manager.mu.Unlock()
	unknown := manager.swayAllowanceLocked(60*time.Second, VariabilityPreference("frantic"))
	normal := manager.swayAllowanceLocked(60*time.Second, VariabilityNormal)
	if unknown != normal {
		t.Fatalf("unknown category earned %d waypoints, normal earned %d", unknown, normal)
	}
}

// The waypoint budget is what keeps restored texture from re-creating the
// retarget churn this work removed. A short Dynamic segment has no room and must
// get nothing; longer segments earn more, capped.
func TestSwayAllowanceScalesWithSegmentLengthAndIsCapped(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	cases := []struct {
		duration time.Duration
		want     int
	}{
		// One waypoint per 8s of stretch. The old 20s cadence gave a typical
		// 20-60s Autopilot stretch a single waypoint whichever variability the
		// model chose, so the axis barely reached the device.
		{4 * time.Second, 0},
		// Below swayMinTexturedSegment nothing is earned: the fast end is where
		// retarget churn is the risk, not flatness.
		{10 * time.Second, 0},
		{20 * time.Second, 2},
		{60 * time.Second, maxSwayPoints},
		{120 * time.Second, maxSwayPoints},
		{300 * time.Second, maxSwayPoints},
	}
	for _, testCase := range cases {
		manager.mu.Lock()
		got := manager.swayAllowanceLocked(testCase.duration, VariabilityRestless)
		manager.mu.Unlock()
		if got != testCase.want {
			t.Fatalf("%s earned %d waypoints, want %d", testCase.duration, got, testCase.want)
		}
	}
}

func TestNormalVariabilityTakesLessThanRestless(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	manager.mu.Lock()
	normal := manager.swayAllowanceLocked(120*time.Second, VariabilityNormal)
	restless := manager.swayAllowanceLocked(120*time.Second, VariabilityRestless)
	manager.mu.Unlock()
	if normal >= restless {
		t.Fatalf("normal earned %d and restless %d; restless must be the busier one", normal, restless)
	}
	if normal < 1 {
		t.Fatal("a segment long enough to earn a waypoint must not round normal down to zero")
	}
}

// Sway positions intent inside the user's band. It must never move a waypoint
// outside the configured speed limits, whatever the amplitude maths produce.
func TestSwayNeverLeavesTheUserSpeedBand(t *testing.T) {
	settings := config.DefaultSettings().Motion
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	manager, err := NewManager(Options{
		Ensure:            func(context.Context) (Engine, error) { return engine, nil },
		Current:           func() Engine { return engine },
		Settings:          func() config.MotionSettings { return settings },
		AutopilotSettings: func() config.AutopilotSettings { return config.DefaultAutopilotSettings() },
		Traces:            diagnostics.NewTraceRing(64),
		Now:               clock.Now,
		Tick:              2 * time.Millisecond,
		Seed:              11,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Shutdown)

	// Aim the segment at each edge of the band so a symmetric wander would spill
	// out of it if the clamp were missing.
	for _, speed := range []int{settings.SpeedMinPercent, settings.SpeedMaxPercent} {
		choice := swayChoice(VariabilityRestless)
		choice.segment.SpeedPercent = speed
		manager.mu.Lock()
		points := manager.planSwayLocked(time.Unix(0, 0), 120*time.Second, choice, manager.generation)
		manager.mu.Unlock()
		for _, point := range points {
			if point.speedPercent < settings.SpeedMinPercent || point.speedPercent > settings.SpeedMaxPercent {
				t.Fatalf("waypoint %d%% escaped the %d-%d%% band",
					point.speedPercent, settings.SpeedMinPercent, settings.SpeedMaxPercent)
			}
		}
	}
}

// A waypoint equal to the current speed would be exactly the no-op retarget this
// change set removed from the hold path.
func TestSwayNeverPlansANoOpWaypoint(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	points := planSway(t, manager, 120*time.Second, VariabilityRestless)
	if len(points) == 0 {
		t.Fatal("expected waypoints for a long restless segment")
	}
	previousSpeed := 30
	for _, point := range points {
		if point.speedPercent == previousSpeed {
			t.Fatalf("planned consecutive %d%% waypoints, which would be a no-op retarget", previousSpeed)
		}
		previousSpeed = point.speedPercent
	}
}

// Sampled offsets are the whole point: a fixed midpoint is what made the old
// drift metronomic. Waypoints must be spread, ordered, and clear of both edges.
func TestSwayWaypointsAreSpacedAndClearOfTheBoundaries(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	start := time.Unix(0, 0)
	duration := 120 * time.Second
	points := planSway(t, manager, duration, VariabilityRestless)
	if len(points) < 2 {
		t.Fatalf("expected several waypoints, got %d", len(points))
	}
	previous := start
	for index, point := range points {
		if point.at.Before(start.Add(swayEdgeGuard)) {
			t.Fatalf("waypoint %d landed inside the opening guard", index)
		}
		if point.at.After(start.Add(duration - swayEdgeGuard)) {
			t.Fatalf("waypoint %d landed inside the closing guard", index)
		}
		if index > 0 && point.at.Sub(previous) < swayMinSpacing {
			t.Fatalf("waypoint %d is %s after the previous one", index, point.at.Sub(previous))
		}
		previous = point.at
	}
}

// Offsets must not be identical run to run, or the texture is metronomic again.
func TestSwayOffsetsVaryBetweenSegments(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	first := planSway(t, manager, 120*time.Second, VariabilityRestless)
	second := planSway(t, manager, 120*time.Second, VariabilityRestless)
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("expected waypoints in both schedules")
	}
	identical := len(first) == len(second)
	if identical {
		for index := range first {
			if !first[index].at.Equal(second[index].at) ||
				first[index].speedPercent != second[index].speedPercent {
				identical = false
				break
			}
		}
	}
	if identical {
		t.Fatal("two consecutive schedules were identical; sampling is not varying")
	}
}

// dueSway must pop even when application later fails, or a single bad waypoint
// would be retried every tick and starve the speech clock behind it.
func TestDueSwayPopsOnReadSoAFailureCannotStarveSpeech(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	now := time.Unix(100, 0)
	manager.mu.Lock()
	manager.mode = ModeAutopilot
	manager.generation = 3
	manager.swayPoints = []swayPoint{
		{generation: 3, at: now.Add(-time.Second), speedPercent: 34},
		{generation: 3, at: now.Add(time.Minute), speedPercent: 26},
	}
	manager.mu.Unlock()

	point, ok := manager.dueSway(now, 3)
	if !ok || point.speedPercent != 34 {
		t.Fatalf("first pop = %+v ok=%t", point, ok)
	}
	if _, ok := manager.dueSway(now, 3); ok {
		t.Fatal("the future waypoint must not be due yet")
	}
	manager.mu.Lock()
	remaining := len(manager.swayPoints)
	manager.mu.Unlock()
	if remaining != 1 {
		t.Fatalf("remaining waypoints = %d, want 1", remaining)
	}
}

// A stale waypoint belongs to a superseded segment, so its texture must not be
// applied to whatever is playing now even when the tick carries the manager's
// current generation.
func TestDueSwayDiscardsASupersededSchedule(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	now := time.Unix(100, 0)
	manager.mu.Lock()
	manager.mode = ModeAutopilot
	manager.generation = 9
	manager.swayPoints = []swayPoint{{
		generation:   8,
		at:           now.Add(-time.Second),
		speedPercent: 40,
	}}
	manager.mu.Unlock()

	if _, ok := manager.dueSway(now, 9); ok {
		t.Fatal("a waypoint from an older generation was applied")
	}
	manager.mu.Lock()
	remaining := len(manager.swayPoints)
	manager.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("stale schedule retained %d waypoints", remaining)
	}
}

func TestPauseShiftsSwayScheduleWithTheSegmentDeadline(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	now := time.Unix(100, 0)
	manager.mu.Lock()
	manager.mode = ModeAutopilot
	manager.generation = 4
	manager.deadline = now.Add(time.Minute)
	manager.speedChangedAt = now.Add(-30 * time.Second)
	manager.arc.startedAt = now.Add(-time.Minute)
	manager.swayPoints = []swayPoint{{
		generation:   4,
		at:           now.Add(10 * time.Second),
		speedPercent: 34,
	}}
	manager.mu.Unlock()

	manager.freezeDeadline()

	manager.mu.Lock()
	deadline := manager.deadline
	waypointAt := manager.swayPoints[0].at
	speedChangedAt := manager.speedChangedAt
	arcStartedAt := manager.arc.startedAt
	tick := manager.options.Tick
	manager.mu.Unlock()
	if !deadline.Equal(now.Add(time.Minute + tick)) {
		t.Fatalf("paused deadline = %s, want one tick later", deadline)
	}
	if !waypointAt.Equal(now.Add(10*time.Second + tick)) {
		t.Fatalf("paused waypoint = %s, want one tick later", waypointAt)
	}
	if !speedChangedAt.Equal(now.Add(-30*time.Second + tick)) {
		t.Fatalf("paused speed history = %s, want one tick later", speedChangedAt)
	}
	if !arcStartedAt.Equal(now.Add(-time.Minute + tick)) {
		t.Fatalf("paused arc clock = %s, want one tick later", arcStartedAt)
	}
}

func TestAutopilotSpeedHistoryChangesOnlyWhenSpeedChanges(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	now := time.Unix(200, 0)
	manager.options.Now = func() time.Time { return now }
	manager.mu.Lock()
	manager.mode = ModeAutopilot
	manager.generation = 5
	manager.segment = Segment{PatternID: motion.PatternStroke, SpeedPercent: 24}
	manager.speedChangedAt = now.Add(-30 * time.Second)
	manager.mu.Unlock()

	changed := swayChoice(VariabilitySettled)
	changed.segment.SpeedPercent = 36
	if !manager.armAutopilotChoice(ModeAutopilot, &changed, 5) {
		t.Fatal("failed to arm changed-speed segment")
	}
	manager.mu.Lock()
	previous := manager.previousSpeed
	changedAt := manager.speedChangedAt
	trend := manager.speedTrendLocked()
	manager.mu.Unlock()
	if previous != 24 || !changedAt.Equal(now) || trend != SpeedTrendRising {
		t.Fatalf("changed-speed history = previous %d at %s trend %q", previous, changedAt, trend)
	}

	now = now.Add(45 * time.Second)
	unchanged := swayChoice(VariabilitySettled)
	unchanged.segment.SpeedPercent = 36
	if !manager.armAutopilotChoice(ModeAutopilot, &unchanged, 5) {
		t.Fatal("failed to arm same-speed segment")
	}
	manager.mu.Lock()
	previous = manager.previousSpeed
	unchangedAt := manager.speedChangedAt
	trend = manager.speedTrendLocked()
	manager.mu.Unlock()
	if previous != 24 || !unchangedAt.Equal(changedAt) || trend != SpeedTrendRising {
		t.Fatalf("same-speed boundary reset history = previous %d at %s trend %q",
			previous, unchangedAt, trend)
	}
}

func TestAppliedSwayRecordsItsSpeedTransition(t *testing.T) {
	manager := swayTestManager(t, config.DefaultAutopilotSettings())
	now := time.Unix(300, 0)
	manager.options.Now = func() time.Time { return now }
	manager.mu.Lock()
	manager.mode = ModeAutopilot
	manager.generation = 6
	manager.segment = Segment{PatternID: motion.PatternStroke, SpeedPercent: 30}
	manager.mu.Unlock()

	engine := manager.options.Current()
	manager.applyDueSway(
		context.Background(),
		engine,
		6,
		swayPoint{generation: 6, at: now, speedPercent: 38},
	)

	manager.mu.Lock()
	previous := manager.previousSpeed
	current := manager.segment.SpeedPercent
	changedAt := manager.speedChangedAt
	trend := manager.speedTrendLocked()
	manager.mu.Unlock()
	if previous != 30 || current != 38 || !changedAt.Equal(now) || trend != SpeedTrendRising {
		t.Fatalf("sway history = previous %d current %d at %s trend %q",
			previous, current, changedAt, trend)
	}
}

// The two-minute playback fallback was the only thing preventing a permanent
// speech stall, and nothing covered it. It should be the rare safety net, not
// the normal path — the browser now acknowledges failed playback too — but a
// genuinely lost acknowledgement still has to recover.
func TestSpeechPlaybackFallbackRecoversALostAcknowledgement(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(9000, 0)}
	decider := &fakeDecider{decisions: []Decision{{Say: "still here", Next: TimingNormal}}}
	announcer := &announceLog{}
	manager, err := NewManager(Options{
		Ensure:            func(context.Context) (Engine, error) { return engine, nil },
		Current:           func() Engine { return engine },
		Settings:          func() config.MotionSettings { return config.DefaultSettings().Motion },
		AutopilotSettings: func() config.AutopilotSettings { return config.DefaultAutopilotSettings() },
		Traces:            diagnostics.NewTraceRing(64),
		Now:               clock.Now,
		Tick:              2 * time.Millisecond,
		Seed:              13,
		Decide:            decider.decide,
		DecideSpeech:      decider.decide,
		Announce:          announcer.announce,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Shutdown)
	now := clock.Now()
	manager.mu.Lock()
	manager.mode = ModeAutopilot
	manager.generation = 1
	manager.speechWaitingID = "tts-lost"
	manager.speechFallbackAt = now.Add(speechPlaybackAckFallback)
	manager.speechNextTiming = TimingNormal
	manager.mu.Unlock()

	// Before the fallback moment the scheduler is still waiting, deliberately.
	if manager.completeSpeechWait("tts-other", "playback_complete") {
		t.Fatal("an unrelated request id completed the wait")
	}
	manager.mu.Lock()
	stillWaiting := manager.speechWaitingID
	manager.mu.Unlock()
	if stillWaiting != "tts-lost" {
		t.Fatalf("waiting id = %q, want it unchanged", stillWaiting)
	}

	// The matching id releases the wait and arms the next interval.
	if !manager.completeSpeechWait("tts-lost", "playback_ack_timeout") {
		t.Fatal("the fallback did not release the speech wait")
	}
	manager.mu.Lock()
	waiting := manager.speechWaitingID
	fallbackAt := manager.speechFallbackAt
	deadline := manager.speechDeadline
	manager.mu.Unlock()
	if waiting != "" || !fallbackAt.IsZero() {
		t.Fatalf("wait state was not cleared: id=%q fallbackAt=%v", waiting, fallbackAt)
	}
	if deadline.IsZero() {
		t.Fatal("the next speech interval was not scheduled after the fallback")
	}
}
