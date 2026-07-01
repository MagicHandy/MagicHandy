package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

var errMotionUnavailable = errors.New("motion engine is unavailable for the configured transport")

// motionRuntime owns the live motion engine used by the manual UI controls.
// It is nil-safe: when the configured transport is not a full command
// transport, the engine is absent and motion endpoints report unavailable
// rather than panicking.
type motionRuntime struct {
	engine *motion.Engine
}

func newMotionRuntime(runtime Runtime) motionRuntime {
	commandTransport, ok := runtime.Transport.(transport.Transport)
	if !ok || commandTransport == nil {
		return motionRuntime{}
	}
	engine, err := motion.NewEngine(motion.EngineOptions{
		Transport: commandTransport,
		Traces:    runtime.Traces,
	})
	if err != nil {
		return motionRuntime{}
	}
	return motionRuntime{engine: engine}
}

// motionRequest is the optional body for start/target control.
type motionRequest struct {
	Pattern      string `json:"pattern,omitempty"`
	SpeedPercent int    `json:"speed_percent,omitempty"`
}

func (r motionRequest) target() motion.MotionTarget {
	return motion.MotionTarget{
		Label:        "Manual",
		Source:       "manual_ui",
		PatternID:    motion.PatternID(r.Pattern),
		SpeedPercent: r.SpeedPercent,
	}
}

// motionState returns a UI-facing snapshot; the "available" flag lets the
// frontend show an honest "motion unavailable" state instead of guessing.
func (s *Server) motionState() any {
	if s.motion.engine == nil {
		return map[string]any{"available": false}
	}
	snapshot := s.motion.engine.Snapshot()
	return map[string]any{
		"available": true,
		"engine":    snapshot,
	}
}

func (s *Server) handleMotionState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.motionState())
}

func (s *Server) handleMotionStart(w http.ResponseWriter, r *http.Request) {
	if s.motion.engine == nil {
		writeError(w, http.StatusServiceUnavailable, errMotionUnavailable)
		return
	}
	var body motionRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	settings, _ := s.store.Snapshot()
	state, err := s.motion.engine.Start(r.Context(), body.target(), settings.Motion)
	s.writeMotionResult(w, state, err)
}

func (s *Server) handleMotionTarget(w http.ResponseWriter, r *http.Request) {
	if s.motion.engine == nil {
		writeError(w, http.StatusServiceUnavailable, errMotionUnavailable)
		return
	}
	var body motionRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	state, err := s.motion.engine.ApplyTarget(r.Context(), body.target(), "ui_target")
	s.writeMotionResult(w, state, err)
}

// handleMotionQuick patches motion settings (speed/stroke/direction), persists
// them, and applies them to any active loop immediately, so quick controls take
// effect without a save-and-restart cycle.
func (s *Server) handleMotionQuick(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SpeedMinPercent  *int  `json:"speed_min_percent,omitempty"`
		SpeedMaxPercent  *int  `json:"speed_max_percent,omitempty"`
		StrokeMinPercent *int  `json:"stroke_min_percent,omitempty"`
		StrokeMaxPercent *int  `json:"stroke_max_percent,omitempty"`
		ReverseDirection *bool `json:"reverse_direction,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	current, _ := s.store.Snapshot()
	motionSettings := current.Motion
	if body.SpeedMinPercent != nil {
		motionSettings.SpeedMinPercent = *body.SpeedMinPercent
	}
	if body.SpeedMaxPercent != nil {
		motionSettings.SpeedMaxPercent = *body.SpeedMaxPercent
	}
	if body.StrokeMinPercent != nil {
		motionSettings.StrokeMinPercent = *body.StrokeMinPercent
	}
	if body.StrokeMaxPercent != nil {
		motionSettings.StrokeMaxPercent = *body.StrokeMaxPercent
	}
	if body.ReverseDirection != nil {
		motionSettings.ReverseDirection = *body.ReverseDirection
	}
	current.Motion = motionSettings

	saved, err := s.store.Save(current)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.refreshActiveMotion(r.Context(), saved.Motion)

	payload := map[string]any{"motion": saved.Public().Motion}
	if s.motion.engine != nil {
		payload["engine"] = s.motion.engine.Snapshot()
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleMotionStop(w http.ResponseWriter, r *http.Request) {
	if s.motion.engine == nil {
		writeError(w, http.StatusServiceUnavailable, errMotionUnavailable)
		return
	}
	state, err := s.motion.engine.Stop(r.Context(), "ui_stop")
	s.writeMotionResult(w, state, err)
}

// writeMotionResult always returns the resolved engine state so the UI can
// reconcile optimistic controls, and reports transport failures as 502 with the
// state attached rather than a bare error.
func (s *Server) writeMotionResult(w http.ResponseWriter, state motion.ActiveMotionState, err error) {
	status := http.StatusOK
	payload := map[string]any{"available": true, "engine": state}
	if err != nil {
		status = http.StatusBadGateway
		payload["error"] = err.Error()
	}
	writeJSON(w, status, payload)
}

// refreshActiveMotion applies saved settings to running motion immediately so
// quick controls take effect without a stop/start (ADR 0002, Invariant 9).
func (s *Server) refreshActiveMotion(ctx context.Context, settings config.MotionSettings) {
	if s.motion.engine == nil {
		return
	}
	if !s.motion.engine.Snapshot().Running {
		return
	}
	_, _ = s.motion.engine.RefreshSettings(ctx, settings, "settings_saved")
}

// Close stops any active motion loop so no goroutine keeps commanding the
// device after shutdown (goroutine-lifecycle safety gate).
func (s *Server) Close() {
	if s.motion.engine == nil {
		return
	}
	_, _ = s.motion.engine.Stop(context.Background(), "server_shutdown")
}
