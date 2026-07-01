package motion

import (
	"context"
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
	assertReversePointMapping(t, fake.Commands(), state.LastSample)
	assertTraceReason(t, traces.Rows(), "settings_refresh")
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
	assertTraceAnnotation(t, traces.Rows(), "target_update", "phase_preserved=true")
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
	if countCommands(fake.Commands(), transport.CommandKindHSPAdd) < 10 {
		t.Fatalf("commands = %+v, want soak to append many chunks", fake.Commands())
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
