package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type llmRuntime struct {
	provider   llm.Provider
	httpClient *http.Client
	mu         sync.Mutex
	cached     llm.Provider
	cacheKey   string
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

	key := llmCacheKey(settings)
	s.llm.mu.Lock()
	defer s.llm.mu.Unlock()
	if s.llm.cached != nil && s.llm.cacheKey == key {
		provider := s.llm.cached
		return provider, nil
	}
	if s.llm.cached != nil {
		closeLLMProvider(s.llm.cached)
	}
	s.llm.cached = nil
	s.llm.cacheKey = ""

	timeout := time.Duration(settings.RequestTimeoutMillis) * time.Millisecond
	options := llm.HTTPProviderOptions{
		BaseURL: selectedLLMBaseURL(settings),
		Model:   settings.Model,
		Client:  s.llm.httpClient,
		Timeout: timeout,
	}

	var provider llm.Provider
	var err error
	switch settings.Provider {
	case config.LLMProviderLlamaCPP:
		if settings.LlamaCPPMode == config.LlamaCPPModeManaged {
			provider, err = llm.NewManagedLlamaCPPProvider(llm.ManagedLlamaCPPOptions{
				HTTPProviderOptions: options,
				RunnerPath:          settings.LlamaCPPRunnerPath,
				ModelPath:           settings.LlamaCPPModelPath,
			})
		} else {
			provider, err = llm.NewLlamaCPPProvider(options)
		}
	case config.LLMProviderOllama:
		provider, err = llm.NewOllamaProvider(options)
	default:
		return nil, fmt.Errorf("unknown LLM provider %q", settings.Provider)
	}
	if err != nil {
		return nil, err
	}

	s.llm.cached = provider
	s.llm.cacheKey = key
	return provider, nil
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
		"llama_cpp_mode":     settings.LLM.LlamaCPPMode,
		"base_url":           selectedLLMBaseURL(settings.LLM),
		"model":              settings.LLM.Model,
		"prompt_set":         settings.LLM.PromptSet,
		"request_timeout_ms": settings.LLM.RequestTimeoutMillis,
		"managed_ready":      settings.LLM.LlamaCPPRunnerPath != "" && settings.LLM.LlamaCPPModelPath != "",
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

func (s *Server) handleLLMLoad(w http.ResponseWriter, r *http.Request) {
	settings, _ := s.store.Snapshot()
	provider, err := s.newLLMProvider(settings.LLM)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	loadable, ok := provider.(llm.LoadableProvider)
	if !ok {
		writeJSON(w, http.StatusOK, provider.Status(r.Context()))
		return
	}
	writeJSON(w, http.StatusOK, loadable.Load(r.Context()))
}

func (s *Server) handleLLMUnload(w http.ResponseWriter, r *http.Request) {
	settings, _ := s.store.Snapshot()
	provider, err := s.newLLMProvider(settings.LLM)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	loadable, ok := provider.(llm.LoadableProvider)
	if !ok {
		writeJSON(w, http.StatusOK, provider.Status(r.Context()))
		return
	}
	writeJSON(w, http.StatusOK, loadable.Unload(r.Context()))
}

func (s *Server) closeLLM() {
	s.llm.mu.Lock()
	provider := s.llm.cached
	s.llm.cached = nil
	s.llm.cacheKey = ""
	s.llm.mu.Unlock()
	closeLLMProvider(provider)
}

func closeLLMProvider(provider llm.Provider) {
	if closer, ok := provider.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

func llmCacheKey(settings config.LLMSettings) string {
	return strings.Join([]string{
		settings.Provider,
		settings.LlamaCPPMode,
		settings.LlamaCPPBaseURL,
		settings.LlamaCPPRunnerPath,
		settings.LlamaCPPModelPath,
		settings.OllamaBaseURL,
		settings.Model,
		fmt.Sprint(settings.RequestTimeoutMillis),
	}, "\x00")
}
