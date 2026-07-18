package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestLLMModelManagerAPIImportsOllamaAndProtectsSelection(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	ollamaRoot := writeHTTPAPIOllamaFixture(t)

	scanRecorder := httptest.NewRecorder()
	scanRequest := withController(jsonAPIRequest(t, http.MethodPost, "/api/llm/ollama/scan", map[string]any{"path": ollamaRoot}))
	server.Handler().ServeHTTP(scanRecorder, scanRequest)
	if scanRecorder.Code != http.StatusOK {
		t.Fatalf("scan status = %d: %s", scanRecorder.Code, scanRecorder.Body.String())
	}
	var scan struct {
		Candidates []llm.OllamaCandidate `json:"candidates"`
	}
	decodeResponse(t, scanRecorder, &scan)
	if len(scan.Candidates) != 1 || !scan.Candidates[0].Importable {
		t.Fatalf("scan = %+v", scan)
	}

	deniedRecorder := httptest.NewRecorder()
	deniedRequest := jsonAPIRequest(t, http.MethodPost, "/api/llm/imports/ollama", map[string]any{
		"path": ollamaRoot, "candidate_id": scan.Candidates[0].ID,
	})
	server.Handler().ServeHTTP(deniedRecorder, deniedRequest)
	if deniedRecorder.Code != http.StatusConflict {
		t.Fatalf("uncontrolled import status = %d, want %d", deniedRecorder.Code, http.StatusConflict)
	}

	importRecorder := httptest.NewRecorder()
	importRequest := withController(jsonAPIRequest(t, http.MethodPost, "/api/llm/imports/ollama", map[string]any{
		"path": ollamaRoot, "candidate_id": scan.Candidates[0].ID,
	}))
	server.Handler().ServeHTTP(importRecorder, importRequest)
	if importRecorder.Code != http.StatusAccepted {
		t.Fatalf("import status = %d: %s", importRecorder.Code, importRecorder.Body.String())
	}
	var started struct {
		Import llm.ImportJob `json:"import"`
	}
	decodeResponse(t, importRecorder, &started)
	job := waitForAPIImport(t, server, started.Import.ID)
	if job.Status != llm.ImportStatusComplete || job.ModelID == "" {
		t.Fatalf("import job = %+v", job)
	}

	listRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/api/llm/models", nil))
	var listed struct {
		Models []llm.ModelRecord `json:"models"`
	}
	decodeResponse(t, listRecorder, &listed)
	if len(listed.Models) != 1 || listed.Models[0].ID != job.ModelID {
		t.Fatalf("listed models = %+v", listed.Models)
	}

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.Model = listed.Models[0].ID
		return settings
	})
	writeHTTPAPIManagedRuntime(t, server.store.DataDir())
	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/llm/status", nil))
	if statusRecorder.Code != http.StatusOK {
		t.Fatalf("managed status = %d: %s", statusRecorder.Code, statusRecorder.Body.String())
	}
	var status llm.ProviderStatus
	decodeResponse(t, statusRecorder, &status)
	if !status.Managed || status.Loaded || !strings.Contains(status.Message, "not loaded") {
		t.Fatalf("managed status = %+v, want resolved app-owned runtime and model", status)
	}
	deleteRecorder := httptest.NewRecorder()
	deleteRequest := withController(httptest.NewRequest(http.MethodDelete, "/api/llm/models/"+job.ModelID, nil))
	server.Handler().ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusConflict {
		t.Fatalf("delete selected status = %d, want %d", deleteRecorder.Code, http.StatusConflict)
	}

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.Model = "another-model"
		return settings
	})
	deleteRecorder = httptest.NewRecorder()
	deleteRequest = withController(httptest.NewRequest(http.MethodDelete, "/api/llm/models/"+job.ModelID, nil))
	server.Handler().ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
}

func TestLLMLoadAndUnloadRequireController(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	for _, path := range []string{"/api/llm/load", "/api/llm/unload"} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		if recorder.Code != http.StatusConflict {
			t.Fatalf("%s status = %d, want %d", path, recorder.Code, http.StatusConflict)
		}
	}
}

func TestManagedLLMRuntimeAPIIsReadOnlyWithoutController(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/llm/runtime", nil))
	if statusRecorder.Code != http.StatusOK {
		t.Fatalf("runtime status = %d: %s", statusRecorder.Code, statusRecorder.Body.String())
	}
	var snapshot llm.ManagedLlamaRuntimeSnapshot
	decodeResponse(t, statusRecorder, &snapshot)
	if snapshot.Runtime.State != llm.ManagedRuntimeStateMissing || snapshot.Runtime.ExpectedVersion != llm.ManagedLlamaVersion {
		t.Fatalf("runtime snapshot = %+v", snapshot)
	}

	denied := httptest.NewRecorder()
	server.Handler().ServeHTTP(denied, jsonAPIRequest(t, http.MethodPost, "/api/llm/runtime/build", map[string]any{"backend": "cpu"}))
	if denied.Code != http.StatusConflict {
		t.Fatalf("uncontrolled build = %d, want %d", denied.Code, http.StatusConflict)
	}

	invalid := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalid, withController(jsonAPIRequest(t, http.MethodPost, "/api/llm/runtime/build", map[string]any{"backend": "invalid"})))
	if invalid.Code != http.StatusConflict {
		t.Fatalf("invalid build = %d, want %d", invalid.Code, http.StatusConflict)
	}
}

