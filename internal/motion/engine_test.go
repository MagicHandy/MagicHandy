package motion

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestEngineContinuousFakePlaybackAndStop(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(128)
	engine := newTestEngine(t, fake, traces, 5*time.Millisecond)

	_, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(35 * time.Millisecond)

	state, err := engine.Stop(context.Background(), "test_stop")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if state.Running {
		t.Fatalf("state = %+v, want stopped", state)
	}

	commands := fake.Commands()
	if countCommands(commands, transport.CommandKindPointsAdd) < 2 {
		t.Fatalf("commands = %+v, want continuous HSP add chunks", commands)
	}
	if countCommands(commands, transport.CommandKindPointsPlay) != 1 {
		t.Fatalf("commands = %+v, want one HSP play", commands)
	}
	if countCommands(commands, transport.CommandKindStop) != 1 {
		t.Fatalf("commands = %+v, want explicit stop", commands)
	}

	afterStop := len(fake.Commands())
	_, err = engine.RefreshSettings(context.Background(), config.DefaultSettings().Motion, "ignored_refresh")
	if err != nil {
		t.Fatalf("RefreshSettings after stop: %v", err)
	}
	time.Sleep(15 * time.Millisecond)
	if got := len(fake.Commands()); got != afterStop {
		t.Fatalf("command count after stop = %d, want %d", got, afterStop)
	}
}

func TestCompletedStartOutlivesRequestCancellation(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(64), 5*time.Millisecond)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	if _, err := engine.Start(requestCtx, testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancelRequest()
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	commandsBefore := len(fake.Commands())
	time.Sleep(20 * time.Millisecond)
	if state := engine.Snapshot(); !state.Running || state.LastError != "" {
		t.Fatalf("completed Start followed request cancellation: %+v", state)
	}
	if commandsAfter := len(fake.Commands()); commandsAfter <= commandsBefore {
		t.Fatalf("dispatch loop stopped with request context: commands %d -> %d", commandsBefore, commandsAfter)
	}
}

func TestEngineHonorsTransportTimingFloor(t *testing.T) {
	commandTransport := &timingCapabilityTransport{
		Fake:            transport.NewFake(),
		minimumInterval: 300 * time.Millisecond,
	}
	engine, err := NewEngine(EngineOptions{Transport: commandTransport})
	if err != nil {
		t.Fatal(err)
	}
	if engine.sampleInterval != 300*time.Millisecond {
		t.Fatalf("sample interval = %v, want selected device timing floor", engine.sampleInterval)
	}
}

func TestEnginePrebuffersTransportMinimumLeadBeforePlay(t *testing.T) {
	commandTransport := &timingCapabilityTransport{
		Fake:                transport.NewFake(),
		minimumBufferedLead: 150 * time.Millisecond,
	}
	engine := newTestEngine(t, commandTransport, diagnostics.NewTraceRing(32), time.Hour)
	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	commands := commandTransport.Commands()
	playIndex := -1
	addsBeforePlay := 0
	var bufferedThrough int64
	engine.mu.Lock()
	selectedLead := engine.leadMillisLocked()
	engine.mu.Unlock()
	for index, command := range commands {
		switch command.Kind {
		case transport.CommandKindPointsAdd:
			if playIndex < 0 {
				addsBeforePlay++
				points := command.PointsAdd.Points
				bufferedThrough = points[len(points)-1].TimeMillis
			}
		case transport.CommandKindPointsPlay:
			playIndex = index
		}
	}
	if playIndex < 0 || addsBeforePlay < 2 {
		t.Fatalf("commands = %+v, want multiple prebuffer adds before Play", commands)
	}
	if bufferedThrough < selectedLead {
		t.Fatalf("prebuffer tail = %dms, want selected lead of at least %dms", bufferedThrough, selectedLead)
	}
}

