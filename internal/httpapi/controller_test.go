package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type blockingControllerStopTransport struct {
	*transport.Fake
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingControllerStopTransport() *blockingControllerStopTransport {
	return &blockingControllerStopTransport{
		Fake:    transport.NewFake(),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (t *blockingControllerStopTransport) Stop(ctx context.Context, command transport.StopCommand) (transport.CommandResult, error) {
	t.once.Do(func() { close(t.started) })
	select {
	case <-t.release:
	case <-ctx.Done():
		return transport.CommandResult{}, ctx.Err()
	}
	return t.Fake.Stop(ctx, command)
}

type failingControllerStopTransport struct {
	*transport.Fake
}

func (t *failingControllerStopTransport) Stop(_ context.Context, _ transport.StopCommand) (transport.CommandResult, error) {
	err := errors.New("test physical stop failed")
	return transport.CommandResult{
		Kind:      transport.CommandKindStop,
		Transport: "failing_test",
		OK:        false,
		Status:    "failed",
		Error:     err.Error(),
	}, err
}

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

func TestControllerTakeoverStopsMotionBeforeTransferringOwnership(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	_ = controllerFromState(t, server, "client-a")
	startRecorder := httptest.NewRecorder()
	startRequest := withControllerID(
		httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":30}`)),
		"client-a",
	)
	server.Handler().ServeHTTP(startRecorder, startRequest)
	if startRecorder.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d: %s", startRecorder.Code, http.StatusOK, startRecorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	request := withControllerID(httptest.NewRequest(http.MethodPost, "/api/controller/takeover", strings.NewReader(`{}`)), "client-b")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("takeover status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response controllerTakeoverResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode takeover: %v", err)
	}
	if !response.Changed || !response.StopConfirmed || response.StopSequence == 0 {
		t.Fatalf("takeover response = %+v, want changed confirmed Stop", response)
	}
	if !response.Controller.Active || response.Controller.ClientID != "client-b" {
		t.Fatalf("takeover controller = %+v, want client-b active", response.Controller)
	}
	if engine := server.currentMotionEngine(); engine == nil || engine.Snapshot().Running {
		t.Fatalf("motion engine = %+v, want installed and stopped", engine)
	}
	if commands := fake.Commands(); len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("commands = %+v, want takeover Stop last", commands)
	}
	if previous := controllerFromState(t, server, "client-a"); previous.Active || !previous.ReadOnly {
		t.Fatalf("previous controller = %+v, want read-only", previous)
	}
	if current := controllerFromState(t, server, "client-b"); !current.Active || current.ReadOnly {
		t.Fatalf("new controller = %+v, want active", current)
	}
}

func TestControllerTakeoverLocksEveryClientWhileStopIsPending(t *testing.T) {
	blocking := newBlockingControllerStopTransport()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       blocking,
		MotionTransport: blocking,
	})
	t.Cleanup(server.Close)

	_ = controllerFromState(t, server, "client-a")
	startRecorder := httptest.NewRecorder()
	startRequest := withControllerID(
		httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":30}`)),
		"client-a",
	)
	server.Handler().ServeHTTP(startRecorder, startRequest)
	if startRecorder.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d: %s", startRecorder.Code, http.StatusOK, startRecorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	request := withControllerID(httptest.NewRequest(http.MethodPost, "/api/controller/takeover", strings.NewReader(`{}`)), "client-b")
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Handler().ServeHTTP(recorder, request)
	}()
	<-blocking.started

	for _, clientID := range []string{"client-a", "client-b", "client-c"} {
		snapshot := controllerFromState(t, server, clientID)
		if snapshot.Active || !snapshot.ReadOnly || !snapshot.TakeoverInProgress {
			t.Fatalf("%s during takeover = %+v, want locked handoff", clientID, snapshot)
		}
	}
	rejected := httptest.NewRecorder()
	rejectedRequest := withControllerID(
		httptest.NewRequest(http.MethodPost, "/api/motion/quick", strings.NewReader(`{"speed_max_percent":25}`)),
		"client-a",
	)
	server.Handler().ServeHTTP(rejected, rejectedRequest)
	if rejected.Code != http.StatusConflict {
		t.Fatalf("old controller during takeover status = %d, want %d: %s", rejected.Code, http.StatusConflict, rejected.Body.String())
	}

	close(blocking.release)
	<-done
	if recorder.Code != http.StatusOK {
		t.Fatalf("takeover status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if current := controllerFromState(t, server, "client-b"); !current.Active || current.TakeoverInProgress {
		t.Fatalf("completed takeover = %+v, want client-b active", current)
	}
}

func TestControllerTakeoverTransfersAfterUnconfirmedPhysicalStop(t *testing.T) {
	failing := &failingControllerStopTransport{Fake: transport.NewFake()}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       failing,
		MotionTransport: failing,
	})
	t.Cleanup(server.Close)

	_ = controllerFromState(t, server, "client-a")
	startRecorder := httptest.NewRecorder()
	startRequest := withControllerID(
		httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":30}`)),
		"client-a",
	)
	server.Handler().ServeHTTP(startRecorder, startRequest)
	if startRecorder.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d: %s", startRecorder.Code, http.StatusOK, startRecorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	request := withControllerID(httptest.NewRequest(http.MethodPost, "/api/controller/takeover", strings.NewReader(`{}`)), "client-b")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("takeover status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response controllerTakeoverResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode takeover: %v", err)
	}
	if response.StopConfirmed || response.Warning == "" {
		t.Fatalf("takeover response = %+v, want unconfirmed Stop warning", response)
	}
	if !response.Controller.Active || response.Controller.ClientID != "client-b" {
		t.Fatalf("takeover controller = %+v, want client-b active", response.Controller)
	}
	if engine := server.currentMotionEngine(); engine == nil || engine.Snapshot().Running {
		t.Fatalf("motion engine = %+v, want locally stopped despite transport failure", engine)
	}
}

func TestControllerTakeoverRequiresHeaderAndIsIdempotentForOwner(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	_ = controllerFromState(t, server, "client-a")
	for _, target := range []string{"/api/controller/takeover", "/api/controller/takeover?client_id=client-b"} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{}`)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want %d: %s", target, recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
	}

	recorder := httptest.NewRecorder()
	request := withControllerID(httptest.NewRequest(http.MethodPost, "/api/controller/takeover", strings.NewReader(`{}`)), "client-a")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("owner takeover status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response controllerTakeoverResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode owner takeover: %v", err)
	}
	if response.Changed || !response.StopConfirmed || !response.Controller.Active {
		t.Fatalf("owner takeover response = %+v, want unchanged active owner", response)
	}
	if commands := fake.Commands(); len(commands) != 0 {
		t.Fatalf("idempotent owner takeover commands = %+v, want none", commands)
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
