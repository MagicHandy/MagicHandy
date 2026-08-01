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

const llmAutoloadTimeout = 45 * time.Second

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
		if err := closeLLMProvider(s.llm.cached); err != nil {
			return nil, fmt.Errorf("close previous LLM provider: %w", err)
		}
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
				ContextSize:         settings.LlamaCPPContextSize,
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

func (s *Server) startLLMAutoload(settings config.LLMSettings) {
	if s.llm.provider != nil ||
		settings.Provider != config.LLMProviderLlamaCPP ||
		settings.LlamaCPPMode != config.LlamaCPPModeManaged ||
		settings.ManagedLoadPolicy != config.LLMManagedLoadStartup {
		return
	}

	s.llmAutoloadMu.Lock()
	if s.llmAutoloadCancel != nil {
		s.llmAutoloadMu.Unlock()
		return
	}
	s.llmAutoloadID++
	autoloadID := s.llmAutoloadID
	ctx, cancel := context.WithCancel(s.lifecycleCtx)
	s.llmAutoloadCancel = cancel
	s.llmAutoloadWG.Add(1)
	s.llmAutoloadMu.Unlock()

	go func() {
		defer func() {
			s.llmAutoloadMu.Lock()
			if s.llmAutoloadID == autoloadID {
				s.llmAutoloadCancel = nil
			}
			s.llmAutoloadMu.Unlock()
			s.llmAutoloadWG.Done()
		}()

		loadCtx, loadCancel := context.WithTimeout(ctx, llmAutoloadTimeout)
		defer loadCancel()
		started := time.Now()
		provider, err := s.newLLMProvider(loadCtx, settings)
		if err != nil {
			if loadCtx.Err() == nil {
				s.logger.Warn("managed LLM startup preload could not prepare", "error", err)
			}
			return
		}
		loadable, ok := provider.(llm.LoadableProvider)
		if !ok {
			return
		}
		status := loadable.Load(loadCtx)
		if loadCtx.Err() != nil {
			return
		}
		if !status.Available {
			s.logger.Warn("managed LLM startup preload failed",
				"elapsed_ms", time.Since(started).Milliseconds(), "message", status.Message)
			return
		}
		s.logger.Info("managed LLM startup preload complete",
			"model", settings.Model, "elapsed_ms", time.Since(started).Milliseconds())
	}()
}

func (s *Server) stopLLMAutoload() {
	s.llmAutoloadMu.Lock()
	cancel := s.llmAutoloadCancel
	s.llmAutoloadCancel = nil
	s.llmAutoloadID++
	s.llmAutoloadMu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.llmAutoloadWG.Wait()
}

func (s *Server) applyLLMSettingsTransition(previous, next config.LLMSettings) error {
	runtimeChanged := llmRuntimeSettingsChanged(previous, next)
	loadPolicyChanged := previous.ManagedLoadPolicy != next.ManagedLoadPolicy
	if !runtimeChanged && !loadPolicyChanged {
		return nil
	}
	s.stopLLMAutoload()
	var transitionErr error
	if runtimeChanged || next.ManagedLoadPolicy == config.LLMManagedLoadOnDemand {
		if err := s.closeLLM(); err != nil {
			transitionErr = fmt.Errorf("apply LLM settings: %w", err)
		}
	}
	s.startLLMAutoload(next)
	return transitionErr
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
	managed := settings.LLM.Provider == config.LLMProviderLlamaCPP && settings.LLM.LlamaCPPMode == config.LlamaCPPModeManaged
	state := map[string]any{
		"provider":                settings.LLM.Provider,
		"llama_cpp_mode":          settings.LLM.LlamaCPPMode,
		"base_url":                selectedLLMBaseURL(settings.LLM),
		"model":                   settings.LLM.Model,
		"prompt_set":              settings.LLM.PromptSet,
		"request_timeout_ms":      settings.LLM.RequestTimeoutMillis,
		"llama_cpp_context_size":  settings.LLM.LlamaCPPContextSize,
		"max_output_tokens":       settings.LLM.MaxOutputTokens,
		"reasoning_mode":          settings.LLM.ReasoningMode,
		"managed_load_policy":     settings.LLM.ManagedLoadPolicy,
		"model_manager_available": false,
	}
	var managedRuntimeInstalled bool
	if managed {
		runtimeStatus := s.managedLLM.Snapshot().Runtime
		state["managed_runtime"] = runtimeStatus.State
		state["managed_ready"] = false
		managedRuntimeInstalled = runtimeStatus.Installed
	}
	if s.models != nil {
		if snapshot, err := s.models.Snapshot(ctx); err == nil {
			state["model_manager_available"] = true
			activeImports := 0
			for _, job := range snapshot.Imports {
				if job.Status == llm.ImportStatusQueued || job.Status == llm.ImportStatusCopying {
					activeImports++
				}
			}
			state["managed_model_count"] = len(snapshot.Models)
			state["active_import_count"] = activeImports
			if managed && managedRuntimeInstalled {
				for _, model := range snapshot.Models {
					if model.ID == settings.LLM.Model {
						state["managed_ready"] = model.State == "ready"
						break
					}
				}
			}
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

func (s *Server) closeLLM() error {
	s.llm.mu.Lock()
	defer s.llm.mu.Unlock()
	provider := s.llm.cached
	if err := closeLLMProvider(provider); err != nil {
		return err
	}
	s.llm.cached = nil
	s.llm.cacheKey = ""
	return nil
}

func closeLLMProvider(provider llm.Provider) error {
	if closer, ok := provider.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func llmCacheKey(settings config.LLMSettings, managedKey string) string {
	parts := []string{
		settings.Provider,
		settings.Model,
		fmt.Sprint(settings.RequestTimeoutMillis),
	}
	switch settings.Provider {
	case config.LLMProviderLlamaCPP:
		parts = append(parts, settings.LlamaCPPMode, selectedLLMBaseURL(settings))
		if settings.LlamaCPPMode == config.LlamaCPPModeManaged {
			parts = append(parts, fmt.Sprint(settings.LlamaCPPContextSize), managedKey)
		}
	case config.LLMProviderOllama:
		parts = append(parts, settings.OllamaBaseURL)
	default:
		parts = append(parts, settings.LlamaCPPMode, settings.LlamaCPPBaseURL, settings.OllamaBaseURL, managedKey)
	}
	return strings.Join(parts, "\x00")
}

func llmRuntimeSettingsChanged(previous, next config.LLMSettings) bool {
	return llmCacheKey(previous, "") != llmCacheKey(next, "")
}
