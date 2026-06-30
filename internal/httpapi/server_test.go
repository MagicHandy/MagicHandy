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
	if body.Features["motion"] != "not_implemented" {
		t.Fatalf("motion feature = %q, want not_implemented", body.Features["motion"])
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

	store, err := config.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	server, err := New(fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>MagicHandy</title>")},
		"app.css":    {Data: []byte("body { margin: 0; }")},
		"app.js":     {Data: []byte("console.log('ready');")},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), store, VersionInfo{
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
