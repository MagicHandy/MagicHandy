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
	beforeSequence := s.stopSequence.Load()
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
	admission := engine.AdmissionGeneration()
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
	if sequence != beforeSequence+1 {
		writeError(w, http.StatusConflict, motion.ErrStartupRecoveryUnavailable)
		return
	}
	resetCtx, cancel := context.WithCancel(r.Context())
	defer cancel()
	detach := s.controller.bindRequest(controllerActor(r, clientIDFromRequest(r)), cancel)
	if detach == nil {
		writeError(w, http.StatusConflict, errors.New("controller changed before the position reset"))
		return
	}
	defer detach()
	s.motion.lifecycleMu.Lock()
	current := s.currentMotionEngine() == engine && s.stopSequence.Load() == sequence
	s.motion.lifecycleMu.Unlock()
	if !current {
		writeError(w, http.StatusConflict, motion.ErrStartupRecoveryUnavailable)
		return
	}
	state, err := engine.ResetStartupPositionAtGeneration(resetCtx, body.FailureID, admission)
	if errors.Is(err, motion.ErrStartupRecoveryUnavailable) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		message := "Position recheck was interrupted. Try again when ready."
		if errors.Is(err, context.DeadlineExceeded) {
			message = "The device did not respond in time. Position still needs a recheck."
		}
		writeJSON(w, http.StatusConflict, map[string]any{"available": true, "engine": s.clientMotionSnapshot(r, state), "error": message})
		return
	}
	s.writeMotionResult(w, r, state, err)
}

func (s *Server) recordPositionResetSequence(sequence uint64) {
	for {
		previous := s.positionResetSequence.Load()
		if previous >= sequence || s.positionResetSequence.CompareAndSwap(previous, sequence) {
			return
		}
	}
}

func (s *Server) lastStopWasPositionReset() bool {
	sequence := s.stopSequence.Load()
	return sequence > 0 && s.positionResetSequence.Load() == sequence
}

func (s *Server) stopCommandClearedMessage() string {
	if s.lastStopWasPositionReset() {
		return "Position reset cleared the command."
	}
	return "Emergency Stop cleared the command."
}