func TestEngineUsesExtendedBufferedLeadOnlyForMedia(t *testing.T) {
	commandTransport := &timingCapabilityTransport{
		Fake:                     transport.NewFake(),
		minimumBufferedLead:      1500 * time.Millisecond,
		minimumMediaBufferedLead: 10 * time.Second,
	}
	engine, err := NewEngine(EngineOptions{Transport: commandTransport})
	if err != nil {
		t.Fatal(err)
	}

	engine.mu.Lock()
	interactiveLead := engine.leadMillisLocked()
	engine.plan.Target.Media = &MediaTimelineDefinition{}
	mediaLead := engine.leadMillisLocked()
	engine.mu.Unlock()

	if interactiveLead != 1500 {
		t.Fatalf("interactive lead = %dms, want 1500ms", interactiveLead)
	}
	if mediaLead != 10_000 {
		t.Fatalf("media lead = %dms, want 10000ms", mediaLead)
	}
}

func TestEngineBatchesMediaPrebufferBeforePlay(t *testing.T) {
	commandTransport := &timingCapabilityTransport{
		Fake:                     transport.NewFake(),
		minimumBufferedLead:      1500 * time.Millisecond,
		minimumMediaBufferedLead: 10 * time.Second,
		maximumPoints:            100,
	}
	engine, err := NewEngine(EngineOptions{
		Transport:        commandTransport,
		DispatchInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	target := MotionTarget{
		Label:        "media",
		Source:       "video",
		SpeedPercent: 100,
		Media: &MediaTimelineDefinition{
			ID: "media", Name: "Media", DurationMillis: 30_000,
			Points: []CurvePoint{
				{TimeMillis: 0, PositionPercent: 10},
				{TimeMillis: 30_000, PositionPercent: 90},
			},
		},
	}
	if _, err := engine.Start(context.Background(), target, config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	playSeen := false
	addsBeforePlay := 0
	largestBatch := 0
	var bufferedThrough int64
	for _, command := range commandTransport.Commands() {
		switch command.Kind {
		case transport.CommandKindPointsAdd:
			if playSeen {
				continue
			}
			addsBeforePlay++
			points := command.PointsAdd.Points
			largestBatch = max(largestBatch, len(points))
			if len(points) > commandTransport.maximumPoints {
				t.Fatalf("append contained %d points, owner cap is %d", len(points), commandTransport.maximumPoints)
			}
			bufferedThrough = points[len(points)-1].TimeMillis
		case transport.CommandKindPointsPlay:
			playSeen = true
		}
	}
	if !playSeen || addsBeforePlay > 3 {
		t.Fatalf("commands = %+v, want media prebuffered in at most three adds before Play", commandTransport.Commands())
	}
	if largestBatch != 2 {
		t.Fatalf("largest prebuffer batch = %d points, want only the two authored linear endpoints", largestBatch)
	}
	if bufferedThrough < 10_000 {
		t.Fatalf("prebuffer tail = %dms, want at least 10000ms", bufferedThrough)
	}
}

func TestEngineLeadUsesActualEmittedTail(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(32), time.Hour)
	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	engine.mu.Lock()
	runEpoch := engine.runEpoch
	engine.startedAt = engine.now()
	engine.nextSampleMillis = 600
	engine.lastSample = &MotionSample{TimeMillis: 400, PositionPercent: 50}
	engine.mu.Unlock()
	before := countCommands(fake.Commands(), transport.CommandKindPointsAdd)
	if err := engine.dispatchIfLeadNeeded(context.Background(), runEpoch, "actual_tail_test"); err != nil {
		t.Fatalf("dispatchIfLeadNeeded: %v", err)
	}
	after := countCommands(fake.Commands(), transport.CommandKindPointsAdd)
	if after != before+1 {
		t.Fatalf("add count = %d, want %d; nominal chunk end hid an underfilled emitted tail", after, before+1)
	}
}

func TestEngineClampsSubMillisecondSampleInterval(t *testing.T) {
	engine, err := NewEngine(EngineOptions{
		Transport:      transport.NewFake(),
		SampleInterval: time.Microsecond,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if engine.sampleInterval != time.Millisecond {
		t.Fatalf("sample interval = %v, want 1ms wire-time floor", engine.sampleInterval)
	}
}

func TestEngineRepeatedIdleStopStillAttemptsTransport(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(16), time.Hour)

	if _, err := engine.Stop(context.Background(), "first_idle_stop"); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Stop(context.Background(), "second_idle_stop"); err != nil {
		t.Fatal(err)
	}
	if count := countCommands(fake.Commands(), transport.CommandKindStop); count != 2 {
		t.Fatalf("idle Stop count = %d, want one transport attempt per activation", count)
	}
}

func TestEngineStopDetachesCallerCancellation(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(16), time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := engine.Stop(ctx, "canceled_caller_stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if count := countCommands(fake.Commands(), transport.CommandKindStop); count != 1 {
		t.Fatalf("Stop count = %d, want detached transport attempt", count)
	}
}

func TestEngineStopsFiniteProgramAtCompletion(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(64), 5*time.Millisecond)
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	program := ProgramDefinition{
		ID: "short-program", Name: "Short program", DurationMillis: 500,
		Points: []CurvePoint{{TimeMillis: 0}, {TimeMillis: 500, PositionPercent: 100}},
	}
	_, err := engine.Start(context.Background(), MotionTarget{
		ProgramID: program.ID, Program: &program, SpeedPercent: 100,
	}, settings)
	if err != nil {
		t.Fatalf("Start program: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for state := engine.Snapshot(); (state.Running || state.Completing) && time.Now().Before(deadline); state = engine.Snapshot() {
		time.Sleep(10 * time.Millisecond)
	}
	if state := engine.Snapshot(); state.Running || state.Completing || state.Target.ProgramID != program.ID {
		t.Fatalf("program state = %+v, want completed finite target", state)
	} else if state.Phase != 1 {
		t.Fatalf("completed program phase = %.3f, want 1", state.Phase)
	}
	if count := countCommands(fake.Commands(), transport.CommandKindStop); count != 1 {
		t.Fatalf("program stop count = %d, want one explicit engine stop", count)
	}
}

func TestEngineRejectsStartUntilProgramCompletionStopReturns(t *testing.T) {
	blocking := &completionBlockingTransport{
		Fake:    transport.NewFake(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	engine := newTestEngine(t, blocking, diagnostics.NewTraceRing(64), 5*time.Millisecond)
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	program := ProgramDefinition{
		ID: "completion-race", Name: "Completion race", DurationMillis: 500,
		Points: []CurvePoint{{TimeMillis: 0}, {TimeMillis: 500, PositionPercent: 100}},
	}
	if _, err := engine.Start(context.Background(), MotionTarget{
		ProgramID: program.ID, Program: &program, SpeedPercent: 100,
	}, settings); err != nil {
		t.Fatalf("Start program: %v", err)
	}
	select {
	case <-blocking.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("program completion did not enter transport Stop")
	}
	if state := engine.Snapshot(); state.Running || !state.Completing {
		t.Fatalf("completion state = %+v, want completing and not running", state)
	}
	if _, err := engine.Start(context.Background(), testTarget(), settings); err == nil {
		t.Fatal("new motion started while program completion Stop was in flight")
	}
	close(blocking.release)
	waitForEngineState(t, engine, 2*time.Second, func(state ActiveMotionState) bool {
		return !state.Running && !state.Completing
	})
	if _, err := engine.Start(context.Background(), testTarget(), settings); err != nil {
		t.Fatalf("Start after completion: %v", err)
	}
	if _, err := engine.Stop(context.Background(), "cleanup"); err != nil {
		t.Fatalf("Stop after completion: %v", err)
	}
}

func TestEngineFreezesPhaseAfterStop(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	fake := transport.NewFake(transport.WithClock(func() time.Time { return now }))
	engine, err := NewEngine(EngineOptions{
		Transport:        fake,
		Traces:           diagnostics.NewTraceRing(32),
		Now:              func() time.Time { return now },
		DispatchInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	now = now.Add(time.Second)
	stopped, err := engine.Stop(context.Background(), "phase_freeze")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	now = now.Add(30 * time.Second)
	if later := engine.Snapshot(); later.Phase != stopped.Phase {
		t.Fatalf("stopped phase advanced from %.6f to %.6f", stopped.Phase, later.Phase)
	}
}

func TestEngineProjectsRelativePatternIntoStrokeWindowOnlyAtTransport(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(32), time.Hour)
	engine.chunkSize = 12
	pattern := PatternDefinition{
		ID: "relative-burst", Name: "Relative burst", Kind: PatternKindBurst, CycleMillis: 500,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 0},
			{TimeMillis: 250, PositionPercent: 100},
			{TimeMillis: 500, PositionPercent: 0},
		},
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	settings.StrokeMinPercent = 20
	settings.StrokeMaxPercent = 80
	if _, err := engine.Start(context.Background(), MotionTarget{
		PatternID: pattern.ID, Pattern: &pattern, SpeedPercent: 100,
	}, settings); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	commands := fake.Commands()
	if len(commands) == 0 || commands[0].StrokeWindow == nil ||
		commands[0].StrokeWindow.MinPercent != 20 || commands[0].StrokeWindow.MaxPercent != 80 {
		t.Fatalf("stroke-window command = %+v", commands)
	}
	add := lastPointsAdd(commands)
	if add == nil {
		t.Fatalf("HSP points missing: %+v", commands)
	}
	minimum, maximum := 100.0, 0.0
	for _, point := range add.Points {
		minimum = min(minimum, point.PositionPercent)
		maximum = max(maximum, point.PositionPercent)
	}
	if minimum >= 20 || maximum <= 80 {
		t.Fatalf("engine pre-projected relative samples to %g..%g; want semantic span beyond 20..80", minimum, maximum)
	}
}

func TestEngineDispatchLoopOutlivesStartRequestContext(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(128), 2*time.Millisecond)
	t.Cleanup(func() {
		_, _ = engine.Stop(context.Background(), "cleanup")
	})

	ctx, cancel := context.WithCancel(context.Background())
	_, err := engine.Start(ctx, testTarget(), config.DefaultSettings().Motion)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel()

	initialAdds := countCommands(fake.Commands(), transport.CommandKindPointsAdd)
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if countCommands(fake.Commands(), transport.CommandKindPointsAdd) > initialAdds {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("HSP add commands stayed at %d after start context cancellation; dispatch loop stopped", initialAdds)
}

func TestEngineSettingsRefreshAppliesWhileActive(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(128)
	engine := newTestEngine(t, fake, traces, time.Hour)
	defer func() {
		_, _ = engine.Stop(context.Background(), "cleanup")
	}()

	_, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	refreshed := config.MotionSettings{
		SpeedMinPercent:  10,
		SpeedMaxPercent:  30,
		StrokeMinPercent: 10,
		StrokeMaxPercent: 90,
		ReverseDirection: true,
	}
	state, err := engine.RefreshSettings(context.Background(), refreshed, "settings_refresh")
	if err != nil {
		t.Fatalf("RefreshSettings: %v", err)
	}

	if state.Target.SpeedPercent != 30 || !state.Settings.ReverseDirection {
		t.Fatalf("state = %+v, want clamped speed and reverse setting", state)
	}
	assertRefreshedStrokeWindow(t, fake.Commands())
	assertNoRestartBeforeStop(t, fake.Commands())
	assertNoTraceRestartBeforeStop(t, traces.Rows())
	assertEngineEmitsSemanticPosition(t, fake.Commands(), state.LastSample)
	assertTraceReason(t, traces.Rows(), "settings_refresh")
}

func TestEngineSnapshotReturnsIndependentTargetPointers(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(128), time.Hour)
	defer func() {
		_, _ = engine.Stop(context.Background(), "cleanup")
	}()

	_, err := engine.Start(context.Background(), MotionTarget{
		Label:        "focused",
		Source:       "test",
		PatternID:    PatternStroke,
		SpeedPercent: 40,
		AreaFocus:    &AreaFocus{MinPercent: 20, MaxPercent: 80},
		SoftAnchor:   &SoftAnchor{PositionPercent: 55, WeightPercent: 25},
	}, config.DefaultSettings().Motion)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	snapshot := engine.Snapshot()
	snapshot.Target.AreaFocus.MinPercent = 99
	snapshot.Target.SoftAnchor.PositionPercent = 99

	fresh := engine.Snapshot()
	if fresh.Target.AreaFocus.MinPercent != 20 {
		t.Fatalf("area focus mutated through snapshot: %+v", fresh.Target.AreaFocus)
	}
	if fresh.Target.SoftAnchor.PositionPercent != 55 {
		t.Fatalf("soft anchor mutated through snapshot: %+v", fresh.Target.SoftAnchor)
	}
}

func TestEngineApplyTargetPreservesSamePatternWithoutRestart(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(128)
	engine := newTestEngine(t, fake, traces, time.Hour)
	defer func() {
		_, _ = engine.Stop(context.Background(), "cleanup")
	}()

	_, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	state, err := engine.ApplyTarget(context.Background(), MotionTarget{
		Label:        "slower same pattern",
		Source:       "test",
		PatternID:    PatternStroke,
		SpeedPercent: 30,
	}, "target_update")
	if err != nil {
		t.Fatalf("ApplyTarget: %v", err)
	}

	if state.Target.SpeedPercent != 30 {
		t.Fatalf("state = %+v, want updated semantic speed", state)
	}
	assertNoRestartBeforeStop(t, fake.Commands())
	assertNoTraceRestartBeforeStop(t, traces.Rows())
	assertTraceAnnotation(t, traces.Rows(), "target_update", "phase_preserved=true;bridge_points=true")
}

func TestEngineRetargetUsesLatencyAwareLeadAndTraceExportFields(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	fake := transport.NewFake(
		transport.WithClock(func() time.Time { return now }),
		transport.WithLatency(900*time.Millisecond),
	)
	traces := diagnostics.NewTraceRing(256)
	engine := newTestEngine(t, fake, traces, time.Hour)
	defer func() {
		_, _ = engine.Stop(context.Background(), "cleanup")
	}()

	_, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	state, err := engine.ApplyTarget(context.Background(), MotionTarget{
		Label:        "cross pattern",
		Source:       "test",
		PatternID:    PatternPulse,
		SpeedPercent: 70,
	}, "cross_pattern_change")
	if err != nil {
		t.Fatalf("ApplyTarget: %v", err)
	}

	retarget := findRetargetTrace(t, traces.Rows(), "cross_pattern_change")
	if retarget.RecentCommandLatencyMillis != 900 {
		t.Fatalf("recent latency = %d, want 900", retarget.RecentCommandLatencyMillis)
	}
	if retarget.SelectedLeadMillis < 900 {
		t.Fatalf("lead = %d, want at least observed latency", retarget.SelectedLeadMillis)
	}
	if retarget.PreviousPlanID == "" || retarget.NextPlanID == "" {
		t.Fatalf("retarget trace = %+v, want plan ids", retarget)
	}
	if retarget.PreviousTarget == nil || retarget.NextTarget == nil {
		t.Fatalf("retarget trace = %+v, want previous and next target snapshots", retarget)
	}
	if retarget.PhasePreserved {
		t.Fatalf("retarget trace = %+v, want cross-pattern phase not preserved", retarget)
	}
	if state.Running == false {
		t.Fatalf("state = %+v, want still running after retarget", state)
	}
	assertNoRestartBeforeStop(t, fake.Commands())
}

func TestEngineAreaRetargetInsertsBoundedBridgePoint(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(256)
	engine := newTestEngine(t, fake, traces, time.Hour)
	defer func() {
		_, _ = engine.Stop(context.Background(), "cleanup")
	}()

	_, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err = engine.ApplyTarget(context.Background(), MotionTarget{
		Label:        "upper area",
		Source:       "test",
		PatternID:    PatternStroke,
		SpeedPercent: 80,
		AreaFocus:    &AreaFocus{MinPercent: 0, MaxPercent: 30},
	}, "area_change")
	if err != nil {
		t.Fatalf("ApplyTarget: %v", err)
	}

	retarget := findRetargetTrace(t, traces.Rows(), "area_change")
	if !retarget.PhasePreserved {
		t.Fatalf("retarget = %+v, want same-pattern phase preservation", retarget)
	}
	if !retarget.BridgePointsInserted {
		t.Fatalf("retarget = %+v, want bridge point for large area shift", retarget)
	}
	assertTraceAnnotation(t, traces.Rows(), "area_change", "phase_preserved=true;bridge_points=true")
}

func TestEngineStopsForUnhealthyPlaybackDuringRetarget(t *testing.T) {
	fake := newPlaybackStateTransport("playing")
	traces := diagnostics.NewTraceRing(128)
	engine := newTestEngine(t, fake, traces, time.Hour)

	_, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.SetPlaybackState("starved")
	state, err := engine.ApplyTarget(context.Background(), MotionTarget{
		Label:        "recovery target",
		Source:       "test",
		PatternID:    PatternPulse,
		SpeedPercent: 60,
	}, "cross_pattern_change")
	if err == nil {
		t.Fatal("ApplyTarget succeeded despite unhealthy playback")
	}
	if state.Running {
		t.Fatalf("state = %+v, want recovery stopped", state)
	}
	if countCommands(fake.Commands(), transport.CommandKindStop) != 1 {
		t.Fatalf("commands = %+v, want recovery stop", fake.Commands())
	}
	if !hasTraceAnnotationPrefix(traces.Rows(), "recovery_retarget_lead_points", "recovery=") {
		t.Fatalf("trace rows = %+v, want recovery annotation", traces.Rows())
	}
}

func TestPlaybackStateRecoveryClassification(t *testing.T) {
	for _, state := range []string{"not_initialized", "stopped", "paused", "starved", "starving", "rejected", "stale"} {
		if !playbackStateNeedsRecovery(state) {
			t.Errorf("playback state %q did not require recovery", state)
		}
	}
	for _, state := range []string{"", "unknown", "buffering", "playing"} {
		if playbackStateNeedsRecovery(state) {
			t.Errorf("playback state %q unexpectedly required recovery", state)
		}
	}
}

func TestEngineAllowsStoppedPlaybackStateDuringStartup(t *testing.T) {
	fake := newPlaybackStateTransport("stopped")
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(128), time.Hour)

	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start rejected expected pre-play stopped state: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	fake.SetPlaybackState("stopped")
	state, err := engine.ApplyTarget(context.Background(), testTarget(), "remote_stop_check")
	if err == nil {
		t.Fatal("ApplyTarget accepted stopped state after startup")
	}
	if state.Running {
		t.Fatalf("state = %+v, want recovery stopped", state)
	}
}

func TestEngineRecoveryStopWaitsForInFlightDispatch(t *testing.T) {
	fake := newBlockingAddTransport("playing")
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(128), time.Millisecond)

	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.BlockFutureAdds()
	select {
	case <-fake.addStarted:
	case <-time.After(time.Second):
		t.Fatal("dispatch loop did not start an append command")
	}

	fake.SetPlaybackState("starved")
	done := make(chan error, 1)
	go func() {
		_, err := engine.ApplyTarget(context.Background(), MotionTarget{
			Label:        "recovery target",
			Source:       "test",
			PatternID:    PatternPulse,
			SpeedPercent: 60,
		}, "cross_pattern_change")
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("recovery returned before blocked append was drained: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(fake.releaseAdd)

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("ApplyTarget succeeded despite unhealthy playback")
		}
	case <-time.After(time.Second):
		t.Fatal("recovery did not finish after blocked append was released")
	}
}

func TestEngineStopIsFinalAfterRequestOriginatedAppend(t *testing.T) {
	fake := newBlockingAddTransport("playing")
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(128), time.Hour)
	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}

	fake.BlockFutureAdds()
	applyDone := make(chan error, 1)
	go func() {
		_, err := engine.ApplyTarget(context.Background(), MotionTarget{
			Label:        "blocked target",
			Source:       "test",
			PatternID:    PatternPulse,
			SpeedPercent: 60,
		}, "blocked_retarget")
		applyDone <- err
	}()
	select {
	case <-fake.addStarted:
	case <-time.After(time.Second):
		t.Fatal("request-originated append did not start")
	}

	stopDone := make(chan error, 1)
	go func() {
		_, err := engine.Stop(context.Background(), "final_order_stop")
		stopDone <- err
	}()
	select {
	case err := <-stopDone:
		t.Fatalf("Stop returned before the in-flight append drained: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if count := countCommands(fake.Commands(), transport.CommandKindStop); count != 0 {
		t.Fatalf("Stop reached the wire before append drained: %+v", fake.Commands())
	}

	close(fake.releaseAdd)
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not finish after append drained")
	}
	select {
	case err := <-applyDone:
		if err == nil {
			t.Fatal("retarget succeeded after its run was stopped")
		}
	case <-time.After(time.Second):
		t.Fatal("retarget did not return after Stop")
	}

	commands := fake.Commands()
	if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("commands = %+v, want Stop as the final wire command", commands)
	}
}

