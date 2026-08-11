package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
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
		TopP:                  0.95,
		RepeatPenalty:         1.2,
		RepeatLastN:           40,
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
	if body["top_p"] != 0.95 || body["repeat_penalty"] != 1.2 || body["repeat_last_n"] != float64(40) {
		t.Fatalf("sampling controls = %#v", body)
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
	for _, key := range []string{"top_p", "repeat_penalty", "repeat_last_n"} {
		if _, ok := body[key]; ok {
			t.Fatalf("repair request unexpectedly included %q: %#v", key, body)
		}
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
		Temperature:   0.3,
		TopP:          0.95,
		RepeatPenalty: 1.2,
		RepeatLastN:   40,
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
	if options["temperature"] != 0.3 || options["top_p"] != 0.95 || options["repeat_penalty"] != 1.2 || options["repeat_last_n"] != float64(40) {
		t.Fatalf("sampling controls = %#v", options)
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

func TestProviderStreamsRequireCompletionMarker(t *testing.T) {
	ollama := `{"message":{"content":"partial"},"done":false}` + "\n"
	if raw, err := readOllamaStream(strings.NewReader(ollama), nil); raw != "partial" || !errors.Is(err, errIncompleteStream) {
		t.Fatalf("readOllamaStream incomplete = %q, %v", raw, err)
	}

	llama := "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"
	if raw, err := readOpenAIEventStream(strings.NewReader(llama), nil); raw != "partial" || !errors.Is(err, errIncompleteStream) {
		t.Fatalf("readOpenAIEventStream incomplete = %q, %v", raw, err)
	}

	terminalWithoutDone := "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n"
	if raw, err := readOpenAIEventStream(strings.NewReader(terminalWithoutDone), nil); raw != "" || err != nil {
		t.Fatalf("terminal finish reason = %q, %v", raw, err)
	}
}

func TestProviderStreamsBoundAggregateOutput(t *testing.T) {
	delta := strings.Repeat("x", maxStreamResponseBytes/2+1)
	chunk := "data: " + mustJSON(t, map[string]any{
		"choices": []any{map[string]any{"delta": map[string]any{"content": delta}}},
	}) + "\n\n"
	stream := chunk + chunk
	if _, err := readOpenAIEventStream(strings.NewReader(stream), nil); !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("oversized stream error = %v", err)
	}
}

func TestProvidersRejectUnsafeBaseURLs(t *testing.T) {
	for _, baseURL := range []string{
		"file:///tmp/model",
		"http://user:secret@127.0.0.1:11434",
		"http://127.0.0.1:11434?token=secret",
		"http://127.0.0.1:11434/#fragment",
	} {
		if _, err := NewOllamaProvider(HTTPProviderOptions{BaseURL: baseURL, Model: "model"}); err == nil {
			t.Fatalf("NewOllamaProvider accepted %q", baseURL)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
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
	if message := os.Getenv("MAGICHANDY_TEST_LLAMA_RUNNER_EXIT"); message != "" {
		_, _ = os.Stderr.WriteString(message + "\n")
		return
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

func TestLlamaCPPStatusDistinguishesModelLoadingFromFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"Loading model","type":"unavailable_error","code":503}}`))
	}))
	defer server.Close()

	provider, err := NewLlamaCPPProvider(HTTPProviderOptions{
		BaseURL: server.URL,
		Model:   "local-model",
	})
	if err != nil {
		t.Fatalf("NewLlamaCPPProvider: %v", err)
	}

	status := provider.Status(t.Context())
	if status.Available || !status.Loaded || !status.Loading {
		t.Fatalf("loading status = %+v", status)
	}
	if status.Message != "llama.cpp is loading the model" {
		t.Fatalf("loading message = %q", status.Message)
	}
}

func TestLlamaCPPStatusKeepsUnknownServiceUnavailableAsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"model is not loading because the runtime is unavailable"}}`))
	}))
	defer server.Close()

	provider, err := NewLlamaCPPProvider(HTTPProviderOptions{BaseURL: server.URL, Model: "local-model"})
	if err != nil {
		t.Fatalf("NewLlamaCPPProvider: %v", err)
	}
	status := provider.Status(t.Context())
	if status.Loading || status.Message != "health endpoint returned 503" {
		t.Fatalf("unavailable status = %+v", status)
	}
}

func TestManagedLlamaCPPStatusTreatsAmbiguousServiceUnavailableAsLoading(t *testing.T) {
	status := normalizeManagedLlamaLoadingStatus(ProviderStatus{
		Loaded:  true,
		Message: "health endpoint returned 503",
	})
	if !status.Loading || status.Message != "llama.cpp is loading the model" {
		t.Fatalf("managed loading status = %+v", status)
	}

	for _, unchanged := range []ProviderStatus{
		{Loaded: false, Message: "health endpoint returned 503"},
		{Loaded: true, Message: "health endpoint returned 500"},
		{Loaded: true, Available: true, Message: "ready"},
	} {
		got := normalizeManagedLlamaLoadingStatus(unchanged)
		if got.Loaded != unchanged.Loaded || got.Available != unchanged.Available ||
			got.Loading != unchanged.Loading || got.Message != unchanged.Message {
			t.Fatalf("normalizeManagedLlamaLoadingStatus(%+v) = %+v", unchanged, got)
		}
	}
}

