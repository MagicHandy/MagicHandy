package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

var errMotionUnavailable = errors.New("motion engine is unavailable for the configured transport")

// motionRuntime owns the live motion engine used by the manual UI controls.
// It is nil-safe: when the configured transport is not a full command
// transport, the engine is absent and motion endpoints report unavailable
// rather than panicking.
type motionRuntime struct {
	mu        sync.Mutex
	engine    *motion.Engine
	owner     string
	transport transport.Transport
}

func newMotionRuntime(runtime Runtime) motionRuntime {
	return motionRuntime{transport: runtime.MotionTransport}
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
	if engine := s.currentMotionEngine(); engine != nil {
		snapshot := engine.Snapshot()
		if snapshot.Running || snapshot.Paused {
			return map[string]any{
				"available": true,
				"engine":    snapshot,
			}
		}
	}
	if _, err := s.newSelectedMotionTransport(); err != nil {
		return map[string]any{
			"available": false,
			"error":     err.Error(),
		}
	}
	return map[string]any{
		"available": true,
	}
}

func (s *Server) handleMotionState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.motionState())
}

func (s *Server) handleMotionEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming responses are unavailable"))
		return
	}

	clientID := clientIDFromRequest(r)
	setSSEHeaders(w)
	w.WriteHeader(http.StatusOK)

	emit := func() bool {
		if clientID != "" {
			s.controller.Touch(clientID)
		}
		if err := writeSSE(w, "motion", s.motionState()); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !emit() {
		return
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !emit() {
				return
			}
		}
	}
}

func (s *Server) handleMotionStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	stopSequence := s.stopSequence.Load()

	var body motionRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	engine, err := s.motionEngineForStart()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	settings, _ := s.store.Snapshot()
	admission := engine.AdmissionGeneration()
	if s.stopSequence.Load() != stopSequence {
		writeError(w, http.StatusConflict, errors.New("motion start was invalidated by Emergency Stop"))
		return
	}
	state, err := engine.StartAtGeneration(r.Context(), body.target(), settings.Motion, admission)
	s.writeMotionResult(w, state, err)
}

func (s *Server) handleMotionTarget(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}

	engine := s.currentMotionEngine()
	if engine == nil {
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
	state, err := engine.ApplyTarget(r.Context(), body.target(), "ui_target")
	s.writeMotionResult(w, state, err)
}

// handleMotionQuick patches motion settings (speed/stroke/direction), persists
// them, and applies them to any active loop immediately, so quick controls take
// effect without a save-and-restart cycle.
func (s *Server) handleMotionQuick(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}

	var body struct {
		SpeedMinPercent  *int    `json:"speed_min_percent,omitempty"`
		SpeedMaxPercent  *int    `json:"speed_max_percent,omitempty"`
		StrokeMinPercent *int    `json:"stroke_min_percent,omitempty"`
		StrokeMaxPercent *int    `json:"stroke_max_percent,omitempty"`
		ReverseDirection *bool   `json:"reverse_direction,omitempty"`
		Style            *string `json:"style,omitempty"`
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
	if body.Style != nil {
		motionSettings.Style = *body.Style
	}
	current.Motion = motionSettings

	saved, err := s.store.Save(current)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.refreshActiveMotion(r.Context(), saved.Motion)

	payload := map[string]any{"motion": saved.Public().Motion}
	if engine := s.currentMotionEngine(); engine != nil {
		payload["engine"] = engine.Snapshot()
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleMotionStop(w http.ResponseWriter, r *http.Request) {
	// Publish every emergency-stop activation, including repeated idle stops,
	// so browser-owned capture can discard pending speech in every client.
	s.stopSequence.Add(1)
	s.cancelActiveChats()
	pendingASR := s.voice.InvalidateAll(voice.RoleASR)
	pendingTTS := s.voice.InvalidateAll(voice.RoleTTS)
	defer func() {
		s.voice.CancelInvalidated(voice.RoleASR, pendingASR)
		s.voice.CancelInvalidated(voice.RoleTTS, pendingTTS)
	}()
	// Mark autonomous modes stopped before touching the engine, but drain their
	// goroutine only after Engine.Stop has canceled any blocked mode startup.
	finishModeStop := func() {}
	if s.modes != nil {
		finishModeStop = s.modes.BeginUserStop()
	}
	defer finishModeStop()
	engine := s.currentMotionEngine()
	if engine == nil {
		s.stopSelectedTransportWithoutEngine(w, r)
		return
	}
	state, err := engine.Stop(r.Context(), "ui_stop")
	s.writeMotionResult(w, state, err)
}

func (s *Server) stopSelectedTransportWithoutEngine(w http.ResponseWriter, r *http.Request) {
	commandTransport, err := s.newSelectedMotionTransport()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"available": false,
			"stopped":   true,
			"error":     fmt.Sprintf("stop could not reach the configured transport: %v", err),
		})
		return
	}
	result, stopErr := commandTransport.Stop(r.Context(), transport.StopCommand{Reason: "ui_stop_no_engine"})
	payload := map[string]any{
		"available":        true,
		"transport_result": result,
	}
	status := http.StatusOK
	if stopErr != nil {
		status = http.StatusBadGateway
		payload["error"] = stopErr.Error()
	}
	writeJSON(w, status, payload)
}

// handleMotionPause freezes active motion (phase retained for resume). Unlike
// Stop it is a control action, so read-only clients cannot trigger it.
func (s *Server) handleMotionPause(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	engine := s.currentMotionEngine()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, errMotionUnavailable)
		return
	}
	state, err := engine.Pause(r.Context(), "ui_pause")
	s.writeMotionResult(w, state, err)
}

