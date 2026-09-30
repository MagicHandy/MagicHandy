package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCatalogEntriesArePinnedAndWellFormed(t *testing.T) {
	models := CatalogModels()
	if len(models) == 0 {
		t.Fatal("catalog is empty")
	}
	defaults := 0
	seen := map[string]bool{}
	for _, model := range models {
		if seen[model.ID] {
			t.Fatalf("duplicate catalog ID %q", model.ID)
		}
		seen[model.ID] = true
		if model.Default {
			defaults++
		}
		if _, err := hex.DecodeString(model.SHA256); err != nil || len(model.SHA256) != 64 {
			t.Fatalf("%s: SHA-256 %q is not 64 hex characters", model.ID, model.SHA256)
		}
		if !catalogURLPinned(model) {
			t.Fatalf("%s: download URL %q is neither a content-addressed blob nor a commit-pinned file", model.ID, model.DownloadURL)
		}
		if model.SizeBytes <= 0 || model.VRAMMiB <= 0 || model.MinVRAMMiB < model.VRAMMiB {
			t.Fatalf("%s: size or memory figures are inconsistent: %+v", model.ID, model)
		}
		if model.License == "" || !strings.HasPrefix(model.LicenseURL, "https://") || !strings.HasPrefix(model.SourceURL, "https://") {
			t.Fatalf("%s: license and source must be shown before download: %+v", model.ID, model)
		}
		if found, ok := FindCatalogModel(" " + model.ID + " "); !ok || found.SHA256 != model.SHA256 {
			t.Fatalf("FindCatalogModel(%q) = %+v, %v", model.ID, found, ok)
		}
	}
	if defaults != 1 {
		t.Fatalf("catalog defaults = %d, want exactly one", defaults)
	}
	models[0].SHA256 = "mutated"
	if CatalogModels()[0].SHA256 == "mutated" {
		t.Fatal("CatalogModels exposed the shared catalog slice")
	}
}

func TestCatalogFitRecommendsTheFirstModelEachCardHolds(t *testing.T) {
	fits := func(hardware CatalogHardware) []string {
		var out []string
		for _, model := range CatalogModels() {
			out = append(out, CatalogFit(model, hardware))
		}
		return out
	}
	cases := []struct {
		name     string
		hardware CatalogHardware
		want     []string
	}{
		{"no NVIDIA GPU", CatalogHardware{}, []string{CatalogFitNoGPU, CatalogFitNoGPU, CatalogFitNoGPU}},
		{"unknown memory", CatalogHardware{NVIDIA: true}, []string{CatalogFitUnknownVRAM, CatalogFitUnknownVRAM, CatalogFitUnknownVRAM}},
		{"4 GB card", CatalogHardware{NVIDIA: true, VRAMMiB: 4096}, []string{CatalogFitBelowMinimum, CatalogFitBelowMinimum, CatalogFitBelowMinimum}},
		{"8 GB card", CatalogHardware{NVIDIA: true, VRAMMiB: 8192}, []string{CatalogFitBelowMinimum, CatalogFitBelowMinimum, CatalogFitRecommended}},
		{"12 GB card", CatalogHardware{NVIDIA: true, VRAMMiB: 12282}, []string{CatalogFitRecommended, CatalogFitSupported, CatalogFitSupported}},
		{"16 GB card", CatalogHardware{NVIDIA: true, VRAMMiB: 16303}, []string{CatalogFitRecommended, CatalogFitSupported, CatalogFitSupported}},
	}
	for _, test := range cases {
		if got := fits(test.hardware); strings.Join(got, ",") != strings.Join(test.want, ",") {
			t.Errorf("%s: fits = %v, want %v", test.name, got, test.want)
		}
	}
}

// huggingFacePinnedURL accepts only a resolve URL fixed to one 40-hex commit.
var huggingFacePinnedURL = regexp.MustCompile(`^https://huggingface\.co/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+/resolve/[0-9a-f]{40}/[A-Za-z0-9._-]+\.gguf$`)

