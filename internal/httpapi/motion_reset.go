package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func (s *Server) handleMotionPositionReset(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		FailureID string `json:"failure_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	engine := s.currentMotionEngine()
	if engine == nil || body.FailureID == "" {
		writeError(w, http.StatusConflict, motion.ErrStartupRecoveryUnavailable)
		return
	}
	snapshot := engine.Snapshot()
	if snapshot.StartupRecovery == nil || snapshot.StartupRecovery.ID != body.FailureID ||
		snapshot.StartupRecovery.Resetting || snapshot.Running || snapshot.Starting || snapshot.Completing {
		writeError(w, http.StatusConflict, motion.ErrStartupRecoveryUnavailable)
		return
	}
	// A reset is a user Stop intent: pending chat/mode work must not resume
	// playback when the fresh read succeeds. Detach this request from its own
	// cancellation; a later Emergency Stop still cancels the engine's reset.
	finish, sequence := s.beginGlobalStop("position_reset", r.Context())
	defer finish()
	finish()
	resetCtx, cancel := context.WithCancel(r.Context())
	defer cancel()
	detach := s.controller.bindRequest(controllerActor(r, clientIDFromRequest(r)), cancel)
	if detach == nil {
		writeError(w, http.StatusConflict, errors.New("controller changed before the position reset"))
		return
	}
	defer detach()
	s.motion.lifecycleMu.Lock()
	defer s.motion.lifecycleMu.Unlock()
	if s.currentMotionEngine() != engine || s.stopSequence.Load() != sequence {
		writeError(w, http.StatusConflict, motion.ErrStartupRecoveryUnavailable)
		return
	}
	state, err := engine.ResetStartupPosition(resetCtx, body.FailureID)
	if errors.Is(err, motion.ErrStartupRecoveryUnavailable) {
		writeError(w, http.StatusConflict, err)
		return
	}
	s.writeMotionResult(w, r, state, err)
}
