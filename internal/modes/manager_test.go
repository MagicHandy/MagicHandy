package modes

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakeEngine implements the narrow Engine surface for manager behavior tests.
type fakeEngine struct {
	mu       sync.Mutex
	running  bool
	paused   bool
	starts   []motion.MotionTarget
	targets  []motion.MotionTarget
	reasons  []string
	startErr error
}

func (f *fakeEngine) Start(_ context.Context, target motion.MotionTarget, _ config.MotionSettings) (motion.ActiveMotionState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return motion.ActiveMotionState{}, f.startErr
	}
	f.running = true
	f.paused = false
	f.starts = append(f.starts, target)
	return motion.ActiveMotionState{Running: true, Target: target}, nil
}

func (f *fakeEngine) ApplyTarget(_ context.Context, target motion.MotionTarget, reason string) (motion.ActiveMotionState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.targets = append(f.targets, target)
	f.reasons = append(f.reasons, reason)
	return motion.ActiveMotionState{Running: f.running, Target: target}, nil
}

func (f *fakeEngine) Snapshot() motion.ActiveMotionState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return motion.ActiveMotionState{Running: f.running, Paused: f.paused}
}

func (f *fakeEngine) setState(running bool, paused bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running = running
	f.paused = paused
}

func (f *fakeEngine) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.starts), len(f.targets)
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestManager(t *testing.T, engine *fakeEngine, clock *fakeClock, traces *diagnostics.TraceRing) *Manager {
	t.Helper()
	manager, err := NewManager(Options{
		Ensure:   func(context.Context) (Engine, error) { return engine, nil },
		Current:  func() Engine { return engine },
		Settings: func() config.MotionSettings { return config.DefaultSettings().Motion },
		Traces:   traces,
		Now:      clock.Now,
		Tick:     2 * time.Millisecond,
		Seed:     42,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Shutdown)
	return manager
}

func waitFor(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not reached in time")
}

func TestArmSegmentUsesLatencyAwareDwellFloor(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	manager := &Manager{options: Options{Now: clock.Now}}
	manager.armSegment(Segment{DurationMillis: 1000}, 9000)
	if got := manager.deadline.Sub(clock.Now()); got != 9750*time.Millisecond {
		t.Fatalf("latency dwell = %s, want 9.75s", got)
	}
	manager.armSegment(Segment{DurationMillis: 1000}, 30000)
	if got := manager.deadline.Sub(clock.Now()); got != maximumLatencyDwell {
		t.Fatalf("capped latency dwell = %s, want %s", got, maximumLatencyDwell)
	}
}

func TestFreestyleCrossesSegmentBoundariesWithoutRestarting(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	traces := diagnostics.NewTraceRing(256)
	manager := newTestManager(t, engine, clock, traces)

	if _, err := manager.Start(context.Background(), ModeFreestyle); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, time.Second, func() bool { starts, _ := engine.counts(); return starts == 1 })
	waitFor(t, 2*time.Second, func() bool { return manager.Status().SegmentIndex >= 1 })

	// Cross four segment boundaries by jumping the planner clock.
	for range 4 {
		before := manager.Status().SegmentIndex
		clock.Advance(150 * time.Second)
		waitFor(t, 2*time.Second, func() bool { return manager.Status().SegmentIndex > before })
	}

	starts, retargets := engine.counts()
	if starts != 1 {
		t.Fatalf("starts = %d, want exactly 1 (segment changes must retarget, not restart)", starts)
	}
	if retargets < 4 {
		t.Fatalf("retargets = %d, want >= 4", retargets)
	}

	// Planner decisions are traceable: seed, style, and full score tables.
	countSegmentRows := func() int {
		rows := 0
		for _, row := range traces.Rows() {
			if row.Planner != nil && row.Planner.Event == "freestyle_segment" {
				rows++
			}
		}
		return rows
	}
	waitFor(t, 2*time.Second, func() bool { return countSegmentRows() >= 4 })
	for _, row := range traces.Rows() {
		if row.Planner != nil && row.Planner.Event == "freestyle_segment" {
			if row.Planner.Seed != 42 || len(row.Planner.Scores) != 3 || row.Planner.Style == "" {
				t.Fatalf("planner row incomplete: %+v", row.Planner)
			}
		}
	}
}

