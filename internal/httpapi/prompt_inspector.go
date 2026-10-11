package httpapi

import (
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

// handlePromptComposition returns the exact backend-composed system prompt for
// the active conversation. It is intentionally read-only and available to a
// read-only client: inspectability must not require device-control ownership.
func (s *Server) handlePromptComposition(w http.ResponseWriter, r *http.Request) {
	sessionID, err := s.chatWorkspace.ResolveActive(r.Context(), r.URL.Query().Get("session_id"))
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	settings, _ := s.store.Snapshot()
	settings.LLM = settings.LLM.ConversationSettings()
	promptContext, err := s.loadInteractiveChatPromptContext(r.Context(), sessionID, settings.LLM)
	if err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	promptID := effectivePersonaPromptSet(settings.LLM.PromptSet, promptContext.Persona)
	prompt, memories, storageDomain, err := s.resolveInteractiveChatPersonalization(r.Context(), promptID)
	if err != nil {
		s.writePersonalizationStorageError(w, storageDomain, err)
		return
	}
	patterns, err := s.chatPatternChoicesFor(promptContext.Capabilities)
	if err != nil {
		s.writeLibraryStorageError(w, err)
		return
	}
	motionContext := s.chatTurnMotion(settings, promptContext.UserRequests, sessionID)
	composition := chat.ComposeInteractivePrompt(
		prompt,
		memories,
		patterns,
		promptContext.Capabilities,
		&motionContext,
		promptContext.ConversationContext,
		chatPromptBudget(settings.LLM),
	)
	payload := map[string]any{
		"session_id":        sessionID,
		"provider":          settings.LLM.Provider,
		"model":             settings.LLM.Model,
		"prompt_set":        prompt.ID,
		"prompt_set_name":   prompt.Name,
		"global_prompt_set": settings.LLM.PromptSet,
		"prompt_set_source": "global",
		"composition":       composition,
		"response_schema":   chat.InteractiveResponseSchema(patterns, promptContext.Capabilities, &motionContext),
		"reasoning_mode":    settings.LLM.ReasoningMode,
		"max_tokens":        settings.LLM.ChatMaxOutputTokens(effectiveReplyLength(settings.LLM, promptContext.Persona)),
		"lore":              promptContext.Lore,
	}
	if promptContext.Persona != nil {
		if promptContext.Persona.PromptSetID != "" {
			payload["prompt_set_source"] = "persona"
		}
		payload["persona_id"] = promptContext.Persona.ID
		payload["persona_name"] = promptContext.Persona.Name
		payload["lore_mode"] = promptContext.Persona.LoreMode
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, payload)
}