// catalogURLPinned accepts only download URLs whose bytes cannot change
// without the pinned digest or commit changing with them. HTTP handlers only
// pass curated entries, so the catalog test is where the rule is enforced.
func catalogURLPinned(model CatalogModel) bool {
	if strings.HasPrefix(model.DownloadURL, ollamaRegistryBlobBase) {
		return strings.HasSuffix(model.DownloadURL, "/blobs/sha256:"+model.SHA256)
	}
	return huggingFacePinnedURL.MatchString(model.DownloadURL)
}

func TestCatalogURLPinnedRejectsMovableURLs(t *testing.T) {
	sha := strings.Repeat("a", 64)
	cases := []struct {
		url  string
		want bool
	}{
		{ollamaRegistryBlobBase + "owner/model/blobs/sha256:" + sha, true},
		{ollamaRegistryBlobBase + "owner/model/blobs/sha256:" + strings.Repeat("b", 64), false},
		{"https://huggingface.co/owner/repo/resolve/" + strings.Repeat("c", 40) + "/model-Q4_0.gguf", true},
		{"https://huggingface.co/owner/repo/resolve/main/model-Q4_0.gguf", false},
		{"https://huggingface.co/owner/repo/resolve/" + strings.Repeat("c", 40) + "/model.bin", false},
		{"https://example.com/owner/repo/resolve/" + strings.Repeat("c", 40) + "/model.gguf", false},
	}
	for _, test := range cases {
		if got := catalogURLPinned(CatalogModel{SHA256: sha, DownloadURL: test.url}); got != test.want {
			t.Errorf("catalogURLPinned(%q) = %v, want %v", test.url, got, test.want)
		}
	}
}

