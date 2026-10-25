package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
)

func (s *Server) motionControlRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/motion/vibration/follow", s.handleVibrationFollow)
	mux.HandleFunc("POST /api/motion/pause", s.handleMotionPause)
	mux.HandleFunc("POST /api/motion/resume", s.handleMotionResume)
	mux.HandleFunc("POST /api/motion/edge", s.handleEdge)
	mux.HandleFunc("POST /api/motion/edge/resume", s.handleEdgeResume)
}

func (s *Server) handleEdge(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	status, err := s.activateEdge(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleEdgeResume(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	status, err := s.resumeEdge(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) decideEdge(ctx context.Context, input modes.EdgeInput) (modes.EdgeDecision, error) {
	settings, _ := s.store.Snapshot()
	if settings.LLM.MotionPlanner.Provider == config.MotionPlannerDecisions {
		return modes.EdgeDecision{}, errors.New("the selected Decisions planner does not support holding plans")
	}
	settings, admission, err := s.autopilotRoleSettings(ctx, chat.AutopilotKindMotion, settings)
	if err != nil {
		return modes.EdgeDecision{}, err
	}
	provider, err := s.prepareLLMProvider(settings.LLM)
	if err != nil {
		return modes.EdgeDecision{}, err
	}
	providerCtx, _, release, err := s.acquireProviderRequest(ctx, llmRequestAutonomous, settings.LLM)
	if err != nil {
		return modes.EdgeDecision{}, err
	}
	defer release()
	plan, _, err := (chat.EdgeService{Provider: provider, Model: settings.LLM.Model, ReasoningMode: settings.LLM.ReasoningMode}).Complete(providerCtx, chat.EdgeContext(input))
	if err == nil && settings.LLM.IsHosted() {
		err = s.validateCloudAdmission(providerCtx, admission)
	}
	return modes.EdgeDecision(plan), err
}
