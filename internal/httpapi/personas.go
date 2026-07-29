package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/persona"
)

func (s *Server) personaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/personas", s.handlePersonasGet)
	mux.HandleFunc("POST /api/personas", s.handlePersonaCreate)
	mux.HandleFunc("PATCH /api/personas/{id}", s.handlePersonaUpdate)
	mux.HandleFunc("DELETE /api/personas/{id}", s.handlePersonaDelete)
	mux.HandleFunc("POST /api/personas/{id}/duplicate", s.handlePersonaDuplicate)
	mux.HandleFunc("GET /api/personas/{id}/portrait", s.handlePersonaPortrait)
	mux.HandleFunc("POST /api/personas/{id}/portrait", s.handlePersonaPortraitUpload)
	mux.HandleFunc("DELETE /api/personas/{id}/portrait", s.handlePersonaPortraitDelete)
	mux.HandleFunc("GET /api/personas/{id}/lore", s.handlePersonaLoreGet)
	mux.HandleFunc("POST /api/personas/{id}/lore", s.handlePersonaLoreCreate)
	mux.HandleFunc("PATCH /api/personas/{id}/lore/{lore_id}", s.handlePersonaLoreUpdate)
	mux.HandleFunc("DELETE /api/personas/{id}/lore/{lore_id}", s.handlePersonaLoreDelete)
	mux.HandleFunc("PUT /api/chat/sessions/{id}/persona", s.handleChatSessionPersona)
}

// personasPayload is the whole page state in one response: the library, which
// persona the active conversation is using, and the enum vocabularies.
//
// The option lists come from the same single source that feeds the Settings
// selects, so the page can never offer a register the server would reject.
func (s *Server) personasPayload(ctx context.Context) (map[string]any, error) {
	personas, err := s.personas.List(ctx)
	if err != nil {
		return nil, err
	}
	activeID, sessionID := "", ""
	if sessions, sessionErr := s.chatLog.Sessions(); sessionErr == nil {
		for _, session := range sessions {
			if session.Active {
				activeID = session.PersonaID
				sessionID = session.ID
				break
			}
		}
	}
	sets, err := s.personalization.prompts.List()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"personas":          personas,
		"active_persona_id": activeID,
		"active_session_id": sessionID,
		"prompt_sets":       sets,
		"options": map[string]any{
			"chat_voices":       config.LLMChatVoices(),
			"reaction_styles":   config.LLMReactionStyles(),
			"focus_areas":       chat.AreaZones(),
			"lore_modes":        persona.LoreModes(),
			"max_name":          persona.MaxNameChars,
			"max_description":   persona.MaxDescriptionChars,
			"max_portrait_edge": persona.MaxPortraitEdge,
			"max_lore_entries":  persona.MaxLoreEntries,
			"max_lore_text":     persona.MaxLoreTextChars,
			"max_lore_total":    persona.MaxLoreTotalChars,
			"max_lore_keywords": persona.MaxLoreKeywords,
		},
	}, nil
}

func (s *Server) handlePersonasGet(w http.ResponseWriter, r *http.Request) {
	s.writePersonasPayload(w, r, http.StatusOK, nil)
}

func (s *Server) handlePersonaCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var draft persona.Draft
	if err := decodeJSON(r, &draft); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := s.personas.Create(r.Context(), draft)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	s.writePersonasPayload(w, r, http.StatusCreated, &item)
}

func (s *Server) handlePersonaUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var draft persona.Draft
	if err := decodeJSON(r, &draft); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := s.personas.Update(r.Context(), strings.TrimSpace(r.PathValue("id")), draft)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	s.writePersonasPayload(w, r, http.StatusOK, &item)
}

func (s *Server) handlePersonaDuplicate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	item, err := s.personas.Duplicate(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	s.writePersonasPayload(w, r, http.StatusCreated, &item)
}

// handlePersonaDelete removes a persona. Sessions that used it keep their
// recorded id and resolve to the global axis values, so a past conversation is
// never rewritten by a deletion.
func (s *Server) handlePersonaDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if err := s.personas.Delete(r.Context(), strings.TrimSpace(r.PathValue("id"))); err != nil {
		s.writePersonaError(w, err)
		return
	}
	s.writePersonasPayload(w, r, http.StatusOK, nil)
}

func (s *Server) handlePersonaPortrait(w http.ResponseWriter, r *http.Request) {
	file, err := s.personas.OpenPortrait(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// A portrait changes only when replaced, and the row's stamp changes with it,
	// so the tile URL busts its own cache and a short max-age is safe.
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.ServeContent(w, r, "portrait.jpg", info.ModTime(), file)
}

// handlePersonaPortraitUpload accepts a JPEG the browser already downscaled on
// the canvas path video covers use. The server still decodes and bounds it: the
// result is a file on disk that is later served back with an image type.
func (s *Server) handlePersonaPortraitUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, persona.MaxPortraitBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("portrait could not be read"))
		return
	}
	if len(data) > persona.MaxPortraitBytes {
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("portrait is too large"))
		return
	}
	item, err := s.personas.SavePortrait(r.Context(), strings.TrimSpace(r.PathValue("id")), data)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	s.writePersonasPayload(w, r, http.StatusOK, &item)
}

