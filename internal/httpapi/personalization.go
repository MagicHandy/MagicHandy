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
		_ = memoryStore.Close()
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
	snapshot, err := s.personalization.memory.Snapshot()
	if err != nil {
		return map[string]any{
			"available":     false,
			"enabled":       false,
			"count":         0,
			"enabled_count": 0,
		}
	}
	enabledCount := 0
	for _, item := range snapshot.Memories {
		if item.Enabled {
			enabledCount++
		}
	}
	return map[string]any{
		"available":     true,
		"enabled":       snapshot.Enabled,
		"count":         len(snapshot.Memories),
		"enabled_count": enabledCount,
	}
}

func (s *Server) handleMemoryGet(w http.ResponseWriter, _ *http.Request) {
	s.writeMemorySnapshot(w)
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
		s.writeMemoryError(w, err)
		return
	}
	s.writeMemorySnapshot(w)
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
		s.writeMemoryError(w, err)
		return
	}
	s.writeMemorySnapshot(w)
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
		s.writeMemoryError(w, err)
		return
	}
	s.writeMemorySnapshot(w)
}

func (s *Server) handleMemoryRemove(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if err := s.personalization.memory.Remove(r.PathValue("id")); err != nil {
		s.writeMemoryError(w, err)
		return
	}
	s.writeMemorySnapshot(w)
}

func (s *Server) handleMemoryClear(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if err := s.personalization.memory.Clear(); err != nil {
		s.writeMemoryError(w, err)
		return
	}
	s.writeMemorySnapshot(w)
}

func (s *Server) writeMemorySnapshot(w http.ResponseWriter) {
	snapshot, err := s.personalization.memory.Snapshot()
	if err != nil {
		s.writePersonalizationStorageError(w, "memory", err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) writeMemoryError(w http.ResponseWriter, err error) {
	status := memoryErrorStatus(err)
	if status == http.StatusInternalServerError {
		s.writePersonalizationStorageError(w, "memory", err)
		return
	}
	writeError(w, status, err)
}

func memoryErrorStatus(err error) int {
	switch {
	case errors.Is(err, memory.ErrMemoryNotFound):
		return http.StatusNotFound
	case errors.Is(err, memory.ErrMemoryInvalid):
		return http.StatusBadRequest
	case errors.Is(err, memory.ErrMemoryLimit):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// --- Prompt sets ----------------------------------------------------------------

func (s *Server) promptSetsPayload() (map[string]any, error) {
	settings, _ := s.store.Snapshot()
	sets, err := s.personalization.prompts.List()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"selected": settings.LLM.PromptSet,
		"default":  chat.DefaultPromptSetID,
		"sets":     sets,
	}, nil
}

func (s *Server) handlePromptSetsGet(w http.ResponseWriter, _ *http.Request) {
	s.writePromptSetsPayload(w, nil)
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
		s.writePromptSetError(w, err)
		return
	}
	s.writePromptSetsPayload(w, &set)
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
		s.writePromptSetError(w, err)
		return
	}
	s.writePromptSetsPayload(w, &set)
}

func (s *Server) handlePromptSetDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.personalization.prompts.Delete(id); err != nil {
		s.writePromptSetError(w, err)
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
	s.writePromptSetsPayload(w, nil)
}

func (s *Server) writePromptSetsPayload(w http.ResponseWriter, set *chat.PromptSet) {
	payload, err := s.promptSetsPayload()
	if err != nil {
		s.writePersonalizationStorageError(w, "prompt set", err)
		return
	}
	if set != nil {
		payload["set"] = *set
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) writePromptSetError(w http.ResponseWriter, err error) {
	status := promptSetErrorStatus(err)
	if status == http.StatusInternalServerError {
		s.writePersonalizationStorageError(w, "prompt set", err)
		return
	}
	writeError(w, status, err)
}

func (s *Server) writePersonalizationStorageError(w http.ResponseWriter, domain string, err error) {
	s.logger.Error("personalization storage operation failed", "domain", domain, "error", err)
	writeError(w, http.StatusInternalServerError, errors.New(domain+" storage is unavailable"))
}

func promptSetErrorStatus(err error) int {
	switch {
	case errors.Is(err, chat.ErrPromptSetNotFound):
		return http.StatusNotFound
	case errors.Is(err, chat.ErrPromptSetProtected):
		return http.StatusForbidden
	case errors.Is(err, chat.ErrPromptSetInvalid):
		return http.StatusBadRequest
	case errors.Is(err, chat.ErrPromptSetLimit):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
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
