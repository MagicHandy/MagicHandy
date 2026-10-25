package httpapi

import (
	"context"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
)

// prepareInteractiveChatService resolves the original request's personalization,
// provider and authoritative motion once. A repair keeps this exact service.
func (s *Server) prepareInteractiveChatService(ctx context.Context, w http.ResponseWriter, r *http.Request,
	settings config.Settings, sessionID string, holder chat.MotionHolder, promptContext interactiveChatPromptContext,
) (chat.Service, bool) {
	promptID := effectivePersonaPromptSet(settings.LLM.PromptSet, promptContext.Persona)
	prompt, memories, storageDomain, err := s.resolveInteractiveChatPersonalization(ctx, promptID)
	if err != nil {
		s.writePersonalizationStorageError(w, storageDomain, err)
		return chat.Service{}, false
	}
	capabilities := promptContext.Capabilities
	capabilities.MotionHolder = holder
	patternChoices, err := s.chatPatternChoicesFor(capabilities)
	if err != nil {
		s.writeLibraryStorageError(w, err)
		return chat.Service{}, false
	}
	provider, err := s.prepareLLMProvider(settings.LLM)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": s.clientRuntimeError(r, err)})
		return chat.Service{}, false
	}
	motionContext := s.chatTurnMotion(settings, promptContext.UserRequests, sessionID)
	_, repairAvailable := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate)
	return chat.Service{
		Provider: provider, Prompt: prompt, Model: settings.LLM.Model,
		MaxTokens:             settings.LLM.ChatMaxOutputTokens(effectiveReplyLength(settings.LLM, promptContext.Persona)),
		ReasoningMode:         settings.LLM.ReasoningMode,
		ReasoningBudgetTokens: managedLlamaReasoningBudget(settings.LLM, s.managedLLM.Snapshot().Runtime.Current),
		PromptBudget:          chatPromptBudget(settings.LLM), Memories: memories, Patterns: patternChoices,
		MotionContext: &motionContext, ConversationContext: promptContext.ConversationContext, Capabilities: &capabilities,
		HostedResponseRepair: settings.LLM.AdvancedMotion() && settings.LLM.HostedResponseRepair() && repairAvailable,
	}, true
}
