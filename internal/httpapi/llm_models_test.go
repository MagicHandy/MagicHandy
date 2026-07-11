package httpapi

import (
	"bytes"
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
		settings.LLM.LlamaCPPModelPath = listed.Models[0].ModelPath
		settings.LLM.Model = listed.Models[0].ID
		return settings
	})
	deleteRecorder := httptest.NewRecorder()
	deleteRequest := withController(httptest.NewRequest(http.MethodDelete, "/api/llm/models/"+job.ModelID, nil))
	server.Handler().ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusConflict {
		t.Fatalf("delete selected status = %d, want %d", deleteRecorder.Code, http.StatusConflict)
	}

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.LlamaCPPModelPath = ""
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
