package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type motionEnvelope struct {
	Available bool                     `json:"available"`
	Error     string                   `json:"error"`
	Engine    motion.ActiveMotionState `json:"engine"`
}

func callMotion(t *testing.T, server *Server, method, path, body string) motionEnvelope {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	request := httptest.NewRequest(method, path, reader)
	request = withController(request)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s status = %d, body = %s", method, path, recorder.Code, recorder.Body.String())
	}
	var envelope motionEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s %s: %v", method, path, err)
	}
	return envelope
}

func TestMotionStartStateStop(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"stroke","speed_percent":60}`)
	if !started.Available || !started.Engine.Running {
		t.Fatalf("expected running motion after start, got %+v", started)
	}
	if started.Engine.Target.SpeedPercent != 60 {
		t.Fatalf("target speed = %d, want 60", started.Engine.Target.SpeedPercent)
	}

	state := callMotion(t, server, http.MethodGet, "/api/motion/state", "")
	if !state.Engine.Running {
		t.Fatalf("state should report running: %+v", state)
	}

	stopped := callMotion(t, server, http.MethodPost, "/api/motion/stop", `{}`)
	if stopped.Engine.Running {
		t.Fatalf("motion should be stopped, got %+v", stopped)
	}
}

func TestMotionStateReportsPausedEngine(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"stroke","speed_percent":60}`)
	if !started.Engine.Running {
		t.Fatalf("expected running motion after start, got %+v", started)
	}
	paused := callMotion(t, server, http.MethodPost, "/api/motion/pause", `{}`)
	if paused.Engine.Running || !paused.Engine.Paused {
		t.Fatalf("expected paused motion, got %+v", paused)
	}

	state := callMotion(t, server, http.MethodGet, "/api/motion/state", "")
	if !state.Available || state.Engine.Running || !state.Engine.Paused {
		t.Fatalf("state should expose paused engine for Resume UI: %+v", state)
	}
}

func TestMotionStartClampsSpeedToSettings(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.SpeedMinPercent = 10
		settings.Motion.SpeedMaxPercent = 40
		return settings
	})

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":95}`)
	if started.Engine.Target.SpeedPercent != 40 {
		t.Fatalf("speed should clamp to max 40, got %d", started.Engine.Target.SpeedPercent)
	}
}

func TestMotionUnavailableWithoutSelectedTransportPrerequisites(t *testing.T) {
	server := newTestServerWithRuntime(t, Runtime{})
	t.Cleanup(server.Close)

	stateRecorder := httptest.NewRecorder()
	stateRequest := httptest.NewRequest(http.MethodGet, "/api/motion/state", nil)
	server.Handler().ServeHTTP(stateRecorder, stateRequest)
	if stateRecorder.Code != http.StatusOK {
		t.Fatalf("state status = %d, want %d", stateRecorder.Code, http.StatusOK)
	}
	var state motionEnvelope
	if err := json.Unmarshal(stateRecorder.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode motion state: %v", err)
	}
	if state.Available {
		t.Fatalf("motion state should be unavailable without Intiface: %s", stateRecorder.Body.String())
	}
	if !strings.Contains(stateRecorder.Body.String(), "Intiface client is unavailable") {
		t.Fatalf("state body = %s, want Intiface error", stateRecorder.Body.String())
	}

	startRecorder := httptest.NewRecorder()
	startRequest := withController(httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":35}`)))
	server.Handler().ServeHTTP(startRecorder, startRequest)
	if startRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("start status = %d, want %d: %s", startRecorder.Code, http.StatusServiceUnavailable, startRecorder.Body.String())
	}
	if strings.Contains(startRecorder.Body.String(), "fake_handy") {
		t.Fatalf("motion start should not fall back to fake transport: %s", startRecorder.Body.String())
	}
}

func TestMotionStartUsesSelectedCloudTransport(t *testing.T) {
	requests := make(chan capturedCloudRequest, 16)
	cloudServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- captureCloudRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"hsp_available":true,"playback_state":"playing"}`))
	}))
	defer cloudServer.Close()

	server := newTestServerWithRuntime(t, Runtime{CloudBaseURL: cloudServer.URL})
	t.Cleanup(server.Close)
	saveCloudSettings(t, server)

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"stroke","speed_percent":35}`)
	if !started.Available || !started.Engine.Running {
		t.Fatalf("expected Cloud-backed motion to run, got %+v", started)
	}
	if started.Engine.LastResult == nil || started.Engine.LastResult.Transport == "fake_handy" {
		t.Fatalf("motion result used fake transport: %+v", started.Engine.LastResult)
	}

	wantRequests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPut, path: "/slider/stroke"},
		{method: http.MethodPut, path: "/hsp/setup"},
		{method: http.MethodPut, path: "/hsp/add"},
		{method: http.MethodGet, path: "/servertime"},
		{method: http.MethodPut, path: "/hsp/play"},
	}
	for _, want := range wantRequests {
		seen := readCapturedCloudRequest(t, requests)
		if seen.Method != want.method || seen.Path != want.path {
			t.Fatalf("request = %+v, want %s %s", seen, want.method, want.path)
		}
		if seen.Path != "/servertime" && (seen.ApplicationID != "dev-app-id" || seen.ConnectionKey != cloudTestConnectionKey) {
			t.Fatalf("auth headers = %+v, want settings-derived credentials", seen)
		}
	}
}

func TestMotionRefreshAppliesToActiveLoopImmediately(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":70}`)
	if !started.Engine.Running {
		t.Fatalf("expected running motion")
	}

	tighter := config.MotionSettings{SpeedMinPercent: 10, SpeedMaxPercent: 25, StrokeMinPercent: 0, StrokeMaxPercent: 100}
	server.refreshActiveMotion(context.Background(), tighter)

	snapshot := server.motion.engine.Snapshot()
	if !snapshot.Running {
		t.Fatalf("refresh must not stop active motion")
	}
	if snapshot.Settings.SpeedMaxPercent != 25 {
		t.Fatalf("settings not applied: max = %d, want 25", snapshot.Settings.SpeedMaxPercent)
	}
	if snapshot.Target.SpeedPercent > 25 {
		t.Fatalf("target speed %d should be reclamped to <= 25", snapshot.Target.SpeedPercent)
	}
}

func TestMotionStateExposedInAggregateState(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	request := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	var body struct {
		Motion struct {
			Available bool `json:"available"`
		} `json:"motion"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if !body.Motion.Available {
		t.Fatalf("state should report motion available with a command transport")
	}
}

func TestMotionEventsStreamsMotionState(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/api/motion/events?client_id=events-client", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("motion events request: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", got)
	}

	reader := bufio.NewReader(response.Body)
	var block strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE line: %v", err)
		}
		if line == "\n" || line == "\r\n" {
			break
		}
		block.WriteString(line)
	}
	if !strings.Contains(block.String(), "event: motion") || !strings.Contains(block.String(), `"available":true`) {
		t.Fatalf("SSE block = %q, want motion availability event", block.String())
	}
}
