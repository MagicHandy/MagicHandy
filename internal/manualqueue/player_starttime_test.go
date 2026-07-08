package manualqueue

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestPlayerPlayAlignsStartTimeWithFirstBufferedPoint(t *testing.T) {
	fake := transport.NewFake()
	player := NewPlayer(fake)

	session := Session{
		Points: []transport.TimedPoint{
			{PositionPercent: 20, TimeMillis: 300},
			{PositionPercent: 80, TimeMillis: 330},
		},
		Actions: []Action{
			{At: 300, Pos: 20},
			{At: 330, Pos: 80},
		},
		DurationMS: 330,
		StrokeMin:  0,
		StrokeMax:  100,
	}

	if err := player.Start(context.Background(), session, 0); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var playStart int64
	for time.Now().Before(deadline) {
		for _, command := range fake.Commands() {
			if command.Kind == transport.CommandKindHSPPlay && command.HSPPlay != nil {
				playStart = command.HSPPlay.StartTimeMillis
				break
			}
		}
		if playStart == 300 {
			player.Stop(context.Background())
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	player.Stop(context.Background())
	t.Fatalf("HSP play start_time = %d, want 300", playStart)
}
