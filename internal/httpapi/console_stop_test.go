package httpapi

import (
	"net/http"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// The launch console's Stop key uses the app's emergency-stop path, so the
// device stops even with no browser open.
func TestConsoleStopStopsMotionThroughEmergencyStop(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	t.Cleanup(server.Close)

	_ = callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"flow-full-sweeps","speed_percent":30}`)
	result, err := server.ConsoleStop(t.Context())
	if err != nil || !result.Available || !result.Confirmed {
		t.Fatalf("console stop = %+v, %v; want a confirmed stop", result, err)
	}
	commands := fake.Commands()
	if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("last command = %+v, want stop", commands)
	}
}
