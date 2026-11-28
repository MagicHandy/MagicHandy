package motion

import (
	"context"
	"math"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestSamplingFailureAfterPlayStopsRun(t *testing.T) {
	points := []CurvePoint{{0, 20}, {3000, 50}, {3020, 70}, {3040, 30}, {3060, 70}, {5000, 50}, {8000, 20}}
	target := MotionTarget{Source: TargetSourceMedia, Media: &MediaTimelineDefinition{ID: "clip", Name: "Clip", DurationMillis: 8000, Points: points}}
	owner := &timingCapabilityTransport{Fake: transport.NewFake(), minimumInterval: 50 * time.Millisecond}
	start := time.Now()
	var elapsed atomic.Int64
	engine, err := NewEngine(EngineOptions{Transport: owner, Now: func() time.Time { return start.Add(time.Duration(elapsed.Load()) * time.Millisecond) }, SampleInterval: 100 * time.Millisecond, ChunkSize: 3, DispatchInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Start(t.Context(), target, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	elapsed.Store(3500)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && countCommands(owner.Commands(), transport.CommandKindStop) == 0 {
		time.Sleep(time.Millisecond)
	}
	state, commands := engine.Snapshot(), owner.Commands()
	if state.Running || countCommands(commands, transport.CommandKindStop) != 1 || commands[len(commands)-1].Stop == nil || !commands[len(commands)-1].Stop.Fault {
		t.Fatalf("sampler recovery: running=%v commands=%d last=%s", state.Running, len(commands), commands[len(commands)-1].Kind)
	}
	engine.mu.Lock()
	reason := engine.lastError
	engine.mu.Unlock()
	if !strings.Contains(reason, "50ms command interval") || countCommands(commands, transport.CommandKindPointsPlay) != 1 {
		t.Fatalf("cause=%q; expected a running owner's minimum interval", reason)
	}
}

func TestRetiredDispatchTickPreservesFault(t *testing.T) {
	for _, running := range []bool{false, true} {
		owner := transport.NewFake()
		engine, err := NewEngine(EngineOptions{Transport: owner, DispatchInterval: time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		const cause = "owner rejected the command interval"
		engine.runEpoch, engine.running, engine.lastError = 2, running, cause
		tick := make(chan struct{}, 1)
		engine.now = func() time.Time {
			select {
			case tick <- struct{}{}:
			default:
			}
			return time.Now()
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			engine.runLoop(ctx, 1)
		}()
		select {
		case <-tick:
		case <-time.After(2 * time.Second):
			cancel()
			<-done
			t.Fatal("the retired loop never entered its queued tick")
		}
		cancel()
		<-done
		if engine.lastError != cause || len(owner.Commands()) != 0 {
			t.Fatalf("retired tick with running=%t: error=%q commands=%d", running, engine.lastError, len(owner.Commands()))
		}
	}
}

func TestBurstRetargetsUseFirstUncommittedHandoff(t *testing.T) {
	start := time.Now()
	var elapsed atomic.Int64
	owner := transport.NewFake()
	engine, err := NewEngine(EngineOptions{Transport: owner, Now: func() time.Time { return start.Add(time.Duration(elapsed.Load()) * time.Millisecond) }})
	if err != nil {
		t.Fatal(err)
	}
	target := MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 35}
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	if _, err = engine.Start(t.Context(), target, settings); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	referenceOwner := transport.NewFake()
	reference, err := NewEngine(EngineOptions{Transport: referenceOwner, Now: engine.now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reference.Start(t.Context(), target, settings); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = reference.Stop(context.Background(), "cleanup") })
	first := target
	first.SpeedPercent = 36
	if _, err = reference.ApplyTarget(t.Context(), first, "first_edit"); err != nil {
		t.Fatal(err)
	}
	initial := owner.Commands()
	for speed := 36; speed <= 45; speed++ {
		target.SpeedPercent = speed
		if _, err = engine.ApplyTarget(t.Context(), target, "interactive"); err != nil {
			t.Fatal(err)
		}
	}
	engine.mu.Lock()
	handoff, epoch := engine.plan.HandoffMillis, engine.runEpoch
	bound := engine.leadMillisLocked() + 2*int64(engine.chunkSize)*engine.sampleInterval.Milliseconds()
	engine.mu.Unlock()
	before := owner.Commands()
	if handoff > bound || !reflect.DeepEqual(initial, before[:len(initial)]) {
		t.Fatalf("handoff %dms, owner bound %dms; accepted prefix must remain intact", handoff, bound)
	}
	elapsed.Store(handoff - 100)
	if err = engine.dispatchIfLeadNeeded(t.Context(), epoch, "refill"); err != nil {
		t.Fatal(err)
	}
	if err = reference.dispatchIfLeadNeeded(t.Context(), reference.runEpoch, "refill"); err != nil {
		t.Fatal(err)
	}
	var referencePoints []transport.TimedPoint
	for _, cmd := range referenceOwner.Commands() {
		if cmd.PointsAdd != nil {
			referencePoints = append(referencePoints, cmd.PointsAdd.Points...)
		}
	}
	for _, cmd := range owner.Commands()[len(before):] {
		if add := cmd.PointsAdd; add != nil && add.SpeedPercent == 45 {
			for _, point := range add.Points {
				if math.Abs(point.PositionPercent-interpolatedReference(referencePoints, point.TimeMillis)) < 1e-6 {
					continue
				}
				if point.TimeMillis > bound {
					t.Fatalf("first different wire point %dms > %dms", point.TimeMillis, bound)
				}
				t.Logf("latest edit handoff %dms, first different wire point %dms, bound %dms", handoff, point.TimeMillis, bound)
				return
			}
		}
	}
	t.Fatal("latest edit did not change the submitted trajectory")
}

func interpolatedReference(points []transport.TimedPoint, at int64) float64 {
	index := sort.Search(len(points), func(i int) bool { return points[i].TimeMillis >= at })
	if index == 0 {
		return points[0].PositionPercent
	}
	if index == len(points) {
		return points[len(points)-1].PositionPercent
	}
	left, right := points[index-1], points[index]
	return left.PositionPercent + (right.PositionPercent-left.PositionPercent)*float64(at-left.TimeMillis)/float64(right.TimeMillis-left.TimeMillis)
}

func TestPlaybackClockRetainsMonotonicTime(t *testing.T) {
	deviceWall := time.Now()
	owner := transport.NewFake(transport.WithClock(func() time.Time { return deviceWall }))
	engine, err := NewEngine(EngineOptions{Transport: owner})
	if err != nil {
		t.Fatal(err)
	}
	now := engine.now()
	if reflect.DeepEqual(now, now.Round(0)) {
		t.Fatal("default playback clock stripped the monotonic reading")
	}
	// Device timestamps may jump in either direction; elapsed playback is on
	// the engine clock even while those timestamps are retained in diagnostics.
	for index, delta := range []time.Duration{12 * time.Hour, -24 * time.Hour} {
		deviceWall = deviceWall.Add(delta)
		ack, stopErr := owner.Stop(t.Context(), transport.StopCommand{Reason: "clock_test"})
		if stopErr != nil {
			t.Fatal(stopErr)
		}
		engine.startedAt = now
		elapsed := time.Duration(index+1) * time.Second
		if got := engine.estimatedPlaybackMillisLocked(now.Add(elapsed)); got != elapsed.Milliseconds() || ack.CompletedAt != deviceWall.UTC().Format(time.RFC3339Nano) {
			t.Fatalf("elapsed %dms, wire time %s", got, ack.CompletedAt)
		}
	}
}

func BenchmarkProgramObservation(b *testing.B) {
	for _, size := range []int{2, 4096} {
		name := "small"
		if size > 2 {
			name = "full"
		}
		points := make([]CurvePoint, size)
		for i := range points {
			points[i] = CurvePoint{TimeMillis: int64(i) * 200, PositionPercent: float64(20 + i%2*60)}
		}
		target := MotionTarget{Source: "library", Program: &ProgramDefinition{ID: "observed", Name: "Observed", DurationMillis: points[size-1].TimeMillis, Points: points}, SpeedPercent: 40}
		settings := config.DefaultSettings().Motion
		plan := NewMotionPlan("observed", target, settings, 0, 0, time.Now())
		if plan.compileErr != nil {
			b.Fatal(plan.compileErr)
		}
		engine := &Engine{running: true, now: time.Now, plan: plan, settings: settings, startedAt: time.Now()}
		for _, projection := range []struct {
			name string
			read func() ActiveMotionState
		}{{"internal", engine.Snapshot}, {"browser", engine.Observation}} {
			b.Run(name+"/"+projection.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					runtime.KeepAlive(projection.read())
				}
			})
		}
	}
}

func TestStaleResumeCannotRestartSubsequentPausedRun(t *testing.T) {
	fake := transport.NewFake()
	engine, err := NewEngine(EngineOptions{Transport: fake})
	if err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultSettings().Motion
	target := MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 30}
	if _, err = engine.Start(t.Context(), target, settings); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	stale := engine.AdmissionGeneration()
	if _, err = engine.Stop(t.Context(), "invalidate"); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Start(t.Context(), target, settings); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Pause(t.Context(), "pause"); err != nil {
		t.Fatal(err)
	}
	before := len(fake.Commands())
	if _, err = engine.ResumeAtGeneration(t.Context(), "stale", settings, stale); err == nil || len(fake.Commands()) != before || !engine.Observation().Paused {
		t.Fatal("superseded Resume reached a later run")
	}
}
