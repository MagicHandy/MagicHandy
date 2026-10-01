package httpapi

import (
	"context"
	"fmt"
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

// TestFreestyleStreamReplansOnTheRealEngineWithoutStallOrBlend is the
// Freestyle starvation and continuity gate: repeated control changes on the
// real engine over the fake transport must keep one continuous stream (one
// HSP play, no stop, uninterrupted point dispatch), and each replacement
// window must join the queued motion without a crossfade bridge.
func TestFreestyleStreamReplansOnTheRealEngineWithoutStallOrBlend(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		// A large ring: engine dispatch rows must not evict the planner rows
		// this test counts.
		Traces:          diagnostics.NewTraceRing(4096),
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	// Re-wire the manager with fast ticks so each control change is applied
	// within a few milliseconds.
	manager, err := modes.NewManager(modes.Options{
		Ensure: func(context.Context) (modes.Engine, error) {
			engine, admission, err := server.motionEngineForStart()
			if err != nil {
				return nil, err
			}
			return admittedMotionEngine{Engine: engine, admission: admission}, nil
		},
		Current: func() modes.Engine {
			engine := server.currentMotionEngine()
			if engine == nil {
				return nil
			}
			return engine
		},
		Settings:          func() config.MotionSettings { s, _ := server.store.Snapshot(); return s.Motion },
		FreestyleSettings: func() config.FreestyleSettings { s, _ := server.store.Snapshot(); return s.Freestyle },
		Traces:            server.traces,
		Tick:              5 * time.Millisecond,
		Seed:              42,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	server.modes = manager
	t.Cleanup(manager.Shutdown)

	if _, err := manager.Start(t.Context(), modes.ModeFreestyle); err != nil {
		t.Fatalf("start freestyle: %v", err)
	}
	waitForEngineRunning(t, server)
	defaults := config.DefaultFreestyleSettings()
	for index, pace := range []int{70, 35, 85, 50} {
		body := fmt.Sprintf(`{"feel":"custom","pace_percent":%d,"length_percent":%d,"focus_percent":%d,"roaming_percent":%d,"variety_percent":%d,"accent":"even","shape":"steady","shape_minutes":15}`,
			pace, defaults.LengthPercent, defaults.FocusPercent, defaults.RoamingPercent, defaults.VarietyPercent)
		if recorder := personalizationRequest(t, server, http.MethodPut, "/api/modes/freestyle/preferences", body); recorder.Code != http.StatusOK {
			t.Fatalf("save preferences = %d: %s", recorder.Code, recorder.Body.String())
		}
		waitForTraceCount(t, server, "freestyle_preferences", index+1)
		time.Sleep(60 * time.Millisecond)
	}

	engine := server.currentMotionEngine()
	if engine == nil || !engine.Snapshot().Running {
		t.Fatal("engine not running after control changes; freestyle stalled")
	}
	counts := countModeCommands(fake.Commands())
	if counts.plays != 1 || counts.stops != 0 || counts.adds < 4 {
		t.Fatalf("commands = %+v, want one play, no stop and continuous dispatch", counts)
	}
	assertFreestyleRetargetsWithoutBlend(t, server, 4)

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

func assertFreestyleRetargetsWithoutBlend(t *testing.T, server *Server, want int) {
	t.Helper()
	retargets := 0
	for _, row := range server.traces.Rows() {
		if row.Retarget != nil && row.Reason == "freestyle_preferences" {
			retargets++
			if row.Retarget.BridgePointsInserted {
				t.Fatalf("a replacement window needed a crossfade: %+v", row.Retarget)
			}
		}
	}
	if retargets != want {
		t.Fatalf("engine retargets = %d, want %d", retargets, want)
	}

}

func waitForEngineRunning(t *testing.T, server *Server) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if engine := server.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("engine did not start")
}

func waitForTraceCount(t *testing.T, server *Server, event string, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		count := 0
		for _, row := range server.traces.Rows() {
			if row.Planner != nil && row.Planner.Event == event {
				count++
			}
		}
		if count >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s rows did not reach %d", event, want)
}

func TestFreestylePreferencesValidateAndKeepNamedFeels(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	// A named feel is authoritative over stray control values.
	recorder := personalizationRequest(t, server, http.MethodPut, "/api/modes/freestyle/preferences",
		`{"feel":"intense","pace_percent":5,"length_percent":5,"focus_percent":5,"roaming_percent":5,"variety_percent":5,"accent":"tip","shape":"edge","shape_minutes":20}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", recorder.Code, recorder.Body.String())
	}
	saved, _ := server.store.Snapshot()
	intense, _ := config.FreestyleFeelPreset(config.FreestyleFeelIntense)
	if saved.Freestyle.PacePercent != intense.PacePercent || saved.Freestyle.Accent != intense.Accent ||
		saved.Freestyle.Shape != config.FreestyleShapeEdge || saved.Freestyle.ShapeMinutes != 20 {
		t.Fatalf("saved = %+v", saved.Freestyle)
	}
	for _, body := range []string{
		`{"feel":"custom","pace_percent":101,"length_percent":50,"focus_percent":50,"roaming_percent":50,"variety_percent":50,"accent":"even","shape":"steady","shape_minutes":15}`,
		`{"feel":"custom","pace_percent":50,"length_percent":50,"focus_percent":50,"roaming_percent":50,"variety_percent":50,"accent":"sideways","shape":"steady","shape_minutes":15}`,
		`{"feel":"custom","pace_percent":50,"length_percent":50,"focus_percent":50,"roaming_percent":50,"variety_percent":50,"accent":"even","shape":"spiral","shape_minutes":15}`,
		`{"feel":"custom","pace_percent":50,"length_percent":50,"focus_percent":50,"roaming_percent":50,"variety_percent":50,"accent":"even","shape":"build","shape_minutes":241}`,
	} {
		if recorder := personalizationRequest(t, server, http.MethodPut, "/api/modes/freestyle/preferences", body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid preferences accepted: %d %s", recorder.Code, body)
		}
	}
	// Read-only clients cannot change them.
	request := withControllerID(httptest.NewRequest(http.MethodPut, "/api/modes/freestyle/preferences",
		strings.NewReader(`{"feel":"gentle","shape":"steady","shape_minutes":15}`)), "reader-b")
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("read-only save = %d, want 409", recorder.Code)
	}
}

type modeCommandCounts struct {
	plays int
	adds  int
	stops int
}

func countModeCommands(commands []transport.Command) modeCommandCounts {
	var counts modeCommandCounts
	for _, command := range commands {
		switch command.Kind {
		case transport.CommandKindPointsPlay:
			counts.plays++
		case transport.CommandKindPointsAdd:
			counts.adds++
		case transport.CommandKindStop:
			counts.stops++
		}
	}
	return counts
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

func TestMotionStopClearsWaitingModeWithoutEngine(t *testing.T) {
	server := newTestServerWithRuntime(t, Runtime{})
	t.Cleanup(server.Close)

	started := personalizationRequest(t, server, http.MethodPost, "/api/modes/start", `{"mode":"freestyle"}`)
	if started.Code != http.StatusOK {
		t.Fatalf("start freestyle = %d: %s", started.Code, started.Body.String())
	}
	if !server.modes.Status().Active {
		t.Fatal("freestyle should be active while waiting for a startable engine")
	}

	stopped := personalizationRequest(t, server, http.MethodPost, "/api/motion/stop", "")
	if stopped.Code != http.StatusOK {
		t.Fatalf("motion stop = %d, want 200 for idempotent safety stop: %s", stopped.Code, stopped.Body.String())
	}
	if server.modes.Status().Active {
		t.Fatal("waiting freestyle mode survived Stop")
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
	model := personalizationRequest(t, server, http.MethodPost, "/api/motion/quick", `{"handy_model":"handy_2_pro"}`)
	if model.Code != http.StatusOK || !strings.Contains(model.Body.String(), `"handy_model":"handy_2_pro"`) {
		t.Fatalf("Handy model update = %d: %s", model.Code, model.Body.String())
	}
	settings, _ = server.store.Snapshot()
	if settings.Motion.HandyModel != config.HandyModel2Pro {
		t.Fatalf("persisted Handy model = %q, want %q", settings.Motion.HandyModel, config.HandyModel2Pro)
	}
	invalid := personalizationRequest(t, server, http.MethodPost, "/api/motion/quick", `{"style":"chaotic"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid style = %d, want 400", invalid.Code)
	}
	invalidModel := personalizationRequest(t, server, http.MethodPost, "/api/motion/quick", `{"handy_model":"overclock"}`)
	if invalidModel.Code != http.StatusBadRequest {
		t.Fatalf("invalid Handy model = %d, want 400", invalidModel.Code)
	}
}
