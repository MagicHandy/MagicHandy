package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestCancelledMediaArmDoesNotStopCurrentMotionOrConsumeSequence(t *testing.T) {
	server := newTestServer(t)
	if started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":30}`); !started.Engine.Running {
		t.Fatal("pattern motion did not start")
	}
	fake := server.transport.(*transport.Fake)
	before := countTransportCommands(fake.Commands(), transport.CommandKindStop)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	event := mediaSyncEvent{VideoID: "cancelled", SessionID: "cancelled-session", EventSequence: 1, State: "playing", Event: "seeked", PlaybackRate: 1}
	_, err := server.mediaSync.Handle(ctx, event, server.stopSequence.Load())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want cancellation", err)
	}
	if !server.currentMotionEngine().Snapshot().Running {
		t.Fatal("cancelled seek stopped the current motion source")
	}
	if got := countTransportCommands(fake.Commands(), transport.CommandKindStop); got != before {
		t.Fatalf("cancelled seek issued %d extra Stops", got-before)
	}
	if !server.mediaSync.acceptEvent(event) {
		t.Fatal("cancelled seek consumed the session sequence")
	}
}
