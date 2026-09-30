package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestLLMCatalogRatesModelsAgainstDetectedGPU(t *testing.T) {
	for name, testCase := range map[string]struct {
		hardware map[string]any
		want     []string
	}{
		"16 GB NVIDIA": {
			hardware: map[string]any{"nvidia": true, "gpu_name": "NVIDIA GeForce RTX 5070 Ti", "vram_mib": "16303"},
			want:     []string{llm.CatalogFitRecommended, llm.CatalogFitSupported, llm.CatalogFitSupported},
		},
		"8 GB NVIDIA": {
			hardware: map[string]any{"nvidia": true, "gpu_name": "NVIDIA GeForce RTX 3070", "vram_mib": "8192"},
			want:     []string{llm.CatalogFitBelowMinimum, llm.CatalogFitBelowMinimum, llm.CatalogFitRecommended},
		},
		"no NVIDIA": {
			hardware: map[string]any{"nvidia": false},
			want:     []string{llm.CatalogFitNoGPU, llm.CatalogFitNoGPU, llm.CatalogFitNoGPU},
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := newTestServer(t)
			setTestHardware(server, testCase.hardware)
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/llm/catalog", nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("catalog status = %d: %s", recorder.Code, recorder.Body.String())
			}
			var body struct {
				Models   []catalogModelView  `json:"models"`
				Hardware catalogHardwareView `json:"hardware"`
			}
			decodeResponse(t, recorder, &body)
			if len(body.Models) != len(testCase.want) {
				t.Fatalf("catalog models = %d, want %d", len(body.Models), len(testCase.want))
			}
			for index, model := range body.Models {
				if model.Fit != testCase.want[index] {
					t.Errorf("%s fit = %q, want %q", model.ID, model.Fit, testCase.want[index])
				}
				if model.SHA256 == "" || model.License == "" || model.SourceURL == "" || model.DownloadURL == "" {
					t.Errorf("%s is missing the facts shown before download: %+v", model.ID, model)
				}
			}
		})
	}
}

