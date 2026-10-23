package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestEdgeHTTPSharedEngineAndStop(t *testing.T) {
	fake := transport.NewFake()
	s := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, Traces: diagnostics.NewTraceRing(1024)})
	t.Cleanup(s.Close)
	post := func(path string, body string, want int) {
		t.Helper()
		r := personalizationRequest(t, s, http.MethodPost, path, body)
		if r.Code != want {
			t.Fatalf("%s: %d %s", path, r.Code, r.Body.String())
		}
	}
	post("/api/motion/edge", "{}", http.StatusConflict)
	post("/api/motion/start", `{"pattern":"flow-full-sweeps","speed_percent":35}`, http.StatusOK)
	before := s.currentMotionEngine().Snapshot()
	start := time.Now()
	post("/api/motion/edge", "{}", http.StatusOK)
	after := s.currentMotionEngine().Snapshot()
	t.Logf("Edge HTTP acceptance %s; accepted buffer lead %dms", time.Since(start), after.NextSampleMillis-after.RunningMillis)
	if before.StreamID != after.StreamID || !after.Running || after.Target.Source != "edge" || after.Target.SpeedPercent >= before.Target.SpeedPercent {
		t.Fatal("Edge did not retarget the existing stream")
	}
	if counts := countModeCommands(fake.Commands()); counts.plays != 1 || counts.stops != 0 {
		t.Fatalf("hold restarted the stream: %+v", counts)
	}
	post("/api/motion/edge/resume", "{}", http.StatusOK)
	if s.currentMotionEngine().Snapshot().Target.SpeedPercent != 35 {
		t.Fatal("manual resume lost saved target")
	}
	post("/api/motion/edge", "{}", http.StatusOK)
	post("/api/motion/stop", "{}", http.StatusOK)
	post("/api/motion/edge/resume", "{}", http.StatusConflict)
	if s.modes.Status().EdgeHold != nil || s.currentMotionEngine().Snapshot().Running {
		t.Fatal("Stop left an active hold")
	}
	if counts := countModeCommands(fake.Commands()); counts.plays != 1 {
		t.Fatal("return after Stop started playback")
	}
}

func TestEdgeFreestyleKeepsOneStream(t *testing.T) {
	fake := transport.NewFake()
	s := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	t.Cleanup(s.Close)
	if _, err := s.modes.Start(t.Context(), modes.ModeFreestyle); err != nil {
		t.Fatal(err)
	}
	waitForEngineRunning(t, s)
	stream := s.currentMotionEngine().Snapshot().StreamID
	if _, err := s.modes.ActivateEdge(t.Context()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if s.currentMotionEngine().Snapshot().Target.Source != "edge" {
		t.Fatal("Freestyle replaced the hold")
	}
	if _, err := s.modes.ResumeEdge(t.Context()); err != nil {
		t.Fatal(err)
	}
	state := s.currentMotionEngine().Snapshot()
	if state.StreamID != stream || state.Target.Source != modes.ModeFreestyle {
		t.Fatal("return lost the Freestyle stream")
	}
	if counts := countModeCommands(fake.Commands()); counts.plays != 1 || counts.stops != 0 {
		t.Fatalf("private restart: %+v", counts)
	}
}
