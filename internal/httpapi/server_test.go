package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestHealthzReturnsOK(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body field = %v, want ok", body["status"])
	}
	if body["service"] != serviceName {
		t.Fatalf("service body field = %v, want %s", body["service"], serviceName)
	}
}

func TestStatusAdvertisesPhaseOnePlaceholders(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body struct {
		UI       string            `json:"ui"`
		Features map[string]string `json:"features"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if body.UI != "embedded" {
		t.Fatalf("ui = %q, want embedded", body.UI)
	}
	if body.Features["motion"] != "manual" {
		t.Fatalf("motion feature = %q, want manual", body.Features["motion"])
	}
}

func TestStateReturnsSettingsSnapshotWithoutSecrets(t *testing.T) {
	server := newTestServer(t)
	secret := "secret-connection-key"
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.HandyConnectionKey = secret
		return settings
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); body == "" || contains(body, secret) {
		t.Fatalf("state response leaked secret or was empty: %q", body)
	}

	var body struct {
		Settings struct {
			Device struct {
				ConnectionKeySet bool `json:"connection_key_set"`
			} `json:"device"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode state response: %v", err)
	}
	if !body.Settings.Device.ConnectionKeySet {
		t.Fatal("state did not report that a connection key is configured")
	}
}

func TestTransportDiagnosticsEndpointRedactsSecrets(t *testing.T) {
	server := newTestServer(t)
	_, err := server.transport.(*transport.Fake).Stop(
		t.Context(),
		transport.StopCommand{Reason: "secret-connection-key"},
	)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/transport/diagnostics", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if strings.Contains(recorder.Body.String(), "secret-connection-key") {
		t.Fatalf("transport diagnostics leaked secret: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"kind":"stop"`) {
		t.Fatalf("transport diagnostics missing stop command shape: %s", recorder.Body.String())
	}
}

func TestTraceExportReturnsStableJSON(t *testing.T) {
	server := newTestServer(t)
	result := transport.CommandResult{
		CommandID:     "fake-000001",
		Kind:          transport.CommandKindHSPPlay,
		Transport:     "fake_handy",
		OK:            true,
		Status:        "recorded",
		LatencyMillis: 3,
		CompletedAt:   "2026-06-30T12:00:00Z",
	}
	server.traces.Add(diagnostics.MotionTraceRow{
		Timestamp:       "2026-06-30T12:00:00Z",
		Source:          "test",
		Reason:          "export",
		TransportResult: &result,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	want := `{"schema_version":"motion_trace.v1","rows":[{"sequence":1,"timestamp":"2026-06-30T12:00:00Z","source":"test","reason":"export","transport_result":{"command_id":"fake-000001","kind":"hsp_play","transport":"fake_handy","ok":true,"status":"recorded","latency_ms":3,"completed_at":"2026-06-30T12:00:00Z"}}],"dropped_rows":0}` + "\n"
	if recorder.Body.String() != want {
		t.Fatalf("trace export mismatch\nwant: %s\ngot:  %s", want, recorder.Body.String())
	}
}

func TestSettingsAPIReadsAndSavesSettings(t *testing.T) {
	server := newTestServer(t)
	secret := "new-secret"
	body := `{
		"server": {"port": 49719},
		"device": {
			"hsp_dispatch_owner": "cloud_rest",
			"firmware_api_requirement": "firmware_v4_api_v3_required",
			"api_application_id_source": "developer_override",
			"api_application_id_override": "dev-app-id",
			"handy_connection_key": "new-secret"
		},
		"motion": {
			"speed_min_percent": 15,
			"speed_max_percent": 90,
			"stroke_min_percent": 10,
			"stroke_max_percent": 95,
			"reverse_direction": true
		},
		"llm": {
			"provider": "ollama",
			"llama_cpp_base_url": "http://127.0.0.1:8080/",
			"ollama_base_url": "http://127.0.0.1:11434/",
			"model": "test-model",
			"prompt_set": "magichandy_motion_v1",
			"request_timeout_ms": 45000
		},
		"diagnostics": {"verbosity": "debug"}
	}`

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if contains(recorder.Body.String(), secret) {
		t.Fatal("settings save response leaked the connection key")
	}

	settings, _ := server.store.Snapshot()
	if settings.Server.Port != 49719 {
		t.Fatalf("port = %d, want 49719", settings.Server.Port)
	}
	if settings.Device.HandyConnectionKey != secret {
		t.Fatal("connection key was not saved in the durable settings snapshot")
	}
	if !settings.Motion.ReverseDirection {
		t.Fatal("reverse direction was not saved")
	}
	if settings.LLM.Provider != config.LLMProviderOllama || settings.LLM.Model != "test-model" {
		t.Fatalf("LLM settings were not saved: %+v", settings.LLM)
	}
	if strings.HasSuffix(settings.LLM.OllamaBaseURL, "/") {
		t.Fatalf("Ollama URL should be normalized without trailing slash: %q", settings.LLM.OllamaBaseURL)
	}
}

func TestSettingsAPIRejectsInvalidSettings(t *testing.T) {
	server := newTestServer(t)
	body := `{
		"server": {"port": 49717},
		"device": {
			"hsp_dispatch_owner": "legacy",
			"firmware_api_requirement": "firmware_v4_api_v3_required",
			"api_application_id_source": "bundled_app_id"
		},
		"motion": {
			"speed_min_percent": 20,
			"speed_max_percent": 80,
			"stroke_min_percent": 0,
			"stroke_max_percent": 100,
			"reverse_direction": false
		},
		"diagnostics": {"verbosity": "normal"}
	}`

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestStaticShellIsServed(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q, want text/html; charset=utf-8", got)
	}
}

func TestMissingAssetReturnsNotFound(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/missing.js", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	fake := transport.NewFake()
	return newTestServerWithRuntime(t, Runtime{
		Traces:          diagnostics.NewTraceRing(8),
		Transport:       fake,
		MotionTransport: fake,
	})
}

func newTestServerWithRuntime(t *testing.T, runtime Runtime) *Server {
	t.Helper()

	if runtime.Traces == nil {
		runtime.Traces = diagnostics.NewTraceRing(8)
	}
	if runtime.Transport == nil {
		runtime.Transport = transport.NewFake()
	}
	store, err := config.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	server, err := New(fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>MagicHandy</title>")},
		"app.css":    {Data: []byte("body { margin: 0; }")},
		"app.js":     {Data: []byte("console.log('ready');")},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), store, runtime, VersionInfo{
		Version: "test",
		Commit:  "test",
	})
	if err != nil {
		t.Fatalf("New server: %v", err)
	}
	return server
}

func saveSettings(t *testing.T, store *config.Store, mutate func(config.Settings) config.Settings) {
	t.Helper()

	settings, _ := store.Snapshot()
	if _, err := store.Save(mutate(settings)); err != nil {
		t.Fatalf("Save settings: %v", err)
	}
}

func contains(value string, fragment string) bool {
	return strings.Contains(value, fragment)
}
