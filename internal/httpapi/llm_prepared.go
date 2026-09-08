package httpapi

import (
	"context"

	"github.com/mapledaemon/MagicHandy/internal/chat"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type llmGenerationKey struct{}

// preparedLLMProvider resolves the runtime only after the caller is admitted
// by llmRequests. A queued request cannot replace the currently active model,
// and a settings transition invalidates the prepared request's generation.
type preparedLLMProvider struct {
	server     *Server
	settings   config.LLMSettings
	generation uint64
}

func chatPromptBudget(settings config.LLMSettings) chat.PromptBudgetSettings {
	budget := chat.PromptBudgetSettings{MaxOutputTokens: settings.MaxOutputTokens}
	if settings.Provider == config.LLMProviderLlamaCPP && settings.LlamaCPPMode == config.LlamaCPPModeManaged {
		budget.ContextSize = settings.LlamaCPPContextSize
		budget.ReasoningBudgetTokens = managedLlamaReasoningBudget(settings, true)
	}
	return budget
}

func (s *Server) prepareLLMProvider(settings config.LLMSettings) (llm.Provider, error) {
	s.llm.mu.Lock()
	generation := s.llm.generation
	s.llm.mu.Unlock()
	return &preparedLLMProvider{server: s, settings: settings, generation: generation}, nil
}

func (p *preparedLLMProvider) StreamChat(ctx context.Context, request llm.ChatRequest, onDelta func(string) error) (string, error) {
	p.server.llm.mu.Lock()
	retired := p.generation != p.server.llm.generation
	p.server.llm.mu.Unlock()
	if retired {
		return "", context.Canceled
	}
	// Settings may have changed before this handle was prepared. Validate the
	// saved configuration as well as its generation before touching a runtime.
	current, _ := p.server.store.Snapshot()
	if llmRuntimeSettingsChanged(current.LLM, p.settings) {
		return "", context.Canceled
	}
	ctx = context.WithValue(ctx, llmGenerationKey{}, p.generation)
	provider, err := p.server.newLLMProvider(ctx, p.settings)
	if err != nil {
		return "", err
	}
	return provider.StreamChat(ctx, request, onDelta)
}

func (p *preparedLLMProvider) Status(ctx context.Context) llm.ProviderStatus {
	provider, err := p.server.resolveLLMProvider(ctx, p.settings, false)
	if err != nil {
		return llm.ProviderStatus{Provider: p.settings.Provider, Model: p.settings.Model, Message: err.Error()}
	}
	return provider.Status(ctx)
}