func TestOllamaDaemonModelListDoesNotRequireSelectedModel(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"model-a:latest","size":1024,"details":{"format":"gguf","family":"llama","parameter_size":"3B","quantization_level":"Q4_K_M"}}]}`))
	}))
	t.Cleanup(ollama.Close)
	server := newTestServer(t)
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.OllamaBaseURL = ollama.URL
		settings.LLM.Model = "not-installed"
		return settings
	})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/llm/ollama/models", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Available bool                  `json:"available"`
		Models    []llm.OllamaModelInfo `json:"models"`
	}
	decodeResponse(t, recorder, &response)
	if !response.Available || len(response.Models) != 1 || response.Models[0].Name != "model-a:latest" {
		t.Fatalf("response = %+v", response)
	}
}

func TestOllamaProviderDoesNotRequireManagedLlamaRuntime(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"existing:latest"}]}`))
	}))
	t.Cleanup(ollama.Close)

	server := newTestServer(t)
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.Provider = config.LLMProviderOllama
		settings.LLM.OllamaBaseURL = ollama.URL
		settings.LLM.Model = "existing:latest"
		return settings
	})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/llm/status", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("Ollama status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var status llm.ProviderStatus
	decodeResponse(t, recorder, &status)
	if !status.Available || !status.ModelAvailable || status.Provider != config.LLMProviderOllama {
		t.Fatalf("Ollama status = %+v, want available without managed llama.cpp", status)
	}
}

func TestModelManagerStorageFailuresAreExplicitAndRedacted(t *testing.T) {
	server := newTestServer(t)
	ollamaRoot := writeHTTPAPIOllamaFixture(t)
	if _, err := server.store.Datastore().SQL().Exec(`DROP TABLE llm_models`); err != nil {
		t.Fatalf("remove model inventory table: %v", err)
	}

	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/llm/models", nil),
		withController(jsonAPIRequest(t, http.MethodPost, "/api/llm/ollama/scan", map[string]any{"path": ollamaRoot})),
	}
	for _, request := range requests {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("%s %s status = %d: %s", request.Method, request.URL.Path, recorder.Code, recorder.Body.String())
		}
		if got := recorder.Body.String(); !strings.Contains(got, "model manager storage is unavailable") || strings.Contains(got, "closed") {
			t.Fatalf("%s %s exposed unstable storage details: %s", request.Method, request.URL.Path, got)
		}
	}

	state := server.llmState(context.Background()).(map[string]any)
	if available, ok := state["model_manager_available"].(bool); !ok || available {
		t.Fatalf("model_manager_available = %#v, want false", state["model_manager_available"])
	}
	if ready, ok := state["managed_ready"].(bool); !ok || ready {
		t.Fatalf("managed_ready = %#v, want false", state["managed_ready"])
	}
}

func writeHTTPAPIOllamaFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "models")
	blobs := filepath.Join(root, "blobs")
	manifests := filepath.Join(root, "manifests", "registry.ollama.ai", "library", "api-fixture")
	for _, directory := range []string{blobs, manifests} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	model := append([]byte("GGUF"), make([]byte, 4096)...)
	modelDigest := writeHTTPAPIBlob(t, blobs, model)
	configData := []byte(`{"model_format":"gguf","model_family":"llama","model_type":"1B","file_type":"Q4_0"}`)
	configDigest := writeHTTPAPIBlob(t, blobs, configData)
	manifest := map[string]any{
		"schemaVersion": 2,
		"config": map[string]any{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"digest":    configDigest,
			"size":      len(configData),
		},
		"layers": []map[string]any{{
			"mediaType": "application/vnd.ollama.image.model",
			"digest":    modelDigest,
			"size":      len(model),
		}},
	}
	payload, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(manifests, "latest"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeHTTPAPIManagedRuntime(t *testing.T, dataDir string) {
	t.Helper()
	root := llm.ManagedLlamaRuntimeRoot(dataDir)
	runnerRelative := "installs/test/bin/llama-server.exe"
	runner := filepath.Join(root, filepath.FromSlash(runnerRelative))
	if err := os.MkdirAll(filepath.Dir(runner), 0o700); err != nil {
		t.Fatalf("create managed runtime: %v", err)
	}
	if err := os.WriteFile(runner, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write managed runner: %v", err)
	}
	manifest := map[string]any{
		"schema_version": 1,
		"runtime":        "llama.cpp",
		"version":        llm.ManagedLlamaVersion,
		"commit":         llm.ManagedLlamaCommit,
		"backend":        "cpu",
		"runner":         runnerRelative,
		"source":         "built_from_source",
		"built_at":       time.Now().UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal managed runtime: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "active.json"), payload, 0o600); err != nil {
		t.Fatalf("write managed runtime manifest: %v", err)
	}
}

func writeHTTPAPIBlob(t *testing.T, directory string, payload []byte) string {
	t.Helper()
	sum := sha256.Sum256(payload)
	hexDigest := hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(directory, "sha256-"+hexDigest), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return "sha256:" + hexDigest
}

func jsonAPIRequest(t *testing.T, method, path string, payload any) *http.Request {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
}

func waitForAPIImport(t *testing.T, server *Server, id string) llm.ImportJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/llm/imports/"+id, nil))
		var response struct {
			Import llm.ImportJob `json:"import"`
		}
		decodeResponse(t, recorder, &response)
		if !strings.Contains(" queued copying ", " "+response.Import.Status+" ") {
			return response.Import
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("import %s did not finish", id)
	return llm.ImportJob{}
}
