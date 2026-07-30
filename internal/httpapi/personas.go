package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/charcard"
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/persona"
)

func (s *Server) personaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/personas", s.handlePersonasGet)
	mux.HandleFunc("POST /api/personas", s.handlePersonaCreate)
	mux.HandleFunc("POST /api/personas/import", s.handlePersonaImport)
	mux.HandleFunc("POST /api/personas/import-url", s.handlePersonaImportURL)
	mux.HandleFunc("PATCH /api/personas/{id}", s.handlePersonaUpdate)
	mux.HandleFunc("DELETE /api/personas/{id}", s.handlePersonaDelete)
	mux.HandleFunc("POST /api/personas/{id}/duplicate", s.handlePersonaDuplicate)
	mux.HandleFunc("GET /api/personas/{id}/export", s.handlePersonaExport)
	mux.HandleFunc("GET /api/personas/{id}/portrait", s.handlePersonaPortrait)
	mux.HandleFunc("POST /api/personas/{id}/portrait", s.handlePersonaPortraitUpload)
	mux.HandleFunc("DELETE /api/personas/{id}/portrait", s.handlePersonaPortraitDelete)
	mux.HandleFunc("GET /api/personas/{id}/lore", s.handlePersonaLoreGet)
	mux.HandleFunc("POST /api/personas/{id}/lore", s.handlePersonaLoreCreate)
	mux.HandleFunc("PATCH /api/personas/{id}/lore/{lore_id}", s.handlePersonaLoreUpdate)
	mux.HandleFunc("DELETE /api/personas/{id}/lore/{lore_id}", s.handlePersonaLoreDelete)
	mux.HandleFunc("PUT /api/chat/sessions/{id}/persona", s.handleChatSessionPersona)
}

type defaultPersonaView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ChatVoice   string `json:"chat_voice"`
	PromptSetID string `json:"prompt_set_id"`
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
	sessions, err := s.chatLog.Sessions()
	if err != nil {
		return nil, fmt.Errorf("read active persona session: %w", err)
	}
	for _, session := range sessions {
		if session.Active {
			activeID = session.PersonaID
			sessionID = session.ID
			break
		}
	}
	if activeID != "" && !personaListContains(personas, activeID) {
		// Deleted personas remain in session provenance, but resolution falls
		// back to the built-in persona. Report that effective state to the UI.
		activeID = ""
	}
	sets, err := s.personalization.prompts.List()
	if err != nil {
		return nil, err
	}
	settings, _ := s.store.Snapshot()
	return map[string]any{
		"personas": personas,
		"default_persona": defaultPersonaView{
			Name:        "MagicHandy",
			Description: settings.LLM.PersonaDescription,
			ChatVoice:   settings.LLM.ChatVoice,
			PromptSetID: settings.LLM.PromptSet,
		},
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
			"max_greeting":      persona.MaxGreetingChars,
			"max_portrait_edge": persona.MaxPortraitEdge,
			"max_lore_entries":  persona.MaxLoreEntries,
			"max_lore_text":     persona.MaxLoreTextChars,
			"max_lore_total":    persona.MaxLoreTotalChars,
			"max_lore_keywords": persona.MaxLoreKeywords,
			"max_lore_keyword":  persona.MaxLoreKeywordChars,
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
	unlock, ok := s.beginPersonaMutation(w)
	if !ok {
		return
	}
	defer unlock()
	payload, ok := s.preparePersonasMutation(w, r)
	if !ok {
		return
	}
	item, err := s.personas.Create(r.Context(), draft)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	upsertPersonaPayload(payload, item)
	payload["persona"] = item
	writeJSON(w, http.StatusCreated, payload)
}

