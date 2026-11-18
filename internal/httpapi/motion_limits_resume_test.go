package httpapi

import (
	"net/http"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestResumeUsesLimitsSavedWhilePaused(t *testing.T) {
	owner := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: owner, MotionTransport: owner})
	t.Cleanup(server.Close)
	callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"flow-full-sweeps","speed_percent":64}`)
	callMotion(t, server, http.MethodPost, "/api/motion/pause", "")
	before := len(owner.Commands())
	callMotion(t, server, http.MethodPost, "/api/motion/quick", `{"stroke_min_percent":30,"stroke_max_percent":65,"speed_max_percent":40,"reverse_direction":true}`)
	if len(owner.Commands()) != before {
		t.Fatal("saving paused limits sent a device command")
	}
	resumed := callMotion(t, server, http.MethodPost, "/api/motion/resume", "")
	commands := owner.Commands()[before:]
	if len(commands) == 0 || commands[0].StrokeWindow == nil {
		t.Fatal("Resume did not set the physical window before sending points")
	}
	window := commands[0].StrokeWindow
	if window.MinPercent != 30 || window.MaxPercent != 65 || !window.ReverseDirection || resumed.Engine.Target.SpeedPercent > 40 {
		t.Fatalf("Resume sent window %+v and speed %d", window, resumed.Engine.Target.SpeedPercent)
	}
}
