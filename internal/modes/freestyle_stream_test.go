package modes

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type freestylePreferences struct {
	mu       sync.Mutex
	settings config.FreestyleSettings
	motion   config.MotionSettings
}

func (p *freestylePreferences) get() config.FreestyleSettings {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.settings
}

func (p *freestylePreferences) limits() config.MotionSettings {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.motion
}

func (p *freestylePreferences) update(change func(*config.FreestyleSettings, *config.MotionSettings)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	change(&p.settings, &p.motion)
}

func newFreestyleTestManager(t *testing.T, engine *fakeEngine, clock *fakeClock, preferences *freestylePreferences) (*Manager, *diagnostics.TraceRing) {
	t.Helper()
	engine.mu.Lock()
	engine.now = clock.Now
	engine.mu.Unlock()
	traces := diagnostics.NewTraceRing(512)
	manager, err := NewManager(Options{
		Ensure:            func(context.Context) (Engine, error) { return engine, nil },
		Current:           func() Engine { return engine },
		Settings:          preferences.limits,
		FreestyleSettings: preferences.get,
		Traces:            traces,
		Now:               clock.Now,
		Tick:              2 * time.Millisecond,
		Seed:              42,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	return manager, traces
}

func defaultFreestylePreferences() *freestylePreferences {
	return &freestylePreferences{settings: config.DefaultFreestyleSettings(), motion: config.DefaultSettings().Motion}
}

func lastFreestyleTarget(engine *fakeEngine) *motion.FlowSpec {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	return engine.target.Flow
}

func TestFreestylePreferenceChangeRampsAfterQueuedMotion(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	preferences := defaultFreestylePreferences()
	manager, _ := newFreestyleTestManager(t, engine, clock, preferences)
	if _, err := manager.Start(context.Background(), ModeFreestyle); err != nil {
		t.Fatal(err)
	}
	waitForAutonomousStart(t, manager, engine)
	clock.Advance(20 * time.Second) // 25 strokes into the first window.

	preferences.update(func(settings *config.FreestyleSettings, _ *config.MotionSettings) {
		settings.Feel, settings.PacePercent, settings.FocusPercent = config.FreestyleFeelCustom, 90, 80
	})
	waitFor(t, time.Second, func() bool { return retargetCount(engine) == 1 })
	flow := lastFreestyleTarget(engine)
	window := flow.Freestyle
	// The playhead is at stroke 25; the fake engine can edit from stroke 28.
	if window.StartStroke != 25 || len(window.Keyframes) != 2 {
		t.Fatalf("window = start %d keyframes %+v", window.StartStroke, window.Keyframes)
	}
	change := window.Keyframes[1]
	if change.Stroke < 28 || change.Linear || change.Controls.PacePercent != 90 || change.Controls.FocusPercent != 80 {
		t.Fatalf("control change = %+v, want an eased ramp after the queued strokes", change)
	}
	if window.Keyframes[0].Controls.PacePercent != 50 {
		t.Fatalf("queued history changed: %+v", window.Keyframes[0])
	}
	// Nothing further changes until the window runs low.
	time.Sleep(20 * time.Millisecond)
	if retargetCount(engine) != 1 {
		t.Fatalf("an unchanged stream was re-planned: %d retargets", retargetCount(engine))
	}
}

func TestFreestyleLimitChangeReplansTheBand(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	preferences := defaultFreestylePreferences()
	manager, _ := newFreestyleTestManager(t, engine, clock, preferences)
	if _, err := manager.Start(context.Background(), ModeFreestyle); err != nil {
		t.Fatal(err)
	}
	waitForAutonomousStart(t, manager, engine)
	preferences.update(func(_ *config.FreestyleSettings, limits *config.MotionSettings) {
		limits.SpeedMinPercent, limits.SpeedMaxPercent = 30, 45
	})
	waitFor(t, time.Second, func() bool { return retargetCount(engine) == 1 })
	flow := lastFreestyleTarget(engine)
	if flow.SpeedPercent != 45 || flow.Freestyle.MinSpeedPercent != 30 {
		t.Fatalf("band = %d-%d, want 30-45", flow.Freestyle.MinSpeedPercent, flow.SpeedPercent)
	}
}

func TestFreestyleCooldownComesToRestAndEnds(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	preferences := defaultFreestylePreferences()
	preferences.update(func(settings *config.FreestyleSettings, _ *config.MotionSettings) {
		settings.Shape, settings.ShapeMinutes = config.FreestyleShapeCooldown, 1
	})
	manager, traces := newFreestyleTestManager(t, engine, clock, preferences)
	if _, err := manager.Start(context.Background(), ModeFreestyle); err != nil {
		t.Fatal(err)
	}
	waitForAutonomousStart(t, manager, engine)
	start := lastFreestyleTarget(engine).Freestyle
	if start.Keyframes[0].Controls.EnergyPercent != 100 || len(start.Keyframes) < 3 {
		t.Fatalf("cooldown did not plan its descent: %+v", start.Keyframes)
	}
	// The shape clock advances at most five seconds per tick.
	for range 14 {
		clock.Advance(5 * time.Second)
		time.Sleep(10 * time.Millisecond)
	}
	waitFor(t, time.Second, func() bool {
		flow := lastFreestyleTarget(engine)
		return flow != nil && flow.Freestyle.EndStroke > 0
	})
	final := lastFreestyleTarget(engine).Freestyle
	if final.StartStroke+final.Strokes != final.EndStroke {
		t.Fatalf("final window %+v does not end at the stream's end", final)
	}
	if status := manager.Status(); status.Freestyle == nil || !status.Freestyle.Ending || status.Freestyle.ShapePhase != ShapePhaseFinished {
		t.Fatalf("status = %+v", status.Freestyle)
	}
	// The engine finishes the final window and stops; Freestyle ends there.
	engine.setState(false, false)
	waitFor(t, time.Second, func() bool { return !manager.Status().Active })
	if starts, _ := engine.counts(); starts != 1 {
		t.Fatalf("a finished cooldown restarted: %d starts", starts)
	}
	found := false
	for _, row := range traces.Rows() {
		found = found || (row.Reason == "mode_stopped" && row.Planner != nil && row.Planner.Note == "freestyle_complete")
	}
	if !found {
		t.Fatal("completion was not traced")
	}
}

func TestFreestyleRecoveryResumesTheStreamFromRest(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	manager, _ := newFreestyleTestManager(t, engine, clock, defaultFreestylePreferences())
	if _, err := manager.Start(context.Background(), ModeFreestyle); err != nil {
		t.Fatal(err)
	}
	waitForAutonomousStart(t, manager, engine)
	clock.Advance(16 * time.Second) // Stroke 20 of the first window.
	waitFor(t, time.Second, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		return manager.freestyle.lastStroke == 20
	})
	engine.setState(false, false) // A transport failure, not a user Stop.
	waitFor(t, time.Second, func() bool { starts, _ := engine.counts(); return starts == 2 })
	engine.mu.Lock()
	first, second := engine.starts[0].Flow, engine.starts[1].Flow
	engine.mu.Unlock()
	if second.Seed != first.Seed || second.Freestyle.FromStroke != 22 || second.Freestyle.StartStroke != 22 {
		t.Fatalf("recovery = seed %d from %d start %d, want the same stream from rest at 22",
			second.Seed, second.Freestyle.FromStroke, second.Freestyle.StartStroke)
	}
}

func TestFreestyleShapeChangeBeginsANewArc(t *testing.T) {
	engine := &fakeEngine{}
	clock := &fakeClock{now: time.Unix(0, 0)}
	preferences := defaultFreestylePreferences()
	manager, _ := newFreestyleTestManager(t, engine, clock, preferences)
	if _, err := manager.Start(t.Context(), ModeFreestyle); err != nil {
		t.Fatal(err)
	}
	waitForAutonomousStart(t, manager, engine)
	for range 16 {
		clock.Advance(5 * time.Second)
		time.Sleep(10 * time.Millisecond)
	}
	preferences.update(func(settings *config.FreestyleSettings, _ *config.MotionSettings) {
		settings.Shape, settings.ShapeMinutes = config.FreestyleShapeCooldown, 1
	})
	waitFor(t, time.Second, func() bool {
		return lastFreestyleTarget(engine).Freestyle.Keyframes[len(lastFreestyleTarget(engine).Freestyle.Keyframes)-1].Controls.EnergyPercent < 100
	})
	status := manager.Status().Freestyle
	if status == nil || status.Ending || status.ShapeProgressPercent != 0 || status.EnergyPercent != 100 {
		t.Fatalf("new cooldown inherited the old session clock: %+v", status)
	}
}

func TestFreestyleShapesStayInsideTheChosenPace(t *testing.T) {
	for _, shape := range []string{config.FreestyleShapeSteady, config.FreestyleShapeBuild, config.FreestyleShapeWaves,
		config.FreestyleShapeEdge, config.FreestyleShapeCooldown} {
		settings := config.DefaultFreestyleSettings()
		settings.Shape, settings.ShapeMinutes = shape, 10
		previous := shapeAt(settings, 7, 0)
		phases := map[string]bool{}
		for second := 0; second <= 1800; second++ {
			reading := shapeAt(settings, 7, time.Duration(second)*time.Second)
			if reading.energy < 0 || reading.energy > 1 || reading.tease < 0 || reading.tease > 1 {
				t.Fatalf("%s at %ds left its bounds: %+v", shape, second, reading)
			}
			// Shapes move gradually; even Edge's back-off takes seconds.
			if diff := reading.energy - previous.energy; diff > 0.05 || diff < -0.2 {
				t.Fatalf("%s jumped at %ds: %.3f -> %.3f", shape, second, previous.energy, reading.energy)
			}
			phases[reading.phase] = true
			previous = reading
		}
		assertFreestyleShapePhases(t, settings, phases)
	}
}

func assertFreestyleShapePhases(t *testing.T, settings config.FreestyleSettings, phases map[string]bool) {
	t.Helper()
	shape := settings.Shape
	switch shape {
	case config.FreestyleShapeBuild:
		if !phases[ShapePhaseBuilding] || !phases[ShapePhaseHolding] || shapeAt(settings, 7, 0).energy > 0.2 {
			t.Fatalf("build phases %v", phases)
		}
	case config.FreestyleShapeCooldown:
		if !shapeAt(settings, 7, 10*time.Minute).finished || shapeAt(settings, 7, 9*time.Minute).finished {
			t.Fatal("cooldown did not finish at its duration")
		}
	case config.FreestyleShapeEdge:
		for _, phase := range []string{ShapePhaseBuilding, ShapePhasePeak, ShapePhaseBackingOff, ShapePhaseTeasing} {
			if !phases[phase] {
				t.Fatalf("edge never reached %s: %v", phase, phases)
			}
		}
	case config.FreestyleShapeWaves:
		if !phases[ShapePhaseRising] || !phases[ShapePhaseFalling] {
			t.Fatalf("waves phases %v", phases)
		}
	}
}