func TestEngineStopsAfterUncertainAppendFailure(t *testing.T) {
	commandTransport := &acceptedErrorTransport{Fake: transport.NewFake()}
	engine := newTestEngine(t, commandTransport, diagnostics.NewTraceRing(128), time.Hour)
	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	commandTransport.failNextAdd = true

	state, err := engine.ApplyTarget(context.Background(), MotionTarget{
		Label:        "failing target",
		Source:       "test",
		PatternID:    PatternPulse,
		SpeedPercent: 60,
	}, "failing_retarget")
	if err == nil {
		t.Fatal("ApplyTarget succeeded after an uncertain append failure")
	}
	if state.Running {
		t.Fatalf("state = %+v, want recovery stopped", state)
	}
	commands := commandTransport.Commands()
	if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("commands = %+v, want explicit Stop after uncertain append", commands)
	}
}

func TestEngineStopsAfterUncertainStartupPlayFailure(t *testing.T) {
	commandTransport := &acceptedErrorTransport{Fake: transport.NewFake(), failPlay: true}
	engine := newTestEngine(t, commandTransport, diagnostics.NewTraceRing(128), time.Hour)

	state, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion)
	if err == nil {
		t.Fatal("Start succeeded after an uncertain Play failure")
	}
	if state.Running {
		t.Fatalf("state = %+v, want startup aborted", state)
	}
	commands := commandTransport.Commands()
	if countCommands(commands, transport.CommandKindPointsPlay) != 1 {
		t.Fatalf("commands = %+v, want simulated accepted Play", commands)
	}
	if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("commands = %+v, want explicit Stop after uncertain Play", commands)
	}
}

