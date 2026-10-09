package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type hostedRequestLanes struct {
	mu    sync.Mutex
	lanes map[string]*llmRequestCoordinator
}

func (l *hostedRequestLanes) lane(key string) *llmRequestCoordinator {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lanes == nil {
		l.lanes = map[string]*llmRequestCoordinator{}
	}
	if l.lanes[key] == nil {
		l.lanes[key] = &llmRequestCoordinator{}
	}
	return l.lanes[key]
}

func (l *hostedRequestLanes) invalidate() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, lane := range l.lanes {
		lane.invalidate()
	}
}

func (s *Server) acquireConversationRequest(ctx context.Context, priority llmRequestPriority) (context.Context, time.Duration, func(), error) {
	settings, _ := s.store.Snapshot()
	return s.acquireProviderRequest(ctx, priority, settings.LLM.ConversationSettings())
}

func (s *Server) acquireProviderRequest(ctx context.Context, priority llmRequestPriority, settings config.LLMSettings) (context.Context, time.Duration, func(), error) {
	settings = settings.ConversationSettings()
	if !settings.IsHosted() {
		return s.llmRequests.acquire(ctx, priority)
	}
	connection := settings.ActiveConnection
	if connection == nil {
		return nil, 0, nil, errors.New("hosted model connection is unavailable")
	}
	return s.hostedRequests.lane(connection.ID+"\x00"+connection.Provider+"\x00"+connection.BaseURL).acquire(ctx, priority)
}

func hostedProviderKey(settings config.LLMSettings) string {
	encoded, _ := json.Marshal(settings.ActiveConnection)
	return string(encoded) + fmt.Sprint(settings.RequestTimeoutMillis)
}

func (s *Server) hostedProvider(ctx context.Context, settings config.LLMSettings) (llm.Provider, error) {
	connection := settings.ActiveConnection
	if connection == nil || s.cloudPlanning.auth == nil {
		return nil, &llm.CloudError{Kind: "unavailable"}
	}
	key := hostedProviderKey(settings)
	s.cloudPlanning.mu.Lock()
	provider := s.cloudPlanning.providers[key]
	revision := s.cloudPlanning.revision
	s.cloudPlanning.mu.Unlock()
	if provider != nil {
		return provider, nil
	}
	provider, err := s.newHostedProvider(ctx, *connection, time.Duration(settings.RequestTimeoutMillis)*time.Millisecond)
	if err != nil {
		return nil, err
	}
	s.cloudPlanning.mu.Lock()
	defer s.cloudPlanning.mu.Unlock()
	if revision != s.cloudPlanning.revision {
		return nil, &llm.CloudError{Kind: "stale"}
	}
	if s.cloudPlanning.providers == nil {
		s.cloudPlanning.providers = map[string]llm.Provider{}
	}
	// Manual model comparisons can temporarily create additional providers.
	// Keep that metadata cache bounded independently of request admission.
	if len(s.cloudPlanning.providers) >= 32 {
		clear(s.cloudPlanning.providers)
	}
	s.cloudPlanning.providers[key] = provider
	return provider, nil
}

func (s *Server) newHostedProvider(ctx context.Context, connection config.ModelConnection, timeout time.Duration) (llm.Provider, error) {
	connection = connection.Normalize()
	var token llm.TokenSource
	var err error
	if connection.Provider == config.LLMProviderChatGPT {
		token, err = s.cloudPlanning.auth.ActiveTokenSource(ctx)
	} else if !connection.NoAuthentication {
		token = s.cloudPlanning.auth.ConnectionKeySource(connection)
	}
	if err != nil {
		return nil, err
	}
	switch connection.Provider {
	case config.LLMProviderChatGPT, config.LLMProviderOpenAI:
		baseURL := connection.BaseURL
		if s.cloudPlanning.baseURL != "" {
			baseURL = s.cloudPlanning.baseURL
		}
		return llm.NewOpenAIResponsesProvider(llm.OpenAIOptions{Model: connection.Model, ReasoningEffort: connection.ReasoningEffort, Token: token, PlanUsage: connection.Provider == config.LLMProviderChatGPT, Client: s.cloudPlanning.client, Timeout: timeout, BaseURL: baseURL})
	case config.LLMProviderOpenRouter, config.LLMProviderCompatible:
		return llm.NewCompatibleProvider(llm.CompatibleOptions{HTTPProviderOptions: llm.HTTPProviderOptions{BaseURL: connection.BaseURL, Model: connection.Model, Client: s.cloudPlanning.client, Timeout: timeout}, Provider: connection.Provider, Token: token, OutputMode: connection.OutputMode, SupportedParameters: connection.SupportedParameters, AllowFallbacks: connection.AllowFallbacks, AllowedProviders: connection.AllowedProviders, DataCollection: connection.DataCollection, ZeroDataRetention: connection.ZeroDataRetention})
	default:
		return nil, errors.New("unsupported hosted model protocol")
	}
}
