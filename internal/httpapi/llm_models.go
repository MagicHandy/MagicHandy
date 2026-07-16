package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const maxModelManagerRequestBytes = 8 * 1024

func (s *Server) handleLLMModels(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.models.Snapshot(r.Context())
	if err != nil {
		s.writeModelManagerStorageError(w, err)
		return
	}
	settings, _ := s.store.Snapshot()
	runtimeSnapshot := s.managedLLM.Snapshot()
	response := map[string]any{
		"models":                snapshot.Models,
		"imports":               snapshot.Imports,
		"store_path":            snapshot.StorePath,
		"suggested_ollama_path": selectedOllamaModelsPath(settings.LLM),
		"runtime":               runtimeSnapshot.Runtime,
	}
	if runtimeSnapshot.Build != nil {
		response["runtime_build"] = runtimeSnapshot.Build
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleManagedLLMRuntime(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.managedLLM.Snapshot())
}

func (s *Server) handleBuildManagedLLMRuntime(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request struct {
		Backend string `json:"backend"`
	}
	if !decodeModelManagerRequest(w, r, &request) {
		return
	}
	if err := s.closeLLM(); err != nil {
		s.logger.Warn("stop active LLM runner before rebuild", "error", err)
		writeError(w, http.StatusServiceUnavailable, errors.New("the active LLM runner could not be stopped before rebuilding"))
		return
	}
	build, err := s.managedLLM.StartBuild(request.Backend)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"build": build})
}

func (s *Server) handleCancelManagedLLMRuntimeBuild(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	build, err := s.managedLLM.CancelBuild()
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"build": build})
}

func (s *Server) handleOllamaModels(w http.ResponseWriter, r *http.Request) {
	settings, _ := s.store.Snapshot()
	models, err := llm.ListOllamaModels(r.Context(), settings.LLM.OllamaBaseURL, s.llm.httpClient)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"available": false,
			"models":    []llm.OllamaModelInfo{},
			"message":   err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"models":    models,
	})
}

func (s *Server) handleOllamaScan(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request struct {
		Path string `json:"path"`
	}
	if !decodeModelManagerRequest(w, r, &request) {
		return
	}
	settings, _ := s.store.Snapshot()
	scan, err := s.models.ScanOllama(r.Context(), firstNonBlank(request.Path, selectedOllamaModelsPath(settings.LLM)))
	if err != nil {
		if errors.Is(err, llm.ErrModelInventoryUnavailable) {
			s.writeModelManagerStorageError(w, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, scan)
}

func (s *Server) handleOllamaImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request struct {
		Path        string `json:"path"`
		CandidateID string `json:"candidate_id"`
	}
	if !decodeModelManagerRequest(w, r, &request) {
		return
	}
	settings, _ := s.store.Snapshot()
	job, err := s.models.StartOllamaImport(
		r.Context(),
		firstNonBlank(request.Path, selectedOllamaModelsPath(settings.LLM)),
		request.CandidateID,
	)
	if err != nil {
		if errors.Is(err, llm.ErrModelInventoryUnavailable) {
			s.writeModelManagerStorageError(w, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"import": job})
}

func (s *Server) handleGGUFImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request struct {
		Path        string `json:"path"`
		DisplayName string `json:"display_name"`
	}
	if !decodeModelManagerRequest(w, r, &request) {
		return
	}
	job, err := s.models.StartGGUFImport(request.Path, request.DisplayName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"import": job})
}

func (s *Server) handleLLMImport(w http.ResponseWriter, r *http.Request) {
	job, err := s.models.Import(r.PathValue("id"))
	if errors.Is(err, llm.ErrImportNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		s.writeModelManagerStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"import": job})
}

func (s *Server) handleCancelLLMImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	job, err := s.models.CancelImport(r.PathValue("id"))
	if errors.Is(err, llm.ErrImportNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		s.writeModelManagerStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"import": job})
}

func (s *Server) handleDeleteLLMModel(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	settings, _ := s.store.Snapshot()
	selectedID := ""
	if settings.LLM.Provider == config.LLMProviderLlamaCPP && settings.LLM.LlamaCPPMode == config.LlamaCPPModeManaged {
		selectedID = settings.LLM.Model
	}
	err := s.models.Delete(r.Context(), r.PathValue("id"), selectedID)
	switch {
	case errors.Is(err, llm.ErrModelNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, llm.ErrModelSelected):
		writeError(w, http.StatusConflict, err)
	case err != nil:
		s.writeModelManagerStorageError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) writeModelManagerStorageError(w http.ResponseWriter, err error) {
	s.logger.Error("model manager storage operation failed", "error", err)
	writeError(w, http.StatusInternalServerError, errors.New("model manager storage is unavailable"))
}

func decodeModelManagerRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxModelManagerRequestBytes)
	if err := decodeJSON(r, target); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func selectedOllamaModelsPath(settings config.LLMSettings) string {
	return firstNonBlank(settings.OllamaModelsPath, llm.SuggestedOllamaModelsPath())
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