func TestManagedLlamaCPPStatusRequiresManagedRuntimeAndModel(t *testing.T) {
	provider, err := NewManagedLlamaCPPProvider(ManagedLlamaCPPOptions{
		HTTPProviderOptions: HTTPProviderOptions{
			BaseURL: "http://127.0.0.1:8080",
			Model:   "local-model",
		},
		ContextSize: 32768,
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

func TestManagedLlamaCPPRejectsNonpositiveContextSize(t *testing.T) {
	for _, contextSize := range []int{0, -1} {
		_, err := NewManagedLlamaCPPProvider(ManagedLlamaCPPOptions{
			HTTPProviderOptions: HTTPProviderOptions{
				BaseURL: "http://127.0.0.1:8080",
				Model:   "local-model",
			},
			ContextSize: contextSize,
		})
		if err == nil || !strings.Contains(err.Error(), "context size must be positive") {
			t.Fatalf("context size %d error = %v", contextSize, err)
		}
	}
}

func TestManagedLlamaCPPRequiresLoopbackHTTPBaseURL(t *testing.T) {
	for _, baseURL := range []string{
		"https://127.0.0.1:8080",
		"http://192.168.1.20:8080",
		"http://127.0.0.1:8080/prefix",
	} {
		if _, _, err := llamaHostPort(baseURL); err == nil {
			t.Fatalf("llamaHostPort accepted %q", baseURL)
		}
	}
	for _, baseURL := range []string{
		"http://127.0.0.1:8080",
		"http://localhost:8080",
		"http://0.0.0.0:8080",
	} {
		if _, _, err := llamaHostPort(baseURL); err != nil {
			t.Fatalf("llamaHostPort rejected %q: %v", baseURL, err)
		}
	}
}

func TestManagedLlamaCPPSelectsFallbackWhenPreferredPortIsOccupied(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on occupied test port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	preferredPort := listener.Addr().(*net.TCPAddr).Port
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("test model"), 0o600); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	argsPath := filepath.Join(dir, "args.txt")
	countPath := filepath.Join(dir, "starts.txt")
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER", "1")
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER_ARGS", argsPath)
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER_COUNT", countPath)

	provider, err := NewManagedLlamaCPPProvider(ManagedLlamaCPPOptions{
		HTTPProviderOptions: HTTPProviderOptions{
			BaseURL: fmt.Sprintf("http://127.0.0.1:%d", preferredPort),
			Model:   "local-model",
		},
		RunnerPath:  os.Args[0],
		ModelPath:   modelPath,
		ContextSize: 32768,
	})
	if err != nil {
		t.Fatalf("NewManagedLlamaCPPProvider: %v", err)
	}
	t.Cleanup(func() {
		provider.Unload(context.Background())
	})
	if err := provider.ensureStarted(); err != nil {
		t.Fatalf("ensureStarted: %v", err)
	}
	if got := waitForStartCount(t, countPath); got != 1 {
		t.Fatalf("runner starts = %d, want 1", got)
	}

	selectedBaseURL := provider.baseStatus().BaseURL
	_, selectedPort, err := llamaHostPort(selectedBaseURL)
	if err != nil {
		t.Fatalf("selected base URL %q: %v", selectedBaseURL, err)
	}
	if selectedPort == preferredPort {
		t.Fatalf("selected occupied port %d", preferredPort)
	}
	if provider.clientSnapshot().baseURL != selectedBaseURL {
		t.Fatalf("HTTP client base URL = %q, want %q", provider.clientSnapshot().baseURL, selectedBaseURL)
	}
	arguments, err := os.ReadFile(argsPath) // #nosec G304 -- temp fixture path.
	if err != nil {
		t.Fatalf("read runner arguments: %v", err)
	}
	if !strings.Contains(string(arguments), fmt.Sprintf("--port\n%d", selectedPort)) {
		t.Fatalf("runner arguments %q do not use selected port %d", arguments, selectedPort)
	}
}

func TestManagedLlamaCPPLoadReportsEarlyRunnerExit(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("test model"), 0o600); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER", "1")
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER_EXIT", "test runner could not bind")

	provider, err := NewManagedLlamaCPPProvider(ManagedLlamaCPPOptions{
		HTTPProviderOptions: HTTPProviderOptions{
			BaseURL: "http://127.0.0.1:18080",
			Model:   "local-model",
		},
		RunnerPath:  os.Args[0],
		ModelPath:   modelPath,
		ContextSize: 32768,
	})
	if err != nil {
		t.Fatalf("NewManagedLlamaCPPProvider: %v", err)
	}

	started := time.Now()
	status := provider.Load(t.Context())
	if status.Available || status.Loaded {
		t.Fatalf("status = %+v, want exited runner", status)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("early runner exit took %s to report", elapsed)
	}
	if !strings.Contains(status.Message, "exited before becoming ready") || !strings.Contains(status.Message, "test runner could not bind") {
		t.Fatalf("status message = %q, want early-exit diagnostics", status.Message)
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
		RunnerPath:  os.Args[0],
		ModelPath:   modelPath,
		ContextSize: 65536,
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
	for _, required := range []string{
		"--offline",
		"--no-ui",
		"--alias",
		"local-model",
		"--ctx-size\n65536",
		"--parallel\n1",
		"-m",
		modelPath,
	} {
		if !strings.Contains(arguments, required) {
			t.Fatalf("runner arguments %q do not contain %q", arguments, required)
		}
	}
	for _, option := range []string{"--ctx-size", "--parallel"} {
		if count := strings.Count(arguments, option); count != 1 {
			t.Fatalf("runner arguments %q contain %q %d times, want once", arguments, option, count)
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
