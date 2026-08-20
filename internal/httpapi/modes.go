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
		AutopilotSettings: func() config.AutopilotSettings {
			settings, _ := s.store.Snapshot()
			return settings.Autopilot
		},
		MotionGenerationMode: func() string {
			settings, _ := s.store.Snapshot()
			return settings.LLM.MotionGenerationMode
		},
		Traces:       s.traces,
		Decide:       s.autopilotDecide,
		DecideSpeech: s.autopilotDecideSpeech,
		CanAnnounce:  s.autopilotCanAnnounce,
		Announce:     s.autopilotAnnounce,
	})
}

func (s *Server) handleModesGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.modes.Status())
}

func (s *Server) handleAutopilotPreferences(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var preferences config.AutopilotSettings
	if err := decodeJSON(r, &preferences); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var updateErr error
	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), func(current config.Settings) (config.Settings, error) {
		current.Autopilot = preferences
		var next config.Settings
		next, updateErr = config.NormalizeSettings(current)
		return next, updateErr
	})
	if updateErr != nil {
		writeError(w, http.StatusBadRequest, updateErr)
		return
	}
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, errors.New("autopilot preferences could not be saved"))
		return
	}
	payload := map[string]any{"autopilot": saved.Autopilot}
	status := http.StatusOK
	if runtimeErr != nil {
		status = http.StatusBadGateway
		payload["error"] = "Autopilot preferences were saved, but the active runtime could not apply them"
	}
	writeJSON(w, status, payload)
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
	settings, _ := s.store.Snapshot()
	if body.Mode == modes.ModeAutopilot && settings.LLM.MotionGenerationMode == config.LLMMotionModeOff {
		writeError(w, http.StatusBadRequest, errors.New("autopilot motion is off; choose Creative or Pattern library in the sidebar"))
		return
	}
	s.chatLifecycleMu.Lock()
	defer s.chatLifecycleMu.Unlock()
	s.personaMutationMu.Lock()
	defer s.personaMutationMu.Unlock()
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
	s.chatLifecycleMu.Lock()
	defer s.chatLifecycleMu.Unlock()
	s.personaMutationMu.Lock()
	defer s.personaMutationMu.Unlock()
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

// handleAutopilotArc lets the user place or clear visible session buildup. The
// bar is as much the user's as the model's: a progression you can see but not
// move would be a readout, not an override.
func (s *Server) handleAutopilotArc(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if s.modes == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("mode manager is unavailable"))
		return
	}
	var body struct {
		// Percent places the bar. Reset clears it and re-anchors the clock.
		Percent *int `json:"percent,omitempty"`
		Reset   bool `json:"reset,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var applied bool
	switch {
	case body.Reset:
		applied = s.modes.ResetSessionArc()
	case body.Percent != nil:
		if *body.Percent < 0 || *body.Percent > 100 {
			writeError(w, http.StatusBadRequest, errors.New("session buildup percent must be between 0 and 100"))
			return
		}
		applied = s.modes.SetSessionArcPercent(*body.Percent)
	default:
		writeError(w, http.StatusBadRequest, errors.New("supply percent or reset"))
		return
	}
	if !applied {
		writeError(w, http.StatusConflict, errors.New("start Autopilot before placing session buildup"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_arc": s.modes.SessionArcSnapshot()})
}
