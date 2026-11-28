package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func (s *Server) motionControlRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/motion/vibration/follow", s.handleVibrationFollow)
	mux.HandleFunc("POST /api/motion/pause", s.handleMotionPause)
	mux.HandleFunc("POST /api/motion/resume", s.handleMotionResume)
	mux.HandleFunc("POST /api/motion/reset-position", s.handleMotionPositionReset)
	mux.HandleFunc("POST /api/motion/edge", s.handleEdge)
	mux.HandleFunc("POST /api/motion/edge/resume", s.handleEdgeResume)
}

func (s *Server) handleEdge(w http.ResponseWriter, r *http.Request) {
	s.handleEdgeControl(w, r, false)
}

func (s *Server) handleEdgeResume(w http.ResponseWriter, r *http.Request) {
	s.handleEdgeControl(w, r, true)
}

func (s *Server) handleEdgeControl(w http.ResponseWriter, r *http.Request, returning bool) {
	if !s.requireController(w, r) {
		return
	}
	status, err := s.edgeControl(r, returning)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) edgeControl(r *http.Request, returning bool) (modes.Status, error) {
	switch r.URL.Query().Get("style") {
	case "still":
		return s.stillEdge(r.Context(), returning)
	case "", "moving":
		if returning {
			return s.resumeEdge(r.Context())
		}
		return s.activateEdge(r.Context())
	default:
		return modes.Status{}, errors.New("unknown Edge hold style")
	}
}

func (s *Server) stillEdge(ctx context.Context, returning bool) (modes.Status, error) {
	engine := s.currentMotionEngine()
	if engine == nil {
		return modes.Status{}, errMotionUnavailable
	}
	before := engine.Snapshot()
	if returning {
		if !before.StillHold {
			return modes.Status{}, errors.New("there is no still hold to resume")
		}
		done, ok := s.modes.BeginUserResume()
		current := engine.Snapshot()
		if !ok || !current.StillHold || current.PlanID != before.PlanID || ctx.Err() != nil {
			done(false)
			return modes.Status{}, errors.New("the still hold changed")
		}
		state, err := s.motion.life.Resume(ctx, engine, func() config.MotionSettings {
			latest, _ := s.store.Snapshot()
			return latest.EffectiveMotion()
		})
		done(err == nil && state.Running && !state.Paused)
		return s.modes.Status(), err
	}
	if !before.AllowsMovingHold() {
		return modes.Status{}, motion.ErrEdgeUnavailable
	}
	session, seq := s.edgeChatAnchor(ctx)
	done, ok := s.modes.BeginUserPause()
	current := engine.Snapshot()
	if !ok || !current.AllowsMovingHold() || current.PlanID != before.PlanID || ctx.Err() != nil {
		done(false)
		return modes.Status{}, errors.New("the running motion changed")
	}
	s.interruptForEdge()
	at := time.Now()
	state, err := engine.HoldStill(ctx, current.PlanID)
	done(state.Paused)
	if err == nil && state.StillHold {
		s.recordEdge(ctx, session, at, seq, "still", 0)
	}
	return s.modes.Status(), err
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
	service := chat.EdgeService{Provider: provider, Model: settings.LLM.Model, ReasoningMode: settings.LLM.ReasoningMode}
	if edgeFeedbackEnabled(settings.LLM) {
		service.MaxTokens = max(settings.LLM.MaxOutputTokens, settings.LLM.ConnectionOutputFloor())
		service.ReasoningBudgetTokens = managedLlamaReasoningBudget(settings.LLM, s.managedLLM.Snapshot().Runtime.Current)
		if settings.LLM.MotionPlanner.ContextPolicy != config.ContextTechnical && input.ChatSessionID == admission.session && input.ChatSessionID != "" {
			read, cancel := context.WithTimeout(providerCtx, 25*time.Millisecond)
			service.Feedback, _ = s.chatLog.EdgeFeedbackBefore(read, input.ChatSessionID, input.PressedAt, input.AfterSeq)
			cancel()
		}
	}
	state := chat.EdgeContext{TempoOnly: input.TempoOnly, PreviousSpeed: input.PreviousSpeed, HoldingSpeed: input.HoldingSpeed,
		SpeedMin: input.SpeedMin, SpeedMax: input.SpeedMax, RangeMin: input.RangeMin, RangeMax: input.RangeMax}
	plan, _, err := service.Complete(providerCtx, state)
	if err == nil && settings.LLM.AdvancedMotion() {
		err = s.validateCloudAdmission(providerCtx, admission)
	}
	return modes.EdgeDecision(plan), err
}

func edgeFeedbackEnabled(settings config.LLMSettings) bool {
	return settings.ModelCapabilityLevel() == config.ModelCapabilityAdvanced
}

func edgePlannerUsesFeedback(settings config.LLMSettings) bool {
	if settings.MotionPlanner.Provider == config.MotionPlannerDecisions || settings.MotionPlanner.ContextPolicy == config.ContextTechnical {
		return false
	}
	planner, err := settings.PlanningSettings()
	return err == nil && edgeFeedbackEnabled(planner)
}

func (s *Server) recordEdge(ctx context.Context, session string, at time.Time, seq int64, kind string, pace int) {
	if session != "" {
		ctx, cancel := context.WithTimeout(ctx, 25*time.Millisecond)
		defer cancel()
		if err := s.chatLog.RecordEdge(ctx, session, at, seq, kind, pace); err != nil {
			s.logger.Warn("could not record Edge observation")
		}
	}
}

func (s *Server) edgeChatAnchor(ctx context.Context) (string, int64) {
	settings, _ := s.store.Snapshot()
	if !edgeFeedbackEnabled(settings.LLM.ConversationSettings()) && !edgePlannerUsesFeedback(settings.LLM) {
		return "", 0
	}
	// A busy chat store may cost context, never an open-ended wait for relief.
	read, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	session, seq, _ := s.chatLog.EdgeChatAnchor(read)
	return session, seq
}

func (s *Server) recordMovingEdge(ctx context.Context, input modes.EdgeInput) {
	kind := "moving"
	if input.TempoOnly {
		kind = "motor_tempo"
	}
	s.recordEdge(ctx, input.ChatSessionID, input.PressedAt, input.AfterSeq, kind, input.HoldingSpeed)
}

func (s *Server) chatEdgeObservation(ctx context.Context, session string) *chat.EdgeObservation {
	read, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	obs, err := s.chatLog.LatestEdge(read, session)
	if err != nil || obs == nil {
		return nil
	}
	if edge := s.modes.Status().EdgeHold; edge != nil && edge.Active && edge.ChatSessionID == session && edge.PressedAt.Equal(obs.PressedAt) {
		obs.Active = "manual return"
		if edge.Automatic {
			obs.Active = "automatic return"
		}
	}
	return obs
}

func (s *Server) recordEdgePlan(ctx context.Context, input modes.EdgeInput, plan modes.EdgeDecision) {
	if input.ChatSessionID != "" {
		ctx, cancel := context.WithTimeout(ctx, 25*time.Millisecond)
		defer cancel()
		if err := s.chatLog.RecordEdgePlan(ctx, input.ChatSessionID, input.PressedAt, chat.EdgePlan(plan)); err != nil {
			s.logger.Warn("could not record accepted Edge plan")
		}
	}
}
