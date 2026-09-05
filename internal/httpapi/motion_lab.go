package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func (s *Server) motionLabRoutes(mux *http.ServeMux) {
	s.labTestRoutes(mux)
	mux.HandleFunc("POST /api/motion/lab/preview", s.withLabs(s.handleLabPreview))
	mux.HandleFunc("POST /api/motion/lab/proposal", s.withLabs(s.handleMotionLabProposal))
	mux.HandleFunc("POST /api/motion/lab/flow", s.withLabs(s.handleFlowPreview))
	mux.HandleFunc("GET /api/labs/llm", s.withLabs(s.handleLLMLabState))
	mux.HandleFunc("POST /api/labs/llm/chat", s.withLabs(s.handleLLMLabChat))
	mux.HandleFunc("POST /api/labs/llm/reset", s.withLabs(s.handleLLMLabReset))
	mux.HandleFunc("GET /api/labs/observations", s.withLabs(s.handleLabObservations))
	mux.HandleFunc("POST /api/labs/observations", s.withLabs(s.handleSaveLabObservation))
	mux.HandleFunc("DELETE /api/labs/observations/{id}", s.withLabs(s.handleDeleteLabObservation))
}

func (s *Server) handleFlowPreview(w http.ResponseWriter, r *http.Request) {
	var spec motion.FlowSpec
	if err := decodeJSON(r, &spec); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, _ := s.store.Snapshot()
	preview, err := motion.PreviewFlow(spec, settings.Motion, true)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, preview)
}

// The preview has no motion side effects and is available to read-only
// clients. Actual auditions use handleMotionStart's controller and Stop gate.
func (s *Server) handleLabPreview(w http.ResponseWriter, r *http.Request) {
	var request motion.LabRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, _ := s.store.Snapshot()
	preview, err := motion.PreviewMotionLab(request, settings.Motion)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleMotionLabProposal(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	stopSequence := s.stopSequence.Load()
	var body struct {
		Request motion.LabRequest `json:"request"`
		Message string            `json:"message"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := body.Request.Target("creative"); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	message, err := chat.ValidateUserMessage(body.Message)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, _ := s.store.Snapshot()
	// Reuse the cancellable interactive-work registration without appending to
	// the chat log. Emergency Stop cancels both queued and generating trials.
	sessionID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	trialCtx, finishTrial, err := s.beginChat(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer finishTrial()
	if s.stopSequence.Load() != stopSequence {
		writeError(w, http.StatusConflict, errors.New("lab trial was canceled by Emergency Stop"))
		return
	}
	ctx, _, release, err := s.llmRequests.acquire(trialCtx, llmRequestInteractive)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer release()
	provider, err := s.newLLMProvider(ctx, settings.LLM)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	reference := motionLabPromptReference(body.Request)
	started := time.Now()
	proposal, err := chat.ProposeMotionLab(ctx, provider, settings.LLM.Model, message, reference)
	if ctx.Err() != nil || s.stopSequence.Load() != stopSequence {
		writeError(w, http.StatusConflict, errors.New("lab trial was canceled"))
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	body.Request.RangeAnchorPercent = *proposal.RangeAnchorPercent
	body.Request.OutboundTimePercent = *proposal.OutboundTimePercent
	preview, err := motion.PreviewMotionLab(body.Request, settings.Motion)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"proposal": proposal, "preview": preview, "model": settings.LLM.Model,
		"elapsed_ms": time.Since(started).Milliseconds(), "prompt": chat.MotionLabPrompt,
	})
}

func motionLabPromptReference(request motion.LabRequest) string {
	encoded, _ := json.Marshal(map[string]any{
		"fixed": map[string]any{
			"speed_percent": request.SpeedPercent, "center_percent": request.CenterPercent,
			"span_percent": request.SpanPercent, "span_min_percent": request.SpanMinPercent,
			"span_profile": request.SpanProfile, "variation_percent": request.VariationPercent,
		},
		"editable": map[string]int{
			"range_anchor_percent": request.RangeAnchorPercent, "outbound_time_percent": request.OutboundTimePercent,
		},
	})
	return string(encoded)
}
