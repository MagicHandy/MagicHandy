package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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

func TestRawTransportMotionRoutesAreNotExposed(t *testing.T) {
	server := newTestServer(t)
	paths := []string{
		"/api/transport/cloud/stroke-window",
		"/api/transport/cloud/hsp-add",
		"/api/transport/cloud/hsp-play",
		"/api/transport/bluetooth/stroke-window",
		"/api/transport/bluetooth/hsp-add",
		"/api/transport/bluetooth/hsp-play",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := withController(httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
			server.Handler().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want method-not-allowed for removed raw transport motion route", recorder.Code)
			}
		})
	}
}

func TestBrowserBoundaryRequiresLoopbackSameOrigin(t *testing.T) {
	server := newTestServer(t)

	for name, request := range map[string]*http.Request{
		"cross-site": func() *http.Request {
			request := httptest.NewRequest(http.MethodGet, "/api/state?client_id=attacker", nil)
			request.Host = "127.0.0.1:49717"
			request.Header.Set("Sec-Fetch-Site", "cross-site")
			return request
		}(),
		"dns-rebinding": func() *http.Request {
			request := httptest.NewRequest(http.MethodGet, "/api/state?client_id=attacker", nil)
			request.Host = "attacker.example"
			request.Header.Set("Origin", "http://attacker.example")
			request.Header.Set("Sec-Fetch-Site", "same-origin")
			return request
		}(),
		"scheme-mismatch": func() *http.Request {
			request := httptest.NewRequest(http.MethodGet, "/api/state?client_id=attacker", nil)
			request.Host = "127.0.0.1:49717"
			request.Header.Set("Origin", "https://127.0.0.1:49717")
			request.Header.Set("Sec-Fetch-Site", "same-origin")
			return request
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
		})
	}

	request := withControllerID(httptest.NewRequest(http.MethodGet, "/api/state", nil), "local-client")
	request.Host = "127.0.0.1:49717"
	request.Header.Set("Origin", "http://127.0.0.1:49717")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("same-origin status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestWriteJSONFallsBackWithoutPanicking(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, http.StatusOK, map[string]any{"unsupported": make(chan int)})

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if got := recorder.Body.String(); got != "{\"error\":\"response could not be encoded\"}\n" {
		t.Fatalf("fallback body = %q", got)
	}
}

func TestJSONDecodersRejectOversizedBodies(t *testing.T) {
	body := `{"value":"` + strings.Repeat("x", maxJSONBodyBytes) + `"}`
	for name, decode := range map[string]func(*http.Request, any) error{
		"required": decodeJSON,
		"optional": decodeOptionalJSON,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			var target struct {
				Value string `json:"value"`
			}
			err := decode(request, &target)
			if err == nil || !strings.Contains(err.Error(), "body exceeds") {
				t.Fatalf("oversized body error = %v", err)
			}
		})
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