func TestEngineTraceRecordsExactCommandDespiteConcurrentDiagnostics(t *testing.T) {
	commandTransport := &misleadingDiagnosticsTransport{Fake: transport.NewFake()}
	traces := diagnostics.NewTraceRing(64)
	engine := newTestEngine(t, commandTransport, traces, time.Hour)
	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	for _, row := range traces.Rows() {
		if row.Reason != "start_points" {
			continue
		}
		if row.TransportCommand == nil || row.TransportCommand.Kind != transport.CommandKindPointsAdd || row.TransportCommand.PointsAdd == nil {
			t.Fatalf("start_points trace command = %+v, want exact append command", row.TransportCommand)
		}
		return
	}
	t.Fatalf("trace rows = %+v, want start_points row", traces.Rows())
}

func TestEngineConcurrentRefreshSnapshotAndStop(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(128), 2*time.Millisecond)
	_, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 20 {
			_, _ = engine.RefreshSettings(context.Background(), config.DefaultSettings().Motion, "race_refresh")
			_ = engine.Snapshot()
		}
	}()
	time.Sleep(10 * time.Millisecond)
	if _, err := engine.Stop(context.Background(), "race_stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	<-done
}

func TestEngineShortSoakAgainstFakeTransport(t *testing.T) {
	fake := transport.NewFake()
	engine := newTestEngine(t, fake, diagnostics.NewTraceRing(256), 2*time.Millisecond)
	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	if _, err := engine.Stop(context.Background(), "soak_stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if countCommands(fake.Commands(), transport.CommandKindPointsAdd) < 5 {
		t.Fatalf("commands = %+v, want soak to maintain lead with appended chunks", fake.Commands())
	}
}

