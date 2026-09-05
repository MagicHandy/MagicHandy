package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func newEnabledLabServer(t *testing.T, runtime Runtime) *Server {
	t.Helper()
	server := newTestServerWithRuntime(t, runtime)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings { settings.Labs.Enabled = true; return settings })
	return server
}

func setLabsForTest(t *testing.T, server *Server, enabled bool) {
	t.Helper()
	response := labObservationRequest(t, server, http.MethodPut, "/api/settings/labs", map[string]any{"enabled": enabled}, true)
	if response.Code != http.StatusOK {
		t.Fatalf("set Labs: %d %s", response.Code, response.Body)
	}
}

func TestLabsSettingEnablesReleaseEndpointsAndPersists(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	before, _ := server.store.Snapshot()
	if before.Labs.Enabled {
		t.Fatal("Labs must default off")
	}
	setLabsForTest(t, server, true)
	after, _ := server.store.Snapshot()
	if !after.Labs.Enabled || after.Motion != before.Motion || len(fake.Commands()) != 0 {
		t.Fatal("enable changed motion or was not saved")
	}
	response := labObservationRequest(t, server, http.MethodGet, "/api/labs/llm", nil, false)
	if response.Code != http.StatusOK {
		t.Fatalf("release lab endpoint unavailable: %s", response.Body)
	}
	response = labObservationRequest(t, server, http.MethodGet, "/api/state", nil, true)
	if !strings.Contains(response.Body.String(), `"labs_enabled":true`) {
		t.Fatal("state omitted the saved flag")
	}
	// A settings client that predates Labs must not turn it off on an unrelated Save.
	encoded, _ := json.Marshal(after.Public())
	var update config.SettingsUpdate
	if err := json.Unmarshal(encoded, &update); err != nil {
		t.Fatal(err)
	}
	update.Labs = nil
	response = labObservationRequest(t, server, http.MethodPut, "/api/settings", update, true)
	if response.Code != http.StatusOK {
		t.Fatalf("ordinary settings save: %s", response.Body)
	}
	if saved, _ := server.store.Snapshot(); !saved.Labs.Enabled {
		t.Fatal("an unrelated settings save disabled Labs")
	}
	dataDir := server.store.DataDir()
	server.Close()
	store, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	server = newTestServerWithStore(t, store, Runtime{})
	response = labObservationRequest(t, server, http.MethodGet, "/api/labs/observations", nil, false)
	if response.Code != http.StatusOK {
		t.Fatal("Labs did not stay enabled after restart")
	}
	setLabsForTest(t, server, false)
	response = labObservationRequest(t, server, http.MethodGet, "/api/labs/llm", nil, false)
	if response.Code != http.StatusForbidden {
		t.Fatal("disabled Labs accepted a request")
	}
}

func TestLabsSettingRejectsMissingValuesAndReaderWrites(t *testing.T) {
	server := newTestServer(t)
	setLabsForTest(t, server, false)
	response := labObservationRequest(t, server, http.MethodPut, "/api/settings/labs", map[string]any{"enabled": true}, false)
	if response.Code < 400 {
		t.Fatal("reader enabled Labs")
	}
	response = labObservationRequest(t, server, http.MethodPut, "/api/settings/labs", map[string]any{}, true)
	if response.Code != http.StatusBadRequest {
		t.Fatal("missing enabled value accepted")
	}
}

func TestDisablingLabsStopsAuditionAndPreservesObservations(t *testing.T) {
	fake := transport.NewFake()
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake})
	body := motionObservationBody(server, "Review evidence survives disabling")
	response := labObservationRequest(t, server, http.MethodPost, "/api/labs/observations", body, true)
	if response.Code != http.StatusOK {
		t.Fatalf("save: %s", response.Body)
	}
	start := motionRequest{Lab: &motion.LabStart{Method: "flow", Flow: body.Spec, SettingsKey: body.SettingsKey}}
	response = labObservationRequest(t, server, http.MethodPost, "/api/motion/start", start, true)
	if response.Code != http.StatusOK {
		t.Fatalf("start: %s", response.Body)
	}
	engine := server.currentMotionEngine()
	setLabsForTest(t, server, false)
	if engine.Snapshot().Running || server.currentMotionEngine() != nil {
		t.Fatal("lab audition survived disable")
	}
	count := len(fake.Commands())
	time.Sleep(150 * time.Millisecond)
	if len(fake.Commands()) != count {
		t.Fatal("lab motion dispatched after disable completed")
	}
	response = labObservationRequest(t, server, http.MethodPost, "/api/motion/start", start, true)
	if response.Code != http.StatusForbidden || len(fake.Commands()) != count {
		t.Fatal("disabled audition reached the engine")
	}
	setLabsForTest(t, server, true)
	rows, err := server.readLabObservations(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatal("disabling Labs deleted observations")
	}
}

func TestDisablingLabsDoesNotStopRegularMotion(t *testing.T) {
	fake := transport.NewFake()
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake})
	response := labObservationRequest(t, server, http.MethodPost, "/api/motion/start", motionRequest{SpeedPercent: 30}, true)
	if response.Code != http.StatusOK {
		t.Fatalf("manual start: %s", response.Body)
	}
	engine := server.currentMotionEngine()
	setLabsForTest(t, server, false)
	if !engine.Snapshot().Running || server.currentMotionEngine() != engine {
		t.Fatal("disabling Labs stopped ordinary motion")
	}
}

func TestDisablingLabsCancelsLateReplyEvenAfterReenable(t *testing.T) {
	provider := &stubbornLLMProvider{started: make(chan struct{}), release: make(chan struct{})}
	server := newEnabledLabServer(t, Runtime{LLMProvider: provider})
	result := make(chan int, 1)
	go func() {
		response := labObservationRequest(t, server, http.MethodPost, "/api/labs/llm/chat", map[string]any{"message": "Change the range", "method": "controls", "revision": 0}, true)
		result <- response.Code
	}()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	setLabsForTest(t, server, false)
	setLabsForTest(t, server, true)
	close(provider.release)
	select {
	case status := <-result:
		if status != http.StatusConflict {
			t.Fatalf("late response: %d", status)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not finish")
	}
	if state := server.labState(); len(state.Turns) != 0 || state.Busy {
		t.Fatal("late reply changed the reenabled workspace")
	}
}

func TestDisablingLabsCancelsAndDrainsAnAuditionStart(t *testing.T) {
	fake := &blockingModeStartTransport{Fake: transport.NewFake(), entered: make(chan struct{})}
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake})
	state := server.labState()
	result := make(chan int, 1)
	go func() {
		response := labObservationRequest(t, server, http.MethodPost, "/api/motion/start", motionRequest{Lab: &motion.LabStart{Method: "flow", Flow: &state.Current, SettingsKey: state.SettingsKey}}, true)
		result <- response.Code
	}()
	select {
	case <-fake.entered:
	case <-time.After(time.Second):
		t.Fatal("audition startup did not reach transport")
	}
	setLabsForTest(t, server, false)
	select {
	case code := <-result:
		if code < 400 {
			t.Fatal("canceled audition succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("disable did not drain startup")
	}
	if engine := server.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
		t.Fatal("canceled startup left a running engine")
	}
	count := len(fake.Commands())
	time.Sleep(100 * time.Millisecond)
	if len(fake.Commands()) != count {
		t.Fatal("startup dispatched after disable returned")
	}
}
