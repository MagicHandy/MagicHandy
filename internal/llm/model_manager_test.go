package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestModelManagerScansAndImportsOllamaGGUF(t *testing.T) {
	manager, err := OpenModelManager(t.TempDir())
	if err != nil {
		t.Fatalf("OpenModelManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	ollamaRoot := filepath.Join(t.TempDir(), ".ollama", "models")
	fixture := writeOllamaFixture(t, ollamaRoot, ollamaFixtureOptions{})
	candidate := assertOllamaScan(t, manager, filepath.Dir(ollamaRoot), ollamaRoot)
	job, err := manager.StartOllamaImport(context.Background(), ollamaRoot, candidate.ID)
	if err != nil {
		t.Fatalf("StartOllamaImport: %v", err)
	}
	job = waitForImport(t, manager, job.ID)
	model := assertImportedOllamaModel(t, manager, job, fixture)
	assertManagedDelete(t, manager, model)
}

func assertOllamaScan(t *testing.T, manager *ModelManager, inputPath, wantPath string) OllamaCandidate {
	t.Helper()
	scan, err := manager.ScanOllama(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("ScanOllama: %v", err)
	}
	if scan.Path != wantPath || len(scan.Candidates) != 1 {
		t.Fatalf("scan = %+v", scan)
	}
	candidate := scan.Candidates[0]
	if !candidate.Importable || candidate.Name != "fixture:Q4_K_M" {
		t.Fatalf("candidate = %+v", candidate)
	}
	if candidate.Family != "llama" || candidate.ParameterSize != "3.2B" || candidate.Quantization != "Q4_K_M" {
		t.Fatalf("candidate metadata = %+v", candidate)
	}
	if candidate.License != "Fixture model license" {
		t.Fatalf("license = %q", candidate.License)
	}
	return candidate
}

func assertImportedOllamaModel(
	t *testing.T,
	manager *ModelManager,
	job ImportJob,
	fixture ollamaFixture,
) ModelRecord {
	t.Helper()
	if job.Status != ImportStatusComplete || job.ModelID == "" {
		t.Fatalf("job = %+v", job)
	}
	models, err := manager.List(context.Background())
	if err != nil || len(models) != 1 {
		t.Fatalf("models = %+v, err = %v", models, err)
	}
	model := models[0]
	if model.State != modelStateReady || model.SHA256 != fixture.modelSHA || model.Source != ModelSourceOllama {
		t.Fatalf("model = %+v", model)
	}
	if _, err := os.Stat(fixture.modelPath); err != nil {
		t.Fatalf("Ollama source must remain untouched: %v", err)
	}
	if payload, err := os.ReadFile(model.ModelPath); err != nil || string(payload) != string(fixture.modelData) {
		t.Fatalf("managed model mismatch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(model.ModelPath), "metadata.json")); err != nil {
		t.Fatalf("metadata sidecar: %v", err)
	}
	return model
}

func assertManagedDelete(t *testing.T, manager *ModelManager, model ModelRecord) {
	t.Helper()
	if err := manager.Delete(context.Background(), model.ID, model.ID); !errors.Is(err, ErrModelSelected) {
		t.Fatalf("delete selected model = %v", err)
	}
	if err := manager.Delete(context.Background(), model.ID, ""); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(model.ModelPath); !os.IsNotExist(err) {
		t.Fatalf("managed copy still exists: %v", err)
	}
}

func TestOllamaScanRejectsUnsupportedAndChangedLayers(t *testing.T) {
	manager, err := OpenModelManager(t.TempDir())
	if err != nil {
		t.Fatalf("OpenModelManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	projectorRoot := filepath.Join(t.TempDir(), "projector")
	writeOllamaFixture(t, projectorRoot, ollamaFixtureOptions{projector: true})
	scan, err := manager.ScanOllama(context.Background(), projectorRoot)
	if err != nil {
		t.Fatalf("ScanOllama projector: %v", err)
	}
	if scan.Candidates[0].Importable || !strings.Contains(scan.Candidates[0].Reason, "projector") {
		t.Fatalf("projector candidate = %+v", scan.Candidates[0])
	}

	changedRoot := filepath.Join(t.TempDir(), "changed")
	fixture := writeOllamaFixture(t, changedRoot, ollamaFixtureOptions{})
	if err := os.WriteFile(fixture.modelPath, append(fixture.modelData, 'x'), 0o600); err != nil {
		t.Fatalf("change model blob: %v", err)
	}
	scan, err = manager.ScanOllama(context.Background(), changedRoot)
	if err != nil {
		t.Fatalf("ScanOllama changed: %v", err)
	}
	if scan.Candidates[0].Importable || !strings.Contains(scan.Candidates[0].Reason, "size") {
		t.Fatalf("changed candidate = %+v", scan.Candidates[0])
	}
}

func TestModelManagerImportsStandaloneGGUFAndDeduplicates(t *testing.T) {
	manager, err := OpenModelManager(t.TempDir())
	if err != nil {
		t.Fatalf("OpenModelManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	source := filepath.Join(t.TempDir(), "local.gguf")
	data := append([]byte("GGUF"), make([]byte, 4096)...)
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatalf("write GGUF: %v", err)
	}
	first, err := manager.StartGGUFImport(source, "Local model")
	if err != nil {
		t.Fatalf("StartGGUFImport: %v", err)
	}
	first = waitForImport(t, manager, first.ID)
	second, err := manager.StartGGUFImport(source, "Duplicate name")
	if err != nil {
		t.Fatalf("StartGGUFImport duplicate: %v", err)
	}
	second = waitForImport(t, manager, second.ID)
	if first.ModelID != second.ModelID {
		t.Fatalf("duplicate IDs = %q, %q", first.ModelID, second.ModelID)
	}
	models, _ := manager.List(context.Background())
	if len(models) != 1 {
		t.Fatalf("model count = %d, want 1", len(models))
	}
}

func TestModelManagerRejectsNonGGUFAndProtectsStoreRoot(t *testing.T) {
	manager, err := OpenModelManager(t.TempDir())
	if err != nil {
		t.Fatalf("OpenModelManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	notGGUF := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(notGGUF, []byte("not a model"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartGGUFImport(notGGUF, "bad"); err == nil || !strings.Contains(err.Error(), "not a GGUF") {
		t.Fatalf("StartGGUFImport non-GGUF = %v", err)
	}

	now := nowText()
	record := ModelRecord{
		ID: "unsafe", DisplayName: "unsafe", Provider: "llama_cpp", Source: ModelSourceGGUF,
		Format: "gguf", SizeBytes: 8, SHA256: strings.Repeat("a", 64),
		ModelPath: filepath.Join(manager.modelsDir, "model.gguf"), ImportedAt: now, UpdatedAt: now,
	}
	if err := manager.insertModel(context.Background(), record); err != nil {
		t.Fatalf("insert unsafe fixture: %v", err)
	}
	if err := manager.Delete(context.Background(), record.ID, ""); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("unsafe Delete = %v", err)
	}
	if _, err := os.Stat(manager.modelsDir); err != nil {
		t.Fatalf("model store root was removed: %v", err)
	}
}

func TestModelManagerBoundsConcurrentImports(t *testing.T) {
	manager, err := OpenModelManager(t.TempDir())
	if err != nil {
		t.Fatalf("OpenModelManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	manager.jobs["active-1"] = &modelImportJob{snapshot: ImportJob{Status: ImportStatusCopying}}
	manager.jobs["active-2"] = &modelImportJob{snapshot: ImportJob{Status: ImportStatusQueued}}

	_, err = manager.startImport(modelImportSpec{
		DisplayName: "third", Source: ModelSourceGGUF,
		SourcePath: filepath.Join(t.TempDir(), "third.gguf"), SizeBytes: 8,
	})
	if err == nil || !strings.Contains(err.Error(), "at most 2") {
		t.Fatalf("third concurrent import error = %v", err)
	}
}

func TestOpenModelManagerRemovesOnlyOldImportPartials(t *testing.T) {
	dataDir := t.TempDir()
	downloads := filepath.Join(dataDir, "downloads")
	if err := os.MkdirAll(downloads, 0o700); err != nil {
		t.Fatal(err)
	}
	oldPartial := filepath.Join(downloads, "model-import-old.partial")
	recentPartial := filepath.Join(downloads, "model-import-recent.partial")
	unrelated := filepath.Join(downloads, "other.partial")
	for _, path := range []string{oldPartial, recentPartial, unrelated} {
		if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oldTime := time.Now().Add(-stalePartialAge - time.Hour)
	if err := os.Chtimes(oldPartial, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	manager, err := OpenModelManager(dataDir)
	if err != nil {
		t.Fatalf("OpenModelManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := os.Stat(oldPartial); !os.IsNotExist(err) {
		t.Fatalf("old partial stat = %v, want removed", err)
	}
	for _, path := range []string{recentPartial, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("preserved partial %q: %v", path, err)
		}
	}
}

type ollamaFixtureOptions struct {
	projector bool
}

type ollamaFixture struct {
	modelPath string
	modelData []byte
	modelSHA  string
}

func writeOllamaFixture(t *testing.T, root string, options ollamaFixtureOptions) ollamaFixture {
	t.Helper()
	blobs := filepath.Join(root, "blobs")
	manifestDir := filepath.Join(root, "manifests", "registry.ollama.ai", "library", "fixture")
	for _, directory := range []string{blobs, manifestDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("mkdir fixture: %v", err)
		}
	}
	modelData := append([]byte("GGUF"), make([]byte, 8192)...)
	modelDigest, modelPath := writeOllamaBlob(t, blobs, modelData)
	configData := []byte(`{"model_format":"gguf","model_family":"llama","model_type":"3.2B","file_type":"Q4_K_M"}`)
	configDigest, _ := writeOllamaBlob(t, blobs, configData)
	licenseDigest, _ := writeOllamaBlob(t, blobs, []byte("# Fixture model license\nTerms"))
	layers := []ollamaLayer{
		{MediaType: ollamaModelMediaType, Digest: modelDigest, Size: int64(len(modelData))},
		{MediaType: ollamaLicenseMediaType, Digest: licenseDigest, Size: int64(len("# Fixture model license\nTerms"))},
	}
	if options.projector {
		projectorDigest, _ := writeOllamaBlob(t, blobs, []byte("projector"))
		layers = append(layers, ollamaLayer{MediaType: ollamaProjectorMediaType, Digest: projectorDigest, Size: 9})
	}
	manifest := ollamaManifest{
		SchemaVersion: 2,
		Config:        ollamaLayer{MediaType: "application/vnd.docker.container.image.v1+json", Digest: configDigest, Size: int64(len(configData))},
		Layers:        layers,
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "Q4_K_M"), payload, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return ollamaFixture{modelPath: modelPath, modelData: modelData, modelSHA: strings.TrimPrefix(modelDigest, "sha256:")}
}

func writeOllamaBlob(t *testing.T, directory string, payload []byte) (string, string) {
	t.Helper()
	sum := sha256.Sum256(payload)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	path := filepath.Join(directory, strings.Replace(digest, ":", "-", 1))
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write blob: %v", err)
	}
	return digest, path
}

func waitForImport(t *testing.T, manager *ModelManager, id string) ImportJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := manager.Import(id)
		if err != nil {
			t.Fatalf("Import: %v", err)
		}
		if job.Status == ImportStatusComplete || job.Status == ImportStatusFailed || job.Status == ImportStatusCancelled {
			return job
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("import %s did not finish", id)
	return ImportJob{}
}
