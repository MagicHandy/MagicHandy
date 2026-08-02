package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestSetupStatusDescribesOptionalInstallersWithoutSecrets(t *testing.T) {
	server := newTestServer(t)
	const secret = "private-setup-connection-key"
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.HandyConnectionKey = secret
		return settings
	})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/setup", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), secret) {
		t.Fatal("setup status leaked the connection key")
	}
	var body struct {
		Required     bool                     `json:"required"`
		VoiceModules []setupVoiceModule       `json:"voice_modules"`
		LlamaRuntime setupLlamaRuntimeCatalog `json:"llama_runtime"`
		Parakeet     setupParakeetCatalog     `json:"parakeet"`
		Helpers      map[string]bool          `json:"helpers"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode setup status: %v", err)
	}
	if !body.Required || len(body.VoiceModules) != 2 || len(body.LlamaRuntime.Backends) != 3 || body.Parakeet.Model == "" {
		t.Fatalf("incomplete setup catalog: %+v", body)
	}
	for _, helper := range []string{"llama", "parakeet", "voice"} {
		if _, ok := body.Helpers[helper]; !ok {
			t.Fatalf("setup helper %q was not reported", helper)
		}
	}
}

func TestSetupPreferencesSaveScopedChoicesAndRedactKey(t *testing.T) {
	server := newTestServer(t)
	settings, _ := server.store.Snapshot()
	settings.LLM.Provider = config.LLMProviderOllama
	settings.LLM.Model = "gemma3:4b"
	settings.LLM.OllamaBaseURL = "http://127.0.0.1:11434"
	llmJSON, err := json.Marshal(settings.LLM)
	if err != nil {
		t.Fatalf("marshal LLM settings: %v", err)
	}
	const secret = "private-setup-key"
	body := `{"ui_locale":"ja","chat_locale":"es","device_owner":"intiface","connection_key":"` + secret + `","llm":` + string(llmJSON) + `}`
	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPut, "/api/setup/preferences", strings.NewReader(body)))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), secret) {
		t.Fatal("setup preference response leaked the connection key")
	}

	saved, _ := server.store.Snapshot()
	if saved.UI.Locale != config.LocaleJapanese || saved.LLM.PromptSet != config.PromptSetMagicHandyMotionV1ES {
		t.Fatalf("saved setup locales = ui %q, prompt %q", saved.UI.Locale, saved.LLM.PromptSet)
	}
	if saved.Device.HSPDispatchOwner != config.DispatchOwnerIntiface || saved.Device.HandyConnectionKey != secret {
		t.Fatalf("saved setup device choices = %+v", saved.Device)
	}
	if saved.LLM.Provider != config.LLMProviderOllama || saved.LLM.Model != "gemma3:4b" {
		t.Fatalf("saved setup LLM choices = %+v", saved.LLM)
	}
}

func TestSetupMutationRequiresControllerAndValidChoices(t *testing.T) {
	server := newTestServer(t)
	for name, testCase := range map[string]struct {
		request    *http.Request
		wantStatus int
	}{
		"read only": {
			request:    httptest.NewRequest(http.MethodPut, "/api/setup/preferences", strings.NewReader(`{"device_owner":"cloud_rest"}`)),
			wantStatus: http.StatusConflict,
		},
		"unsupported transport": {
			request:    withController(httptest.NewRequest(http.MethodPut, "/api/setup/preferences", strings.NewReader(`{"device_owner":"legacy"}`))),
			wantStatus: http.StatusBadRequest,
		},
		"invalid llama backend": {
			request:    withController(httptest.NewRequest(http.MethodPost, "/api/setup/llm/install", strings.NewReader(`{"backend":"metal"}`))),
			wantStatus: http.StatusConflict,
		},
		"invalid voice module": {
			request:    withController(httptest.NewRequest(http.MethodPost, "/api/setup/voice/install", strings.NewReader(`{"module":"unknown","device":"cpu"}`))),
			wantStatus: http.StatusConflict,
		},
		"cancel idle queue": {
			request:    withController(httptest.NewRequest(http.MethodDelete, "/api/setup/install", nil)),
			wantStatus: http.StatusConflict,
		},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, testCase.request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, testCase.wantStatus, recorder.Body.String())
			}
		})
	}
}

func TestSetupCompletePersistsWizardState(t *testing.T) {
	server := newTestServer(t)
	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/setup/complete", strings.NewReader(`{}`)))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	settings, _ := server.store.Snapshot()
	if !settings.UI.SetupCompleted {
		t.Fatal("setup completion was not persisted")
	}
}

func TestSetupOutputIsBoundedAndKeepsCompleteUTF8(t *testing.T) {
	input := strings.Repeat("x", setupJobOutputLimit) + "résumé"
	trimmed := trimSetupOutput(input)
	if len(trimmed) > setupJobOutputLimit || !strings.HasSuffix(trimmed, "résumé") {
		t.Fatalf("trimmed setup output has invalid boundary: length=%d suffix=%q", len(trimmed), trimmed[len(trimmed)-8:])
	}
	if got := lastSetupOutputLine("first\r\n\r\nlast\r\n"); got != "last" {
		t.Fatalf("last output line = %q, want last", got)
	}
}
