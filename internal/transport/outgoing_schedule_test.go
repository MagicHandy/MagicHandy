package transport

import (
	"context"
	"testing"
	"time"
)

func TestOutgoingSchedulePositionAtAppliesSyncOffset(t *testing.T) {
	schedule := NewOutgoingSchedule()
	schedule.OnAddHSP(HSPAddCommand{
		StreamID: "test",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 100, TimeMillis: 1000},
		},
	})
	schedule.OnPlayHSP(HSPPlayCommand{StreamID: "test", StartTimeMillis: 0})

	pos, ok := schedule.PositionAt(time.Now(), -100)
	if !ok {
		t.Fatal("expected active schedule")
	}
	if pos < 0 || pos > 15 {
		t.Fatalf("position with -100ms offset = %v, want near start", pos)
	}
}

func TestRecordingTransportRecordsAddAndPlay(t *testing.T) {
	fake := NewFake()
	schedule := NewOutgoingSchedule()
	recording := NewRecordingTransport(fake, schedule)

	ctx := context.Background()
	_, err := recording.AddHSP(ctx, HSPAddCommand{
		StreamID: "stream-1",
		Points: []TimedPoint{
			{PositionPercent: 20, TimeMillis: 300},
			{PositionPercent: 80, TimeMillis: 600},
		},
	})
	if err != nil {
		t.Fatalf("AddHSP: %v", err)
	}
	_, err = recording.PlayHSP(ctx, HSPPlayCommand{
		StreamID:        "stream-1",
		StartTimeMillis: 300,
	})
	if err != nil {
		t.Fatalf("PlayHSP: %v", err)
	}

	snapshot := schedule.VisualSnapshot(time.Now(), -160)
	if snapshot == nil {
		t.Fatal("expected visual snapshot")
	}
	if snapshot["schedule_active"] != true {
		t.Fatalf("schedule_active = %v", snapshot["schedule_active"])
	}
}

func TestOutgoingScheduleStopClears(t *testing.T) {
	schedule := NewOutgoingSchedule()
	schedule.OnAddHSP(HSPAddCommand{
		StreamID: "test",
		Points:   []TimedPoint{{PositionPercent: 50, TimeMillis: 0}},
	})
	schedule.OnPlayHSP(HSPPlayCommand{StreamID: "test"})
	schedule.OnStop("manual_queue_prepare")
	if schedule.Active() {
		t.Fatal("stop should clear schedule")
	}
}
