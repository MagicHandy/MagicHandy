package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestControllerLeaseMakesExtraClientsReadOnly(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	first := controllerFromState(t, server, "client-a")
	if !first.Active || first.ReadOnly {
		t.Fatalf("first controller = %+v, want active", first)
	}
	second := controllerFromState(t, server, "client-b")
	if second.Active || !second.ReadOnly {
		t.Fatalf("second controller = %+v, want read-only", second)
	}

	recorder := httptest.NewRecorder()
	request := withControllerID(httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":30}`)), "client-b")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("read-only start status = %d, want %d: %s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"read_only":true`) {
		t.Fatalf("read-only response = %s, want controller state", recorder.Body.String())
	}
}

func TestMutatingPathsDoNotAcceptQueryControllerIDs(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	_ = controllerFromState(t, server, "client-a")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/motion/start?client_id=client-a", strings.NewReader(`{"speed_percent":30}`))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("query-authorized start status = %d, want %d: %s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if len(fake.Commands()) != 0 {
		t.Fatalf("query-authorized request reached the transport: %+v", fake.Commands())
	}
}

func TestReadOnlyClientCanStillStopMotion(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	_ = controllerFromState(t, server, "client-a")
	startRecorder := httptest.NewRecorder()
	startRequest := withControllerID(httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":30}`)), "client-a")
	server.Handler().ServeHTTP(startRecorder, startRequest)
	if startRecorder.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d: %s", startRecorder.Code, http.StatusOK, startRecorder.Body.String())
	}
	var started motionEnvelope
	if err := json.Unmarshal(startRecorder.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	if !started.Engine.Running {
		t.Fatalf("motion should be running after active controller start")
	}
	_ = controllerFromState(t, server, "client-b")

	recorder := httptest.NewRecorder()
	request := withControllerID(httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)), "client-b")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("read-only stop status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if commands := fake.Commands(); len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("commands = %+v, want final stop", commands)
	}
}

func TestDispatchOwnerSwitchStopsAndClearsActiveMotion(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":30}`)
	if !started.Engine.Running {
		t.Fatalf("motion should be running before dispatch owner switch")
	}

	current, _ := server.store.Snapshot()
	update := config.SettingsUpdate{
		Server: current.Server,
		Device: config.DeviceUpdate{
			HSPDispatchOwner:       config.DispatchOwnerBrowserBluetooth,
			FirmwareAPIRequirement: current.Device.FirmwareAPIRequirement,
			APIApplicationIDSource: current.Device.APIApplicationIDSource,
		},
		Motion:      current.Motion,
		LLM:         config.LLMUpdateFromSettings(current.LLM),
		Diagnostics: current.Diagnostics,
	}
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal update: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(string(data))))
	request.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("settings status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if engine := server.currentMotionEngine(); engine != nil {
		t.Fatalf("motion engine should be cleared after owner switch, got %+v", engine.Snapshot())
	}
	if commands := fake.Commands(); len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("commands = %+v, want dispatch-owner switch to stop old engine", commands)
	}
}

func controllerFromState(t *testing.T, server *Server, clientID string) controllerSnapshot {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := withControllerID(httptest.NewRequest(http.MethodGet, "/api/state", nil), clientID)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("state status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body struct {
		Controller controllerSnapshot `json:"controller"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	return body.Controller
}