func (s *Server) handlePersonaImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, cardImportMaxBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("persona archive could not be read"))
		return
	}
	if len(data) > cardImportMaxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("import is too large"))
		return
	}
	// One import endpoint, two formats: .mhpersona archives and character
	// cards (card PNG or card JSON). Cards are sniffed, not negotiated, so a
	// dragged-in file just works.
	if charcard.LooksLikeCard(data) {
		s.handleCardUpload(w, r, data)
		return
	}
	if len(data) > persona.MaxArchiveBytes {
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("persona archive is too large"))
		return
	}
	portable, err := persona.DecodeArchive(data)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}

	unlock, ok := s.beginPersonaMutation(w)
	if !ok {
		return
	}
	defer unlock()
	payload, ok := s.preparePersonasMutation(w, r)
	if !ok {
		return
	}

	promptSetID := portable.Persona.PromptSetID
	createdPromptSetID := ""
	if profile := portable.BehaviorProfile; profile != nil {
		if profile.Builtin {
			local, found, resolveErr := s.personalization.prompts.Resolve(profile.ID)
			if resolveErr != nil {
				s.writePersonalizationStorageError(w, "prompt set", resolveErr)
				return
			}
			if !found || !local.Builtin {
				s.writePersonaError(w, fmt.Errorf(
					"%w: built-in behavior profile %q is unavailable",
					persona.ErrInvalid,
					profile.ID,
				))
				return
			}
			promptSetID = local.ID
		} else {
			created, createErr := s.personalization.prompts.Create(profile.Name, profile.System)
			if createErr != nil {
				s.writePersonaError(w, portablePromptSetError(createErr))
				return
			}
			promptSetID = created.ID
			createdPromptSetID = created.ID
			sets, listErr := s.personalization.prompts.List()
			if listErr != nil {
				s.rollbackImportedPromptSet(createdPromptSetID)
				s.writePersonalizationStorageError(w, "prompt set", listErr)
				return
			}
			payload["prompt_sets"] = sets
		}
	} else if promptSetID != "" {
		local, found, resolveErr := s.personalization.prompts.Resolve(promptSetID)
		if resolveErr != nil {
			s.writePersonalizationStorageError(w, "prompt set", resolveErr)
			return
		}
		if found {
			promptSetID = local.ID
		} else {
			// A v1 archive exported by this build embeds custom profiles. A
			// missing unembedded reference is therefore from an older or hand-
			// authored file and falls back visibly to Settings.
			promptSetID = ""
		}
	}

	item, err := s.personas.ImportPortable(r.Context(), portable, promptSetID)
	if err != nil {
		s.rollbackImportedPromptSet(createdPromptSetID)
		s.writePersonaError(w, err)
		return
	}
	upsertPersonaPayload(payload, item)
	payload["persona"] = item
	writeJSON(w, http.StatusCreated, payload)
}

func (s *Server) handlePersonaExport(w http.ResponseWriter, r *http.Request) {
	// Export reads a stable persona/profile pair but does not require controller
	// ownership. The mutation lock prevents an edit from changing prompt_set_id
	// between profile resolution and archive assembly.
	s.personaMutationMu.Lock()
	defer s.personaMutationMu.Unlock()

	id := strings.TrimSpace(r.PathValue("id"))
	item, err := s.personas.Get(r.Context(), id)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	var profile *chat.PromptSet
	if item.PromptSetID != "" {
		resolved, found, resolveErr := s.personalization.prompts.Resolve(item.PromptSetID)
		if resolveErr != nil {
			s.writePersonalizationStorageError(w, "prompt set", resolveErr)
			return
		}
		if found {
			profile = &resolved
		}
	}
	archive, exported, err := s.personas.ExportArchive(r.Context(), id, profile)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	w.Header().Set("Content-Type", persona.ArchiveMediaType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		`attachment; filename="%s"`,
		personaArchiveFilename(exported.Name),
	))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(archive) // #nosec G705 -- validated ZIP served as a nosniff attachment.
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
	unlock, ok := s.beginPersonaMutation(w)
	if !ok {
		return
	}
	defer unlock()
	payload, ok := s.preparePersonasMutation(w, r)
	if !ok {
		return
	}
	item, err := s.personas.Update(r.Context(), strings.TrimSpace(r.PathValue("id")), draft)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	upsertPersonaPayload(payload, item)
	payload["persona"] = item
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handlePersonaDuplicate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	unlock, ok := s.beginPersonaMutation(w)
	if !ok {
		return
	}
	defer unlock()
	payload, ok := s.preparePersonasMutation(w, r)
	if !ok {
		return
	}
	item, err := s.personas.Duplicate(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	upsertPersonaPayload(payload, item)
	payload["persona"] = item
	writeJSON(w, http.StatusCreated, payload)
}

