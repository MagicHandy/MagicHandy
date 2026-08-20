package httpapi

import (
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

// handlePromptComposition returns the exact backend-composed system prompt for
// the active conversation. It is intentionally read-only and available to a
// read-only client: inspectability must not require device-control ownership.
func (s *Server) handlePromptComposition(w http.ResponseWriter, r *http.Request) {
	sessionID, err := s.resolveActiveChatSession(r.URL.Query().Get("session_id"))
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	settings, _ := s.store.Snapshot()
	promptContext, err := s.loadInteractiveChatPromptContext(sessionID, settings.LLM)
	if err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	promptID := effectivePersonaPromptSet(settings.LLM.PromptSet, promptContext.Persona)
	prompt, memories, storageDomain, err := s.resolveInteractiveChatPersonalization(promptID)
	if err != nil {
		s.writePersonalizationStorageError(w, storageDomain, err)
		return
	}
	patterns, err := s.chatPatternChoicesFor(promptContext.Capabilities)
	if err != nil {
		s.writeLibraryStorageError(w, err)
		return
	}
	motionContext := s.chatMotionContext(settings.Motion, settings.LLM)
	composition := chat.ComposePrompt(
		prompt,
		memories,
		patterns,
		promptContext.Capabilities,
		&motionContext,
		promptContext.ConversationContext,
	)
	payload := map[string]any{
		"session_id":  sessionID,
		"provider":    settings.LLM.Provider,
		"model":       settings.LLM.Model,
		"prompt_set":  prompt.ID,
		"composition": composition,
		"lore":        promptContext.Lore,
	}
	if promptContext.Persona != nil {
		payload["persona_id"] = promptContext.Persona.ID
		payload["persona_name"] = promptContext.Persona.Name
		payload["lore_mode"] = promptContext.Persona.LoreMode
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, payload)
}
