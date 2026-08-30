package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestParakeetSetupPreselectionRecognizesSavedRuntimeAndPartialState(t *testing.T) {
	base := config.DefaultSettings().Voice
	for name, testCase := range map[string]struct {
		settings config.VoiceSettings
		status   voiceModuleStatus
	}{
		"saved managed provider": {
			settings: func() config.VoiceSettings {
				value := base
				value.ASRProvider = config.VoiceASRProviderParakeet
				value.ParakeetSource = config.ParakeetSourceApp
				return value
			}(),
		},
		"existing runner":   {settings: base, status: voiceModuleStatus{RunnerInstalled: optionalBool(true)}},
		"existing model":    {settings: base, status: voiceModuleStatus{ModelInstalled: optionalBool(true)}},
		"resumable partial": {settings: base, status: voiceModuleStatus{ResumablePartial: optionalBool(true)}},
	} {
		t.Run(name, func(t *testing.T) {
			if !shouldPreselectParakeet(testCase.settings, testCase.status) {
				t.Fatal("Parakeet was not preselected")
			}
		})
	}
	if shouldPreselectParakeet(base, voiceModuleStatus{}) {
		t.Fatal("fresh unselected Parakeet was preselected")
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
		"empty install plan": {
			request:    withController(httptest.NewRequest(http.MethodPost, "/api/setup/install", strings.NewReader(`{}`))),
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

func TestVerifyInstalledVoiceModuleUsesRuntimeReadinessContract(t *testing.T) {
	root := t.TempDir()
	module := setupVoiceModules[0]
	state, err := json.Marshal(map[string]any{
		"schema_version": 2,
		"module":         module.ID,
		"provider":       module.Provider,
		"model":          module.Model,
		"voice":          config.DefaultFasterQwenVoice,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "module-state.json"), state, 0o600); err != nil {
		t.Fatal(err)
	}
	managedTestFile(t, filepath.Join(root, ".venv", managedPythonDirectory(), managedPythonName()))
	managedTestFile(t, filepath.Join(root, "magichandy-faster-qwen-server.py"))

	if err := verifyInstalledVoiceModule(root, module); err == nil || !strings.Contains(err.Error(), "server entry point") {
		t.Fatalf("pre-server verification error = %v", err)
	}
	managedTestFile(t, filepath.Join(root, "source", "examples", "openai_server.py"))
	if err := verifyInstalledVoiceModule(root, module); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("pre-model verification error = %v", err)
	}
	managedFasterQwenSnapshot(t, root, module.Model, "abc123")
	if err := verifyInstalledVoiceModule(root, module); err != nil {
		t.Fatalf("complete module verification: %v", err)
	}
}

func TestApplyInstalledVoiceModuleResetsManagedWorkerOverride(t *testing.T) {
	server := newTestServer(t)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.TTSProvider = config.VoiceTTSProviderFasterQwen
		settings.Voice.TTSWorkerPath = `C:\old-install\voice-openai-tts-worker.exe`
		settings.Voice.TTSWorkerArgs = []string{"--legacy"}
		settings.Voice.TTSReferenceWAV = `C:\voices\reference.wav`
		settings.Voice.TTSReferenceText = "Exact transcript."
		return settings
	})
	module := setupVoiceModules[0]
	if err := server.applyInstalledVoiceModule(t.Context(), setupVoiceInstallResult{
		Module: module, Device: config.TTSDeviceCUDA, AutoLaunch: true, Root: t.TempDir(),
	}); err != nil {
		t.Fatalf("apply installed voice module: %v", err)
	}
	saved, _ := server.store.Snapshot()
	if saved.Voice.TTSWorkerPath != "" || len(saved.Voice.TTSWorkerArgs) != 0 {
		t.Fatalf("managed worker override survived setup: %+v", saved.Voice)
	}
	if saved.Voice.TTSReferenceWAV != `C:\voices\reference.wav` ||
		saved.Voice.TTSReferenceText != "Exact transcript." {
		t.Fatalf("same-provider reinstall discarded the saved reference: %+v", saved.Voice)
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

func TestSetupInstallPlanTracksStepsAndReturnsIndependentSnapshots(t *testing.T) {
	server := newTestServer(t)
	manager := server.setup
	ctx, job, err := manager.reserveJob("install_plan", "selected_components", "", "queued")
	if err != nil {
		t.Fatalf("reserve install plan: %v", err)
	}
	runOrder := make([]string, 0, 2)
	tasks := []setupPlanTask{
		{
			step: setupJobStep{ID: "runtime", Label: "Runtime", Status: setupJobQueued},
			run: func(context.Context, string) error {
				runOrder = append(runOrder, "runtime")
				return nil
			},
		},
		{
			step: setupJobStep{ID: "voice", Label: "Voice", Status: setupJobQueued},
			run: func(context.Context, string) error {
				runOrder = append(runOrder, "voice")
				return nil
			},
		},
	}
	manager.setJobSteps(job.ID, []setupJobStep{tasks[0].step, tasks[1].step})
	manager.wg.Add(1)
	manager.runInstallPlan(ctx, job.ID, tasks)

	snapshot := manager.Snapshot()
	if snapshot == nil || snapshot.Status != setupJobComplete || snapshot.CompletedSteps != 2 || snapshot.TotalSteps != 2 {
		t.Fatalf("completed plan snapshot = %+v", snapshot)
	}
	if strings.Join(runOrder, ",") != "runtime,voice" {
		t.Fatalf("install order = %v", runOrder)
	}
	for _, step := range snapshot.Steps {
		if step.Status != setupJobComplete {
			t.Fatalf("completed step = %+v", step)
		}
	}
	snapshot.Steps[0].Status = "tampered"
	if current := manager.Snapshot(); current.Steps[0].Status != setupJobComplete {
		t.Fatal("setup snapshot shared its mutable step storage")
	}
}

func TestSetupInstallPlanDistinguishesFailureFromCancellation(t *testing.T) {
	t.Run("failure", func(t *testing.T) {
		server := newTestServer(t)
		manager := server.setup
		ctx, job, err := manager.reserveJob("install_plan", "selected_components", "", "queued")
		if err != nil {
			t.Fatalf("reserve install plan: %v", err)
		}
		tasks := []setupPlanTask{{
			step: setupJobStep{ID: "runtime", Label: "Runtime", Status: setupJobQueued},
			run:  func(context.Context, string) error { return errors.New("compiler failed") },
		}}
		manager.setJobSteps(job.ID, []setupJobStep{tasks[0].step})
		manager.wg.Add(1)
		manager.runInstallPlan(ctx, job.ID, tasks)

		snapshot := manager.Snapshot()
		if snapshot.Status != setupJobFailed || snapshot.Steps[0].Status != setupJobFailed {
			t.Fatalf("failed plan snapshot = %+v", snapshot)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		server := newTestServer(t)
		manager := server.setup
		ctx, job, err := manager.reserveJob("install_plan", "selected_components", "", "queued")
		if err != nil {
			t.Fatalf("reserve install plan: %v", err)
		}
		tasks := []setupPlanTask{
			{step: setupJobStep{ID: "runtime", Label: "Runtime", Status: setupJobQueued}, run: func(context.Context, string) error { return nil }},
			{step: setupJobStep{ID: "voice", Label: "Voice", Status: setupJobQueued}, run: func(context.Context, string) error { return nil }},
		}
		manager.setJobSteps(job.ID, []setupJobStep{tasks[0].step, tasks[1].step})
		if _, err := manager.Cancel(); err != nil {
			t.Fatalf("cancel install plan: %v", err)
		}
		manager.wg.Add(1)
		manager.runInstallPlan(ctx, job.ID, tasks)

		snapshot := manager.Snapshot()
		if snapshot.Status != setupJobCancelled {
			t.Fatalf("cancelled plan status = %q", snapshot.Status)
		}
		if snapshot.Steps[0].Status != setupJobCancelled || snapshot.Steps[1].Status != setupJobCancelled {
			t.Fatalf("cancelled plan steps = %+v", snapshot.Steps)
		}
	})
}