func TestCatalogDownloadRequiresControllerAndACatalogID(t *testing.T) {
	server := newTestServer(t)
	for name, testCase := range map[string]struct {
		request    *http.Request
		wantStatus int
	}{
		"read only": {
			request:    jsonAPIRequest(t, http.MethodPost, "/api/llm/imports/catalog", map[string]any{"id": llm.CatalogModels()[0].ID}),
			wantStatus: http.StatusConflict,
		},
		"free-form URL": {
			request:    withController(jsonAPIRequest(t, http.MethodPost, "/api/llm/imports/catalog", map[string]any{"id": "https://example.invalid/model.gguf"})),
			wantStatus: http.StatusBadRequest,
		},
		"unknown install plan model": {
			request:    withController(jsonAPIRequest(t, http.MethodPost, "/api/setup/install", map[string]any{"model": map[string]any{"catalog_id": "unknown"}})),
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

func TestSetupModelStepDownloadsVerifiesAndSelectsTheModel(t *testing.T) {
	server := newTestServer(t)
	data := httpAPITestGGUF()
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "model.gguf", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(download.Close)
	sum := sha256.Sum256(data)
	model := llm.CatalogModel{
		ID: "fixture", DisplayName: "Fixture Gemma", SizeBytes: int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]), SourceName: "fixture/gemma:latest",
		License: "Apache-2.0", DownloadURL: download.URL + "/blobs/sha256:" + hex.EncodeToString(sum[:]),
	}
	_, job, err := server.setup.reserveJob("install_plan", "selected_components", "", "queued")
	if err != nil {
		t.Fatal(err)
	}

	if err := server.downloadCatalogModelForSetup(t.Context(), job.ID, model); err != nil {
		t.Fatalf("setup model download: %v", err)
	}
	saved, _ := server.store.Snapshot()
	if saved.LLM.Provider != config.LLMProviderLlamaCPP || saved.LLM.LlamaCPPMode != config.LlamaCPPModeManaged || saved.LLM.Model == "" {
		t.Fatalf("downloaded model was not selected: %+v", saved.LLM)
	}
	record, err := server.models.Model(t.Context(), saved.LLM.Model)
	if err != nil || record.SHA256 != model.SHA256 {
		t.Fatalf("selected model record = %+v, %v", record, err)
	}
	snapshot := server.setup.Snapshot()
	if snapshot == nil || snapshot.BytesCompleted != model.SizeBytes || !strings.Contains(snapshot.Message, "Fixture Gemma") {
		t.Fatalf("setup job did not mirror download progress: %+v", snapshot)
	}
	if !strings.Contains(snapshot.Output, "Downloading Fixture Gemma") || !strings.Contains(snapshot.Output, "Verified SHA-256 "+model.SHA256) {
		t.Fatalf("setup terminal output = %q", snapshot.Output)
	}
}

func TestSetupInstallPlanQueuesTheModelAfterTheRuntime(t *testing.T) {
	server := newTestServer(t)
	var downloaded []string
	server.setup.downloadModel = func(_ context.Context, _ string, model llm.CatalogModel) error {
		downloaded = append(downloaded, model.ID)
		return nil
	}
	catalogID := llm.CatalogModels()[0].ID
	job, err := server.setup.StartInstallPlan(setupInstallPlanRequest{Model: &setupModelDownloadRequest{CatalogID: catalogID}})
	if err != nil {
		t.Fatalf("start plan: %v", err)
	}
	if len(job.Steps) != 1 || job.Steps[0].ID != "chat_model" {
		t.Fatalf("plan steps = %+v", job.Steps)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		snapshot := server.setup.Snapshot()
		if snapshot != nil && snapshot.Status == setupJobComplete {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("plan did not finish: %+v", snapshot)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if strings.Join(downloaded, ",") != catalogID {
		t.Fatalf("downloaded catalog models = %v", downloaded)
	}
}

func TestInstalledVoiceTurnsOnOnlyWhenReadyAndRequested(t *testing.T) {
	var chatterbox, fasterQwen setupVoiceModule
	for _, module := range setupVoiceModules {
		switch module.ID {
		case "chatterbox":
			chatterbox = module
		case "faster-qwen3-tts":
			fasterQwen = module
		}
	}
	for name, testCase := range map[string]struct {
		module setupVoiceModule
		enable bool
		wantOn bool
	}{
		"chatterbox requested":  {module: chatterbox, enable: true, wantOn: true},
		"chatterbox left off":   {module: chatterbox, enable: false, wantOn: false},
		"reference voice first": {module: fasterQwen, enable: true, wantOn: false},
	} {
		t.Run(name, func(t *testing.T) {
			server := newTestServer(t)
			if err := server.applyInstalledVoiceModule(t.Context(), setupVoiceInstallResult{
				Module: testCase.module, Device: testCase.module.SupportedDevices[0], Root: t.TempDir(), Enable: testCase.enable,
			}); err != nil {
				t.Fatalf("apply installed voice: %v", err)
			}
			saved, _ := server.store.Snapshot()
			if saved.Voice.Enabled != testCase.wantOn || saved.Voice.SpeakReplies != testCase.wantOn {
				t.Fatalf("voice enabled = %v, speak replies = %v, want %v", saved.Voice.Enabled, saved.Voice.SpeakReplies, testCase.wantOn)
			}
		})
	}

	server := newTestServer(t)
	if err := server.applyInstalledParakeet(t.Context(), setupParakeetInstallResult{
		ServerPath: `C:\voice\parakeet-server.exe`, ModelPath: `C:\voice\model.gguf`, Enable: true,
	}); err != nil {
		t.Fatalf("apply installed Parakeet: %v", err)
	}
	if saved, _ := server.store.Snapshot(); !saved.Voice.Enabled || saved.Voice.ASRProvider != config.VoiceASRProviderParakeet {
		t.Fatalf("requested Parakeet was not turned on: %+v", saved.Voice)
	}
}

func TestSetupPreferencesSaveTheHandyModel(t *testing.T) {
	server := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(jsonAPIRequest(t, http.MethodPut, "/api/setup/preferences", map[string]any{"handy_model": config.HandyModel2Pro})))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if saved, _ := server.store.Snapshot(); saved.Motion.HandyModel != config.HandyModel2Pro {
		t.Fatalf("saved Handy model = %q", saved.Motion.HandyModel)
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(jsonAPIRequest(t, http.MethodPut, "/api/setup/preferences", map[string]any{"handy_model": "handy_3"})))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown Handy model status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestLargestNVIDIAGPUHandlesSeveralCards(t *testing.T) {
	for _, testCase := range []struct {
		output, name, memory string
	}{
		{"NVIDIA GeForce RTX 5070 Ti, 16303\r\n", "NVIDIA GeForce RTX 5070 Ti", "16303"},
		{"NVIDIA T400, 2048\nNVIDIA GeForce RTX 4090, 24564\n", "NVIDIA GeForce RTX 4090", "24564"},
		{"NVIDIA Mystery Card, [N/A]\n", "NVIDIA Mystery Card", ""},
		{"", "", ""},
	} {
		name, memory := largestNVIDIAGPU(testCase.output)
		if name != testCase.name || memory != testCase.memory {
			t.Errorf("largestNVIDIAGPU(%q) = %q, %q", testCase.output, name, memory)
		}
	}
}

func setTestHardware(server *Server, hardware map[string]any) {
	server.setup.hardwareOnce.Do(func() {})
	server.setup.hardwareMu.Lock()
	server.setup.hardware = hardware
	server.setup.hardwareMu.Unlock()
}