func TestStateReportsDatastoreRecoveryWithoutLeakingPreservedBytes(t *testing.T) {
	dir := t.TempDir()
	marker := "preserved-private-datastore-marker"
	if err := os.WriteFile(filepath.Join(dir, "magichandy.db"), []byte(marker), 0o600); err != nil {
		t.Fatalf("write corrupt datastore: %v", err)
	}
	settingsStore, err := config.OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	server := newTestServerWithStore(t, settingsStore, Runtime{})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"recovered":true`) || !strings.Contains(body, `"datastore_recovered_path":`) {
		t.Fatalf("state did not report datastore recovery: %s", body)
	}
	if strings.Contains(body, marker) {
		t.Fatalf("state leaked preserved datastore contents: %s", body)
	}
}

func TestBorrowedLogicalStoresDoNotCloseProcessDatastore(t *testing.T) {
	server := newTestServer(t)
	database := server.store.Datastore().SQL()
	borrowers := []struct {
		name  string
		close func() error
	}{
		{name: "memory", close: server.personalization.memory.Close},
		{name: "prompt sets", close: server.personalization.prompts.Close},
		{name: "chat log", close: server.chatLog.Close},
		{name: "patterns", close: server.patterns.Close},
		{name: "media catalog", close: server.media.Close},
		{name: "model inventory", close: server.models.Close},
	}
	for _, borrower := range borrowers {
		if err := borrower.close(); err != nil {
			t.Fatalf("close borrowed %s store: %v", borrower.name, err)
		}
		if err := database.Ping(); err != nil {
			t.Fatalf("borrowed %s store closed process datastore: %v", borrower.name, err)
		}
	}

	server.Close()
	if err := database.Ping(); err == nil {
		t.Fatal("server owner close left process datastore open")
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
		Kind:          transport.CommandKindPointsPlay,
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
	want := `{"schema_version":"motion_trace.v3","rows":[{"sequence":1,"timestamp":"2026-06-30T12:00:00Z","source":"test","reason":"export","transport_result":{"command_id":"fake-000001","kind":"points_play","transport":"fake_handy","ok":true,"status":"recorded","latency_ms":3,"completed_at":"2026-06-30T12:00:00Z"}}],"dropped_rows":0,"intiface_dispatches_dropped":0,"intiface_linear_sent_count":0}` + "\n"
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
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body)))
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
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body)))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestConnectionKeyAPISavesOnlyRedactedCredential(t *testing.T) {
	server := newTestServer(t)
	before, _ := server.store.Snapshot()
	const secret = "connection-secret"

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings/device/connection-key", strings.NewReader(`{"connection_key":"  connection-secret  "}`)))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if contains(recorder.Body.String(), secret) {
		t.Fatal("connection key save response leaked the credential")
	}
	if !contains(recorder.Body.String(), `"connection_key_set":true`) {
		t.Fatalf("response did not confirm the redacted key state: %s", recorder.Body.String())
	}

	after, _ := server.store.Snapshot()
	if after.Device.HandyConnectionKey != secret {
		t.Fatal("connection key was not saved")
	}
	before.Device.HandyConnectionKey = secret
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("scoped connection key update changed unrelated settings\nbefore: %+v\nafter:  %+v", before, after)
	}
}

func TestConnectionKeyAPIRequiresControllerAndNonEmptyKey(t *testing.T) {
	server := newTestServer(t)

	for name, testCase := range map[string]struct {
		request    *http.Request
		wantStatus int
	}{
		"read only": {request: httptest.NewRequest(http.MethodPut, "/api/settings/device/connection-key", strings.NewReader(`{"connection_key":"secret"}`)), wantStatus: http.StatusConflict},
		"empty":     {request: withController(httptest.NewRequest(http.MethodPut, "/api/settings/device/connection-key", strings.NewReader(`{"connection_key":"  "}`))), wantStatus: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, testCase.request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, testCase.wantStatus, recorder.Body.String())
			}
		})
	}

	settings, _ := server.store.Snapshot()
	if settings.Device.HandyConnectionKey != "" {
		t.Fatal("rejected request changed the connection key")
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

func TestServerStartupPrunesMediaOutsideConfiguredLocations(t *testing.T) {
	store, err := config.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	keep := filepath.Join(t.TempDir(), "keep")
	removed := filepath.Join(t.TempDir(), "removed")
	settings, _ := store.Snapshot()
	settings.Media.LibraryPaths = []string{keep}
	if _, err := store.Save(settings); err != nil {
		t.Fatalf("Save settings: %v", err)
	}
	for _, row := range []struct {
		id   string
		root string
	}{
		{id: "keep-video", root: keep},
		{id: "removed-video", root: removed},
	} {
		if _, err := store.Datastore().SQL().Exec(`
			INSERT INTO media_videos(
				id, location_path, relative_path, display_name, size_bytes,
				modified_at, duration_ms, funscript_relative_path, missing, scanned_at
			) VALUES(?, ?, 'video.mp4', ?, 1, 'now', NULL, NULL, 0, 'now')
		`, row.id, row.root, row.id); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}

	server := newTestServerWithStore(t, store, Runtime{})
	videos, err := server.media.List(t.Context())
	if err != nil {
		t.Fatalf("List media: %v", err)
	}
	if len(videos) != 1 || videos[0].ID != "keep-video" {
		t.Fatalf("startup media catalog = %+v, want only keep-video", videos)
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
	return newTestServerWithStore(t, store, runtime)
}

func newTestServerWithStore(t *testing.T, store *config.Store, runtime Runtime) *Server {
	t.Helper()
	if runtime.Traces == nil {
		runtime.Traces = diagnostics.NewTraceRing(8)
	}
	if runtime.Transport == nil {
		runtime.Transport = transport.NewFake()
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
	t.Cleanup(server.Close)
	return server
}

func saveSettings(t *testing.T, store *config.Store, mutate func(config.Settings) config.Settings) {
	t.Helper()

	settings, _ := store.Snapshot()
	if _, err := store.Save(mutate(settings)); err != nil {
		t.Fatalf("Save settings: %v", err)
	}
}

func withController(request *http.Request) *http.Request {
	return withControllerID(request, "test-controller")
}

func withControllerID(request *http.Request, clientID string) *http.Request {
	request.Header.Set(controllerHeaderName, clientID)
	return request
}

func contains(value string, fragment string) bool {
	return strings.Contains(value, fragment)
}
