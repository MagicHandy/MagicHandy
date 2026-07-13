package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLlamaCPPStreamChatSendsGenerationControls(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = make(map[string]any)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{}\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewLlamaCPPProvider(HTTPProviderOptions{BaseURL: server.URL, Model: "test-model"})
	if err != nil {
		t.Fatalf("NewLlamaCPPProvider: %v", err)
	}
	_, err = provider.StreamChat(t.Context(), ChatRequest{
		Messages:              []Message{{Role: "user", Content: "test"}},
		Temperature:           0,
		MaxTokens:             256,
		ReasoningMode:         "auto",
		ReasoningBudgetTokens: 128,
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if body["temperature"] != float64(0) || body["max_tokens"] != float64(256) {
		t.Fatalf("generation controls = %#v", body)
	}
	if body["thinking_budget_tokens"] != float64(128) {
		t.Fatalf("thinking budget = %#v", body["thinking_budget_tokens"])
	}
	if _, ok := body["chat_template_kwargs"]; ok {
		t.Fatalf("automatic reasoning unexpectedly disabled: %#v", body["chat_template_kwargs"])
	}
	_, err = provider.StreamChat(t.Context(), ChatRequest{
		Messages:      []Message{{Role: "user", Content: "repair"}},
		Temperature:   0,
		MaxTokens:     256,
		ReasoningMode: "off",
	}, nil)
	if err != nil {
		t.Fatalf("repair StreamChat: %v", err)
	}
	kwargs, ok := body["chat_template_kwargs"].(map[string]any)
	if !ok || kwargs["enable_thinking"] != false {
		t.Fatalf("chat template kwargs = %#v", body["chat_template_kwargs"])
	}
}

func TestOllamaStreamChatSendsGenerationControls(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"message\":{\"content\":\"{}\"},\"done\":true}\n"))
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(HTTPProviderOptions{BaseURL: server.URL, Model: "test-model"})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}
	_, err = provider.StreamChat(t.Context(), ChatRequest{
		Messages:      []Message{{Role: "user", Content: "test"}},
		Temperature:   0.2,
		MaxTokens:     512,
		ReasoningMode: "off",
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	options, ok := body["options"].(map[string]any)
	if !ok || options["num_predict"] != float64(512) || body["think"] != false {
		t.Fatalf("generation controls = %#v", body)
	}
}

func TestLlamaCPPStreamReportsTokenLimit(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n"
	raw, err := readOpenAIEventStream(strings.NewReader(stream), nil)
	if raw != "" || !errors.Is(err, ErrOutputTruncated) {
		t.Fatalf("readOpenAIEventStream = %q, %v", raw, err)
	}
}

func TestOllamaStreamReportsTokenLimit(t *testing.T) {
	stream := "{\"message\":{\"content\":\"\"},\"done\":true,\"done_reason\":\"length\"}\n"
	raw, err := readOllamaStream(strings.NewReader(stream), nil)
	if raw != "" || !errors.Is(err, ErrOutputTruncated) {
		t.Fatalf("readOllamaStream = %q, %v", raw, err)
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("MAGICHANDY_TEST_LLAMA_RUNNER") == "1" {
		runManagedLlamaRunnerHelper()
		return
	}
	os.Exit(m.Run())
}

func runManagedLlamaRunnerHelper() {
	if path := os.Getenv("MAGICHANDY_TEST_LLAMA_RUNNER_ARGS"); path != "" {
		_ = os.WriteFile(path, []byte(strings.Join(os.Args[1:], "\n")), 0o600) // #nosec G306,G703 -- test fixture path injected by its parent.
	}
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

func TestManagedLlamaCPPStatusRequiresManagedRuntimeAndModel(t *testing.T) {
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
	if !strings.Contains(status.Message, "runtime is not installed") {
		t.Fatalf("status message = %q, want managed runtime setup error", status.Message)
	}
}

func TestManagedLlamaCPPEnsureStartedIsSerialized(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("test model"), 0o600); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	countPath := filepath.Join(dir, "starts.txt")
	argsPath := filepath.Join(dir, "args.txt")
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER", "1")
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER_COUNT", countPath)
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER_ARGS", argsPath)

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
	args, err := os.ReadFile(argsPath) // #nosec G304 -- temp fixture path.
	if err != nil {
		t.Fatalf("read runner arguments: %v", err)
	}
	arguments := string(args)
	for _, required := range []string{"--offline", "--no-ui", "--alias", "local-model", "-m", modelPath} {
		if !strings.Contains(arguments, required) {
			t.Fatalf("runner arguments %q do not contain %q", arguments, required)
		}
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
