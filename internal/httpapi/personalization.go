package httpapi

import (
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/memory"
)

// personalizationRuntime owns the user-managed memory store and prompt-set
// library. Both recover to safe defaults instead of failing startup.
type personalizationRuntime struct {
	memory  *memory.Store
	prompts *chat.PromptLibrary
}

func newPersonalizationRuntime(dataDir string) (personalizationRuntime, error) {
	memoryStore, err := memory.Open(dataDir)
	if err != nil {
		return personalizationRuntime{}, err
	}
	prompts, err := chat.OpenPromptLibrary(dataDir)
	if err != nil {
		return personalizationRuntime{}, err
	}
	return personalizationRuntime{memory: memoryStore, prompts: prompts}, nil
}

func (p personalizationRuntime) Close() {
	if p.memory != nil {
		_ = p.memory.Close()
	}
	if p.prompts != nil {
		_ = p.prompts.Close()
	}
}

// --- Memory -------------------------------------------------------------------

// memoryState is the compact aggregate-state view (counts, not contents).
func (s *Server) memoryState() map[string]any {
	snapshot := s.personalization.memory.Snapshot()
	enabledCount := 0
	for _, item := range snapshot.Memories {
		if item.Enabled {
			enabledCount++
		}
	}
	return map[string]any{
		"enabled":       snapshot.Enabled,
		"count":         len(snapshot.Memories),
		"enabled_count": enabledCount,
	}
}

func (s *Server) handleMemoryGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.personalization.memory.Snapshot())
}

func (s *Server) handleMemoryAdd(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.personalization.memory.Add(body.Text); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.personalization.memory.Snapshot())
}

func (s *Server) handleMemorySetEnabled(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.personalization.memory.SetEnabled(body.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.personalization.memory.Snapshot())
}

func (s *Server) handleMemoryPatchItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.personalization.memory.SetItemEnabled(r.PathValue("id"), body.Enabled); err != nil {
		writeError(w, memoryErrorStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, s.personalization.memory.Snapshot())
}

func (s *Server) handleMemoryRemove(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if err := s.personalization.memory.Remove(r.PathValue("id")); err != nil {
		writeError(w, memoryErrorStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, s.personalization.memory.Snapshot())
}

func (s *Server) handleMemoryClear(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if err := s.personalization.memory.Clear(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.personalization.memory.Snapshot())
}

func memoryErrorStatus(err error) int {
	if errors.Is(err, memory.ErrMemoryNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

// --- Prompt sets ----------------------------------------------------------------

func (s *Server) promptSetsPayload() map[string]any {
	settings, _ := s.store.Snapshot()
	return map[string]any{
		"selected": settings.LLM.PromptSet,
		"default":  chat.DefaultPromptSetID,
		"sets":     s.personalization.prompts.List(),
	}
}

func (s *Server) handlePromptSetsGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.promptSetsPayload())
}

func (s *Server) handlePromptSetCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Name   string `json:"name"`
		System string `json:"system"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	set, err := s.personalization.prompts.Create(body.Name, body.System)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	payload := s.promptSetsPayload()
	payload["set"] = set
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handlePromptSetUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Name   string `json:"name"`
		System string `json:"system"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	set, err := s.personalization.prompts.Update(r.PathValue("id"), body.Name, body.System)
	if err != nil {
		writeError(w, promptSetErrorStatus(err), err)
		return
	}
	payload := s.promptSetsPayload()
	payload["set"] = set
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handlePromptSetDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.personalization.prompts.Delete(id); err != nil {
		writeError(w, promptSetErrorStatus(err), err)
		return
	}

	// Deleting the selected set falls back to the default explicitly, so chat
	// never runs against a dangling selection.
	current, _ := s.store.Snapshot()
	if current.LLM.PromptSet == id {
		current.LLM.PromptSet = chat.DefaultPromptSetID
		if _, err := s.store.Save(current); err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("prompt set deleted, but the selection could not be reset"))
			return
		}
	}
	writeJSON(w, http.StatusOK, s.promptSetsPayload())
}

func promptSetErrorStatus(err error) int {
	switch {
	case errors.Is(err, chat.ErrPromptSetNotFound):
		return http.StatusNotFound
	case errors.Is(err, chat.ErrPromptSetProtected):
		return http.StatusForbidden
	default:
		return http.StatusBadRequest
	}
}

// --- Settings reset --------------------------------------------------------------

// handleSettingsReset restores factory defaults (parity baseline row 7). It is
// an explicit destructive action: the UI double-confirms before calling it.
func (s *Server) handleSettingsReset(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	current, _ := s.store.Snapshot()
	saved, err := s.store.Save(config.DefaultSettings())
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("settings could not be reset"))
		return
	}
	runtimeErr := s.applySettingsRuntimeTransition(r.Context(), current, saved)

	_, status := s.store.Snapshot()
	payload := map[string]any{
		"settings": saved.Public(),
		"status":   status,
	}
	responseStatus := http.StatusOK
	if runtimeErr != nil {
		responseStatus = http.StatusBadGateway
		payload["error"] = "settings were reset, but the active device runtime could not be stopped"
	}
	writeJSON(w, responseStatus, payload)
}
