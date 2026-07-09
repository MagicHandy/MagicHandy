package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestVoiceStatusDefaultsToDisabled(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/voice/status", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var payload struct {
		Voice struct {
			Enabled         bool `json:"enabled"`
			ProtocolVersion int  `json:"protocol_version"`
			Workers         map[string]struct {
				State      string `json:"state"`
				Configured bool   `json:"configured"`
			} `json:"workers"`
		} `json:"voice"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode voice status: %v", err)
	}
	if payload.Voice.Enabled {
		t.Fatal("voice must be disabled by default")
	}
	if payload.Voice.ProtocolVersion != 1 {
		t.Fatalf("protocol_version = %d, want 1", payload.Voice.ProtocolVersion)
	}
	for _, role := range []string{"tts", "asr"} {
		worker, ok := payload.Voice.Workers[role]
		if !ok {
			t.Fatalf("voice status is missing the %s worker", role)
		}
		if worker.State != "disabled" {
			t.Fatalf("%s worker state = %q, want disabled", role, worker.State)
		}
	}
}

func TestVoiceStateAppearsInAppState(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/state", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if _, ok := payload["voice"]; !ok {
		t.Fatal("/api/state must include the voice block")
	}
}

func TestVoiceWorkerStartRequiresController(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/voice/workers/tts/start", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden && recorder.Code != http.StatusConflict {
		t.Fatalf("start without controller = %d, want a controller rejection", recorder.Code)
	}
	if recorder.Code == http.StatusOK {
		t.Fatal("start must not succeed without the controller lease")
	}
}

func TestVoiceWorkerStartWhileDisabledFails(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/tts/start", nil))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if !contains(recorder.Body.String(), "disabled") {
		t.Fatalf("error should say voice is disabled, got %s", recorder.Body.String())
	}
}

func TestVoiceWorkerStartWithoutCommandFails(t *testing.T) {
	server := newTestServer(t)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		return settings
	})
	server.applyVoiceSettingsTransition(snapshotSettings(t, server))

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/asr/start", nil))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if !contains(recorder.Body.String(), "configured") {
		t.Fatalf("error should say no worker is configured, got %s", recorder.Body.String())
	}

	// The unconfigured state must be visible, not an opaque failure.
	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/voice/status", nil))
	if !contains(statusRecorder.Body.String(), "not_configured") {
		t.Fatalf("voice status should report not_configured, got %s", statusRecorder.Body.String())
	}
}

func TestVoiceUnknownRoleIsNotFound(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/kazoo/start", nil))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestVoiceUnknownRequestIsNotFound(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/voice/requests/12345", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestVoiceSettingsRoundTripThroughAPI(t *testing.T) {
	server := newTestServer(t)

	body := `{
		"server": {"port": 49717},
		"device": {
			"hsp_dispatch_owner": "cloud_rest",
			"firmware_api_requirement": "firmware_v4_api_v3_required",
			"api_application_id_source": "bundled_app_id",
			"api_application_id_override": ""
		},
		"motion": {"speed_min_percent": 20, "speed_max_percent": 80, "stroke_min_percent": 0, "stroke_max_percent": 100, "reverse_direction": false, "style": "balanced"},
		"llm": {"provider": "llama_cpp", "llama_cpp_mode": "managed", "llama_cpp_base_url": "http://127.0.0.1:8080", "ollama_base_url": "http://127.0.0.1:11434", "model": "local-model", "prompt_set": "magichandy_motion_v1", "request_timeout_ms": 120000},
		"voice": {"enabled": true, "tts_worker_path": "C:\\workers\\stub.exe", "tts_worker_args": ["-role", "tts"]},
		"diagnostics": {"verbosity": "normal"},
		"clear_connection_key": false
	}`

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body)))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("save settings = %d: %s", recorder.Code, recorder.Body.String())
	}

	settings := snapshotSettings(t, server)
	if !settings.Voice.Enabled {
		t.Fatal("voice enabled flag did not persist")
	}
	if settings.Voice.TTSWorkerPath != `C:\workers\stub.exe` {
		t.Fatalf("tts worker path = %q", settings.Voice.TTSWorkerPath)
	}
	if len(settings.Voice.TTSWorkerArgs) != 2 {
		t.Fatalf("tts worker args = %v", settings.Voice.TTSWorkerArgs)
	}

	// The saved-but-unstarted worker must show as stopped, never autostart.
	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/voice/status", nil))
	if !contains(statusRecorder.Body.String(), `"state":"stopped"`) {
		t.Fatalf("configured tts worker should be stopped, got %s", statusRecorder.Body.String())
	}
}

func snapshotSettings(t *testing.T, server *Server) config.Settings {
	t.Helper()
	settings, _ := server.store.Snapshot()
	return settings
}
