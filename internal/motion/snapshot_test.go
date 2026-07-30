package motion

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestEngineSnapshotSeparatesCurrentPlaybackFromBufferedTail(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	engine := newSnapshotTestEngine(t, &now)
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	initial := engine.Snapshot()
	if initial.CurrentSample == nil || initial.CurrentSample.TimeMillis != 0 {
		t.Fatalf("initial current sample = %+v, want stream time 0", initial.CurrentSample)
	}
	if initial.LastSample == nil || initial.LastSample.TimeMillis <= initial.CurrentSample.TimeMillis {
		t.Fatalf("buffered tail = %+v, want it ahead of current sample %+v", initial.LastSample, initial.CurrentSample)
	}

	now = now.Add(250 * time.Millisecond)
	live := engine.Snapshot()
	if live.CurrentSample == nil || live.CurrentSample.TimeMillis != 250 {
		t.Fatalf("live current sample = %+v, want stream time 250", live.CurrentSample)
	}
	if live.LastSample == nil || live.LastSample.TimeMillis == live.CurrentSample.TimeMillis {
		t.Fatalf("snapshot reused buffered tail as current sample: current=%+v tail=%+v", live.CurrentSample, live.LastSample)
	}
}

func TestEngineSnapshotFreezesCurrentSampleWhilePaused(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	engine := newSnapshotTestEngine(t, &now)

	now = now.Add(250 * time.Millisecond)
	paused, err := engine.Pause(context.Background(), "sample_freeze")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	if paused.CurrentSample == nil {
		t.Fatal("paused current sample is nil")
	}
	frozen := *paused.CurrentSample

	now = now.Add(30 * time.Second)
	later := engine.Snapshot()
	if later.CurrentSample == nil || *later.CurrentSample != frozen {
		t.Fatalf("paused current sample advanced from %+v to %+v", frozen, later.CurrentSample)
	}
}

func newSnapshotTestEngine(t *testing.T, now *time.Time) *Engine {
	t.Helper()
	fake := transport.NewFake(transport.WithClock(func() time.Time { return *now }))
	engine, err := NewEngine(EngineOptions{
		Transport:        fake,
		Traces:           diagnostics.NewTraceRing(32),
		Now:              func() time.Time { return *now },
		DispatchInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if _, err := engine.Start(context.Background(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return engine
}