// handlePersonaDelete removes a persona. Sessions that used it keep their
// recorded id and resolve to the global axis values, so a past conversation is
// never rewritten by a deletion.
func (s *Server) handlePersonaDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	unlock, ok := s.beginPersonaMutation(w)
	if !ok {
		return
	}
	defer unlock()
	payload, ok := s.preparePersonasMutation(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.personas.Delete(r.Context(), id); err != nil {
		s.writePersonaError(w, err)
		return
	}
	removePersonaFromPayload(payload, id)
	if payload["active_persona_id"] == id {
		payload["active_persona_id"] = ""
	}
	writeJSON(w, http.StatusOK, payload)
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
	unlock, ok := s.beginPersonaMutation(w)
	if !ok {
		return
	}
	defer unlock()
	payload, ok := s.preparePersonasMutation(w, r)
	if !ok {
		return
	}
	item, err := s.personas.SavePortrait(r.Context(), strings.TrimSpace(r.PathValue("id")), data)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	upsertPersonaPayload(payload, item)
	payload["persona"] = item
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handlePersonaPortraitDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	unlock, ok := s.beginPersonaMutation(w)
	if !ok {
		return
	}
	defer unlock()
	payload, ok := s.preparePersonasMutation(w, r)
	if !ok {
		return
	}
	item, err := s.personas.DeletePortrait(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	upsertPersonaPayload(payload, item)
	payload["persona"] = item
	writeJSON(w, http.StatusOK, payload)
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
	unlock, ok := s.beginPersonaMutation(w)
	if !ok {
		return
	}
	defer unlock()
	personaID := strings.TrimSpace(r.PathValue("id"))
	payload, ok := s.preparePersonaLoreMutation(w, r, personaID)
	if !ok {
		return
	}
	entry, err := s.personas.CreateLore(r.Context(), personaID, draft)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	upsertLorePayload(payload, entry)
	adjustLoreCount(payload, 1)
	payload["entry"] = entry
	writeJSON(w, http.StatusCreated, payload)
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
	unlock, ok := s.beginPersonaMutation(w)
	if !ok {
		return
	}
	defer unlock()
	personaID := strings.TrimSpace(r.PathValue("id"))
	payload, ok := s.preparePersonaLoreMutation(w, r, personaID)
	if !ok {
		return
	}
	entry, err := s.personas.UpdateLore(
		r.Context(),
		personaID,
		strings.TrimSpace(r.PathValue("lore_id")),
		draft,
	)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	upsertLorePayload(payload, entry)
	payload["entry"] = entry
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handlePersonaLoreDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	unlock, ok := s.beginPersonaMutation(w)
	if !ok {
		return
	}
	defer unlock()
	personaID := strings.TrimSpace(r.PathValue("id"))
	payload, ok := s.preparePersonaLoreMutation(w, r, personaID)
	if !ok {
		return
	}
	loreID := strings.TrimSpace(r.PathValue("lore_id"))
	if err := s.personas.DeleteLore(
		r.Context(),
		personaID,
		loreID,
	); err != nil {
		s.writePersonaError(w, err)
		return
	}
	if removeLoreFromPayload(payload, loreID) {
		adjustLoreCount(payload, -1)
	}
	writeJSON(w, http.StatusOK, payload)
}

// handleChatSessionPersona binds a persona to one conversation. This is the only
// way a persona takes effect: it is a property of the session, never a settings
// write, so the values in Settings stay exactly as the user left them and remain
// the fallback when no persona is selected.
func (s *Server) handleChatSessionPersona(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.chatLifecycleMu.Lock()
	defer s.chatLifecycleMu.Unlock()
	if s.chatGenerationActive() {
		writeError(w, http.StatusConflict, errors.New("wait for the active reply to finish before changing personas"))
		return
	}
	if s.autopilotActive() {
		writeError(w, http.StatusConflict, errors.New("stop Autopilot before changing personas"))
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
	s.personaMutationMu.Lock()
	defer s.personaMutationMu.Unlock()
	payload, ok := s.preparePersonasMutation(w, r)
	if !ok {
		return
	}
	personaID := strings.TrimSpace(body.PersonaID)
	var selected persona.Persona
	if personaID != "" {
		// Selecting a persona that does not exist is rejected rather than stored:
		// a dangling id is tolerated when it comes from history, not when it comes
		// from a live request.
		var err error
		selected, err = s.personas.Get(r.Context(), personaID)
		if err != nil {
			s.writePersonaError(w, err)
			return
		}
	}
	if _, err := s.chatLog.SetSessionPersona(sessionID, personaID); err != nil {
		s.writeChatSessionError(w, err)
		return
	}
	if personaID != "" {
		marked, err := s.personas.MarkUsed(r.Context(), personaID)
		if err != nil {
			// The binding already succeeded; failing the request over the ordering
			// stamp would be worse than a grid that sorts one tile late.
			s.logger.Warn("persona selection was not stamped", "persona_id", personaID, "error", err)
		} else {
			selected = marked
		}
		upsertPersonaPayload(payload, selected)
		s.seedPersonaGreeting(sessionID, selected)
	}
	if activeSessionID, _ := payload["active_session_id"].(string); activeSessionID == sessionID {
		payload["active_persona_id"] = personaID
	}
	writeJSON(w, http.StatusOK, payload)
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

func (s *Server) preparePersonasMutation(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	if s.autopilotActive() {
		writeError(w, http.StatusConflict, errors.New("stop Autopilot before changing personas"))
		return nil, false
	}
	payload, err := s.personasPayload(r.Context())
	if err != nil {
		s.writePersonalizationStorageError(w, "persona", err)
		return nil, false
	}
	return payload, true
}

func (s *Server) beginPersonaMutation(w http.ResponseWriter) (func(), bool) {
	s.chatLifecycleMu.Lock()
	if s.chatGenerationActive() {
		s.chatLifecycleMu.Unlock()
		writeError(w, http.StatusConflict, errors.New("wait for the active reply to finish before changing personas"))
		return nil, false
	}
	if s.autopilotActive() {
		s.chatLifecycleMu.Unlock()
		writeError(w, http.StatusConflict, errors.New("stop Autopilot before changing personas"))
		return nil, false
	}
	s.personaMutationMu.Lock()
	return func() {
		s.personaMutationMu.Unlock()
		s.chatLifecycleMu.Unlock()
	}, true
}

func (s *Server) writePersonaLorePayload(w http.ResponseWriter, r *http.Request, status int, entry *persona.LoreEntry) {
	personaID := strings.TrimSpace(r.PathValue("id"))
	payload, err := s.personaLorePayload(r.Context(), personaID)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	if entry != nil {
		payload["entry"] = *entry
	}
	writeJSON(w, status, payload)
}

func (s *Server) preparePersonaLoreMutation(
	w http.ResponseWriter,
	r *http.Request,
	personaID string,
) (map[string]any, bool) {
	if s.autopilotActive() {
		writeError(w, http.StatusConflict, errors.New("stop Autopilot before changing personas"))
		return nil, false
	}
	payload, err := s.personaLorePayload(r.Context(), personaID)
	if err != nil {
		s.writePersonaError(w, err)
		return nil, false
	}
	return payload, true
}

func (s *Server) personaLorePayload(ctx context.Context, personaID string) (map[string]any, error) {
	item, err := s.personas.Get(ctx, personaID)
	if err != nil {
		return nil, err
	}
	entries, err := s.personas.ListLore(ctx, personaID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"persona": item,
		"entries": entries,
		"options": map[string]any{
			"max_entries":  persona.MaxLoreEntries,
			"max_text":     persona.MaxLoreTextChars,
			"max_total":    persona.MaxLoreTotalChars,
			"max_keywords": persona.MaxLoreKeywords,
			"max_keyword":  persona.MaxLoreKeywordChars,
		},
	}, nil
}

func personaListContains(items []persona.Persona, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func upsertPersonaPayload(payload map[string]any, item persona.Persona) {
	items, _ := payload["personas"].([]persona.Persona)
	replaced := false
	for index := range items {
		if items[index].ID == item.ID {
			items[index] = item
			replaced = true
			break
		}
	}
	if !replaced {
		items = append(items, item)
	}
	sort.SliceStable(items, func(left, right int) bool {
		leftUsed := items[left].LastUsedAt != ""
		rightUsed := items[right].LastUsedAt != ""
		if leftUsed != rightUsed {
			return leftUsed
		}
		if items[left].LastUsedAt != items[right].LastUsedAt {
			return items[left].LastUsedAt > items[right].LastUsedAt
		}
		if items[left].Name != items[right].Name {
			return items[left].Name < items[right].Name
		}
		return items[left].ID < items[right].ID
	})
	payload["personas"] = items
}

func removePersonaFromPayload(payload map[string]any, id string) {
	items, _ := payload["personas"].([]persona.Persona)
	filtered := items[:0]
	for _, item := range items {
		if item.ID != id {
			filtered = append(filtered, item)
		}
	}
	payload["personas"] = filtered
}

func upsertLorePayload(payload map[string]any, entry persona.LoreEntry) {
	entries, _ := payload["entries"].([]persona.LoreEntry)
	for index := range entries {
		if entries[index].ID == entry.ID {
			entries[index] = entry
			payload["entries"] = entries
			return
		}
	}
	payload["entries"] = append(entries, entry)
}

func removeLoreFromPayload(payload map[string]any, id string) bool {
	entries, _ := payload["entries"].([]persona.LoreEntry)
	filtered := entries[:0]
	removed := false
	for _, entry := range entries {
		if entry.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, entry)
	}
	payload["entries"] = filtered
	return removed
}

func adjustLoreCount(payload map[string]any, delta int) {
	item, ok := payload["persona"].(persona.Persona)
	if !ok {
		return
	}
	item.LoreCount += delta
	if item.LoreCount < 0 {
		item.LoreCount = 0
	}
	payload["persona"] = item
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

func portablePromptSetError(err error) error {
	switch {
	case errors.Is(err, chat.ErrPromptSetInvalid):
		return fmt.Errorf("%w: imported behavior profile is invalid: %v", persona.ErrInvalid, err)
	case errors.Is(err, chat.ErrPromptSetLimit):
		return fmt.Errorf("%w: behavior profile library is full", persona.ErrLimit)
	default:
		return err
	}
}

func (s *Server) rollbackImportedPromptSet(id string) {
	if id == "" {
		return
	}
	if err := s.personalization.prompts.Delete(id); err != nil {
		s.logger.Warn(
			"imported behavior profile rollback failed",
			"prompt_set_id",
			id,
			"error",
			err,
		)
	}
}

func personaArchiveFilename(name string) string {
	var result strings.Builder
	pendingSeparator := false
	for _, current := range strings.ToLower(name) {
		if (current >= 'a' && current <= 'z') || (current >= '0' && current <= '9') {
			if pendingSeparator && result.Len() > 0 && result.Len() < 48 {
				result.WriteByte('-')
			}
			pendingSeparator = false
			if result.Len() < 48 {
				result.WriteRune(current)
			}
			continue
		}
		pendingSeparator = result.Len() > 0
	}
	base := strings.Trim(result.String(), "-")
	if base == "" {
		base = "persona"
	}
	return base + persona.ArchiveExtension
}

// activeSessionPersona resolves the persona of whichever conversation is active.
// Background work that speaks into the chat uses this so the assistant does not
// change character the moment it starts talking on its own.
func (s *Server) activeSessionPersona() (*persona.Persona, error) {
	if s.chatLog == nil {
		return nil, errors.New("chat session store is unavailable")
	}
	sessionID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		return nil, fmt.Errorf("read active chat session: %w", err)
	}
	return s.sessionPersona(sessionID)
}
