package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// Check the saved local model without changing the hosted conversation route,
// reading chat history, or passing any generated output to motion.
func (s *Server) handleSetupLocalModelTest(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	settings, _ := s.store.Snapshot()
	local := settings.LLM.LocalSettings()
	if local.IsHosted() || local.Model == "" {
		writeError(w, http.StatusBadRequest, errors.New("configure a local model before testing"))
		return
	}
	local.RequestRole = "local_check"
	stopSequence := s.stopSequence.Load()
	provider, err := s.prepareLLMProvider(local)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	// Local installations without accounts do not use the authenticated request
	// middleware. Bind this probe explicitly so takeover cancels it too.
	detach := s.controller.bindRequest(controllerActor(r, clientIDFromRequest(r)), cancel)
	if detach == nil {
		writeError(w, http.StatusConflict, errors.New("controller changed before local model check"))
		return
	}
	defer detach()
	ctx = context.WithValue(ctx, refusalRetryStopKey{}, stopSequence)
	ctx, _, release, err := s.acquireProviderRequest(ctx, llmRequestInteractive, local)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer release()
	raw, err := provider.StreamChat(ctx, llm.ChatRequest{
		Model: local.Model, MaxTokens: 64, ReasoningMode: "off",
		Messages: []llm.Message{{Role: "system", Content: `Text-only connection test. Return exactly {"ready":true}.`}, {Role: "user", Content: "Confirm readiness."}},
	}, nil)
	current, _ := s.store.Snapshot()
	if s.chatCanceled(ctx, stopSequence) || llmRuntimeSettingsChanged(current.LLM.LocalSettings(), local) {
		writeError(w, http.StatusConflict, errors.New("local model check was canceled; retry against the current settings"))
		return
	}
	var result struct {
		Ready bool `json:"ready"`
	}
	if err == nil && (json.Unmarshal([]byte(raw), &result) != nil || !result.Ready) {
		err = errors.New("the local model did not return a valid readiness response")
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ready": err == nil, "message": message, "model": local.Model})
}
