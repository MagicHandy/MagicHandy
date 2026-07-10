package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestMotionVisualReflectsOutgoingSchedule(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	commandTransport, err := server.newSelectedMotionTransport()
	if err != nil {
		t.Fatalf("newSelectedMotionTransport: %v", err)
	}
	ctx := context.Background()
	_, err = commandTransport.AddHSP(ctx, transport.HSPAddCommand{
		StreamID: "visual-test",
		Points: []transport.TimedPoint{
			{PositionPercent: 10, TimeMillis: 0},
			{PositionPercent: 90, TimeMillis: 1000},
		},
	})
	if err != nil {
		t.Fatalf("AddHSP: %v", err)
	}
	_, err = commandTransport.PlayHSP(ctx, transport.HSPPlayCommand{
		StreamID:        "visual-test",
		StartTimeMillis: 0,
	})
	if err != nil {
		t.Fatalf("PlayHSP: %v", err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/motion/visual", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET visual status = %d", rec.Code)
	}
	var visual map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &visual); err != nil {
		t.Fatalf("decode visual: %v", err)
	}
	if visual["schedule_active"] != true {
		t.Fatalf("schedule_active = %v", visual["schedule_active"])
	}
	if visual["playback_active"] != true {
		t.Fatalf("playback_active = %v", visual["playback_active"])
	}
}