func newTestEngine(
	t *testing.T,
	fake transport.Transport,
	traces *diagnostics.TraceRing,
	dispatchInterval time.Duration,
) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineOptions{
		Transport:        fake,
		Traces:           traces,
		ChunkSize:        4,
		SampleInterval:   25 * time.Millisecond,
		DispatchInterval: dispatchInterval,
		StreamIDPrefix:   "test",
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return engine
}

func testTarget() MotionTarget {
	return MotionTarget{
		Label:        "test target",
		Source:       "test",
		PatternID:    PatternStroke,
		SpeedPercent: 80,
	}
}

type playbackStateTransport struct {
	*transport.Fake
	mu    sync.Mutex
	state string
}

func newPlaybackStateTransport(state string) *playbackStateTransport {
	return &playbackStateTransport{
		Fake:  transport.NewFake(),
		state: state,
	}
}

func (t *playbackStateTransport) Diagnostics() transport.TransportDiagnostics {
	t.mu.Lock()
	defer t.mu.Unlock()
	diagnostics := t.Fake.Diagnostics()
	diagnostics.PlaybackState = t.state
	return diagnostics
}

func (t *playbackStateTransport) SetPlaybackState(state string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = state
}

type blockingAddTransport struct {
	*playbackStateTransport
	blockMu    sync.Mutex
	blockAdds  bool
	addStarted chan struct{}
	releaseAdd chan struct{}
	startOnce  sync.Once
}