func retargetCount(engine *fakeEngine) int {
	_, targets := engine.counts()
	return targets
}

func TestFreestyleSuspendsWhilePausedAndUserPauseIsNeverOverridden(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	manager := newTestManager(t, engine, clock, diagnostics.NewTraceRing(64))

	if _, err := manager.Start(context.Background(), ModeFreestyle); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, time.Second, func() bool { starts, _ := engine.counts(); return starts == 1 })

	engine.setState(false, true) // user paused
	clock.Advance(300 * time.Second)
	time.Sleep(30 * time.Millisecond)
	starts, retargets := engine.counts()
	if starts != 1 || retargets != 0 {
		t.Fatalf("paused freestyle acted: starts=%d retargets=%d", starts, retargets)
	}

	// Resume: the planner continues.
	engine.setState(true, false)
	clock.Advance(300 * time.Second)
	waitFor(t, time.Second, func() bool { return retargetCount(engine) >= 1 })
}

func TestFreestyleStopsAfterUserStopInsteadOfRestarting(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	manager := newTestManager(t, engine, clock, diagnostics.NewTraceRing(64))

	if _, err := manager.Start(context.Background(), ModeFreestyle); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, time.Second, func() bool { starts, _ := engine.counts(); return starts == 1 })

	manager.NotifyUserStop()
	engine.setState(false, false)
	clock.Advance(300 * time.Second)
	time.Sleep(30 * time.Millisecond)

	if manager.Status().Active {
		t.Fatal("freestyle still active after user stop")
	}
	if starts, _ := engine.counts(); starts != 1 {
		t.Fatalf("starts = %d after user stop, want 1", starts)
	}
}

func TestChatKeepaliveRestartsOnlyAfterRecovery(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	traces := diagnostics.NewTraceRing(64)
	manager := newTestManager(t, engine, clock, traces)

	if _, err := manager.Start(context.Background(), ModeChat); err != nil {
		t.Fatalf("Start chat: %v", err)
	}
	// No target yet: nothing to keep alive.
	time.Sleep(20 * time.Millisecond)
	if starts, _ := engine.counts(); starts != 0 {
		t.Fatalf("keepalive started without a chat target: %d", starts)
	}

	target := motion.MotionTarget{Source: "chat", PatternID: motion.PatternPulse, SpeedPercent: 30}
	manager.NotifyChatTarget(target)
	engine.setState(false, false) // engine idle from a recovery stop
	waitFor(t, time.Second, func() bool { starts, _ := engine.counts(); return starts == 1 })

	// A paused engine is a user decision: keepalive must never override it.
	engine.setState(false, true)
	time.Sleep(30 * time.Millisecond)
	if starts, _ := engine.counts(); starts != 1 {
		t.Fatalf("keepalive restarted paused motion: %d starts", starts)
	}

	// After an explicit user/chat stop, keepalive stands down entirely.
	manager.NotifyChatStop()
	engine.setState(false, false)
	if _, err := manager.Start(context.Background(), ModeChat); err != nil {
		t.Fatalf("restart chat mode: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if starts, _ := engine.counts(); starts != 1 {
		t.Fatalf("keepalive restarted after user stop: %d starts", starts)
	}

	rows := traces.Rows()
	sawKeepalive := false
	for _, row := range rows {
		if row.Planner != nil && row.Planner.Event == "chat_keepalive_restart" {
			sawKeepalive = true
		}
	}
	if !sawKeepalive {
		t.Fatal("keepalive restart left no planner trace row")
	}
}

func TestModeSwitchReplacesActiveMode(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	manager := newTestManager(t, engine, clock, diagnostics.NewTraceRing(64))

	if _, err := manager.Start(context.Background(), ModeFreestyle); err != nil {
		t.Fatalf("Start freestyle: %v", err)
	}
	status, err := manager.Start(context.Background(), ModeChat)
	if err != nil {
		t.Fatalf("switch to chat: %v", err)
	}
	if status.Mode != ModeChat {
		t.Fatalf("mode = %q, want chat", status.Mode)
	}
	if _, err := manager.Start(context.Background(), "story"); err == nil {
		t.Fatal("unknown mode accepted")
	}
}
