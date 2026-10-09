package httpapi

import (
	"context"
	"errors"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type refusalRetryStopKey struct{}

func shouldRetryDeclinedChat(settings config.LLMSettings, err error) bool {
	var outcome *llm.CloudError
	return settings.RetryRefusalLocally && settings.IsHosted() && errors.As(err, &outcome) && outcome.Kind == "refusal"
}

// retryDeclinedChat keeps the original turn/history, but composes the ordinary
// local contract and takes the existing local scheduler lane. It never recurses.
func (s *Server) retryDeclinedChat(ctx context.Context, stopSequence uint64, local config.LLMSettings,
	turn config.Settings, service chat.Service, request chat.Request, promptContext interactiveChatPromptContext,
	sessionID string, emit sseEmitter, started time.Time, diagnostics *chat.MessageDiagnostics,
) (chat.Result, bool, error) {
	if s.chatCanceled(ctx, stopSequence) {
		return chat.Result{}, true, context.Canceled
	}
	if local.IsHosted() || local.Model == "" {
		return chat.Result{}, true, errors.New("the request was declined and the local retry model is not configured")
	}
	local.RequestRole = "refusal_fallback"
	local.MotionGenerationMode = turn.LLM.MotionGenerationMode
	provider, err := s.prepareLLMProvider(local)
	if err != nil {
		return chat.Result{}, true, err
	}
	capabilities := chatCapabilities(local, promptContext.Persona)
	capabilities.MoodTracking = promptContext.Capabilities.MoodTracking
	if service.Capabilities != nil {
		capabilities.MotionHolder = service.Capabilities.MotionHolder
	}
	turn.LLM = local
	motionContext := s.chatTurnMotion(turn, promptContext.UserRequests, sessionID)
	service.Provider, service.Model = provider, local.Model
	service.Capabilities, service.MotionContext, service.SingleAttempt = &capabilities, &motionContext, true
	service.MaxTokens = local.ChatMaxOutputTokens(effectiveReplyLength(local, promptContext.Persona))
	service.ReasoningMode = local.ReasoningMode
	service.ReasoningBudgetTokens = managedLlamaReasoningBudget(local, s.managedLLM.Snapshot().Runtime.Current)
	service.PromptBudget = chatPromptBudget(local)
	declined := *diagnostics
	*diagnostics = interactiveDiagnostics(local, service.Prompt.ID, promptContext.Persona)
	diagnostics.RefusalFallbackFrom, diagnostics.RefusalFallbackModel = declined.Provider, declined.Model
	diagnostics.DeclinedRequestMillis = time.Since(started).Milliseconds()
	if err := emit("status", map[string]any{
		"state": "retrying_local", "provider": local.Provider, "model": local.Model,
		"prompt_set": service.Prompt.ID, "session_id": sessionID,
		"persona_id": declined.PersonaID, "persona_name": declined.PersonaName,
		"refusal_fallback_from": declined.Provider, "refusal_fallback_model": declined.Model,
	}); err != nil {
		return chat.Result{}, false, err
	}
	ctx = context.WithValue(ctx, refusalRetryStopKey{}, stopSequence)
	result, ok, completionErr := s.completeInteractiveChat(ctx, local, service, request, emit, started, time.Since(started), diagnostics)
	// The local timing wrapper accounts for its own call only. Keep both attempts
	// visible without labelling the primary refusal as a JSON repair.
	diagnostics.ProviderCalls += declined.ProviderCalls
	diagnostics.SchedulerWaitMillis += declined.SchedulerWaitMillis
	diagnostics.PreparationMillis = max(0, diagnostics.PreparationMillis-diagnostics.DeclinedRequestMillis)
	return result, ok, completionErr
}