type acceptedErrorTransport struct {
	*transport.Fake
	failNextAdd bool
	failPlay    bool
}

type misleadingDiagnosticsTransport struct {
	*transport.Fake
}

func (t *misleadingDiagnosticsTransport) Diagnostics() transport.TransportDiagnostics {
	diagnostics := t.Fake.Diagnostics()
	diagnostics.LastCommand = &transport.Command{Kind: transport.CommandKindHSPState}
	return diagnostics
}

func (t *acceptedErrorTransport) AppendPoints(ctx context.Context, command transport.AppendPointsCommand) (transport.CommandResult, error) {
	result, err := t.Fake.AppendPoints(ctx, command)
	if err != nil || !t.failNextAdd {
		return result, err
	}
	t.failNextAdd = false
	return result, errors.New("append response was lost after acceptance")
}

func (t *acceptedErrorTransport) Play(ctx context.Context, command transport.PlayCommand) (transport.CommandResult, error) {
	result, err := t.Fake.Play(ctx, command)
	if err != nil || !t.failPlay {
		return result, err
	}
	t.failPlay = false
	return result, errors.New("play response was lost after acceptance")
}

func newBlockingAddTransport(state string) *blockingAddTransport {
	return &blockingAddTransport{
		playbackStateTransport: newPlaybackStateTransport(state),
		addStarted:             make(chan struct{}),
		releaseAdd:             make(chan struct{}),
	}
}

