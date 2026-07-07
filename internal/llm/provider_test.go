package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("MAGICHANDY_TEST_LLAMA_RUNNER") == "1" {
		runManagedLlamaRunnerHelper()
		return
	}
	os.Exit(m.Run())
}

func runManagedLlamaRunnerHelper() {
	if path := os.Getenv("MAGICHANDY_TEST_LLAMA_RUNNER_COUNT"); path != "" {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304,G703 -- test helper writes a temp-file path injected by its parent test.
		if err == nil {
			_, _ = file.WriteString("start\n")
			_ = file.Close()
		}
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestOllamaStatusRequiresSelectedModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"other:latest"}]}`))
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(HTTPProviderOptions{
		BaseURL: server.URL,
		Model:   "wanted:latest",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	status := provider.Status(t.Context())
	if status.Available {
		t.Fatalf("status should be unavailable for a missing model: %+v", status)
	}
	if status.ModelAvailable {
		t.Fatalf("model should not be available: %+v", status)
	}
	if !strings.Contains(status.Message, "wanted:latest") {
		t.Fatalf("status message = %q, want selected model name", status.Message)
	}
}

func TestLlamaCPPStatusRequiresSelectedModelWhenModelListExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"other-model"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewLlamaCPPProvider(HTTPProviderOptions{
		BaseURL: server.URL,
		Model:   "wanted-model",
	})
	if err != nil {
		t.Fatalf("NewLlamaCPPProvider: %v", err)
	}

	status := provider.Status(t.Context())
	if status.Available {
		t.Fatalf("status should be unavailable for a missing model: %+v", status)
	}
	if status.ModelAvailable {
		t.Fatalf("model should not be available: %+v", status)
	}
	if !strings.Contains(status.Message, "wanted-model") {
		t.Fatalf("status message = %q, want selected model name", status.Message)
	}
}

func TestManagedLlamaCPPStatusRequiresRunnerAndModelPaths(t *testing.T) {
	provider, err := NewManagedLlamaCPPProvider(ManagedLlamaCPPOptions{
		HTTPProviderOptions: HTTPProviderOptions{
			BaseURL: "http://127.0.0.1:8080",
			Model:   "local-model",
		},
	})
	if err != nil {
		t.Fatalf("NewManagedLlamaCPPProvider: %v", err)
	}

	status := provider.Status(t.Context())
	if status.Available || status.Loaded {
		t.Fatalf("managed provider should require setup before availability: %+v", status)
	}
	if !status.Managed {
		t.Fatalf("managed status should identify managed provider: %+v", status)
	}
	if !strings.Contains(status.Message, "runner path") {
		t.Fatalf("status message = %q, want runner path setup error", status.Message)
	}
}

func TestManagedLlamaCPPEnsureStartedIsSerialized(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("test model"), 0o600); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	countPath := filepath.Join(dir, "starts.txt")
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER", "1")
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER_COUNT", countPath)

	provider, err := NewManagedLlamaCPPProvider(ManagedLlamaCPPOptions{
		HTTPProviderOptions: HTTPProviderOptions{
			BaseURL: "http://127.0.0.1:18080",
			Model:   "local-model",
		},
		RunnerPath: os.Args[0],
		ModelPath:  modelPath,
	})
	if err != nil {
		t.Fatalf("NewManagedLlamaCPPProvider: %v", err)
	}
	t.Cleanup(func() {
		provider.Unload(context.Background())
	})

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- provider.ensureStarted()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("ensureStarted: %v", err)
		}
	}

	if got := waitForStartCount(t, countPath); got != 1 {
		t.Fatalf("runner starts = %d, want 1", got)
	}
}

func waitForStartCount(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(path) // #nosec G304 -- path is the temp-file counter owned by this test.
		if err == nil {
			count := strings.Count(string(data), "start\n")
			if count > 0 || time.Now().After(deadline) {
				return count
			}
		} else if !os.IsNotExist(err) {
			t.Fatalf("read start count: %v", err)
		}
		if time.Now().After(deadline) {
			return 0
		}
		time.Sleep(25 * time.Millisecond)
	}
}
