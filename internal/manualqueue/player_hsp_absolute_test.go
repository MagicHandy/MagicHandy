package manualqueue

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestHSPAbsoluteBatch(t *testing.T) {
	batch := []transport.TimedPoint{
		{TimeMillis: 0, PositionPercent: 10},
		{TimeMillis: 5000, PositionPercent: 90},
	}
	got := hspAbsoluteBatch(30000, batch)
	if got[0].TimeMillis != 30000 || got[1].TimeMillis != 35000 {
		t.Fatalf("absolute batch = %+v, want 30000 and 35000", got)
	}
	if got[0].PositionPercent != 10 || got[1].PositionPercent != 90 {
		t.Fatalf("positions changed: %+v", got)
	}
}

func TestHSPAbsoluteBatchZeroBasePassthrough(t *testing.T) {
	batch := []transport.TimedPoint{{TimeMillis: 42, PositionPercent: 50}}
	got := hspAbsoluteBatch(0, batch)
	if got[0].TimeMillis != 42 {
		t.Fatalf("expected passthrough, got %+v", got)
	}
}
