package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// TestFreestyleDrivesRealEngineAcrossSegmentsWithoutStall is the Phase 11
// starvation gate: many segment boundaries on the real engine over the fake
// transport must keep one continuous stream — exactly one HSP play, no
// restart, and uninterrupted point dispatch.
func TestFreestyleDrivesRealEngineAcrossSegmentsWithoutStall(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		// A large ring: engine dispatch rows must not evict the planner rows
		// this test counts.
		Traces:          diagnostics.NewTraceRing(2048),
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	// Re-wire the manager with test pacing: fast ticks and 80ms segment caps
	// so the run crosses many boundaries in under a second.
	manager, err := modes.NewManager(modes.Options{
		Ensure: func(context.Context) (modes.Engine, error) {
			return server.motionEngineForStart()
		},
		Current: func() modes.Engine {
			engine := server.currentMotionEngine()
			if engine == nil {
				return nil
			}
			return engine
		},
		Settings:           func() config.MotionSettings { s, _ := server.store.Snapshot(); return s.Motion },
		Traces:             server.traces,
		Tick:               5 * time.Millisecond,
		Seed:               42,
		MaxSegmentDuration: 80 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	server.modes = manager
	t.Cleanup(manager.Shutdown)

	if _, err := manager.Start(t.Context(), modes.ModeFreestyle); err != nil {
		t.Fatalf("start freestyle: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if segmentTraceCount(server) >= 4 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := segmentTraceCount(server); got < 4 {
		for _, row := range server.traces.Rows() {
			if row.Planner != nil {
				t.Logf("planner row: event=%s note=%s", row.Planner.Event, row.Planner.Note)
			}
		}
		t.Fatalf("segment boundaries crossed = %d, want >= 4", got)
	}

	engine := server.currentMotionEngine()
	if engine == nil || !engine.Snapshot().Running {
		t.Fatal("engine not running after segment boundaries — freestyle stalled")
	}

	commands := fake.Commands()
	plays, adds, stops := 0, 0, 0
	for _, command := range commands {
		switch command.Kind {
		case transport.CommandKindHSPPlay:
			plays++
		case transport.CommandKindHSPAdd:
			adds++
		case transport.CommandKindStop:
			stops++
		}
	}
	if plays != 1 {
		t.Fatalf("HSP play commands = %d, want exactly 1 (boundaries must not restart the stream)", plays)
	}
	if stops != 0 {
		t.Fatalf("stop commands during freestyle = %d, want 0", stops)
	}
	if adds < 4 {
		t.Fatalf("HSP add chunks = %d, want continuous dispatch", adds)
	}

	// Mode stop ends planning and stops motion.
	recorder := personalizationRequest(t, server, http.MethodPost, "/api/modes/stop", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("mode stop = %d: %s", recorder.Code, recorder.Body.String())
	}
	if server.modes.Status().Active {
		t.Fatal("mode still active after stop")
	}
	if engine := server.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
		t.Fatal("motion still running after mode stop")
	}
}

func segmentTraceCount(server *Server) int {
	count := 0
	for _, row := range server.traces.Rows() {
		if row.Planner != nil && (row.Planner.Event == "freestyle_segment" || row.Planner.Event == "freestyle_start") {
			count++
		}
	}
	return count
}

func TestModesEndpointsAndControllerGating(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	// State exposes idle mode status.
	state := personalizationRequest(t, server, http.MethodGet, "/api/modes", "")
	if state.Code != http.StatusOK || !strings.Contains(state.Body.String(), `"active":false`) {
		t.Fatalf("modes state = %d: %s", state.Code, state.Body.String())
	}

	started := personalizationRequest(t, server, http.MethodPost, "/api/modes/start", `{"mode":"chat"}`)
	if started.Code != http.StatusOK || !strings.Contains(started.Body.String(), `"mode":"chat"`) {
		t.Fatalf("start chat mode = %d: %s", started.Code, started.Body.String())
	}
	unknown := personalizationRequest(t, server, http.MethodPost, "/api/modes/start", `{"mode":"story"}`)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown mode = %d, want 400", unknown.Code)
	}

	// Read-only clients cannot start or stop modes.
	recorder := httptest.NewRecorder()
	request := withControllerID(httptest.NewRequest(http.MethodPost, "/api/modes/start",
		strings.NewReader(`{"mode":"freestyle"}`)), "reader-b")
	request.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("read-only mode start = %d, want 409", recorder.Code)
	}

	stopped := personalizationRequest(t, server, http.MethodPost, "/api/modes/stop", "")
	if stopped.Code != http.StatusOK || !strings.Contains(stopped.Body.String(), `"active":false`) {
		t.Fatalf("mode stop = %d: %s", stopped.Code, stopped.Body.String())
	}
}

func TestUserStopEndsFreestyleThroughMotionStop(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	t.Cleanup(server.Close)

	started := personalizationRequest(t, server, http.MethodPost, "/api/modes/start", `{"mode":"freestyle"}`)
	if started.Code != http.StatusOK {
		t.Fatalf("start freestyle = %d: %s", started.Code, started.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if engine := server.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// "Stop everything" (the same endpoint the UI and Esc use) must end the
	// mode and motion, and nothing may restart afterwards.
	stopped := personalizationRequest(t, server, http.MethodPost, "/api/motion/stop", "")
	if stopped.Code != http.StatusOK {
		t.Fatalf("motion stop = %d: %s", stopped.Code, stopped.Body.String())
	}
	if server.modes.Status().Active {
		t.Fatal("freestyle survived the user stop")
	}
	time.Sleep(50 * time.Millisecond)
	if engine := server.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
		t.Fatal("motion restarted after user stop")
	}
}

func TestQuickEndpointPersistsMotionStyle(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	updated := personalizationRequest(t, server, http.MethodPost, "/api/motion/quick", `{"style":"intense"}`)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"style":"intense"`) {
		t.Fatalf("style update = %d: %s", updated.Code, updated.Body.String())
	}
	settings, _ := server.store.Snapshot()
	if settings.Motion.Style != "intense" {
		t.Fatalf("persisted style = %q, want intense", settings.Motion.Style)
	}
	invalid := personalizationRequest(t, server, http.MethodPost, "/api/motion/quick", `{"style":"chaotic"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid style = %d, want 400", invalid.Code)
	}
}
