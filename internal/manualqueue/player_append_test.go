package manualqueue

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestPlayerAppendExtensionExtendsTimeline(t *testing.T) {
	fake := transport.NewFake()
	player := NewPlayer(fake)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	first := Session{
		Actions: []Action{{At: 0, Pos: 10}, {At: 100, Pos: 90}},
		Points: []transport.TimedPoint{
			{TimeMillis: 0, PositionPercent: 10},
			{TimeMillis: 100, PositionPercent: 90},
		},
		DurationMS: 100,
	}
	if err := player.Start(ctx, first, 0); err != nil {
		t.Fatalf("Start: %v", err)
	}

	extension := Session{
		Actions: []Action{{At: 0, Pos: 20}, {At: 50, Pos: 80}},
		Points: []transport.TimedPoint{
			{TimeMillis: 0, PositionPercent: 20},
			{TimeMillis: 50, PositionPercent: 80},
		},
		DurationMS: 50,
	}
	if err := player.AppendExtension(extension); err != nil {
		t.Fatalf("AppendExtension: %v", err)
	}

	snap := player.Snapshot()
	if snap.DurationMS != 150 {
		t.Fatalf("duration = %d, want 150", snap.DurationMS)
	}
	if len(player.Actions()) != 4 {
		t.Fatalf("actions = %d, want 4", len(player.Actions()))
	}
	if !snap.Running {
		t.Fatal("expected player to remain running after append")
	}

	cancel()
}

func TestPlayerAppendExtensionAfterCompact(t *testing.T) {
	fake := transport.NewFake()
	player := NewPlayer(fake)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	points := make([]transport.TimedPoint, 0, 220)
	for index := 0; index < 200; index++ {
		points = append(points, transport.TimedPoint{
			TimeMillis:      int64(index * 150),
			PositionPercent: 10 + (index % 80),
		})
	}
	first := Session{
		Actions:    []Action{{At: 0, Pos: 10}, {At: 29850, Pos: 90}},
		Points:     points,
		DurationMS: 30000,
	}
	if err := player.Start(ctx, first, 0); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Dispatch enough points to compact while keeping plenty of segment runway.
	time.Sleep(5 * time.Second)

	before := player.Snapshot()
	if !before.Running {
		t.Fatalf("player stopped before append: playhead=%d duration=%d", before.PlayheadMS, before.DurationMS)
	}

	extension := Session{
		Actions: []Action{{At: 0, Pos: 20}, {At: 5000, Pos: 80}},
		Points: []transport.TimedPoint{
			{TimeMillis: 0, PositionPercent: 20},
			{TimeMillis: 5000, PositionPercent: 80},
		},
		DurationMS: 5000,
	}
	if err := player.AppendExtension(extension); err != nil {
		t.Fatalf("AppendExtension: %v", err)
	}

	time.Sleep(2 * time.Second)
	snap := player.Snapshot()
	if !snap.Running {
		t.Fatalf("player stopped after append: playhead=%d duration=%d before=%d/%d", snap.PlayheadMS, snap.DurationMS, before.PlayheadMS, before.DurationMS)
	}
	if snap.DurationMS <= before.DurationMS {
		t.Fatalf("duration = %d, want > %d after extension", snap.DurationMS, before.DurationMS)
	}

	cancel()
}

func TestAppendExtensionOffsetsBeyondLocalPlayhead(t *testing.T) {
	fake := transport.NewFake()
	player := NewPlayer(fake)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	first := Session{
		Actions: []Action{{At: 0, Pos: 10}, {At: 100, Pos: 90}},
		Points: []transport.TimedPoint{
			{TimeMillis: 0, PositionPercent: 10},
			{TimeMillis: 100, PositionPercent: 90},
		},
		DurationMS: 100,
	}
	if err := player.Start(ctx, first, 0); err != nil {
		t.Fatalf("Start: %v", err)
	}

	player.mu.Lock()
	player.baseMS = 50000
	player.startedAt = time.Now().Add(-10 * time.Second)
	player.mu.Unlock()

	extension := Session{
		Actions: []Action{{At: 0, Pos: 20}, {At: 50, Pos: 80}},
		Points: []transport.TimedPoint{
			{TimeMillis: 0, PositionPercent: 20},
			{TimeMillis: 50, PositionPercent: 80},
		},
		DurationMS: 50,
	}
	if err := player.AppendExtension(extension); err != nil {
		t.Fatalf("AppendExtension: %v", err)
	}

	player.mu.Lock()
	last := player.session.Points[len(player.session.Points)-1].TimeMillis
	localPlayhead := player.localPlayheadMSLocked()
	player.mu.Unlock()

	if last < int64(localPlayhead+playerLeadMS) {
		t.Fatalf("last point %d should be beyond local playhead %d + lead", last, localPlayhead)
	}

	cancel()
}

func TestPlayerContinuousDoesNotAutoFinish(t *testing.T) {
	fake := transport.NewFake()
	player := NewPlayer(fake)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := Session{
		Actions: []Action{{At: 0, Pos: 10}, {At: 500, Pos: 90}},
		Points: []transport.TimedPoint{
			{TimeMillis: 0, PositionPercent: 10},
			{TimeMillis: 500, PositionPercent: 90},
		},
		DurationMS: 500,
		Continuous: true,
	}
	if err := player.Start(ctx, session, 0); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(2 * time.Second)
	if !player.Running() {
		t.Fatal("continuous player should stay running after timeline end")
	}

	cancel()
}

func TestTimelineEndMSUsesLastPointBeyondDurationMS(t *testing.T) {
	player := NewPlayer(transport.NewFake())
	player.mu.Lock()
	player.session = Session{
		DurationMS: 30,
		Points: []transport.TimedPoint{
			{TimeMillis: 0, PositionPercent: 50},
			{TimeMillis: 55000, PositionPercent: 60},
		},
	}
	player.mu.Unlock()
	if end := player.TimelineEndMS(); end != 55000 {
		t.Fatalf("TimelineEndMS = %d, want 55000", end)
	}
}
