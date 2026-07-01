package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type llmRuntime struct {
	provider   llm.Provider
	httpClient *http.Client
}

func newLLMRuntime(runtime Runtime) llmRuntime {
	return llmRuntime{
		provider:   runtime.LLMProvider,
		httpClient: runtime.LLMHTTPClient,
	}
}

func (s *Server) newLLMProvider(settings config.LLMSettings) (llm.Provider, error) {
	if s.llm.provider != nil {
		return s.llm.provider, nil
	}

	timeout := time.Duration(settings.RequestTimeoutMillis) * time.Millisecond
	options := llm.HTTPProviderOptions{
		BaseURL: selectedLLMBaseURL(settings),
		Model:   settings.Model,
		Client:  s.llm.httpClient,
		Timeout: timeout,
	}

	switch settings.Provider {
	case config.LLMProviderLlamaCPP:
		return llm.NewLlamaCPPProvider(options)
	case config.LLMProviderOllama:
		return llm.NewOllamaProvider(options)
	default:
		return nil, fmt.Errorf("unknown LLM provider %q", settings.Provider)
	}
}

func selectedLLMBaseURL(settings config.LLMSettings) string {
	switch settings.Provider {
	case config.LLMProviderOllama:
		return settings.OllamaBaseURL
	default:
		return settings.LlamaCPPBaseURL
	}
}

func (s *Server) llmState(_ context.Context) any {
	settings, _ := s.store.Snapshot()
	return map[string]any{
		"provider":           settings.LLM.Provider,
		"base_url":           selectedLLMBaseURL(settings.LLM),
		"model":              settings.LLM.Model,
		"prompt_set":         settings.LLM.PromptSet,
		"request_timeout_ms": settings.LLM.RequestTimeoutMillis,
	}
}

func (s *Server) handleLLMStatus(w http.ResponseWriter, r *http.Request) {
	settings, _ := s.store.Snapshot()
	provider, err := s.newLLMProvider(settings.LLM)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"provider":  settings.LLM.Provider,
			"base_url":  selectedLLMBaseURL(settings.LLM),
			"model":     settings.LLM.Model,
			"available": false,
			"message":   err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, provider.Status(r.Context()))
}
