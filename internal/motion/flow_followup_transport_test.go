package motion

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestLayeredFastPatternsFitSupportedImmediateIntervals(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		for _, shape := range []string{"full", "short", "compound"} {
			for _, floor := range []int64{50, 110} {
				t.Run(fmt.Sprintf("%s/%s/%d", model, shape, floor), func(t *testing.T) {
					target, err := FlowTarget(layeredSpeedFixture(shape, 100), settings)
					if err != nil {
						t.Fatal(err)
					}
					plan := NewMotionPlan("interval", target, settings, 0, 0, time.Unix(0, 0))
					owner := &timingCapabilityTransport{Fake: transport.NewFake(), minimumInterval: time.Duration(floor) * time.Millisecond}
					engine, err := NewEngine(EngineOptions{Transport: owner})
					if err != nil {
						t.Fatal(err)
					}
					engine.plan, engine.settings, engine.positionResolutionPercent = plan, settings, 1
					var previous *MotionSample
					for engine.nextSampleMillis < 2*plan.PeriodMillis {
						points, err := engine.nextMotionSamplesLocked()
						if err != nil {
							t.Fatal(err)
						}
						for _, point := range points {
							if previous != nil {
								dt := point.TimeMillis - previous.TimeMillis
								if dt < floor || math.Abs(math.Round(point.PositionPercent)-math.Round(previous.PositionPercent))*1000/float64(dt) > referenceTravelRateForSpeed(100, model)+1e-8 {
									t.Fatal("paced command escaped interval or velocity bounds")
								}
							}
							p := point
							previous = &p
						}
					}
				})
			}
		}
	}
}

func TestLayeredDoesNotAliasTurnsForAnUnsupportedSlowTransport(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	target, err := FlowTarget(layeredSpeedFixture("short", 100), settings)
	if err != nil {
		t.Fatal(err)
	}
	plan := NewMotionPlan("slow-transport", target, settings, 0, 0, time.Unix(0, 0))
	owner := &timingCapabilityTransport{Fake: transport.NewFake(), minimumInterval: 200 * time.Millisecond}
	engine, err := NewEngine(EngineOptions{Transport: owner})
	if err != nil {
		t.Fatal(err)
	}
	engine.plan, engine.settings = plan, settings
	if _, err = engine.nextMotionSamplesLocked(); err == nil || !strings.Contains(err.Error(), "command interval") {
		t.Fatalf("unsupported cadence must report its limit, not silently lose turns: %v", err)
	}
}

// This is a fake transport and a manually prepared live run. Synchronize Stop
// with the planning lock so the test exercises contention rather than sleeping
// and hoping the requests overlap. There is no physical device or private loop.
func TestLayeredStopDuringExpensiveEdgeJoinCannotResume(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	target, err := FlowTarget(layeredSpeedFixture("compound", 5), settings)
	if err != nil {
		t.Fatal(err)
	}
	hold, err := HoldingTarget(ActiveMotionState{Target: target}, settings)
	if err != nil {
		t.Fatal(err)
	}
	old := NewMotionPlan("before", target, settings, 0, 0, time.Unix(0, 0))
	at := old.PeriodMillis * 57 / 100
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	clock := func() time.Time {
		once.Do(func() { close(entered); <-release })
		return time.Unix(0, 0).Add(time.Duration(at-1000) * time.Millisecond)
	}
	fake := transport.NewFake()
	engine, err := NewEngine(EngineOptions{Transport: fake, Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	engine.running, engine.runEpoch, engine.plan, engine.settings = true, 1, old, settings
	engine.startedAt, engine.nextSampleMillis = time.Unix(0, 0), at
	engine.runCtx, engine.cancel = runCtx, cancel
	planned := make(chan error, 1)
	go func() { planned <- engine.retarget(1, hold, "edge") }()
	<-entered
	stopped := make(chan error, 1)
	attempted := make(chan struct{})
	started := time.Now()
	go func() { close(attempted); _, err := engine.Stop(t.Context(), "contended stop"); stopped <- err }()
	<-attempted
	close(release)
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop stalled behind motion planning")
	}
	t.Logf("contended Stop completed in %s", time.Since(started))
	if err := <-planned; err != nil && !errors.Is(err, errRunInvalidated) {
		t.Fatal(err)
	}
	if engine.Snapshot().Running {
		t.Fatal("completed planner revived stopped motion")
	}
	commands := fake.Commands()
	if len(commands) != 1 || commands[0].Kind != transport.CommandKindStop {
		t.Fatal("stale planner sent device commands")
	}
}
