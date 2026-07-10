package httpapi

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestResolveWorkerBinaryOrder(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	toolsDir := filepath.Join(root, "data", "tools")
	if err := os.MkdirAll(appDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(toolsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	name := "voice-parakeet-worker"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	beside := filepath.Join(appDir, name)
	tool := filepath.Join(toolsDir, name)
	if err := os.WriteFile(beside, []byte("worker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tool, []byte("worker"), 0o600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(appDir, "magichandy.exe")
	if got := resolveWorkerBinary("explicit-worker", executable, filepath.Join(root, "data"), "voice-parakeet-worker"); got != "explicit-worker" {
		t.Fatalf("explicit resolution = %q", got)
	}
	if got := resolveWorkerBinary("", executable, filepath.Join(root, "data"), "voice-parakeet-worker"); got != beside {
		t.Fatalf("beside-app resolution = %q, want %q", got, beside)
	}
	if err := os.Remove(beside); err != nil {
		t.Fatal(err)
	}
	if got := resolveWorkerBinary("", executable, filepath.Join(root, "data"), "voice-parakeet-worker"); got != tool {
		t.Fatalf("tools resolution = %q, want %q", got, tool)
	}
}

func TestVoiceManagerConfigComposesFirstPartyProviders(t *testing.T) {
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceTTSProviderElevenLabs
	settings.TTSWorkerPath = `C:\workers\eleven.exe`
	settings.ElevenLabsVoiceID = "voice-id"
	settings.ElevenLabsModelID = "model-id"
	settings.ElevenLabsAPIKey = "private-key"
	settings.ASRProvider = config.VoiceASRProviderParakeet
	settings.ASRWorkerPath = `C:\workers\parakeet.exe`
	settings.ParakeetServerPath = `C:\parakeet\server.exe`
	settings.ParakeetModelPath = `C:\parakeet\model.gguf`
	settings.ParakeetServerPort = 9011

	got := voiceManagerConfig(settings, "", "")
	if got.TTS.Command != settings.TTSWorkerPath || strings.Join(got.TTS.Args, "|") != "-voice-id|voice-id|-model-id|model-id" {
		t.Fatalf("ElevenLabs composition = %+v", got.TTS)
	}
	if got.TTS.Env["ELEVENLABS_API_KEY"] != "private-key" || strings.Contains(strings.Join(got.TTS.Args, " "), "private-key") {
		t.Fatalf("ElevenLabs secret must be environment-only: %+v", got.TTS)
	}
	wantASR := `-server-path|C:\parakeet\server.exe|-server-model|C:\parakeet\model.gguf|-server-port|9011`
	if got.ASR.Command != settings.ASRWorkerPath || strings.Join(got.ASR.Args, "|") != wantASR {
		t.Fatalf("Parakeet composition = %+v, want args %q", got.ASR, wantASR)
	}
}

func TestVoiceManagerConfigPreservesCustomAndDisablesNone(t *testing.T) {
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceProviderCustom
	settings.TTSWorkerPath = "custom-tts"
	settings.TTSWorkerArgs = []string{"--unchanged", "value"}
	settings.ASRProvider = config.VoiceProviderNone
	settings.ASRWorkerPath = "hidden-custom-asr"
	got := voiceManagerConfig(settings, "", "")
	if !got.TTS.Enabled || got.TTS.Command != "custom-tts" || strings.Join(got.TTS.Args, "|") != "--unchanged|value" {
		t.Fatalf("custom behavior changed: %+v", got.TTS)
	}
	if got.ASR.Enabled || got.ASR.Command != "" {
		t.Fatalf("provider none must disable hidden command: %+v", got.ASR)
	}
}

func TestSilentTestWAVBase64ProducesValidPCMSilence(t *testing.T) {
	audio, err := base64.StdEncoding.DecodeString(silentTestWAVBase64())
	if err != nil {
		t.Fatalf("decode test WAV: %v", err)
	}
	if len(audio) < 44 || string(audio[0:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || string(audio[12:16]) != "fmt " || string(audio[36:40]) != "data" {
		t.Fatalf("test payload is not a canonical WAV header")
	}
	if got := binary.LittleEndian.Uint32(audio[24:28]); got != 16000 {
		t.Fatalf("sample rate = %d, want 16000", got)
	}
	if got := binary.LittleEndian.Uint16(audio[22:24]); got != 1 {
		t.Fatalf("channels = %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint16(audio[34:36]); got != 16 {
		t.Fatalf("bit depth = %d, want 16", got)
	}
	if got, want := int(binary.LittleEndian.Uint32(audio[40:44])), len(audio)-44; got != want {
		t.Fatalf("data length = %d, want %d", got, want)
	}
	for _, sample := range audio[44:] {
		if sample != 0 {
			t.Fatal("test WAV must contain silence")
		}
	}
}

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
		settings.Voice.ASRProvider = config.VoiceProviderCustom
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

func TestVoiceTranscriptionUsesASRQueueAndReturnsTranscript(t *testing.T) {
	server := newTestServer(t)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.ASRProvider = config.VoiceProviderCustom
		settings.Voice.ASRWorkerPath = chatStubBinary(t)
		settings.Voice.ASRWorkerArgs = []string{"-role", "asr", "-start-loaded"}
		return settings
	})
	server.applyVoiceSettingsTransition(snapshotSettings(t, server))

	start := httptest.NewRecorder()
	server.Handler().ServeHTTP(start, withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/asr/start", nil)))
	if start.Code != http.StatusOK {
		t.Fatalf("start ASR = %d: %s", start.Code, start.Body.String())
	}

	body := `{"audio_b64":"` + strings.Repeat("A", 128*1024) + `","audio_format":"webm"}`
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/voice/transcriptions", strings.NewReader(body))))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("transcribe = %d: %s", recorder.Code, recorder.Body.String())
	}
	var accepted struct {
		Request struct {
			ID string `json:"id"`
		} `json:"request"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		pending, ok := server.voice.Request(accepted.Request.ID)
		if !ok {
			t.Fatal("accepted request was not tracked")
		}
		snapshot := pending.Snapshot()
		if snapshot.State == "done" {
			if len(snapshot.Transcript) != 1 || snapshot.Transcript[0].Text != "stub transcript" {
				t.Fatalf("transcript = %+v", snapshot)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("transcription did not finish: %+v", snapshot)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestVoiceTranscriptionRejectsUnsupportedAudioFormat(t *testing.T) {
	server := newTestServer(t)
	body := strings.NewReader(`{"audio_b64":"AA==","audio_format":"mp3"}`)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/voice/transcriptions", body)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "webm, ogg, or wav") {
		t.Fatalf("unsupported format = %d: %s", recorder.Code, recorder.Body.String())
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