func (s *Server) handleMotionResume(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	engine := s.currentMotionEngine()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, errMotionUnavailable)
		return
	}
	state, err := engine.Resume(r.Context(), "ui_resume")
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
	engine := s.currentMotionEngine()
	if engine == nil {
		return
	}
	if !engine.Snapshot().Running {
		return
	}
	_, _ = engine.RefreshSettings(ctx, settings, "settings_saved")
}

func (s *Server) applySettingsRuntimeTransition(ctx context.Context, previous config.Settings, next config.Settings) {
	s.applyVoiceSettingsTransition(next)
	ownerChanged := previous.Device.HSPDispatchOwner != next.Device.HSPDispatchOwner
	intifaceAddressChanged := previous.Device.IntifaceServerAddress != next.Device.IntifaceServerAddress
	if ownerChanged || intifaceAddressChanged {
		// Owner switches stop first — including any autonomous mode.
		if s.modes != nil {
			s.modes.Stop("dispatch_owner_changed")
		}
		s.stopAndClearMotionEngine(ctx, "dispatch_owner_changed")
		if ownerChanged || next.Device.HSPDispatchOwner == config.DispatchOwnerIntiface {
			s.closeIntiface()
		}
		return
	}
	s.refreshActiveMotion(ctx, next.Motion)
}

func (s *Server) stopAndClearMotionEngine(ctx context.Context, reason string) {
	s.motion.mu.Lock()
	engine := s.motion.engine
	s.motion.engine = nil
	s.motion.owner = ""
	s.motion.mu.Unlock()

	if engine == nil {
		return
	}
	snapshot := engine.Snapshot()
	if snapshot.Running || snapshot.Paused {
		_, _ = engine.Stop(ctx, reason)
	}
}

// Close stops any active motion loop so no goroutine keeps commanding the
// device after shutdown (goroutine-lifecycle safety gate).
func (s *Server) Close() {
	s.closeLLM()
	if s.managedLLM != nil {
		s.managedLLM.Close()
	}
	if s.modes != nil {
		s.modes.Shutdown()
	}
	s.stopVoiceAutoload()
	if s.voice != nil {
		s.voice.Shutdown()
	}
	if s.chatLog != nil {
		_ = s.chatLog.Close()
	}
	if s.patterns != nil {
		_ = s.patterns.Close()
	}
	if s.models != nil {
		_ = s.models.Close()
	}
	s.stopAndClearMotionEngine(context.Background(), "server_shutdown")
	s.closeIntiface()
	s.personalization.Close()
	if s.store != nil {
		_ = s.store.Close()
	}
}

func (s *Server) motionEngineForStart() (*motion.Engine, error) {
	settings, _ := s.store.Snapshot()
	owner := settings.Device.HSPDispatchOwner
	engine, engineOwner := s.currentMotionEngineWithOwner()
	if engine != nil && engineOwner != owner {
		s.stopAndClearMotionEngine(context.Background(), "dispatch_owner_changed")
	} else if engine != nil && engine.Snapshot().Running {
		return engine, nil
	}
	commandTransport, err := s.newSelectedMotionTransport()
	if err != nil {
		return nil, err
	}
	engine, err = motion.NewEngine(motion.EngineOptions{
		Transport: commandTransport,
		Traces:    s.traces,
	})
	if err != nil {
		return nil, err
	}
	s.setMotionEngine(engine, owner)
	return engine, nil
}

func (s *Server) newSelectedMotionTransport() (transport.Transport, error) {
	if s.motion.transport != nil {
		return s.motion.transport, nil
	}

	settings, _ := s.store.Snapshot()
	switch settings.Device.HSPDispatchOwner {
	case config.DispatchOwnerCloudREST:
		cloud, err := s.newCloudTransport()
		if err != nil {
			return nil, err
		}
		return cloud, nil
	case config.DispatchOwnerBrowserBluetooth:
		snapshot := s.bluetooth.bridge.Snapshot()
		if !snapshot.Ready {
			message := snapshot.Message
			if message == "" {
				message = "Bluetooth is not connected."
			}
			return nil, fmt.Errorf("browser Bluetooth is not ready: %s", message)
		}
		bluetooth, err := s.newBluetoothTransport()
		if err != nil {
			return nil, err
		}
		return bluetooth, nil
	case config.DispatchOwnerIntiface:
		intiface, err := s.currentIntiface()
		if err != nil {
			return nil, err
		}
		status := intiface.Status()
		if status.SelectedDeviceIndex == nil || status.SelectedActuatorIndex == nil {
			return nil, errors.New("an Intiface linear actuator must be selected")
		}
		return intiface, nil
	default:
		return nil, errMotionUnavailable
	}
}

func (s *Server) currentMotionEngine() *motion.Engine {
	s.motion.mu.Lock()
	defer s.motion.mu.Unlock()
	return s.motion.engine
}

func (s *Server) currentMotionEngineWithOwner() (*motion.Engine, string) {
	s.motion.mu.Lock()
	defer s.motion.mu.Unlock()
	return s.motion.engine, s.motion.owner
}

func (s *Server) setMotionEngine(engine *motion.Engine, owner string) {
	s.motion.mu.Lock()
	defer s.motion.mu.Unlock()
	s.motion.engine = engine
	s.motion.owner = owner
}
