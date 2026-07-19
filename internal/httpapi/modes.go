package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
)

// newModeManager wires the mode manager to the server's engine lifecycle.
// Modes see only the narrow engine surface — construction of transports and
// dispatch-owner rules stay inside the server.
func (s *Server) newModeManager() (*modes.Manager, error) {
	return modes.NewManager(modes.Options{
		Ensure: func(context.Context) (modes.Engine, error) {
			engine, admission, err := s.motionEngineForStart()
			if err != nil {
				return nil, err
			}
			return admittedMotionEngine{Engine: engine, admission: admission}, nil
		},
		Current: func() modes.Engine {
			engine := s.currentMotionEngine()
			if engine == nil {
				return nil
			}
			return engine
		},
		Settings: func() config.MotionSettings {
			settings, _ := s.store.Snapshot()
			return settings.Motion
		},
		Traces:   s.traces,
		Decide:   s.autopilotDecide,
		Announce: s.autopilotAnnounce,
	})
}

func (s *Server) handleModesGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.modes.Status())
}

func (s *Server) handleModeStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	stopSequence := s.stopSequence.Load()
	var body struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status, err := s.modes.Start(r.Context(), body.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if s.stopSequence.Load() != stopSequence {
		s.modes.NotifyUserStop()
		if engine := s.currentMotionEngine(); engine != nil {
			_, _ = engine.Stop(context.Background(), "mode_start_invalidated")
		}
		writeError(w, http.StatusConflict, errors.New("mode start was invalidated by Emergency Stop"))
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleModeStop ends the active mode. By default it also stops motion
// (matching the old app's stop-auto behavior); disabling chat keepalive sends
// stop_motion:false so live chat-driven motion is not interrupted.
func (s *Server) handleModeStop(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	stopMotion := true
	if r.ContentLength != 0 {
		var body struct {
			StopMotion *bool `json:"stop_motion,omitempty"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if body.StopMotion != nil {
			stopMotion = *body.StopMotion
		}
	}
	finishModeStop := s.modes.BeginUserStop()
	defer finishModeStop()
	if stopMotion {
		if engine := s.currentMotionEngine(); engine != nil {
			if _, err := engine.Stop(r.Context(), "mode_stopped"); err != nil {
				writeError(w, http.StatusBadGateway, errors.New("mode stopped, but the motion stop failed: "+err.Error()))
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, s.modes.Status())
}
