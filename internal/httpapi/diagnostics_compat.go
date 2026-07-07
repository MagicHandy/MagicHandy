package httpapi

import (
	"net/http"
)

func (s *Server) handleDiagnosticsCompat(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": serviceName,
		"llm":     s.llmState(r.Context()),
	})
}

func (s *Server) handlePingOllamaCompat(w http.ResponseWriter, r *http.Request) {
	settings, _ := s.store.Snapshot()
	provider, err := s.ensureLLMReady(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":               false,
			"error":            err.Error(),
			"ollama_connected": false,
			"ollama_error":     err.Error(),
		})
		return
	}
	status := provider.Status(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               status.Available,
		"status":           http.StatusOK,
		"ollama_connected": status.Available,
		"ollama_error": func() any {
			if status.Available {
				return nil
			}
			if status.Message != "" {
				return status.Message
			}
			return "LLM is not available"
		}(),
		"provider": settings.LLM.Provider,
		"model":    settings.LLM.Model,
		"base_url": selectedLLMBaseURL(settings.LLM),
		"message":  status.Message,
	})
}
