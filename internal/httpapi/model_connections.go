package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func (s *Server) modelConnectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/llm/connections/key", s.handleModelConnectionKey)
	mux.HandleFunc("POST /api/llm/connections/models", s.handleModelConnectionModels)
	mux.HandleFunc("POST /api/llm/connections/test", s.handleModelConnectionTest)
}

type modelConnectionRequest struct {
	Connection config.ModelConnection `json:"connection"`
	APIKey     *string                `json:"api_key,omitempty"`
}

func decodeModelConnection(w http.ResponseWriter, r *http.Request) (modelConnectionRequest, bool) {
	var body modelConnectionRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return body, false
	}
	body.Connection = body.Connection.Normalize()
	if err := body.Connection.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return body, false
	}
	return body, true
}

func (s *Server) handleModelConnectionKey(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.cloudManager(w, r, true)
	if !ok {
		return
	}
	body, ok := decodeModelConnection(w, r)
	if !ok {
		return
	}
	// Omission reads only the exact binding; an explicit empty string removes it.
	if body.APIKey == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"key_set": manager.ConnectionKeySet(r.Context(), body.Connection)})
		return
	}
	s.invalidateCloudPlanning()
	if err := manager.SetConnectionKey(r.Context(), body.Connection, *body.APIKey); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"key_set": manager.ConnectionKeySet(r.Context(), body.Connection)})
}

func (s *Server) handleModelConnectionModels(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.cloudManager(w, r, true); !ok {
		return
	}
	body, ok := decodeModelConnection(w, r)
	if !ok {
		return
	}
	if body.Connection.Model == "" {
		body.Connection.Model = "catalog"
	}
	provider, err := s.newHostedProvider(r.Context(), body.Connection, 20*time.Second)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	models := []llm.HostedModel{}
	switch modelProvider := provider.(type) {
	case *llm.CompatibleProvider:
		models, err = modelProvider.Models(r.Context())
	case *llm.OpenAIResponsesProvider:
		var catalog []llm.CloudModel
		catalog, err = modelProvider.Models(r.Context())
		for _, model := range catalog {
			efforts := []string{}
			for _, level := range model.SupportedReasoningLevels {
				switch level.Effort {
				case "none", "minimal", "low", "medium", "high", "xhigh", "max":
					efforts = append(efforts, level.Effort)
				}
			}
			models = append(models, llm.HostedModel{ID: model.Slug, Name: model.DisplayName, ReasoningEfforts: efforts, DefaultReasoningEffort: model.DefaultReasoningLevel})
		}
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (s *Server) handleModelConnectionTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.cloudManager(w, r, true); !ok {
		return
	}
	body, ok := decodeModelConnection(w, r)
	if !ok {
		return
	}
	if body.Connection.Model == "" {
		writeError(w, http.StatusBadRequest, errors.New("select a model before testing"))
		return
	}
	settings, _ := s.store.Snapshot()
	resolved := settings.LLM.WithConnection(body.Connection)
	generation := s.cloudPlanning.auth.Generation()
	ctx, _, release, err := s.acquireProviderRequest(r.Context(), llmRequestInteractive, resolved)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer release()
	started := time.Now()
	provider, err := s.newHostedProvider(ctx, body.Connection, time.Duration(settings.LLM.RequestTimeoutMillis)*time.Millisecond)
	if err == nil {
		var raw string
		raw, err = provider.StreamChat(ctx, llm.ChatRequest{Model: body.Connection.Model, MaxTokens: 64, ReasoningMode: "off", Messages: []llm.Message{{Role: "system", Content: "Text-only connection test. Return exactly {\"ready\":true}."}, {Role: "user", Content: "Confirm readiness."}}, JSONSchema: json.RawMessage(`{"type":"object","properties":{"ready":{"type":"boolean"}},"required":["ready"],"additionalProperties":false}`)}, nil)
		var result struct {
			Ready bool `json:"ready"`
		}
		if err == nil && (json.Unmarshal([]byte(raw), &result) != nil || !result.Ready) {
			err = &llm.CloudError{Kind: "incomplete"}
		}
	}
	if ctx.Err() != nil || s.cloudPlanning.auth.Generation() != generation {
		writeError(w, http.StatusConflict, errors.New("connection test was canceled; retry against the current settings"))
		return
	}
	message := ""
	state := "ready"
	if err != nil {
		message = err.Error()
		state = "failed"
		var outcome *llm.CloudError
		if errors.As(err, &outcome) {
			state = outcome.Kind
		}
	}
	elapsedMillis := time.Since(started).Milliseconds()
	s.recordConnectionReadiness(body.Connection, generation, err == nil, message, elapsedMillis)
	writeJSON(w, http.StatusOK, map[string]any{"ready": err == nil, "state": state, "message": message, "provider": body.Connection.Provider, "model": body.Connection.Model, "elapsed_ms": elapsedMillis})
}

func (s *Server) decisionConversationEvidence(ctx context.Context, technical string, settings config.Settings) (string, error) {
	session, err := s.chatLog.ActiveSessionIDContext(ctx)
	if err != nil {
		return "", err
	}
	promptContext, err := s.loadInteractiveChatPromptContext(ctx, session, settings.LLM)
	if err != nil {
		return "", err
	}
	promptID := effectivePersonaPromptSet(settings.LLM.PromptSet, promptContext.Persona)
	prompt, memories, _, err := s.resolveInteractiveChatPersonalization(ctx, promptID)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(map[string]any{"technical_motion": json.RawMessage(technical), "conversation": promptContext.History, "conversation_context": promptContext.ConversationContext, "prompt": prompt, "enabled_memories": memories})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
