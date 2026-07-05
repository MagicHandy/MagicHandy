package motion

import (
	"context"
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
	if countCommands(commands, transport.CommandKindHSPAdd) < 2 {
		t.Fatalf("commands = %+v, want continuous HSP add chunks", commands)
	}
	if countCommands(commands, transport.CommandKindHSPPlay) != 1 {
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

	initialAdds := countCommands(fake.Commands(), transport.CommandKindHSPAdd)
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if countCommands(fake.Commands(), transport.CommandKindHSPAdd) > initialAdds {
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
	assertTraceAnnotation(t, traces.Rows(), "target_update", "phase_preserved=true")
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
	if countCommands(fake.Commands(), transport.CommandKindHSPAdd) < 5 {
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

func (t *blockingAddTransport) AddHSP(ctx context.Context, command transport.HSPAddCommand) (transport.CommandResult, error) {
	t.blockMu.Lock()
	block := t.blockAdds
	t.blockMu.Unlock()
	if !block {
		return t.Fake.AddHSP(ctx, command)
	}

	t.startOnce.Do(func() { close(t.addStarted) })
	<-t.releaseAdd
	err := ctx.Err()
	if err == nil {
		err = context.Canceled
	}
	return transport.CommandResult{
		Kind:        transport.CommandKindHSPAdd,
		Transport:   "blocked_append",
		OK:          false,
		Status:      "failed",
		Error:       err.Error(),
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, err
}
