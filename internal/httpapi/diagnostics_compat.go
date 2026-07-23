package httpapi

import (
	"net/http"
)

func (s *Server) handleDiagnosticsCompat(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":           serviceName,
		"llm":               s.llmState(r.Context()),
		"handy_log_recent":  diagnosticsHandyLogRecent(s),
		"handy_log_path":    s.handyLogPath(),
		"handy_log_enabled": s.handyMotionLogEnabled(),
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
			"llm_provider":     settings.LLM.Provider,
			"llm_connected":    false,
			"llm_error":        err.Error(),
		})
		return
	}
	status := provider.Status(r.Context())
	errMessage := func() any {
		if status.Available {
			return nil
		}
		if status.Message != "" {
			return status.Message
		}
		return "LLM is not available"
	}()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               status.Available,
		"status":           http.StatusOK,
		"ollama_connected": status.Available,
		"ollama_error":     errMessage,
		"llm_provider":     settings.LLM.Provider,
		"llm_connected":    status.Available,
		"llm_error":        errMessage,
		"provider":         settings.LLM.Provider,
		"model":            settings.LLM.Model,
		"base_url":         selectedLLMBaseURL(settings.LLM),
		"message":          status.Message,
		"loaded":           status.Loaded,
		"managed":          status.Managed,
	})
}
