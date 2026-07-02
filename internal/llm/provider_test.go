package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