func (t *blockingAddTransport) BlockFutureAdds() {
	t.blockMu.Lock()
	defer t.blockMu.Unlock()
	t.blockAdds = true
}

func (t *blockingAddTransport) AppendPoints(ctx context.Context, command transport.AppendPointsCommand) (transport.CommandResult, error) {
	t.blockMu.Lock()
	block := t.blockAdds
	t.blockMu.Unlock()
	if !block {
		return t.Fake.AppendPoints(ctx, command)
	}

	t.startOnce.Do(func() { close(t.addStarted) })
	<-t.releaseAdd
	err := ctx.Err()
	if err == nil {
		err = context.Canceled
	}
	return transport.CommandResult{
		Kind:        transport.CommandKindPointsAdd,
		Transport:   "blocked_append",
		OK:          false,
		Status:      "failed",
		Error:       err.Error(),
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, err
}

type completionBlockingTransport struct {
	*transport.Fake
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

type timingCapabilityTransport struct {
	*transport.Fake
	minimumInterval          time.Duration
	minimumBufferedLead      time.Duration
	minimumMediaBufferedLead time.Duration
	maximumPoints            int
}

func (t *timingCapabilityTransport) MotionTimingCapabilities() transport.MotionTimingCapabilities {
	return transport.MotionTimingCapabilities{
		MinimumPointInterval:     t.minimumInterval,
		MinimumBufferedLead:      t.minimumBufferedLead,
		MinimumMediaBufferedLead: t.minimumMediaBufferedLead,
	}
}

func (t *timingCapabilityTransport) MotionSamplingCapabilities() transport.MotionSamplingCapabilities {
	return transport.MotionSamplingCapabilities{MaximumPointsPerAppend: t.maximumPoints}
}

func (t *completionBlockingTransport) Stop(ctx context.Context, command transport.StopCommand) (transport.CommandResult, error) {
	t.once.Do(func() { close(t.entered) })
	select {
	case <-t.release:
		return t.Fake.Stop(ctx, command)
	case <-ctx.Done():
		return transport.CommandResult{}, ctx.Err()
	}
}

func waitForEngineState(t *testing.T, engine *Engine, timeout time.Duration, ready func(ActiveMotionState) bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ready(engine.Snapshot()) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("engine state did not converge: %+v", engine.Snapshot())
}