func TestParseContentRange(t *testing.T) {
	cases := []struct {
		value        string
		start, total int64
		ok           bool
	}{
		{"bytes 0-15/7381381696", 0, 7381381696, true},
		{"bytes 42-99/100", 42, 100, true},
		{"bytes 42-99/*", 42, 0, true},
		{"bytes */100", 0, 0, false},
		{"items 0-1/2", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, test := range cases {
		start, total, ok := parseContentRange(test.value)
		if start != test.start || total != test.total || ok != test.ok {
			t.Errorf("parseContentRange(%q) = %d, %d, %v", test.value, start, total, ok)
		}
	}
}

func TestCatalogDownloadCommitsVerifiedModel(t *testing.T) {
	data := testGGUFData(t, ggufFixtureOptions{})
	server, requests := catalogServer(t, data, nil)
	manager, model := catalogFixture(t, server, data)

	job, err := manager.StartCatalogDownload(context.Background(), model)
	if err != nil {
		t.Fatal(err)
	}
	job = waitForImport(t, manager, job.ID)
	if job.Status != ImportStatusComplete || job.ModelID == "" || job.BytesCopied != int64(len(data)) {
		t.Fatalf("catalog download = %+v", job)
	}
	record, err := manager.Model(context.Background(), job.ModelID)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture is served from loopback, not the Ollama registry, so it is
	// labelled as a plain GGUF download.
	if record.Source != ModelSourceGGUF || record.SourceName != model.SourceName || record.SHA256 != model.SHA256 || record.License != model.License {
		t.Fatalf("committed record = %+v", record)
	}
	stored, err := os.ReadFile(record.ModelPath)
	if err != nil || !bytes.Equal(stored, data) {
		t.Fatalf("stored model differs from the download: %v", err)
	}
	if manager.CatalogPartialBytes(model) != 0 {
		t.Fatal("partial download remained after commit")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("download requests = %d, want 1", got)
	}
	if installed, err := manager.CatalogModelInstalled(context.Background(), model); err != nil || installed != job.ModelID {
		t.Fatalf("CatalogModelInstalled = %q, %v", installed, err)
	}
}

func TestCatalogDownloadResumesSavedPartial(t *testing.T) {
	data := testGGUFData(t, ggufFixtureOptions{})
	var ranges []string
	var mu sync.Mutex
	server, _ := catalogServer(t, data, func(r *http.Request) {
		mu.Lock()
		ranges = append(ranges, r.Header.Get("Range"))
		mu.Unlock()
	})
	manager, model := catalogFixture(t, server, data)
	half := len(data) / 2
	partial := filepath.Join(manager.downloadsDir, catalogPartialName(model.SHA256))
	if err := os.WriteFile(partial, data[:half], 0o600); err != nil {
		t.Fatal(err)
	}
	if got := manager.CatalogPartialBytes(model); got != int64(half) {
		t.Fatalf("CatalogPartialBytes = %d, want %d", got, half)
	}

	job, err := manager.StartCatalogDownload(context.Background(), model)
	if err != nil {
		t.Fatal(err)
	}
	if job = waitForImport(t, manager, job.ID); job.Status != ImportStatusComplete {
		t.Fatalf("resumed download = %+v", job)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ranges) != 1 || ranges[0] != "bytes="+strconv.Itoa(half)+"-" {
		t.Fatalf("range requests = %q", ranges)
	}
}

func TestCatalogDownloadDiscardsChecksumMismatch(t *testing.T) {
	data := testGGUFData(t, ggufFixtureOptions{})
	tampered := append([]byte(nil), data...)
	tampered[len(tampered)-1] ^= 0xff
	server, _ := catalogServer(t, tampered, nil)
	manager, model := catalogFixture(t, server, data)

	job, err := manager.StartCatalogDownload(context.Background(), model)
	if err != nil {
		t.Fatal(err)
	}
	job = waitForImport(t, manager, job.ID)
	if job.Status != ImportStatusFailed || !strings.Contains(job.Error, "pinned SHA-256") {
		t.Fatalf("tampered download = %+v", job)
	}
	if manager.CatalogPartialBytes(model) != 0 {
		t.Fatal("tampered bytes were kept for resume")
	}
	models, err := manager.List(context.Background())
	if err != nil || len(models) != 0 {
		t.Fatalf("tampered model entered inventory: %+v, %v", models, err)
	}
}

func TestCatalogDownloadKeepsPartialAcrossFailuresThenResumes(t *testing.T) {
	data := testGGUFData(t, ggufFixtureOptions{})
	var healthy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy.Load() {
			http.ServeContent(w, r, "model.gguf", time.Time{}, bytes.NewReader(data))
			return
		}
		// Advertise the whole file, send a few bytes, then drop the connection.
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data[:8])
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(server.Close)
	manager, model := catalogFixture(t, server, data)

	job, err := manager.StartCatalogDownload(context.Background(), model)
	if err != nil {
		t.Fatal(err)
	}
	job = waitForImport(t, manager, job.ID)
	if job.Status != ImportStatusFailed || !strings.Contains(job.Error, "retry resumes") {
		t.Fatalf("interrupted download = %+v", job)
	}
	if manager.CatalogPartialBytes(model) == 0 {
		t.Fatal("interrupted download discarded its resumable partial")
	}

	healthy.Store(true)
	job, err = manager.StartCatalogDownload(context.Background(), model)
	if err != nil {
		t.Fatal(err)
	}
	if job = waitForImport(t, manager, job.ID); job.Status != ImportStatusComplete {
		t.Fatalf("retried download = %+v", job)
	}
}

func TestCatalogDownloadRetriesAStalledConnection(t *testing.T) {
	data := testGGUFData(t, ggufFixtureOptions{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data[:8])
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
			return
		}
		http.ServeContent(w, r, "model.gguf", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(server.Close)
	manager, model := catalogFixture(t, server, data)
	manager.downloadIdleLimit = 50 * time.Millisecond

	job, err := manager.StartCatalogDownload(context.Background(), model)
	if err != nil {
		t.Fatal(err)
	}
	if job = waitForImport(t, manager, job.ID); job.Status != ImportStatusComplete {
		t.Fatalf("stalled download = %+v", job)
	}
	if calls.Load() != 2 {
		t.Fatalf("requests = %d, want a stalled attempt and one resume", calls.Load())
	}
}

func TestCatalogDownloadCancelKeepsPartial(t *testing.T) {
	data := testGGUFData(t, ggufFixtureOptions{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data[:8])
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})
	manager, model := catalogFixture(t, server, data)

	job, err := manager.StartCatalogDownload(context.Background(), model)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		current, err := manager.Import(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == ImportStatusDownloading && current.BytesCopied > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("download never reported progress: %+v", current)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := manager.CancelImport(job.ID); err != nil {
		t.Fatal(err)
	}
	if job = waitForImport(t, manager, job.ID); job.Status != ImportStatusCancelled {
		t.Fatalf("cancelled download = %+v", job)
	}
	if manager.CatalogPartialBytes(model) != 8 {
		t.Fatalf("cancelled download kept %d bytes, want 8", manager.CatalogPartialBytes(model))
	}
}

func TestCatalogDownloadReusesAnInstalledCopy(t *testing.T) {
	data := testGGUFData(t, ggufFixtureOptions{})
	server, requests := catalogServer(t, data, nil)
	manager, model := catalogFixture(t, server, data)
	source := filepath.Join(t.TempDir(), "already.gguf")
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := manager.StartGGUFImport(source, "Already here")
	if err != nil {
		t.Fatal(err)
	}
	imported = waitForImport(t, manager, imported.ID)

	job, err := manager.StartCatalogDownload(context.Background(), model)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != ImportStatusComplete || job.ModelID != imported.ModelID {
		t.Fatalf("reused copy job = %+v, want model %q", job, imported.ModelID)
	}
	if requests.Load() != 0 {
		t.Fatalf("an installed model was downloaded again (%d requests)", requests.Load())
	}
}

func TestStartCatalogDownloadRejectsUnpinnedEntries(t *testing.T) {
	manager, err := OpenModelManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	unpinned := CatalogModel{ID: "loose", DisplayName: "Loose", SizeBytes: 10, DownloadURL: "https://example.invalid/model.gguf"}
	if _, err := manager.StartCatalogDownload(context.Background(), unpinned); err == nil || !strings.Contains(err.Error(), "not pinned") {
		t.Fatalf("unpinned catalog entry error = %v", err)
	}
	if _, ok := FindCatalogModel("https://example.invalid/model.gguf"); ok {
		t.Fatal("a URL resolved as a catalog ID")
	}
}

func TestOpenModelManagerDropsRetiredCatalogPartials(t *testing.T) {
	dataDir := t.TempDir()
	downloads := filepath.Join(dataDir, "downloads")
	if err := os.MkdirAll(downloads, 0o700); err != nil {
		t.Fatal(err)
	}
	listed := filepath.Join(downloads, catalogPartialName(curatedCatalog[0].SHA256))
	retired := filepath.Join(downloads, catalogPartialName(strings.Repeat("c", 64)))
	for _, path := range []string{listed, retired} {
		if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manager, err := OpenModelManager(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := os.Stat(listed); err != nil {
		t.Fatalf("resumable catalog partial was removed: %v", err)
	}
	if _, err := os.Stat(retired); !os.IsNotExist(err) {
		t.Fatalf("retired catalog partial remains: %v", err)
	}
}

func catalogServer(t *testing.T, data []byte, observe func(*http.Request)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if observe != nil {
			observe(r)
		}
		if r.Header.Get("User-Agent") != catalogDownloadUserAgent {
			http.Error(w, "unexpected user agent", http.StatusBadRequest)
			return
		}
		http.ServeContent(w, r, "model.gguf", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func catalogFixture(t *testing.T, server *httptest.Server, data []byte) (*ModelManager, CatalogModel) {
	t.Helper()
	manager, err := OpenModelManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	manager.downloadClient = server.Client()
	manager.downloadRetryDelay = time.Millisecond
	sum := sha256.Sum256(data)
	return manager, CatalogModel{
		ID:            "fixture",
		DisplayName:   "Fixture model",
		Family:        "gemma4",
		ParameterSize: "1B",
		Quantization:  "Q4_K_M",
		SizeBytes:     int64(len(data)),
		SHA256:        hex.EncodeToString(sum[:]),
		License:       "Apache-2.0",
		SourceName:    "fixture/model:latest",
		DownloadURL:   server.URL + "/v2/fixture/model/blobs/sha256:" + hex.EncodeToString(sum[:]),
	}
}
