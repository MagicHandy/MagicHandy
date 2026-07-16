package httpapi

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type closeTrackingLLMProvider struct {
	closes   atomic.Int32
	closeErr error
}

func (*closeTrackingLLMProvider) StreamChat(context.Context, llm.ChatRequest, func(string) error) (string, error) {
	return "", nil
}

func (*closeTrackingLLMProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{}
}

func (p *closeTrackingLLMProvider) Close() error {
	p.closes.Add(1)
	return p.closeErr
}

func TestLLMCacheKeyIgnoresPromptSet(t *testing.T) {
	settings := config.DefaultSettings().LLM
	settings.PromptSet = "prompt-a"
	first := llmCacheKey(settings, "runtime-a")

	settings.PromptSet = "prompt-b"
	if got := llmCacheKey(settings, "runtime-a"); got != first {
		t.Fatalf("cache key changed after prompt set update; provider should not reload for prompt-only changes")
	}

	settings.Model = "other-model"
	if got := llmCacheKey(settings, "runtime-a"); got == first {
		t.Fatal("cache key did not change after model update")
	}

	settings.Model = "local-model"
	if got := llmCacheKey(settings, "runtime-b"); got == first {
		t.Fatal("cache key did not change after managed runtime update")
	}
}

func TestLLMCacheKeyIgnoresInactiveProviderSettings(t *testing.T) {
	settings := config.DefaultSettings().LLM
	first := llmCacheKey(settings, "runtime-a")

	settings.OllamaBaseURL = "http://192.0.2.20:11434"
	if got := llmCacheKey(settings, "runtime-a"); got != first {
		t.Fatal("managed llama.cpp cache key changed after inactive Ollama URL update")
	}
	settings.LlamaCPPBaseURL = "http://192.0.2.10:9000"
	if got := llmCacheKey(settings, "runtime-a"); got != first {
		t.Fatal("managed llama.cpp cache key changed after its ignored external URL update")
	}

	settings.Provider = config.LLMProviderOllama
	first = llmCacheKey(settings, "runtime-a")
	settings.LlamaCPPMode = config.LlamaCPPModeExternal
	settings.LlamaCPPBaseURL = "http://192.0.2.30:9000"
	if got := llmCacheKey(settings, "runtime-b"); got != first {
		t.Fatal("Ollama cache key changed after inactive llama.cpp settings update")
	}
}

func TestLLMSettingsTransitionClosesStaleProvider(t *testing.T) {
	server := newTestServer(t)
	provider := &closeTrackingLLMProvider{}
	settings, _ := server.store.Snapshot()
	server.llm.cached = provider
	server.llm.cacheKey = llmCacheKey(settings.LLM, "runtime-a")

	next := settings
	next.LLM.Model = "replacement-model"
	if err := server.applySettingsRuntimeTransition(context.Background(), settings, next); err != nil {
		t.Fatalf("applySettingsRuntimeTransition: %v", err)
	}
	if got := provider.closes.Load(); got != 1 {
		t.Fatalf("provider close count = %d, want 1", got)
	}
	if server.llm.cached != nil || server.llm.cacheKey != "" {
		t.Fatal("closed provider remained in the runtime cache")
	}
}

func TestLLMSettingsTransitionKeepsProviderForRequestOnlyChanges(t *testing.T) {
	server := newTestServer(t)
	provider := &closeTrackingLLMProvider{}
	settings, _ := server.store.Snapshot()
	server.llm.cached = provider
	server.llm.cacheKey = llmCacheKey(settings.LLM, "runtime-a")

	next := settings
	next.LLM.PromptSet = "other-prompt"
	next.LLM.MaxOutputTokens++
	next.LLM.ReasoningMode = config.LLMReasoningAuto
	if err := server.applySettingsRuntimeTransition(context.Background(), settings, next); err != nil {
		t.Fatalf("applySettingsRuntimeTransition: %v", err)
	}
	if got := provider.closes.Load(); got != 0 {
		t.Fatalf("provider close count = %d, want 0", got)
	}
	server.llm.cached = nil
}

func TestLLMSettingsTransitionRetainsProviderWhenCloseFails(t *testing.T) {
	server := newTestServer(t)
	wantErr := errors.New("runner still stopping")
	provider := &closeTrackingLLMProvider{closeErr: wantErr}
	settings, _ := server.store.Snapshot()
	server.llm.cached = provider
	server.llm.cacheKey = llmCacheKey(settings.LLM, "runtime-a")

	next := settings
	next.LLM.Model = "replacement-model"
	err := server.applySettingsRuntimeTransition(context.Background(), settings, next)
	if !errors.Is(err, wantErr) {
		t.Fatalf("applySettingsRuntimeTransition error = %v, want %v", err, wantErr)
	}
	if server.llm.cached != provider {
		t.Fatal("failed-to-close provider was discarded instead of remaining retryable")
	}
	provider.closeErr = nil
}

func TestSelectedLLMBaseURLKeepsManagedRunnerOnLoopback(t *testing.T) {
	settings := config.DefaultSettings().LLM
	settings.LlamaCPPBaseURL = "http://192.0.2.10:9000"
	if got := selectedLLMBaseURL(settings); got != config.DefaultLlamaCPPBaseURL {
		t.Fatalf("managed llama.cpp URL = %q, want %q", got, config.DefaultLlamaCPPBaseURL)
	}

	settings.LlamaCPPMode = config.LlamaCPPModeExternal
	if got := selectedLLMBaseURL(settings); got != "http://192.0.2.10:9000" {
		t.Fatalf("external llama.cpp URL = %q", got)
	}

	settings.Provider = config.LLMProviderOllama
	settings.OllamaBaseURL = "http://192.0.2.20:11434"
	if got := selectedLLMBaseURL(settings); got != settings.OllamaBaseURL {
		t.Fatalf("Ollama URL = %q", got)
	}
}
