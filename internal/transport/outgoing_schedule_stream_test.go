package transport

import (
	"testing"
	"time"
)

func TestVisualStreamSnapshotExportsPoints(t *testing.T) {
	schedule := NewOutgoingSchedule()
	now := time.Now()
	schedule.OnAddHSP(HSPAddCommand{
		StreamID: "test",
		Points: []TimedPoint{
			{TimeMillis: 0, PositionPercent: 20},
			{TimeMillis: 500, PositionPercent: 80},
			{TimeMillis: 1000, PositionPercent: 40},
		},
	})
	schedule.OnPlayHSP(HSPPlayCommand{
		StreamID:        "test",
		StartTimeMillis: 0,
	})

	payload := schedule.VisualStreamSnapshot(now, -50)
	if !payload.Active {
		t.Fatal("expected active stream")
	}
	if len(payload.Points) < 2 {
		t.Fatalf("points = %d, want curve samples", len(payload.Points))
	}
}