func (s *Server) handlePersonaPortraitDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	item, err := s.personas.DeletePortrait(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	s.writePersonasPayload(w, r, http.StatusOK, &item)
}

func (s *Server) handlePersonaLoreGet(w http.ResponseWriter, r *http.Request) {
	s.writePersonaLorePayload(w, r, http.StatusOK, nil)
}

func (s *Server) handlePersonaLoreCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var draft persona.LoreDraft
	if err := decodeJSON(r, &draft); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	entry, err := s.personas.CreateLore(r.Context(), strings.TrimSpace(r.PathValue("id")), draft)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	s.writePersonaLorePayload(w, r, http.StatusCreated, &entry)
}

func (s *Server) handlePersonaLoreUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var draft persona.LoreDraft
	if err := decodeJSON(r, &draft); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	entry, err := s.personas.UpdateLore(
		r.Context(),
		strings.TrimSpace(r.PathValue("id")),
		strings.TrimSpace(r.PathValue("lore_id")),
		draft,
	)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	s.writePersonaLorePayload(w, r, http.StatusOK, &entry)
}

func (s *Server) handlePersonaLoreDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if err := s.personas.DeleteLore(
		r.Context(),
		strings.TrimSpace(r.PathValue("id")),
		strings.TrimSpace(r.PathValue("lore_id")),
	); err != nil {
		s.writePersonaError(w, err)
		return
	}
	s.writePersonaLorePayload(w, r, http.StatusOK, nil)
}

// handleChatSessionPersona binds a persona to one conversation. This is the only
// way a persona takes effect: it is a property of the session, never a settings
// write, so the values in Settings stay exactly as the user left them and remain
// the fallback when no persona is selected.
func (s *Server) handleChatSessionPersona(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	sessionID := strings.TrimSpace(r.PathValue("id"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, errors.New("a chat session id is required"))
		return
	}
	var body struct {
		PersonaID string `json:"persona_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	personaID := strings.TrimSpace(body.PersonaID)
	if personaID != "" {
		// Selecting a persona that does not exist is rejected rather than stored:
		// a dangling id is tolerated when it comes from history, not when it comes
		// from a live request.
		if _, err := s.personas.Get(r.Context(), personaID); err != nil {
			s.writePersonaError(w, err)
			return
		}
	}
	if _, err := s.chatLog.SetSessionPersona(sessionID, personaID); err != nil {
		s.writeChatSessionError(w, err)
		return
	}
	if personaID != "" {
		if err := s.personas.MarkUsed(r.Context(), personaID); err != nil {
			// The binding already succeeded; failing the request over the ordering
			// stamp would be worse than a grid that sorts one tile late.
			s.logger.Warn("persona selection was not stamped", "persona_id", personaID, "error", err)
		}
	}
	s.writePersonasPayload(w, r, http.StatusOK, nil)
}

func (s *Server) writePersonasPayload(w http.ResponseWriter, r *http.Request, status int, item *persona.Persona) {
	payload, err := s.personasPayload(r.Context())
	if err != nil {
		s.writePersonalizationStorageError(w, "persona", err)
		return
	}
	if item != nil {
		payload["persona"] = *item
	}
	writeJSON(w, status, payload)
}

func (s *Server) writePersonaLorePayload(w http.ResponseWriter, r *http.Request, status int, entry *persona.LoreEntry) {
	personaID := strings.TrimSpace(r.PathValue("id"))
	item, err := s.personas.Get(r.Context(), personaID)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	entries, err := s.personas.ListLore(r.Context(), personaID)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	payload := map[string]any{
		"persona": item,
		"entries": entries,
		"options": map[string]any{
			"max_entries":  persona.MaxLoreEntries,
			"max_text":     persona.MaxLoreTextChars,
			"max_total":    persona.MaxLoreTotalChars,
			"max_keywords": persona.MaxLoreKeywords,
		},
	}
	if entry != nil {
		payload["entry"] = *entry
	}
	writeJSON(w, status, payload)
}

func (s *Server) writePersonaError(w http.ResponseWriter, err error) {
	status := personaErrorStatus(err)
	if status == http.StatusInternalServerError {
		s.writePersonalizationStorageError(w, "persona", err)
		return
	}
	writeError(w, status, err)
}

func personaErrorStatus(err error) int {
	switch {
	case errors.Is(err, persona.ErrNotFound), errors.Is(err, persona.ErrPortraitNotFound):
		return http.StatusNotFound
	case errors.Is(err, persona.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, persona.ErrPortraitInvalid):
		return http.StatusUnsupportedMediaType
	case errors.Is(err, persona.ErrLimit):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// activeSessionPersona resolves the persona of whichever conversation is active.
// Background work that speaks into the chat uses this so the assistant does not
// change character the moment it starts talking on its own.
func (s *Server) activeSessionPersona() *persona.Persona {
	if s.chatLog == nil {
		return nil
	}
	sessionID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		return nil
	}
	return s.sessionPersona(sessionID)
}
