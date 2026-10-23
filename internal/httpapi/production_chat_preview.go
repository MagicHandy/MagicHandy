package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

// This preview uses the production service and saved chat context. A proposal
// is returned as diagnostic content only: there is no dispatch, persistence of
// dialogue, speech enqueue, or change to the Lab's experimental score.
func (s *Server) handleProductionChatTest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string `json:"message"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	message, err := chat.ValidateUserMessage(body.Message)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sessionID, err := s.chatWorkspace.ResolveActive(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	ctx, finish, err := s.chatWorkspace.BeginTurn(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer finish()
	settings, _ := s.store.Snapshot()
	settings.LLM = settings.LLM.ConversationSettings()
	if settings.LLM.IsHosted() {
		ctx = s.guardHostedChat(ctx)
	}
	stopSequence := s.stopSequence.Load()
	context, err := s.loadInteractiveChatPromptContext(ctx, sessionID, settings.LLM, message)
	if err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	prompt, memories, domain, err := s.resolveInteractiveChatPersonalization(ctx, effectivePersonaPromptSet(settings.LLM.PromptSet, context.Persona))
	if err != nil {
		s.writePersonalizationStorageError(w, domain, err)
		return
	}
	patterns, err := s.chatPatternChoicesFor(context.Capabilities)
	if err != nil {
		s.writeLibraryStorageError(w, err)
		return
	}
	provider, err := s.prepareLLMProvider(settings.LLM)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	motionContext := s.chatTurnMotion(settings, context.UserRequests, sessionID)
	timed := newTimedLLMProvider(provider)
	service := chat.Service{Provider: timed, Prompt: prompt, Memories: memories, Patterns: patterns,
		HostedResponseRepair: settings.LLM.IsHosted() && settings.LLM.HostedResponseRepair(),
		Model:                settings.LLM.Model, MaxTokens: settings.LLM.ChatMaxOutputTokens(effectiveReplyLength(settings.LLM, context.Persona)),
		ReasoningMode: settings.LLM.ReasoningMode, ReasoningBudgetTokens: managedLlamaReasoningBudget(settings.LLM, s.managedLLM.Snapshot().Runtime.Current),
		PromptBudget: chatPromptBudget(settings.LLM), MotionContext: &motionContext, ConversationContext: context.ConversationContext, Capabilities: &context.Capabilities}
	providerCtx, wait, release, err := s.acquireProviderRequest(ctx, llmRequestInteractive, settings.LLM)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer release()
	started := time.Now()
	result, completionErr := service.Complete(providerCtx, chat.Request{Message: message, History: context.History, HistoryMessagesDropped: context.HistoryOmitted}, nil)
	if s.chatCanceled(providerCtx, stopSequence) {
		writeError(w, http.StatusConflict, errors.New("test canceled by Stop or its client"))
		return
	}
	diagnostics := interactiveDiagnostics(settings.LLM, prompt.ID, context.Persona)
	timed.applyDiagnostics(&diagnostics, started, 0, wait)
	result.Input = timed.initialRequest
	item := s.chatFailureEvidence(result, completionErr, diagnostics, settings.LLM)
	if err := s.prepareChatFailure(ctx, &item); err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"valid": completionErr == nil && !result.Malformed, "report": item})
}
