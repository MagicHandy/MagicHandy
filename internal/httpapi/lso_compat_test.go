package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestLsoStatusManagedLlamaIdleReportsConnected(t *testing.T) {
	server := newTestServer(t)
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("test model"), 0o600); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	runnerPath := filepath.Join(dir, "llama-server.exe")
	if err := os.WriteFile(runnerPath, []byte("stub"), 0o600); err != nil {
		t.Fatalf("write runner fixture: %v", err)
	}

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.Provider = config.LLMProviderLlamaCPP
		settings.LLM.LlamaCPPMode = config.LlamaCPPModeManaged
		settings.LLM.LlamaCPPBaseURL = config.DefaultLlamaCPPBaseURL
		settings.LLM.LlamaCPPRunnerPath = runnerPath
		settings.LLM.LlamaCPPModelPath = modelPath
		settings.LLM.Model = "local-model"
		return settings
	})

	connected, errVal := server.lsoLLMStatus(mustSnapshot(t, server))
	if !connected {
		t.Fatalf("lsoLLMStatus connected = false, err = %#v", errVal)
	}
	if errVal != nil {
		t.Fatalf("lsoLLMStatus err = %#v, want nil while idle", errVal)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if payload["llm_provider"] != config.LLMProviderLlamaCPP {
		t.Fatalf("llm_provider = %#v", payload["llm_provider"])
	}
	if payload["llm_connected"] != true {
		t.Fatalf("llm_connected = %#v, want true while managed runner is idle", payload["llm_connected"])
	}
	llm, _ := payload["llm"].(map[string]any)
	if llm == nil {
		t.Fatalf("llm payload missing: %#v", payload)
	}
	if llm["loaded"] != false {
		t.Fatalf("llm.loaded = %#v, want false before first chat", llm["loaded"])
	}
}

func mustSnapshot(t *testing.T, server *Server) config.Settings {
	t.Helper()
	settings, _ := server.store.Snapshot()
	return settings
}
