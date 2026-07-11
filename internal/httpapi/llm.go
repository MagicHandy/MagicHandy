package httpapi

import (
	"context"
	"errors"
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

func (s *Server) newLLMProvider(ctx context.Context, settings config.LLMSettings) (llm.Provider, error) {
	if s.llm.provider != nil {
		return s.llm.provider, nil
	}

	var managedRunnerPath, managedModelPath, managedKey string
	if settings.Provider == config.LLMProviderLlamaCPP && settings.LlamaCPPMode == config.LlamaCPPModeManaged {
		runtimeSnapshot := s.managedLLM.Snapshot()
		if managedRuntimeBuildInProgress(runtimeSnapshot.Build) {
			return nil, errors.New("managed llama.cpp source build is in progress")
		}
		runtimeStatus := runtimeSnapshot.Runtime
		if !runtimeStatus.Installed {
			return nil, fmt.Errorf("managed llama.cpp unavailable: %s", runtimeStatus.Message)
		}
		model, err := s.models.Model(ctx, settings.Model)
		if err != nil {
			return nil, fmt.Errorf("selected managed llama.cpp model %q is unavailable: %w", settings.Model, err)
		}
		if model.State != "ready" {
			return nil, fmt.Errorf("selected managed llama.cpp model %q is %s: %s", settings.Model, model.State, model.Message)
		}
		managedRunnerPath = runtimeStatus.RunnerPath
		managedModelPath = model.ModelPath
		managedKey = strings.Join([]string{runtimeStatus.Commit, runtimeStatus.Backend, managedRunnerPath, model.ID, model.UpdatedAt, managedModelPath}, "\x00")
	}

	key := llmCacheKey(settings, managedKey)
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
				RunnerPath:          managedRunnerPath,
				ModelPath:           managedModelPath,
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

func managedRuntimeBuildInProgress(build *llm.ManagedLlamaRuntimeBuild) bool {
	return build != nil && (build.Status == llm.RuntimeBuildStatusQueued || build.Status == llm.RuntimeBuildStatusBuilding)
}

func selectedLLMBaseURL(settings config.LLMSettings) string {
	switch settings.Provider {
	case config.LLMProviderOllama:
		return settings.OllamaBaseURL
	case config.LLMProviderLlamaCPP:
		if settings.LlamaCPPMode == config.LlamaCPPModeManaged {
			return config.DefaultLlamaCPPBaseURL
		}
		return settings.LlamaCPPBaseURL
	default:
		return settings.LlamaCPPBaseURL
	}
}

func (s *Server) llmState(ctx context.Context) any {
	settings, _ := s.store.Snapshot()
	state := map[string]any{
		"provider":           settings.LLM.Provider,
		"llama_cpp_mode":     settings.LLM.LlamaCPPMode,
		"base_url":           selectedLLMBaseURL(settings.LLM),
		"model":              settings.LLM.Model,
		"prompt_set":         settings.LLM.PromptSet,
		"request_timeout_ms": settings.LLM.RequestTimeoutMillis,
	}
	if settings.LLM.Provider == config.LLMProviderLlamaCPP && settings.LLM.LlamaCPPMode == config.LlamaCPPModeManaged {
		runtimeStatus := s.managedLLM.Snapshot().Runtime
		state["managed_runtime"] = runtimeStatus.State
		model, err := s.models.Model(ctx, settings.LLM.Model)
		state["managed_ready"] = runtimeStatus.Installed && err == nil && model.State == "ready"
	}
	if s.models != nil {
		if snapshot, err := s.models.Snapshot(ctx); err == nil {
			activeImports := 0
			for _, job := range snapshot.Imports {
				if job.Status == llm.ImportStatusQueued || job.Status == llm.ImportStatusCopying {
					activeImports++
				}
			}
			state["managed_model_count"] = len(snapshot.Models)
			state["active_import_count"] = activeImports
		}
	}
	return state
}

func (s *Server) handleLLMStatus(w http.ResponseWriter, r *http.Request) {
	settings, _ := s.store.Snapshot()
	provider, err := s.newLLMProvider(r.Context(), settings.LLM)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"provider":  settings.LLM.Provider,
			"base_url":  selectedLLMBaseURL(settings.LLM),
			"model":     settings.LLM.Model,
			"available": false,
			"managed":   settings.LLM.Provider == config.LLMProviderLlamaCPP && settings.LLM.LlamaCPPMode == config.LlamaCPPModeManaged,
			"loaded":    false,
			"message":   err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, provider.Status(r.Context()))
}

func (s *Server) handleLLMLoad(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	settings, _ := s.store.Snapshot()
	provider, err := s.newLLMProvider(r.Context(), settings.LLM)
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
	if !s.requireController(w, r) {
		return
	}
	settings, _ := s.store.Snapshot()
	provider, err := s.newLLMProvider(r.Context(), settings.LLM)
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

func llmCacheKey(settings config.LLMSettings, managedKey string) string {
	return strings.Join([]string{
		settings.Provider,
		settings.LlamaCPPMode,
		settings.LlamaCPPBaseURL,
		settings.OllamaBaseURL,
		settings.Model,
		fmt.Sprint(settings.RequestTimeoutMillis),
		managedKey,
	}, "\x00")
}
